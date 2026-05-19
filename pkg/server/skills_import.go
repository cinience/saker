// skills_import.go: HTTP handlers and high-level skill import orchestration.
package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/server/skillimport"
)

func (h *Handler) handleSkillImportPreview(req Request) Response {
	var params skillimport.Params
	if err := decodeParams(req.Params, &params); err != nil {
		return h.invalidParams(req.ID, "invalid params: "+err.Error())
	}
	if h.runtime == nil {
		return h.internalError(req.ID, "runtime not initialized")
	}

	result, err := previewSkillImport(h.runtime, params)
	if err != nil {
		return h.invalidParams(req.ID, err.Error())
	}
	return h.success(req.ID, result)
}

func (h *Handler) handleSkillImport(req Request) Response {
	var params skillimport.Params
	if err := decodeParams(req.Params, &params); err != nil {
		return h.invalidParams(req.ID, "invalid params: "+err.Error())
	}
	if h.runtime == nil {
		return h.internalError(req.ID, "runtime not initialized")
	}

	taskID := h.taskTracker.Create("skill/import", "")
	go h.runSkillImportTask(taskID, params)
	return h.success(req.ID, map[string]any{"taskId": taskID})
}

func (h *Handler) runSkillImportTask(taskID string, params skillimport.Params) {
	logLine := func(line string) {
		h.taskTracker.AppendLog(taskID, line)
	}
	progress := func(value int, message string) {
		h.taskTracker.UpdateProgress(taskID, value, message)
	}

	progress(5, "validating import request")
	logLine("Validating import request")

	sourceType, paths, err := skillimport.NormalizeParams(params)
	if err != nil {
		h.taskTracker.Fail(taskID, err.Error())
		return
	}

	progress(15, "preparing import source")
	logLine("Preparing import source")

	sourceRoot, cleanup, err := skillimport.PrepareSource(sourceType, params)
	if err != nil {
		h.taskTracker.Fail(taskID, err.Error())
		return
	}
	if cleanup != nil {
		defer cleanup()
	}

	if sourceType != skillimport.SourcePath && len(paths) == 0 {
		progress(35, "discovering skill directories")
		logLine("Discovering skill directories")
		paths, err = skillimport.DiscoverPaths(sourceRoot)
		if err != nil {
			h.taskTracker.Fail(taskID, err.Error())
			return
		}
		logLine(fmt.Sprintf("Discovered %d skill directories", len(paths)))
	}

	targetRoot, err := skillimport.ResolveTargetRoot(h.runtime, params.TargetScope)
	if err != nil {
		h.taskTracker.Fail(taskID, err.Error())
		return
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		h.taskTracker.Fail(taskID, fmt.Sprintf("create target root: %v", err))
		return
	}

	imported := make([]string, 0, len(paths))
	items := make([]skillimport.ItemResult, 0, len(paths))
	for idx, sourcePath := range paths {
		progress(45+((idx)*45)/maxInt(len(paths), 1), "importing skills")
		logLine("Importing " + sourcePath)

		skillSource := sourcePath
		if sourceType != skillimport.SourcePath {
			skillSource = filepath.Join(sourceRoot, filepath.Clean(sourcePath))
		}

		skillID, err := skillimport.ValidateSkill(skillSource)
		if err != nil {
			h.taskTracker.Fail(taskID, err.Error())
			return
		}
		targetDir := filepath.Join(targetRoot, skillID)
		conflictAction, err := skillimport.PrepareTargetDir(targetDir, params.ConflictStrategy)
		if err != nil {
			h.taskTracker.Fail(taskID, err.Error())
			return
		}
		if conflictAction == "skipped" {
			logLine(fmt.Sprintf("Skipped existing skill %s", skillID))
			items = append(items, skillimport.ItemResult{
				SkillID:    skillID,
				SourcePath: skillSource,
				Path:       targetDir,
				Status:     "skipped",
				Message:    "target already exists",
			})
			continue
		}
		if err := skillimport.CopyDir(skillSource, targetDir); err != nil {
			h.taskTracker.Fail(taskID, err.Error())
			return
		}
		imported = append(imported, targetDir)
		items = append(items, skillimport.ItemResult{
			SkillID:    skillID,
			SourcePath: skillSource,
			Path:       targetDir,
			Status:     "imported",
			Message:    conflictAction,
		})
	}

	progress(92, "reloading skills")
	logLine("Reloading skills")
	if errs := h.runtime.ReloadSkills(); len(errs) > 0 {
		for _, reloadErr := range errs {
			logLine("Reload warning: " + reloadErr.Error())
		}
		h.taskTracker.Fail(taskID, errs[0].Error())
		return
	}

	logLine("Import completed")
	h.taskTracker.UpdateProgress(taskID, 100, "skill import completed")
	h.taskTracker.Complete(taskID, map[string]any{
		"ok":               true,
		"message":          "imported",
		"targetScope":      string(params.TargetScope),
		"path":             firstString(imported),
		"paths":            imported,
		"items":            items,
		"importedSkills":   collectImportItemSkillIDs(items, "imported"),
		"skippedSkills":    collectImportItemSkillIDs(items, "skipped"),
		"conflictStrategy": string(skillimport.NormalizeConflictStrategy(params.ConflictStrategy)),
	})
}

