package api

import (
	"strings"
	"testing"
)

func TestSectionIdentity(t *testing.T) {
	t.Run("always returns Saker identity", func(t *testing.T) {
		s := sectionIdentity("any-model")
		if !strings.Contains(s, "Saker") {
			t.Errorf("sectionIdentity should mention Saker, got %q", s)
		}
	})

	t.Run("does not leak model name", func(t *testing.T) {
		s := sectionIdentity("qwen-plus-2025-04")
		if strings.Contains(s, "qwen-plus-2025-04") {
			t.Errorf("sectionIdentity should not leak the model name, got %q", s)
		}
	})

	t.Run("blocks common model names", func(t *testing.T) {
		s := sectionIdentity("deepseek-v3")
		for _, want := range []string{"Claude", "GPT", "Anthropic", "OpenAI"} {
			if !strings.Contains(s, want) {
				t.Errorf("sectionIdentity should mention %q in deny list, got %q", want, s)
			}
		}
	})
}

func TestSectionIntro(t *testing.T) {
	s := sectionIntro()
	if s == "" {
		t.Fatal("sectionIntro returned empty string")
	}
	if !strings.Contains(s, "interactive agent") {
		t.Error("sectionIntro should contain agent identity")
	}
	if !strings.Contains(s, "security testing") {
		t.Error("sectionIntro should contain cyber risk instruction")
	}
}

func TestSectionSystem(t *testing.T) {
	s := sectionSystem()
	if !strings.Contains(s, "permission mode") {
		t.Error("sectionSystem should mention permission mode")
	}
	if !strings.Contains(s, "system-reminder") {
		t.Error("sectionSystem should mention system-reminder tags")
	}
}

func TestSectionDoingTasks(t *testing.T) {
	s := sectionDoingTasks()
	if !strings.Contains(s, "Read files first") {
		t.Error("sectionDoingTasks should mention reading before modifying")
	}
	if !strings.Contains(s, "OWASP") {
		t.Error("sectionDoingTasks should mention security awareness")
	}
	if !strings.Contains(s, "Report outcomes faithfully") {
		t.Error("sectionDoingTasks should mention faithful reporting")
	}
}

func TestSectionActions(t *testing.T) {
	s := sectionActions()
	if !strings.Contains(s, "reversibility") {
		t.Error("sectionActions should mention reversibility")
	}
	if !strings.Contains(s, "force-pushing") {
		t.Error("sectionActions should list risky operations")
	}
}

func TestSectionUsingTools(t *testing.T) {
	t.Run("with known tools", func(t *testing.T) {
		s := sectionUsingTools([]string{"read", "edit", "write", "grep", "glob", "task_create", "ask_user_question"})
		if !strings.Contains(s, "Read instead of cat") {
			t.Error("should mention Read tool")
		}
		if !strings.Contains(s, "Edit instead of sed") {
			t.Error("should mention Edit tool")
		}
		if !strings.Contains(s, "Grep tool") {
			t.Error("should mention Grep tool")
		}
		if !strings.Contains(s, "Glob tool") {
			t.Error("should mention Glob tool")
		}
		if !strings.Contains(s, "Task tools") {
			t.Error("should mention Task tools")
		}
		if !strings.Contains(s, "ask_user_question") {
			t.Error("should mention AskUserQuestion")
		}
	})

	t.Run("with nil tools", func(t *testing.T) {
		s := sectionUsingTools(nil)
		if !strings.Contains(s, "dedicated tool") {
			t.Error("should still have generic guidance")
		}
		if strings.Contains(s, "Read instead of cat") {
			t.Error("should not mention specific tools when nil")
		}
	})

	t.Run("parallel calls guidance", func(t *testing.T) {
		s := sectionUsingTools(nil)
		if !strings.Contains(s, "parallel") {
			t.Error("should mention parallel tool calls")
		}
	})
}

