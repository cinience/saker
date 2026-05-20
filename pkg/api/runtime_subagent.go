package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	acpclient "github.com/saker-ai/saker/pkg/acp/client"
	"github.com/saker-ai/saker/pkg/agent"
	"github.com/saker-ai/saker/pkg/config"
	"github.com/saker-ai/saker/pkg/message"
	"github.com/saker-ai/saker/pkg/model"
	"github.com/saker-ai/saker/pkg/runtime/skills"
	"github.com/saker-ai/saker/pkg/runtime/subagents"
	toolbuiltin "github.com/saker-ai/saker/pkg/tool/builtin"
)

const subagentNoSpawnDirective = "Do NOT spawn sub-agents. Execute tasks directly."

const subagentEnhancement = `Notes:
- Agent threads always have their cwd reset between bash calls, as a result please only use absolute file paths.
- In your final response, share file paths (always absolute, never relative) that are relevant to the task. Include code snippets only when the exact text is load-bearing (e.g., a bug you found, a function signature the caller asked for) — do not recap code you merely read.
- Do NOT use emojis.
- Do not use a colon before tool calls. Use a period instead.
- Complete the task fully — don't gold-plate, but don't leave it half-done. When you complete the task, respond with a concise report covering what was done and any key findings.`

// subagentMaxIterations chooses the iteration cap for a single subagent run.
// We always honor an explicit unlimited (-1) coming from the runtime — a
// platform deployment that opted out of caps shouldn't have a 50 silently
// re-imposed on its children. Otherwise we apply DefaultSubagentMaxIterations
// (50, mirrors Claude Code's MAX_AGENT_TURNS).
func subagentMaxIterations(runtimeMax int) int {
	if runtimeMax == -1 {
		return -1
	}
	return agent.DefaultSubagentMaxIterations
}

func (rt *Runtime) executeSubagent(ctx context.Context, prompt string, activation skills.ActivationContext, req *Request) (*subagents.Result, string, error) {
	if req == nil {
		return nil, prompt, nil
	}

	def, builtin := applySubagentTarget(req)
	if rt.subMgr == nil {
		return nil, prompt, nil
	}
	meta := map[string]any{
		"entrypoint": req.Mode.EntryPoint,
	}
	if len(req.Metadata) > 0 {
		for k, v := range req.Metadata {
			meta[k] = v
		}
	}
	if session := strings.TrimSpace(req.SessionID); session != "" {
		meta["session_id"] = session
	}
	request := subagents.Request{
		Target:        req.TargetSubagent,
		Instruction:   prompt,
		Activation:    activation,
		ToolWhitelist: normalizeStrings(req.ToolWhitelist),
		Metadata:      meta,
	}
	dispatchCtx := ctx
	if dispatchCtx == nil {
		dispatchCtx = context.Background()
	}
	if subCtx, ok := buildSubagentContext(*req, def, builtin); ok {
		dispatchCtx = subagents.WithContext(dispatchCtx, subCtx)
	}
	res, err := rt.subMgr.Dispatch(dispatchCtx, request)
	if err != nil {
		if errors.Is(err, subagents.ErrDispatchUnauthorized) {
			return nil, prompt, nil
		}
		if errors.Is(err, subagents.ErrNoMatchingSubagent) && req.TargetSubagent == "" {
			return nil, prompt, nil
		}
		return nil, "", err
	}
	text := fmt.Sprint(res.Output)
	if strings.TrimSpace(text) != "" {
		prompt = strings.TrimSpace(text)
	}
	prompt = applyPromptMetadata(prompt, res.Metadata)
	mergeTags(req, res.Metadata)
	applyCommandMetadata(req, res.Metadata)
	return &res, prompt, nil
}

// buildSubagentRunner creates the subagent Runner, optionally wrapping it
// with an ACP runner when external ACP agents are configured.
func (rt *Runtime) buildSubagentRunner() subagents.Runner {
	var runner subagents.Runner = runtimeSubagentRunner{rt: rt}
	if len(rt.opts.ACPAgents) > 0 {
		acpCfg := acpclient.ACPRunnerConfig{Agents: make(map[string]acpclient.ACPAgentConfig, len(rt.opts.ACPAgents))}
		for name, entry := range rt.opts.ACPAgents {
			timeout, _ := time.ParseDuration(entry.Timeout)
			acpCfg.Agents[name] = acpclient.ACPAgentConfig{
				Command: entry.Command,
				Args:    entry.Args,
				Env:     entry.Env,
				Timeout: timeout,
			}
		}
		runner = acpclient.NewACPRunner(acpCfg, runner)
	}
	return runner
}

