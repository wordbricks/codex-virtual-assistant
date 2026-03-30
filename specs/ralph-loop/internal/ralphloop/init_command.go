package ralphloop

import (
	"encoding/json"
	"fmt"
)

func executeInitCommand(runCtx runContext) int {
	workBranch := runCtx.command.InitOptions.WorkBranch
	if workBranch == "" {
		workBranch = "ralph-" + trimToLength(slugifyPrompt(filepathBase(runCtx.repoRoot)), 58)
	}
	request := map[string]any{
		"command":     "init",
		"base_branch": runCtx.command.InitOptions.BaseBranch,
		"work_branch": workBranch,
		"dry_run":     runCtx.command.InitOptions.DryRun,
	}
	sideEffects := []string{
		"create or reuse a git worktree",
		"clean git state and checkout the work branch",
		"install dependencies",
		"verify the project build",
		"prepare environment files and runtime directories",
	}
	if runCtx.command.InitOptions.DryRun {
		return writeDryRun(runCtx, request, sideEffects)
	}

	textProgress(runCtx, "initializing worktree for branch %s", workBranch)
	metadata, err := initWorktree(runCtx.ctx, initWorktreeOptions{
		RepoRoot:     runCtx.repoRoot,
		BaseBranch:   runCtx.command.InitOptions.BaseBranch,
		WorkBranch:   workBranch,
		WorktreeName: deriveWorktreeName(workBranch),
	})
	if err != nil {
		return writeCommandError(runCtx.stdout, runCtx.stderr, runCtx.command.Common.Output, string(runCtx.command.Kind), err)
	}

	payload := map[string]any{
		"command":         "init",
		"status":          "ok",
		"worktree_id":     metadata.WorktreeID,
		"worktree_path":   metadata.WorktreePath,
		"work_branch":     metadata.WorkBranch,
		"base_branch":     metadata.BaseBranch,
		"deps_installed":  metadata.DepsInstalled,
		"build_verified":  metadata.BuildVerified,
		"runtime_root":    metadata.RuntimeRoot,
		"reused_worktree": metadata.ReusedWorktree,
	}
	if runCtx.command.Common.Output == OutputText {
		encoded, _ := json.MarshalIndent(payload, "", "  ")
		_, _ = fmt.Fprintf(runCtx.stdout, "%s\n", encoded)
		return 0
	}
	return writeCommandResult(runCtx, payload)
}

func writeDryRun(runCtx runContext, request map[string]any, sideEffects []string) int {
	payload := map[string]any{
		"command":      runCtx.command.Kind,
		"status":       "ok",
		"dry_run":      true,
		"request":      request,
		"side_effects": sideEffects,
	}
	return writeCommandResult(runCtx, payload)
}

func filepathBase(path string) string {
	trimmed := path
	for len(trimmed) > 1 && trimmed[len(trimmed)-1] == filepathSeparator {
		trimmed = trimmed[:len(trimmed)-1]
	}
	index := len(trimmed) - 1
	for index >= 0 && trimmed[index] != filepathSeparator {
		index--
	}
	return trimmed[index+1:]
}

const filepathSeparator = '/'