func TestSectionToneAndStyle(t *testing.T) {
	s := sectionToneAndStyle()
	if !strings.Contains(s, "emojis") {
		t.Error("should mention emoji policy")
	}
	if !strings.Contains(s, "file_path:line_number") {
		t.Error("should mention file:line format")
	}
}

func TestSectionOutputEfficiency(t *testing.T) {
	s := sectionOutputEfficiency()
	if !strings.Contains(s, "Before your first tool call") {
		t.Error("should instruct to state intent before first tool call")
	}
	if !strings.Contains(s, "End-of-turn summary") {
		t.Error("should instruct end-of-turn summary")
	}
}

func TestSectionAgentTool(t *testing.T) {
	t.Run("with task tool", func(t *testing.T) {
		s := sectionAgentTool([]string{"task_create", "bash"})
		if s == "" {
			t.Error("should return content when task tools present")
		}
		if !strings.Contains(s, "Subagent") {
			t.Error("should mention subagent")
		}
	})

	t.Run("without task tool", func(t *testing.T) {
		s := sectionAgentTool([]string{"bash", "read"})
		if s != "" {
			t.Error("should return empty when no task tools")
		}
	})
}

func TestSectionEnvironment(t *testing.T) {
	env := environmentInfo{
		CWD:       "/test/project",
		IsGitRepo: true,
		Platform:  "linux",
		Shell:     "bash",
		OSVersion: "Linux 6.1",
		ModelName: "claude-sonnet-4-5",
	}
	s := sectionEnvironment(env)
	if !strings.Contains(s, "/test/project") {
		t.Error("should contain CWD")
	}
	if !strings.Contains(s, "true") {
		t.Error("should contain git repo status")
	}
	if !strings.Contains(s, "linux") {
		t.Error("should contain platform")
	}
	if !strings.Contains(s, "claude-sonnet-4-5") {
		t.Error("should contain model name")
	}
	if !strings.Contains(s, "Current date") {
		t.Error("should contain current date")
	}
	if !strings.Contains(s, "Knowledge cutoff") {
		t.Error("should contain knowledge cutoff for known model")
	}

	t.Run("unknown model has no cutoff", func(t *testing.T) {
		env2 := environmentInfo{CWD: "/x", ModelName: "unknown-model"}
		s2 := sectionEnvironment(env2)
		if strings.Contains(s2, "Knowledge cutoff") {
			t.Error("should not include cutoff for unknown model")
		}
	})
}

func TestSectionLanguage(t *testing.T) {
	t.Run("with language", func(t *testing.T) {
		s := sectionLanguage("Chinese")
		if !strings.Contains(s, "Chinese") {
			t.Error("should contain language name")
		}
		if !strings.Contains(s, "Default to") {
			t.Error("should instruct to respond in language")
		}
	})

	t.Run("empty language defaults to English", func(t *testing.T) {
		s := sectionLanguage("")
		if !strings.Contains(s, "English") {
			t.Error("should default to English")
		}
	})
}

func TestSectionSessionGuidance(t *testing.T) {
	t.Run("CLI with skills", func(t *testing.T) {
		s := sectionSessionGuidance([]string{"skill", "bash"}, EntryPointCLI)
		if !strings.Contains(s, "/<skill-name>") {
			t.Error("should mention skill syntax")
		}
		if !strings.Contains(s, "! <command>") {
			t.Error("should mention interactive command syntax for CLI")
		}
	})

	t.Run("CI mode", func(t *testing.T) {
		s := sectionSessionGuidance([]string{"bash"}, EntryPointCI)
		if strings.Contains(s, "! <command>") {
			t.Error("should not mention interactive commands for CI")
		}
	})
}

