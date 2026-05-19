// Package skillimport provides the pipeline logic for importing skills from
// local paths, git repositories, and archive URLs.
package skillimport

type SourceType string

const (
	SourcePath    SourceType = "path"
	SourceGit     SourceType = "git"
	SourceArchive SourceType = "archive"
)

type Scope string

const (
	ScopeLocal  Scope = "local"
	ScopeGlobal Scope = "global"
)

type ConflictMode string

const (
	ConflictOverwrite ConflictMode = "overwrite"
	ConflictSkip      ConflictMode = "skip"
	ConflictError     ConflictMode = "error"
)

type Params struct {
	SourceType       SourceType   `json:"source_type"`
	SourcePath       string       `json:"source_path"`
	SourcePaths      []string     `json:"source_paths"`
	RepoURL          string       `json:"repo_url"`
	ArchiveURL       string       `json:"archive_url"`
	TargetScope      Scope        `json:"target_scope"`
	ConflictStrategy ConflictMode `json:"conflict_strategy"`
}

type ItemResult struct {
	SkillID    string `json:"skill_id"`
	SourcePath string `json:"source_path,omitempty"`
	Path       string `json:"path"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
}

// ConfigProvider abstracts the runtime fields needed by the import pipeline.
type ConfigProvider interface {
	ConfigRoot() string
}