// buildACPAgentDescriptions generates a description block for detected ACP
// agents so the model knows they are available as subagent_type values.
func buildACPAgentDescriptions(agents map[string]config.ACPAgentEntry) string {
	if len(agents) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\nExternal ACP agents (use these as subagent_type to call external agent processes via ACP protocol):\n")
	for name, entry := range agents {
		sb.WriteString(fmt.Sprintf("- %s: External ACP agent (command: %s). Delegates the task to an external agent process and returns the result.\n", name, entry.Command))
	}
	return sb.String()
}

type runtimeSubagentRunner struct {
	rt *Runtime
}

func (r runtimeSubagentRunner) RunSubagent(ctx context.Context, req subagents.RunRequest) (subagents.Result, error) {
	if r.rt == nil {
		return subagents.Result{}, errors.New("api: runtime is nil")
	}

	// Fork path: when target is empty or "fork", run with inherited context.
	if req.ParentContext.IsFork && subagents.IsForkTarget(req.Target) {
		return r.runFork(ctx, req)
	}

	// Traditional path: run an independent agent loop.
	return r.runTraditional(ctx, req)
}

// runTraditional executes a subagent via handler dispatch or a full agent loop.
// If a handler is registered for the target, the handler result is returned directly.
// Otherwise, a real agent loop runs with the model (bypassing skill/command processing
// to avoid polluting the subagent's instruction).
func (r runtimeSubagentRunner) runTraditional(ctx context.Context, req subagents.RunRequest) (subagents.Result, error) {
	sessionID := req.ParentContext.SessionID
	if sessionID == "" {
		sessionID = req.InstanceID
	}

	normalized := Request{
		Prompt:         req.Instruction,
		SessionID:      sessionID,
		TargetSubagent: req.Target,
		ToolWhitelist:  req.ToolWhitelist,
		Metadata:       cloneArguments(req.Metadata),
		Mode:           r.rt.mode,
	}
	subCtx := req.ParentContext.Clone()
	subCtx.ToolDenylist = mergeToolLists(subCtx.ToolDenylist, subagentToolDenylistForDepth(subCtx.Depth, r.rt.opts.SubagentMaxDepth))
	ctx = subagents.WithContext(ctx, subCtx)

	// Extract model tier from metadata (set by runTaskInvocation).
	if m, ok := req.Metadata["task.model"]; ok {
		if tier, ok := m.(string); ok && tier != "" {
			normalized.Model = ModelTier(tier)
		}
	}

	// Try handler dispatch first. If a handler is registered for the target,
	// use its result directly without running a full agent loop.
	if r.rt.subMgr != nil {
		prompt := strings.TrimSpace(req.Instruction)
		activation := normalized.activationContext(prompt)
		subRes, _, err := r.rt.executeSubagent(ctx, prompt, activation, &normalized)
		if err != nil {
			return subagents.Result{Subagent: req.Target, Error: err.Error()}, err
		}
		if subRes != nil {
			// Handler matched and produced a result — return it directly.
			subRes.Subagent = req.Target
			return *subRes, nil
		}
	}

	// No handler matched — run a real agent loop.
	if err := r.rt.beginRun(); err != nil {
		return subagents.Result{Subagent: req.Target, Error: err.Error()}, err
	}
	defer r.rt.endRun()

	if err := r.rt.sessionGate.Acquire(ctx, sessionID); err != nil {
		return subagents.Result{Subagent: req.Target, Error: err.Error()}, err
	}
	defer r.rt.sessionGate.Release(sessionID)

	history := r.rt.histories.Get(sessionID)
	recorder := defaultHookRecorder()
	whitelist := combineToolWhitelists(normalized.ToolWhitelist, nil)

	// Build prompt: only inject the no-spawn directive at the deepest allowed
	// level so intermediate subagents can still spawn children.
	promptPrefix := subagentEnhancement
	currentDepth := 0
	if subCtx, ok := subagents.FromContext(ctx); ok {
		currentDepth = subCtx.Depth
	}
	if r.rt.opts.SubagentMaxDepth > 0 && currentDepth >= r.rt.opts.SubagentMaxDepth-1 {
		promptPrefix = subagentNoSpawnDirective + "\n\n" + promptPrefix
	}

	prep := preparedRun{
		ctx:           ctx,
		prompt:        promptPrefix + "\n\n" + strings.TrimSpace(req.Instruction),
		history:       history,
		normalized:    normalized,
		recorder:      recorder,
		mode:          normalized.Mode,
		toolWhitelist: whitelist,
		// Subagents get the dedicated 50-iteration cap from
		// agent.DefaultSubagentMaxIterations regardless of the runtime-wide
		// MaxIterations. Mirrors Claude Code's MAX_AGENT_TURNS so a
		// self-contained sub-task has predictable headroom without burning
		// the parent run's budget.
		maxIterationsOverride: subagentMaxIterations(r.rt.opts.MaxIterations),
	}
	defer r.rt.persistHistory(sessionID, history)

	// Start mailbox draining: inject external messages into history so the
	// agent loop picks them up on its next iteration.
	mailboxDone := make(chan struct{})
	go func() {
		defer close(mailboxDone)
		r.drainMailbox(ctx, req.InstanceID, history)
	}()
	defer func() { <-mailboxDone }()

	runRes, err := r.rt.runAgentWithMiddleware(prep)
	if err != nil {
		return subagents.Result{Subagent: req.Target, Error: err.Error()}, err
	}

	resp := r.rt.buildResponse(prep, runRes)
	result := subagents.Result{
		Subagent: req.Target,
		Metadata: map[string]any{},
	}
	if resp != nil && resp.Result != nil {
		result.Output = resp.Result.Output
		result.Metadata["usage"] = resp.Result.Usage
		result.Metadata["stop_reason"] = resp.Result.StopReason
	}
	return result, nil
}