func previewSkillImport(rt *api.Runtime, params skillimport.Params) (map[string]any, error) {
	sourceType, paths, err := skillimport.NormalizeParams(params)
	if err != nil {
		return nil, err
	}
	sourceRoot, cleanup, err := skillimport.PrepareSource(sourceType, params)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	if sourceType != skillimport.SourcePath && len(paths) == 0 {
		paths, err = skillimport.DiscoverPaths(sourceRoot)
		if err != nil {
			return nil, err
		}
	}

	targetRoot, err := skillimport.ResolveTargetRoot(rt, params.TargetScope)
	if err != nil {
		return nil, err
	}
	items, err := buildSkillImportPreviewItems(sourceType, paths, sourceRoot, targetRoot)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"items":             items,
		"targetScope":       string(params.TargetScope),
		"conflictStrategy":  string(skillimport.NormalizeConflictStrategy(params.ConflictStrategy)),
		"readySkills":       collectImportItemSkillIDs(items, "ready"),
		"conflictingSkills": collectImportItemSkillIDs(items, "conflict"),
	}, nil
}

func buildSkillImportPreviewItems(sourceType skillimport.SourceType, paths []string, sourceRoot string, targetRoot string) ([]skillimport.ItemResult, error) {
	items := make([]skillimport.ItemResult, 0, len(paths))
	for _, sourcePath := range paths {
		skillSource := sourcePath
		if sourceType != skillimport.SourcePath {
			skillSource = filepath.Join(sourceRoot, filepath.Clean(sourcePath))
		}
		skillID, err := skillimport.ValidateSkill(skillSource)
		if err != nil {
			return nil, err
		}
		targetDir := filepath.Join(targetRoot, skillID)
		status := "ready"
		message := "new import"
		if _, err := os.Stat(targetDir); err == nil {
			status = "conflict"
			message = "target already exists"
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		items = append(items, skillimport.ItemResult{
			SkillID:    skillID,
			SourcePath: skillSource,
			Path:       targetDir,
			Status:     status,
			Message:    message,
		})
	}
	return items, nil
}

func decodeParams(input map[string]any, out any) error {
	raw, err := jsonMarshal(input)
	if err != nil {
		return err
	}
	return jsonUnmarshal(raw, out)
}

var (
	jsonMarshal   = func(v any) ([]byte, error) { return json.Marshal(v) }
	jsonUnmarshal = func(data []byte, v any) error { return json.Unmarshal(data, v) }
)

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstString(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func collectImportItemSkillIDs(items []skillimport.ItemResult, status string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.Status == status {
			out = append(out, item.SkillID)
		}
	}
	return out
}
