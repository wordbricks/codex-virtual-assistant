package ralphloop

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func validateCommand(command ParsedCommand, invokeCwd string) error {
	if err := validateCommon(command.Common, invokeCwd); err != nil {
		return err
	}
	switch command.Kind {
	case commandMain:
		if err := validateResourceIdentifier(command.MainOptions.WorkBranch, "work_branch"); err != nil {
			return err
		}
		if command.MainOptions.MaxIterations <= 0 {
			return fmt.Errorf("invalid value for max_iterations: must be > 0")
		}
		if command.MainOptions.TimeoutSeconds <= 0 {
			return fmt.Errorf("invalid value for timeout: must be > 0")
		}
		if command.MainOptions.TurnIdleTimeoutSeconds <= 0 {
			return fmt.Errorf("invalid value for turn_idle_timeout: must be > 0")
		}
	case commandInit:
		if command.InitOptions.WorkBranch != "" {
			if err := validateResourceIdentifier(command.InitOptions.WorkBranch, "work_branch"); err != nil {
				return err
			}
		}
	case commandTail:
		if err := validateSelector(command.TailOptions.Selector); err != nil {
			return err
		}
	case commandList:
		if err := validateSelector(command.ListOptions.Selector); err != nil {
			return err
		}
	case commandSchemaCmd:
		if err := validateSelector(command.SchemaOptions.Command); err != nil {
			return err
		}
	}
	return nil
}

func validateCommon(common CommonOptions, invokeCwd string) error {
	switch common.Output {
	case OutputText, OutputJSON, OutputNDJSON:
	default:
		return fmt.Errorf("invalid value for --output: %s", common.Output)
	}
	if common.Page <= 0 {
		return fmt.Errorf("invalid value for --page: must be > 0")
	}
	if common.PageSize <= 0 {
		return fmt.Errorf("invalid value for --page-size: must be > 0")
	}
	if common.OutputFile != "" {
		if _, err := sandboxOutputPath(invokeCwd, common.OutputFile); err != nil {
			return err
		}
	}
	return nil
}

func validateSelector(selector string) error {
	if selector == "" {
		return nil
	}
	return validateResourceIdentifier(selector, "selector")
}

func validateResourceIdentifier(value string, label string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if containsControlChars(trimmed) {
		return fmt.Errorf("invalid %s: control characters are not allowed", label)
	}
	lower := strings.ToLower(trimmed)
	for _, token := range []string{"../", "..\\", "%2e%2e", "%2f", "%5c", "%252f", "%255c", "%3f", "%23", "?", "#"} {
		if strings.Contains(lower, token) {
			return fmt.Errorf("invalid %s: path-like traversal and query fragments are not allowed", label)
		}
	}
	return nil
}

func containsControlChars(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return true
		}
	}
	return false
}

func sanitizeText(value string) string {
	replacer := strings.NewReplacer(
		"\x00", "",
		"\r", "",
		"\u2028", " ",
		"\u2029", " ",
		"<system>", "[system]",
		"</system>", "[/system]",
		"<assistant>", "[assistant]",
		"</assistant>", "[/assistant]",
	)
	return strings.TrimSpace(replacer.Replace(value))
}

func sandboxOutputPath(invokeCwd string, requested string) (string, error) {
	if err := validateResourceIdentifier(requested, "output-file"); err != nil {
		return "", err
	}
	base, err := filepath.EvalSymlinks(invokeCwd)
	if err != nil {
		base, err = filepath.Abs(invokeCwd)
		if err != nil {
			return "", err
		}
	}
	target := requested
	if !filepath.IsAbs(target) {
		target = filepath.Join(base, target)
	}
	target = filepath.Clean(target)
	parent := filepath.Dir(target)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		resolvedParent = parent
	}
	resolvedTarget := filepath.Join(resolvedParent, filepath.Base(target))
	rel, err := filepath.Rel(base, resolvedTarget)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid output-file: path escapes current working directory")
	}
	if parsed, err := url.Parse(requested); err == nil && parsed.Scheme != "" {
		return "", fmt.Errorf("invalid output-file: URL-like paths are not allowed")
	}
	return resolvedTarget, nil
}