// runFork executes a fork subagent that inherits the parent's conversation
// history and system prompt for prompt cache sharing.
func (r runtimeSubagentRunner) runFork(ctx context.Context, req subagents.RunRequest) (subagents.Result, error) {
	if r.rt.histories == nil {
		return subagents.Result{Subagent: subagents.ForkSubagentType, Error: "histories not initialized"}, errors.New("api: histories not initialized")
	}
	parentSessionID := req.ParentContext.SessionID
	if parentSessionID == "" {
		parentSessionID = "default"
	}
	childSessionID := parentSessionID + ":fork-" + req.InstanceID

	// Get parent history and check for recursive forking.
	parentHistory := r.rt.histories.Get(parentSessionID)
	parentMsgs := parentHistory.All()
	if subagents.IsInForkChild(parentMsgs) {
		return subagents.Result{
			Subagent: subagents.ForkSubagentType,
			Error:    "cannot fork from within a fork child",
		}, errors.New("api: cannot fork from within a fork child")
	}

	// Optionally truncate parent history to the last N turns for smaller context.
	if req.ParentContext.ForkTurns > 0 {
		parentMsgs = subagents.TruncateToLastNTurns(parentMsgs, req.ParentContext.ForkTurns)
	}

	// Create child history and copy parent messages for cache sharing.
	childHistory := r.rt.histories.Get(childSessionID)
	for _, msg := range parentMsgs {
		childHistory.Append(msg)
	}

	// Build fork directive and append as user message.
	directive := subagents.BuildChildDirective(req.Instruction)

	// Use parent's system prompt if provided, else fall back to runtime default.
	systemPrompt := r.rt.opts.SystemPrompt
	if req.ParentContext.ParentSystemPrompt != "" {
		systemPrompt = req.ParentContext.ParentSystemPrompt
	}

	// Build a prepared run with the inherited state.
	recorder := defaultHookRecorder()
	normalized := Request{
		SessionID: childSessionID,
		Mode:      r.rt.mode,
		Metadata:  cloneArguments(req.Metadata),
	}
	childCtx := subagents.Context{SessionID: childSessionID, ToolDenylist: defaultSubagentToolDenylist()}
	ctx = subagents.WithContext(ctx, childCtx)
	// Extract model tier
	if m, ok := req.Metadata["task.model"]; ok {
		if tier, ok := m.(string); ok && tier != "" {
			normalized.Model = ModelTier(tier)
		}
	}

	// Override system prompt for fork child to use parent's (cache-identical).
	origSystemPrompt := r.rt.opts.SystemPrompt
	r.rt.opts.SystemPrompt = systemPrompt
	defer func() { r.rt.opts.SystemPrompt = origSystemPrompt }()

	prep := preparedRun{
		ctx:        ctx,
		prompt:     directive,
		history:    childHistory,
		normalized: normalized,
		recorder:   recorder,
		mode:       r.rt.mode,
		// Same 50-iter contract as the traditional subagent path — a fork
		// child should not be able to outlive its parent's budget by accident.
		maxIterationsOverride: subagentMaxIterations(r.rt.opts.MaxIterations),
	}

	result, err := r.rt.runAgentWithMiddleware(prep)
	if err != nil {
		return subagents.Result{
			Subagent: subagents.ForkSubagentType,
			Error:    err.Error(),
		}, err
	}

	res := subagents.Result{
		Subagent: subagents.ForkSubagentType,
		Metadata: map[string]any{},
	}
	if result.output != nil {
		res.Output = result.output.Content
		res.Metadata["usage"] = result.usage
		res.Metadata["stop_reason"] = result.reason
	}
	return res, nil
}

