package ralphloop

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type initWorktreeOptions struct {
	RepoRoot     string
	BaseBranch   string
	WorkBranch   string
	WorktreeName string
}

type worktreeInitMetadata struct {
	WorktreeID     string `json:"worktree_id"`
	WorktreePath   string `json:"worktree_path"`
	WorkBranch     string `json:"work_branch"`
	BaseBranch     string `json:"base_branch"`
	RuntimeRoot    string `json:"runtime_root"`
	DepsInstalled  bool   `json:"deps_installed"`
	BuildVerified  bool   `json:"build_verified"`
	ReusedWorktree bool   `json:"reused_worktree"`
}

func initWorktree(ctx context.Context, options initWorktreeOptions) (worktreeInitMetadata, error) {
	repoRoot, err := filepath.Abs(options.RepoRoot)
	if err != nil {
		return worktreeInitMetadata{}, err
	}
	workBranch := sanitizeBranchName(options.WorkBranch)
	if workBranch == "" {
		workBranch = "ralph-" + trimToLength(slugifyPrompt(filepath.Base(repoRoot)), 58)
	}
	worktreeName := options.WorktreeName
	if strings.TrimSpace(worktreeName) == "" {
		worktreeName = deriveWorktreeName(workBranch)
	}
	baseBranch := strings.TrimSpace(options.BaseBranch)
	if baseBranch == "" {
		baseBranch = "main"
	}
	unbornRepo := repoHasNoCommits(ctx, repoRoot)

	targetPath := repoRoot
	reusedWorktree := isLinkedWorktree(repoRoot) || unbornRepo
	if !reusedWorktree {
		targetPath = filepath.Join(repoRoot, ".worktrees", worktreeName)
		if _, err := os.Stat(targetPath); err == nil {
			reusedWorktree = true
		}
	}
	if !reusedWorktree {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return worktreeInitMetadata{}, err
		}
		result, err := runCommand(ctx, repoRoot, "git", "-C", repoRoot, "worktree", "add", "-B", workBranch, targetPath, baseBranch)
		if err != nil {
			return worktreeInitMetadata{}, fmt.Errorf("git worktree add failed: %s", commandFailureMessage(result, err, "git worktree add"))
		}
	}

	if err := ensureCleanGitState(ctx, targetPath, workBranch, unbornRepo); err != nil {
		return worktreeInitMetadata{}, err
	}
	worktreeID, err := deriveWorktreeID(targetPath)
	if err != nil {
		return worktreeInitMetadata{}, err
	}
	runtimeRoot := filepath.ToSlash(filepath.Join(".worktree", worktreeID))
	if err := ensureRuntimeDirs(targetPath, runtimeRoot); err != nil {
		return worktreeInitMetadata{}, err
	}
	if err := ensureEnvConfig(targetPath, worktreeID); err != nil {
		return worktreeInitMetadata{}, err
	}
	depsInstalled, err := installDependencies(ctx, targetPath)
	if err != nil {
		return worktreeInitMetadata{}, err
	}
	buildVerified, err := verifyBuild(ctx, targetPath)
	if err != nil {
		return worktreeInitMetadata{}, err
	}
	return worktreeInitMetadata{
		WorktreeID:     worktreeID,
		WorktreePath:   targetPath,
		WorkBranch:     workBranch,
		BaseBranch:     baseBranch,
		RuntimeRoot:    runtimeRoot,
		DepsInstalled:  depsInstalled,
		BuildVerified:  buildVerified,
		ReusedWorktree: reusedWorktree,
	}, nil
}

func ensureRalphLogPath(worktree worktreeInitMetadata) (string, error) {
	logPath := filepath.Join(worktree.WorktreePath, worktree.RuntimeRoot, "logs", "ralph-loop.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return "", err
	}
	return logPath, nil
}