func TestSectionMultimodal(t *testing.T) {
	t.Run("with all multimodal tools", func(t *testing.T) {
		s := sectionMultimodal([]string{"image_read", "analyze_video", "video_sampler", "web_fetch", "web_search"})
		if !strings.Contains(s, "multimodal agent") {
			t.Error("should declare multimodal capability")
		}
		if !strings.Contains(s, "image_read") {
			t.Error("should mention ImageRead")
		}
		if !strings.Contains(s, "analyze_video") {
			t.Error("should mention analyze_video")
		}
		if !strings.Contains(s, "web_search") {
			t.Error("should mention WebSearch")
		}
		if !strings.Contains(s, "web_fetch") {
			t.Error("should mention WebFetch")
		}
	})

	t.Run("without multimodal tools", func(t *testing.T) {
		s := sectionMultimodal([]string{"bash", "read"})
		if s != "" {
			t.Error("should return empty when no multimodal tools")
		}
	})

	t.Run("image only", func(t *testing.T) {
		s := sectionMultimodal([]string{"image_read"})
		if !strings.Contains(s, "PNG") {
			t.Error("should mention supported formats")
		}
		if strings.Contains(s, "analyze_video") {
			t.Error("should not mention analyze_video when no video tools")
		}
	})
}

func TestBuildDefaultSystemPrompt(t *testing.T) {
	opts := Options{
		EntryPoint: EntryPointCLI,
		Language:   "English",
	}
	env := environmentInfo{
		CWD:       "/test",
		IsGitRepo: false,
		Platform:  "darwin",
		Shell:     "zsh",
		OSVersion: "Darwin 23.0",
	}
	tools := []string{"bash", "read", "edit", "grep", "glob", "task_create"}

	prompt := buildDefaultSystemPrompt(opts, env, tools)

	sections := []string{
		"interactive agent",            // intro
		"permission mode",              // system
		"software engineering",         // doing tasks
		"reversibility",                // actions
		"dedicated tool",               // using tools
		"emojis",                       // tone
		"Before your first tool call",  // output efficiency
		"/test",                        // environment
		"darwin",                       // platform
		"Default to",                   // language
	}

	for _, expected := range sections {
		if !strings.Contains(prompt, expected) {
			t.Errorf("buildDefaultSystemPrompt missing expected content: %q", expected)
		}
	}
}

func TestBuildDefaultSystemPrompt_UserProvidedNotOverridden(t *testing.T) {
	// When SystemPrompt is already set, buildDefaultSystemPrompt should NOT be
	// called (tested via the agent.go integration). This test verifies the
	// function itself always produces content regardless.
	opts := Options{EntryPoint: EntryPointCLI}
	env := environmentInfo{CWD: "/x", Platform: "linux", Shell: "bash", OSVersion: "Linux"}
	prompt := buildDefaultSystemPrompt(opts, env, nil)
	if prompt == "" {
		t.Fatal("buildDefaultSystemPrompt should never return empty")
	}
}

func TestBuildSystemPromptBlocks(t *testing.T) {
	opts := Options{
		EntryPoint: EntryPointCLI,
		Language:   "Chinese",
	}
	env := environmentInfo{
		CWD:       "/project",
		IsGitRepo: true,
		Platform:  "linux",
		Shell:     "bash",
		OSVersion: "Linux 6.1",
	}
	tools := []string{"bash", "read", "task_create"}

	blocks := buildSystemPromptBlocks(opts, env, tools)

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks (static + boundary + dynamic), got %d", len(blocks))
	}

	// Static block should contain core sections
	if !strings.Contains(blocks[0], "interactive agent") {
		t.Error("static block should contain intro")
	}
	if !strings.Contains(blocks[0], "dedicated tool") {
		t.Error("static block should contain tool guidance")
	}

	// Boundary marker
	if blocks[1] != SystemPromptDynamicBoundary {
		t.Errorf("block[1] should be boundary marker, got %q", blocks[1])
	}

	// Dynamic block should contain environment and language
	if !strings.Contains(blocks[2], "/project") {
		t.Error("dynamic block should contain CWD")
	}
	if !strings.Contains(blocks[2], "Chinese") {
		t.Error("dynamic block should contain language")
	}
}