func (rt *Runtime) ensureSubagentExecutor() *subagents.Executor {
	if rt == nil || rt.subMgr == nil {
		return nil
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.subStore == nil {
		rt.subStore = subagents.NewMemoryStore()
	}
	if rt.subExec == nil {
		var opts []subagents.ExecutorOption
		if rt.opts.SubagentMaxThreads > 0 {
			opts = append(opts, subagents.WithMaxConcurrency(rt.opts.SubagentMaxThreads))
		}
		if rt.opts.SubagentMaxDepth > 0 {
			opts = append(opts, subagents.WithMaxDepth(rt.opts.SubagentMaxDepth))
		}
		rt.subExec = subagents.NewExecutor(rt.subMgr, rt.subStore, rt.buildSubagentRunner(), opts...)
		rt.subExec.SetOnComplete(rt.onSubagentComplete)
	}
	return rt.subExec
}

// onSubagentComplete is called when any subagent finishes. For background
// subagents, it injects a completion notification into the parent session history.
func (rt *Runtime) onSubagentComplete(inst subagents.Instance) {
	if !inst.Background {
		return
	}
	parentSession := inst.ParentSessionID
	if parentSession == "" {
		return
	}
	history := rt.histories.Get(parentSession)
	if history == nil {
		return
	}
	var summary string
	if inst.Result != nil {
		summary = fmt.Sprintf("%v", inst.Result.Output)
		if len(summary) > 500 {
			summary = summary[:500] + "..."
		}
	}
	status := string(inst.Status)
	if inst.Error != "" {
		status += ": " + inst.Error
	}
	notification := fmt.Sprintf("[Subagent %s completed (%s)] %s", inst.ID, status, summary)
	history.AppendNotification(notification)
}

// drainMailbox reads from the instance's mailbox and appends messages to
// history until the context is cancelled (agent loop finished).
func (r runtimeSubagentRunner) drainMailbox(ctx context.Context, instanceID string, history *message.History) {
	if r.rt == nil || r.rt.subExec == nil || instanceID == "" {
		return
	}
	inst, err := r.rt.subExec.Get(ctx, instanceID)
	if err != nil {
		return
	}
	mbox := inst.Mailbox()
	if mbox == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-mbox:
			if !ok {
				return
			}
			history.AppendNotification(fmt.Sprintf("[Inter-agent message] %s", msg))
		}
	}
}

// sendInputToSubagent delivers a message to a running subagent. Used by tools.
func (rt *Runtime) sendInputToSubagent(ctx context.Context, id string, msg string) error {
	exec := rt.ensureSubagentExecutor()
	if exec == nil {
		return errors.New("api: subagent executor is not configured")
	}
	return exec.SendInput(id, msg)
}

