package ralphloop

import "testing"

func TestBuildSetupPromptIncludesInitCommand(t *testing.T) {
	prompt := buildSetupPrompt(setupPromptOptions{
		UserPrompt:   "Implement CLI",
		PlanPath:     "/repo/docs/exec-plans/active/cli.md",
		WorktreePath: "/repo/.worktrees/cli",
		WorktreeID:   "cli-1234",
		WorkBranch:   "ralph-cli",
		BaseBranch:   "main",
	})
	if want := "./ralph-loop init --base-branch main --work-branch ralph-cli"; !containsSubstring(prompt, want) {
		t.Fatalf("setup prompt missing %q", want)
	}
}

func TestBuildPrPromptIncludesAutoMerge(t *testing.T) {
	prompt := buildPrPrompt(prPromptOptions{
		PlanPath:   "/repo/docs/exec-plans/active/cli.md",
		BaseBranch: "main",
	})
	if want := "gh pr merge --auto --squash"; !containsSubstring(prompt, want) {
		t.Fatalf("pr prompt missing %q", want)
	}
}

func containsSubstring(haystack string, needle string) bool {
	return len(haystack) >= len(needle) && stringIndex(haystack, needle) >= 0
}

func stringIndex(haystack string, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