func TestSystemPromptBuilder(t *testing.T) {
	b := NewSystemPromptBuilder()
	b.Register(SystemPromptSection{
		Name:      "identity",
		Builder:   func() string { return "I am an agent" },
		Cacheable: true,
	})
	b.Register(SystemPromptSection{
		Name:      "env",
		Builder:   func() string { return "CWD: /test" },
		Cacheable: false,
	})
	b.Register(SystemPromptSection{
		Name:      "empty",
		Builder:   func() string { return "" },
		Cacheable: true,
	})

	static, dynamic := b.Build()
	if static != "I am an agent" {
		t.Errorf("static block = %q, want %q", static, "I am an agent")
	}
	if dynamic != "CWD: /test" {
		t.Errorf("dynamic block = %q, want %q", dynamic, "CWD: /test")
	}
}

func TestCollectGitContext(t *testing.T) {
	t.Run("non-git directory returns empty", func(t *testing.T) {
		s := CollectGitContext(t.TempDir())
		if s != "" {
			t.Errorf("expected empty for non-git dir, got %q", s)
		}
	})
}

func TestBuildMCPInstructionsSection(t *testing.T) {
	t.Run("no servers returns empty", func(t *testing.T) {
		s := BuildMCPInstructionsSection(nil)
		if s != "" {
			t.Errorf("expected empty, got %q", s)
		}
	})

	t.Run("servers without instructions returns empty", func(t *testing.T) {
		servers := []MCPServerInfo{{Name: "test", Instructions: ""}}
		s := BuildMCPInstructionsSection(servers)
		if s != "" {
			t.Errorf("expected empty, got %q", s)
		}
	})

	t.Run("formats server instructions", func(t *testing.T) {
		servers := []MCPServerInfo{
			{Name: "github", Instructions: "Use for repo management"},
			{Name: "empty", Instructions: ""},
			{Name: "slack", Instructions: "Use for messaging"},
		}
		s := BuildMCPInstructionsSection(servers)
		if !strings.Contains(s, "# MCP Server Instructions") {
			t.Error("should have section header")
		}
		if !strings.Contains(s, "## github") {
			t.Error("should include github server")
		}
		if !strings.Contains(s, "Use for repo management") {
			t.Error("should include github instructions")
		}
		if !strings.Contains(s, "## slack") {
			t.Error("should include slack server")
		}
		if strings.Contains(s, "## empty") {
			t.Error("should exclude server with empty instructions")
		}
	})
}

func TestSectionContextManagement(t *testing.T) {
	s := sectionContextManagement()
	if !strings.Contains(s, "Write down") {
		t.Error("should instruct to write down important information")
	}
	if !strings.Contains(s, "automatically cleared") {
		t.Error("should mention automatic clearing")
	}
	if !strings.Contains(s, "Context management") {
		t.Error("should have context management header")
	}
	if !strings.Contains(s, "summarized") {
		t.Error("should mention summarization")
	}
}

func TestSectionScratchpad(t *testing.T) {
	t.Run("empty dir returns empty", func(t *testing.T) {
		if s := sectionScratchpad(""); s != "" {
			t.Errorf("expected empty, got %q", s)
		}
	})

	t.Run("non-empty dir includes path", func(t *testing.T) {
		s := sectionScratchpad("/tmp/saker/scratchpad/sess-1")
		if !strings.Contains(s, "/tmp/saker/scratchpad/sess-1") {
			t.Error("should contain the scratchpad path")
		}
		if !strings.Contains(s, "Scratchpad Directory") {
			t.Error("should have section header")
		}
	})
}

func TestSystemPromptDynamicBoundary(t *testing.T) {
	if SystemPromptDynamicBoundary == "" {
		t.Fatal("SystemPromptDynamicBoundary should not be empty")
	}
}

func TestToolNameSet(t *testing.T) {
	set := toolNameSet([]string{"bash", "read", "GREP"})
	if !set["bash"] {
		t.Error("should lowercase tool names")
	}
	if !set["read"] {
		t.Error("should include Read")
	}
	if !set["grep"] {
		t.Error("should include GREP lowercased")
	}
	if set["write"] {
		t.Error("should not include Write")
	}
}
