package agentmessage

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

type LifecycleCard struct {
	Badge      string
	Title      string
	Details    []string
	ReplyHint  string
	Actions    []LifecycleAction
	StatusText string
}

type LifecycleAction struct {
	Label   string
	Value   string
	Variant string
}

func RenderLifecycleCard(card LifecycleCard) (string, error) {
	elements := map[string]any{
		"screen": map[string]any{
			"type":     "Stack",
			"props":    map[string]any{"direction": "vertical", "gap": "md"},
			"children": []string{"header", "body"},
		},
		"header": map[string]any{
			"type":     "Heading",
			"props":    map[string]any{"text": strings.TrimSpace(card.Title), "level": "h3"},
			"children": []string{},
		},
		"body": map[string]any{
			"type":     "Stack",
			"props":    map[string]any{"direction": "vertical", "gap": "sm"},
			"children": []string{},
		},
	}

	bodyChildren := []string{}
	if strings.TrimSpace(card.Badge) != "" {
		elements["badge"] = map[string]any{
			"type":     "Badge",
			"props":    map[string]any{"text": strings.TrimSpace(card.Badge), "variant": "secondary"},
			"children": []string{},
		}
		bodyChildren = append(bodyChildren, "badge")
	}

	if strings.TrimSpace(card.StatusText) != "" {
		elements["status"] = map[string]any{
			"type":     "Text",
			"props":    map[string]any{"text": strings.TrimSpace(card.StatusText), "variant": "body"},
			"children": []string{},
		}
		bodyChildren = append(bodyChildren, "status")
	}

	if len(card.Actions) > 0 || strings.TrimSpace(card.ReplyHint) != "" {
		actions := make([]map[string]any, 0, len(card.Actions))
		for _, action := range card.Actions {
			actions = append(actions, map[string]any{
				"label":   strings.TrimSpace(action.Label),
				"value":   strings.TrimSpace(action.Value),
				"variant": strings.TrimSpace(action.Variant),
			})
		}
		elements["approval"] = map[string]any{
			"type": "ApprovalCard",
			"props": map[string]any{
				"title":     strings.TrimSpace(card.Title),
				"details":   nonEmptyDetails(card.Details),
				"replyHint": strings.TrimSpace(card.ReplyHint),
				"actions":   actions,
			},
			"children": []string{},
		}
		bodyChildren = append(bodyChildren, "approval")
	} else {
		for idx, detail := range nonEmptyDetails(card.Details) {
			key := fmt.Sprintf("detail-%d", idx)
			elements[key] = map[string]any{
				"type":     "Text",
				"props":    map[string]any{"text": detail, "variant": "muted"},
				"children": []string{},
			}
			bodyChildren = append(bodyChildren, key)
		}
	}

	elements["body"].(map[string]any)["children"] = bodyChildren

	spec := map[string]any{
		"root":     "screen",
		"elements": elements,
	}
	payload, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("marshal lifecycle card: %w", err)
	}
	return string(payload), nil
}

func StartedCard(run assistant.Run, summary string) LifecycleCard {
	return LifecycleCard{
		Badge:      "Run started",
		Title:      "CVA accepted your request",
		StatusText: firstNonEmpty(summary, "The run has entered the workflow."),
		Details: []string{
			fmt.Sprintf("Run id: %s", run.ID),
			fmt.Sprintf("Current phase: %s", run.Phase),
		},
	}
}

func WaitingCard(run assistant.Run) LifecycleCard {
	card := LifecycleCard{
		Badge:      "Waiting",
		Title:      "CVA needs your input",
		StatusText: "The run is paused until the requested input arrives.",
	}
	if run.WaitingFor != nil {
		card.Details = []string{
			firstNonEmpty(run.WaitingFor.Title, "Additional input is required."),
			run.WaitingFor.Prompt,
			run.WaitingFor.RiskSummary,
		}
		card.ReplyHint = "Reply in this chat to let CVA continue."
		card.Actions = []LifecycleAction{
			{Label: "Continue", Value: "continue", Variant: "primary"},
		}
	}
	return card
}

func CompletedCard(run assistant.Run) LifecycleCard {
	summary := "The final report was delivered separately in this chat."
	if run.LatestEvaluation != nil && strings.TrimSpace(run.LatestEvaluation.Summary) != "" {
		summary = run.LatestEvaluation.Summary
	}
	return LifecycleCard{
		Badge:      "Completed",
		Title:      "CVA finished the run",
		StatusText: summary,
		Details: []string{
			fmt.Sprintf("Run id: %s", run.ID),
			"The substantive result was sent as the final report card.",
		},
	}
}

func ExhaustedCard(run assistant.Run) LifecycleCard {
	summary := "The run exhausted the available generation attempts."
	if run.LatestEvaluation != nil && strings.TrimSpace(run.LatestEvaluation.Summary) != "" {
		summary = run.LatestEvaluation.Summary
	}
	return LifecycleCard{
		Badge:      "Exhausted",
		Title:      "CVA could not complete the run",
		StatusText: summary,
		Details: []string{
			fmt.Sprintf("Run id: %s", run.ID),
			"Try a narrower request or start a follow-up run with new guidance.",
		},
	}
}

func FailedCard(run assistant.Run, summary string) LifecycleCard {
	return LifecycleCard{
		Badge:      "Failed",
		Title:      "CVA stopped before completion",
		StatusText: firstNonEmpty(summary, "The run could not continue safely."),
		Details: []string{
			fmt.Sprintf("Run id: %s", run.ID),
			"Inspect the run details and retry with the missing information or access.",
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nonEmptyDetails(values []string) []string {
	details := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			details = append(details, trimmed)
		}
	}
	return details
}