func (rt *Runtime) spawnSubagent(ctx context.Context, prompt string, activation skills.ActivationContext, req *Request) (subagents.SpawnHandle, error) {
	if rt == nil {
		return subagents.SpawnHandle{}, errors.New("api: runtime is nil")
	}
	exec := rt.ensureSubagentExecutor()
	if exec == nil {
		return subagents.SpawnHandle{}, errors.New("api: subagent manager is not configured")
	}
	if req == nil {
		return subagents.SpawnHandle{}, errors.New("api: request is nil")
	}
	def, builtin := applySubagentTarget(req)
	meta := map[string]any{
		"entrypoint": req.Mode.EntryPoint,
	}
	for k, v := range req.Metadata {
		meta[k] = v
	}
	if session := strings.TrimSpace(req.SessionID); session != "" {
		meta["session_id"] = session
	}
	var parentCtx subagents.Context
	if subCtx, ok := buildSubagentContext(*req, def, builtin); ok {
		parentCtx = subCtx
	}
	// Fork path: mark context as fork and pass parent's system prompt
	// so the child can inherit the parent's conversation for cache sharing.
	if subagents.IsForkTarget(req.TargetSubagent) {
		parentCtx.IsFork = true
		parentCtx.ParentSystemPrompt = rt.opts.SystemPrompt
		if parentCtx.SessionID == "" {
			parentCtx.SessionID = strings.TrimSpace(req.SessionID)
		}
	}
	// Propagate depth: inherit from the current context and increment.
	if currentCtx, ok := subagents.FromContext(ctx); ok {
		parentCtx.Depth = currentCtx.Depth + 1
	}

	// Check background flag from metadata.
	background := false
	if bg, ok := meta["task.background"]; ok {
		if b, ok := bg.(bool); ok {
			background = b
		}
	}
	return exec.Spawn(subagents.WithTaskDispatch(ctx), subagents.SpawnRequest{
		Target:        req.TargetSubagent,
		Instruction:   prompt,
		Activation:    activation,
		ToolWhitelist: normalizeStrings(req.ToolWhitelist),
		Metadata:      meta,
		ParentContext: parentCtx,
		Background:    background,
	})
}

func (rt *Runtime) waitSubagent(ctx context.Context, id string, timeout time.Duration) (subagents.WaitResult, error) {
	exec := rt.ensureSubagentExecutor()
	if exec == nil {
		return subagents.WaitResult{}, errors.New("api: subagent manager is not configured")
	}
	return exec.Wait(ctx, subagents.WaitRequest{ID: id, Timeout: timeout})
}

// selectModelForSubagent returns the appropriate model for the given subagent type.
// Priority: 1) Request.Model override, 2) SubagentModelMapping, 3) default Model.
// Returns the selected model and the tier used (empty string if default).
func (rt *Runtime) selectModelForSubagent(subagentType string, requestTier ModelTier) (model.Model, ModelTier) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Priority 1: Request-level override (方案 C)
	if requestTier != "" {
		if m, ok := rt.opts.ModelPool[requestTier]; ok && m != nil {
			return m, requestTier
		}
	}

	// Priority 2: Subagent type mapping (方案 A)
	if rt.opts.SubagentModelMapping != nil {
		canonical := strings.ToLower(strings.TrimSpace(subagentType))
		if tier, ok := rt.opts.SubagentModelMapping[canonical]; ok {
			if rt.opts.ModelPool != nil {
				if m, ok := rt.opts.ModelPool[tier]; ok && m != nil {
					return m, tier
				}
			}
		}
	}

	// Priority 3: Default model
	return rt.opts.Model, ""
}

// ---------------------------------------------------------------------------
// AgentRunner adapter — bridges the tool interface to the runtime executor
// ---------------------------------------------------------------------------

type runtimeAgentRunner struct {
	rt *Runtime
}

func (rt *Runtime) agentRunnerAdapter() toolbuiltin.AgentRunner {
	return &runtimeAgentRunner{rt: rt}
}

func (r *runtimeAgentRunner) SpawnAgent(ctx context.Context, req toolbuiltin.SpawnAgentRequest) (*toolbuiltin.SpawnAgentResult, error) {
	exec := r.rt.ensureSubagentExecutor()
	if exec == nil {
		return nil, errors.New("api: subagent executor is not configured")
	}
	sessionID := defaultSessionID(r.rt.mode.EntryPoint)
	parentCtx := subagents.Context{SessionID: sessionID}
	if req.ForkContext {
		parentCtx.IsFork = true
		parentCtx.ParentSystemPrompt = r.rt.opts.SystemPrompt
		parentCtx.ForkTurns = req.ForkTurns
	}
	if currentCtx, ok := subagents.FromContext(ctx); ok {
		parentCtx.Depth = currentCtx.Depth + 1
		if parentCtx.SessionID == "" {
			parentCtx.SessionID = currentCtx.SessionID
		}
	}

	meta := map[string]any{}
	if req.Model != "" {
		meta["task.model"] = req.Model
	}
	if req.Background {
		meta["task.background"] = true
	}

	target := req.SubagentType
	if req.ForkContext && target == "" {
		target = subagents.ForkSubagentType
	}

	handle, err := exec.Spawn(subagents.WithTaskDispatch(ctx), subagents.SpawnRequest{
		Target:        target,
		Instruction:   req.Prompt,
		Metadata:      meta,
		ParentContext: parentCtx,
		Background:    req.Background,
	})
	if err != nil {
		return nil, err
	}

	// Generate a nickname and store it in metadata.
	existingNicks := r.collectNicknames()
	nick := subagents.GenerateNickname(existingNicks, handle.ID)
	_ = exec.Store().Update(handle.ID, func(inst *subagents.Instance) error {
		if inst.Metadata == nil {
			inst.Metadata = map[string]any{}
		}
		inst.Metadata["nickname"] = nick
		return nil
	})

	return &toolbuiltin.SpawnAgentResult{
		AgentID:  handle.ID,
		Nickname: nick,
	}, nil
}

