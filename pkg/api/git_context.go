package api

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxGitStatusChars = 2000

// GitContext holds git status information collected at session start.
type GitContext struct {
	Branch        string
	DefaultBranch string
	UserName      string
	Status        string
	RecentCommits string
}

// CollectGitContext gathers git status at session start for injection into the system prompt.
// Returns empty string if projectRoot is not a git repository.
func CollectGitContext(projectRoot string) string {
	gitDir := filepath.Join(projectRoot, ".git")
	if fi, err := os.Stat(gitDir); err != nil || !fi.IsDir() {
		return ""
	}

	ctx := collectGitInfo(projectRoot)
	return formatGitContext(ctx)
}

func collectGitInfo(root string) GitContext {
	var ctx GitContext

	ctx.Branch = gitCmd(root, "branch", "--show-current")
	if ctx.Branch == "" {
		ctx.Branch = gitCmd(root, "rev-parse", "--short", "HEAD")
	}

	ctx.DefaultBranch = detectDefaultBranch(root)
	ctx.UserName = gitCmd(root, "config", "user.name")

	status := gitCmd(root, "--no-optional-locks", "status", "--short")
	if len(status) > maxGitStatusChars {
		status = status[:maxGitStatusChars] + "\n... (truncated, run `git status` for full output)"
	}
	ctx.Status = status

	ctx.RecentCommits = gitCmd(root, "--no-optional-locks", "log", "--oneline", "-n", "5")

	return ctx
}

func detectDefaultBranch(root string) string {
	for _, candidate := range []string{"main", "master"} {
		if out := gitCmd(root, "rev-parse", "--verify", candidate); out != "" {
			return candidate
		}
	}
	remote := gitCmd(root, "remote")
	if remote == "" {
		return "main"
	}
	firstRemote := strings.Split(remote, "\n")[0]
	head := gitCmd(root, "symbolic-ref", fmt.Sprintf("refs/remotes/%s/HEAD", firstRemote))
	if head != "" {
		parts := strings.Split(head, "/")
		return parts[len(parts)-1]
	}
	return "main"
}

func formatGitContext(ctx GitContext) string {
	var sb strings.Builder
	sb.WriteString("gitStatus: This is the git status at the start of the conversation. Note that this status is a snapshot in time, and will not update during the conversation.\n\n")
	sb.WriteString(fmt.Sprintf("Current branch: %s\n\n", ctx.Branch))
	sb.WriteString(fmt.Sprintf("Main branch (you will usually use this for PRs): %s\n\n", ctx.DefaultBranch))
	if ctx.UserName != "" {
		sb.WriteString(fmt.Sprintf("Git user: %s\n\n", ctx.UserName))
	}
	if ctx.Status != "" {
		sb.WriteString(fmt.Sprintf("Status:\n%s\n\n", ctx.Status))
	} else {
		sb.WriteString("Status:\n(clean)\n\n")
	}
	if ctx.RecentCommits != "" {
		sb.WriteString(fmt.Sprintf("Recent commits:\n%s", ctx.RecentCommits))
	}
	return sb.String()
}

func gitCmd(root string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
