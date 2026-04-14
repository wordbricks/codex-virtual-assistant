package main

import (
	"fmt"
	"strings"

	"github.com/siisee11/CodexVirtualAssistant/internal/config"
	workspacepkg "github.com/siisee11/CodexVirtualAssistant/internal/workspace"
)

func cmdWorkspace(args []string, jsonMode bool) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cva workspace <lint> [project_slug ...]")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	switch args[0] {
	case "lint":
		reports, err := workspacepkg.LintProjects(cfg.EffectiveProjectsDir(), args[1:])
		if err != nil {
			return err
		}
		if jsonMode {
			return printJSON(reports)
		}
		fmt.Print(formatWorkspaceLintReports(reports))
		for _, report := range reports {
			if len(report.Failures) > 0 {
				return fmt.Errorf("workspace lint failed")
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown workspace subcommand: %s", args[0])
	}
}

func formatWorkspaceLintReports(reports []workspacepkg.LintReport) string {
	var b strings.Builder
	for idx, report := range reports {
		if idx > 0 {
			b.WriteString("\n")
		}
		if len(report.Failures) == 0 {
			fmt.Fprintf(&b, "%s: ok\n", report.ProjectSlug)
			continue
		}
		fmt.Fprintf(&b, "%s: %d issue(s)\n", report.ProjectSlug, len(report.Failures))
		for _, failure := range report.Failures {
			fmt.Fprintf(&b, "- [%s] %s: %s\n", failure.Kind, failure.Path, failure.Message)
		}
	}
	return b.String()
}