func (r *runtimeAgentRunner) SendInput(ctx context.Context, agentID string, msg string) error {
	exec := r.rt.ensureSubagentExecutor()
	if exec == nil {
		return errors.New("api: subagent executor is not configured")
	}
	resolved, err := subagents.ResolveAgentID(exec.Store(), agentID)
	if err != nil {
		return err
	}
	return exec.SendInput(resolved, msg)
}

func (r *runtimeAgentRunner) WaitAgent(ctx context.Context, agentID string, timeout time.Duration) (*toolbuiltin.WaitAgentResult, error) {
	exec := r.rt.ensureSubagentExecutor()
	if exec == nil {
		return nil, errors.New("api: subagent executor is not configured")
	}
	resolved, err := subagents.ResolveAgentID(exec.Store(), agentID)
	if err != nil {
		return nil, err
	}
	waited, err := exec.Wait(ctx, subagents.WaitRequest{ID: resolved, Timeout: timeout})
	if err != nil {
		return nil, err
	}
	result := &toolbuiltin.WaitAgentResult{
		AgentID:  resolved,
		TimedOut: waited.TimedOut,
		Status:   string(waited.Instance.Status),
	}
	if waited.Instance.Result != nil {
		result.Output = fmt.Sprint(waited.Instance.Result.Output)
	}
	return result, nil
}

func (r *runtimeAgentRunner) CloseAgent(ctx context.Context, agentID string) (*toolbuiltin.CloseAgentResult, error) {
	exec := r.rt.ensureSubagentExecutor()
	if exec == nil {
		return nil, errors.New("api: subagent executor is not configured")
	}
	resolved, err := subagents.ResolveAgentID(exec.Store(), agentID)
	if err != nil {
		return nil, err
	}
	if cancelErr := exec.Cancel(resolved); cancelErr != nil {
		return nil, cancelErr
	}
	inst, _ := exec.Get(ctx, resolved)
	result := &toolbuiltin.CloseAgentResult{
		AgentID: resolved,
		Status:  string(inst.Status),
	}
	if inst.Result != nil {
		result.Output = fmt.Sprint(inst.Result.Output)
	}
	return result, nil
}

func (r *runtimeAgentRunner) ListAgents(ctx context.Context, parentSession string) ([]toolbuiltin.AgentInfo, error) {
	exec := r.rt.ensureSubagentExecutor()
	if exec == nil {
		return nil, errors.New("api: subagent executor is not configured")
	}
	instances := exec.Store().ListBySession(parentSession)
	out := make([]toolbuiltin.AgentInfo, 0, len(instances))
	for _, inst := range instances {
		nick := ""
		if n, ok := inst.Metadata["nickname"].(string); ok {
			nick = n
		}
		out = append(out, toolbuiltin.AgentInfo{
			ID:       inst.ID,
			Nickname: nick,
			Profile:  inst.Profile,
			Status:   string(inst.Status),
		})
	}
	return out, nil
}

func (r *runtimeAgentRunner) collectNicknames() []string {
	exec := r.rt.ensureSubagentExecutor()
	if exec == nil {
		return nil
	}
	all := exec.Store().ListAll()
	nicks := make([]string, 0, len(all))
	for _, inst := range all {
		if n, ok := inst.Metadata["nickname"].(string); ok && n != "" {
			nicks = append(nicks, n)
		}
	}
	return nicks
}