func cleanupWorktree(ctx context.Context, repoRoot string, worktreePath string) error {
	resolvedRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return err
	}
	resolvedWorktree, err := filepath.Abs(worktreePath)
	if err != nil {
		return err
	}
	if filepath.Clean(resolvedRepoRoot) == filepath.Clean(resolvedWorktree) {
		return nil
	}
	result, err := runCommand(ctx, repoRoot, "git", "-C", repoRoot, "worktree", "remove", "--force", worktreePath)
	if err != nil {
		return fmt.Errorf("failed to remove worktree %s: %s", worktreePath, commandFailureMessage(result, err, "git worktree remove"))
	}
	return nil
}

func ensureCleanGitState(ctx context.Context, worktreePath string, workBranch string, unbornRepo bool) error {
	if strings.TrimSpace(workBranch) == "" {
		return fmt.Errorf("missing work branch")
	}
	status, err := runCommand(ctx, worktreePath, "git", "-C", worktreePath, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("git status failed: %s", commandFailureMessage(status, err, "git status"))
	}
	if strings.TrimSpace(status.Stdout) != "" && !unbornRepo {
		stash, err := runCommand(ctx, worktreePath, "git", "-C", worktreePath, "stash", "push", "--include-untracked", "-m", "ralph-loop init auto-stash")
		if err != nil {
			return fmt.Errorf("git stash failed: %s", commandFailureMessage(stash, err, "git stash"))
		}
	}
	if unbornRepo {
		headRef, err := runCommand(ctx, worktreePath, "git", "-C", worktreePath, "symbolic-ref", "--short", "HEAD")
		if err == nil && strings.TrimSpace(headRef.Stdout) == workBranch {
			return nil
		}
		switchResult, switchErr := runCommand(ctx, worktreePath, "git", "-C", worktreePath, "symbolic-ref", "HEAD", "refs/heads/"+workBranch)
		if switchErr != nil {
			return fmt.Errorf("git symbolic-ref failed: %s", commandFailureMessage(switchResult, switchErr, "git symbolic-ref"))
		}
		return nil
	}
	currentBranch, err := runCommand(ctx, worktreePath, "git", "-C", worktreePath, "branch", "--show-current")
	if err == nil && strings.TrimSpace(currentBranch.Stdout) == workBranch {
		return nil
	}
	check, err := runCommand(ctx, worktreePath, "git", "-C", worktreePath, "rev-parse", "--verify", workBranch)
	if err == nil && strings.TrimSpace(check.Stdout) != "" {
		checkOut, err := runCommand(ctx, worktreePath, "git", "-C", worktreePath, "checkout", workBranch)
		if err != nil {
			return fmt.Errorf("git checkout failed: %s", commandFailureMessage(checkOut, err, "git checkout"))
		}
		return nil
	}
	create, err := runCommand(ctx, worktreePath, "git", "-C", worktreePath, "checkout", "-b", workBranch)
	if err != nil {
		return fmt.Errorf("git checkout -b failed: %s", commandFailureMessage(create, err, "git checkout -b"))
	}
	return nil
}

func deriveWorktreeID(worktreePath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(worktreePath)
	if err != nil {
		resolved = worktreePath
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	sum := sha1.Sum([]byte(abs))
	prefix := sanitizeToken(filepath.Base(abs), 32)
	if prefix == "" {
		prefix = "worktree"
	}
	return fmt.Sprintf("%s-%x", prefix, sum[:4]), nil
}

func ensureRuntimeDirs(worktreePath string, runtimeRoot string) error {
	for _, suffix := range []string{"logs", "tmp", "run", "telemetry"} {
		if err := os.MkdirAll(filepath.Join(worktreePath, runtimeRoot, suffix), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func ensureEnvConfig(worktreePath string, worktreeID string) error {
	example := filepath.Join(worktreePath, ".env.example")
	envPath := filepath.Join(worktreePath, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		if data, err := os.ReadFile(example); err == nil {
			if err := os.WriteFile(envPath, data, 0o644); err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat(envPath); err == nil {
		return upsertEnvVar(envPath, "DISCODE_WORKTREE_ID", worktreeID)
	}
	return nil
}

func upsertEnvVar(path string, key string, value string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	replaced := false
	for index, line := range lines {
		if strings.HasPrefix(line, key+"=") {
			lines[index] = key + "=" + value
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, key+"="+value)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func installDependencies(ctx context.Context, worktreePath string) (bool, error) {
	switch {
	case fileExists(filepath.Join(worktreePath, "bun.lockb")) || fileExists(filepath.Join(worktreePath, "bun.lock")):
		result, err := runCommand(ctx, worktreePath, "bun", "install")
		return true, wrapCommandError("bun install", result, err)
	case fileExists(filepath.Join(worktreePath, "pnpm-lock.yaml")):
		result, err := runCommand(ctx, worktreePath, "pnpm", "install", "--frozen-lockfile")
		return true, wrapCommandError("pnpm install", result, err)
	case fileExists(filepath.Join(worktreePath, "yarn.lock")):
		result, err := runCommand(ctx, worktreePath, "yarn", "install", "--frozen-lockfile")
		return true, wrapCommandError("yarn install", result, err)
	case fileExists(filepath.Join(worktreePath, "package.json")):
		result, err := runCommand(ctx, worktreePath, "npm", "install")
		return true, wrapCommandError("npm install", result, err)
	case fileExists(filepath.Join(worktreePath, "Cargo.toml")):
		result, err := runCommand(ctx, worktreePath, "cargo", "fetch")
		return true, wrapCommandError("cargo fetch", result, err)
	case fileExists(filepath.Join(worktreePath, "go.mod")):
		result, err := runCommand(ctx, worktreePath, "go", "mod", "download")
		return true, wrapCommandError("go mod download", result, err)
	default:
		return false, nil
	}
}

func verifyBuild(ctx context.Context, worktreePath string) (bool, error) {
	switch {
	case fileExists(filepath.Join(worktreePath, "package.json")):
		buildScript, err := packageHasBuildScript(worktreePath)
		if err != nil {
			return false, err
		}
		if buildScript {
			result, err := runCommand(ctx, worktreePath, "npm", "run", "build")
			return true, wrapCommandError("npm run build", result, err)
		}
		return false, nil
	case fileExists(filepath.Join(worktreePath, "Cargo.toml")):
		result, err := runCommand(ctx, worktreePath, "cargo", "build")
		return true, wrapCommandError("cargo build", result, err)
	case fileExists(filepath.Join(worktreePath, "go.mod")):
		result, err := runCommand(ctx, worktreePath, "go", "test", "./...")
		return true, wrapCommandError("go test ./...", result, err)
	default:
		return false, nil
	}
}

func packageHasBuildScript(worktreePath string) (bool, error) {
	content, err := os.ReadFile(filepath.Join(worktreePath, "package.json"))
	if err != nil {
		return false, err
	}
	return bytes.Contains(content, []byte(`"build"`)), nil
}

func isLinkedWorktree(repoRoot string) bool {
	info, err := os.Stat(filepath.Join(repoRoot, ".git"))
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func wrapCommandError(name string, result commandResult, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s failed: %s", name, commandFailureMessage(result, err, name))
}

func repoHasNoCommits(ctx context.Context, repoRoot string) bool {
	result, err := runCommand(ctx, repoRoot, "git", "-C", repoRoot, "rev-parse", "--verify", "HEAD")
	if err == nil && strings.TrimSpace(result.Stdout) != "" {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(result.Stderr + "\n" + result.Stdout))
	return strings.Contains(message, "unknown revision") || strings.Contains(message, "ambiguous argument 'head'") || strings.Contains(message, "needed a single revision")
}

type commandResult struct {
	Stdout string
	Stderr string
}

func runCommand(ctx context.Context, dir string, command string, args ...string) (commandResult, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return commandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}, err
}

func commandFailureMessage(result commandResult, err error, fallback string) string {
	if message := strings.TrimSpace(result.Stderr); message != "" {
		return message
	}
	if message := strings.TrimSpace(result.Stdout); message != "" {
		return message
	}
	if err != nil {
		return err.Error()
	}
	return fallback
}
