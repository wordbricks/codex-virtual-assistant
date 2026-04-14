package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
)

func statusColor(s assistant.RunStatus) string {
	switch s {
	case assistant.RunStatusCompleted:
		return colorGreen
	case assistant.RunStatusFailed, assistant.RunStatusExhausted:
		return colorRed
	case assistant.RunStatusCancelled:
		return colorYellow
	case assistant.RunStatusWaiting:
		return colorCyan
	default:
		return colorBlue
	}
}

func statusIcon(s assistant.RunStatus) string {
	switch s {
	case assistant.RunStatusCompleted:
		return "v"
	case assistant.RunStatusFailed, assistant.RunStatusExhausted:
		return "x"
	case assistant.RunStatusCancelled:
		return "-"
	case assistant.RunStatusWaiting:
		return "?"
	default:
		return ">"
	}
}

func formatEvent(ev assistant.RunEvent) string {
	ts := ev.CreatedAt.Local().Format("15:04:05")
	dim := colorDim + ts + colorReset

	switch ev.Type {
	case assistant.EventTypePhaseChanged:
		return fmt.Sprintf("%s %s%s%s %s", dim, colorBold, ev.Phase, colorReset, ev.Summary)
	case assistant.EventTypeAttemptLogged:
		return fmt.Sprintf("%s %sattempt%s %s", dim, colorCyan, colorReset, ev.Summary)
	case assistant.EventTypeEvaluation:
		score, _ := ev.Data["score"].(float64)
		passed, _ := ev.Data["passed"].(bool)
		icon := colorRed + "FAIL" + colorReset
		if passed {
			icon = colorGreen + "PASS" + colorReset
		}
		return fmt.Sprintf("%s %sevaluation%s [%s score:%d] %s", dim, colorYellow, colorReset, icon, int(score), ev.Summary)
	case assistant.EventTypeArtifactAdded:
		title, _ := ev.Data["title"].(string)
		if title == "" {
			title = ev.Summary
		}
		return fmt.Sprintf("%s %sartifact%s %s", dim, colorGreen, colorReset, title)
	case assistant.EventTypeWaiting:
		return fmt.Sprintf("%s %sWAITING%s %s", dim, colorCyan, colorReset, ev.Summary)
	case assistant.EventTypeToolCallStart:
		tool, _ := ev.Data["tool_name"].(string)
		return fmt.Sprintf("%s %stool%s %s %s", dim, colorDim, colorReset, tool, ev.Summary)
	case assistant.EventTypeToolCallEnd:
		return fmt.Sprintf("%s %stool done%s %s", dim, colorDim, colorReset, ev.Summary)
	case assistant.EventTypeReasoning:
		msg := ev.Summary
		if len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		return fmt.Sprintf("%s %sthinking%s %s", dim, colorDim, colorReset, msg)
	case assistant.EventTypeRunCreated:
		return fmt.Sprintf("%s %srun created%s %s", dim, colorBold, colorReset, ev.RunID)
	default:
		return fmt.Sprintf("%s %s %s", dim, ev.Type, ev.Summary)
	}
}

func formatRunSummary(r assistant.Run) string {
	var b strings.Builder
	sc := statusColor(r.Status)
	fmt.Fprintf(&b, "%s[%s %s]%s %s\n", sc, statusIcon(r.Status), r.Status, colorReset, r.ID)
	fmt.Fprintf(&b, "  Request: %s\n", truncate(r.UserRequestRaw, 80))
	if r.TaskSpec.Goal != "" && r.TaskSpec.Goal != r.UserRequestRaw {
		fmt.Fprintf(&b, "  Goal:    %s\n", truncate(r.TaskSpec.Goal, 80))
	}
	fmt.Fprintf(&b, "  Phase:   %s  Attempts: %d/%d\n", r.Phase, r.AttemptCount, r.MaxGenerationAttempts)
	fmt.Fprintf(&b, "  Created: %s", r.CreatedAt.Local().Format(time.DateTime))
	if r.CompletedAt != nil {
		dur := r.CompletedAt.Sub(r.CreatedAt).Round(time.Second)
		fmt.Fprintf(&b, "  Duration: %s", dur)
	}
	b.WriteString("\n")
	if r.WaitingFor != nil {
		fmt.Fprintf(&b, "  %sWaiting: [%s] %s%s\n", colorCyan, r.WaitingFor.Kind, r.WaitingFor.Prompt, colorReset)
	}
	return b.String()
}

func formatRunRecord(rec *store.RunRecord) string {
	var b strings.Builder
	b.WriteString(formatRunSummary(rec.Run))

	if len(rec.Artifacts) > 0 {
		b.WriteString("\n  Artifacts:\n")
		for _, a := range rec.Artifacts {
			url := a.URL
			if url == "" {
				url = a.Path
			}
			fmt.Fprintf(&b, "    [%s] %s  %s\n", a.Kind, a.Title, url)
		}
	}

	if len(rec.Evaluations) > 0 {
		last := rec.Evaluations[len(rec.Evaluations)-1]
		icon := colorRed + "FAIL" + colorReset
		if last.Passed {
			icon = colorGreen + "PASS" + colorReset
		}
		fmt.Fprintf(&b, "\n  Latest Evaluation: %s (score: %d)\n", icon, last.Score)
		fmt.Fprintf(&b, "    %s\n", last.Summary)
	}
	if len(rec.ScheduledRuns) > 0 {
		b.WriteString("\n  Scheduled Runs:\n")
		for _, scheduledRun := range rec.ScheduledRuns {
			fmt.Fprintf(&b, "    [%s] %s  %s\n", scheduledRun.Status, scheduledRun.ID, scheduledRun.ScheduledFor.Local().Format(time.DateTime))
		}
	}

	return b.String()
}

func formatChatList(chats []assistant.Chat) string {
	if len(chats) == 0 {
		return "No chats found.\n"
	}
	var b strings.Builder
	for _, c := range chats {
		sc := statusColor(c.Status)
		fmt.Fprintf(&b, "%s[%s]%s %s  %s\n", sc, c.Status, colorReset, c.ID, truncate(c.Title, 60))
		fmt.Fprintf(&b, "      %s\n", c.UpdatedAt.Local().Format(time.DateTime))
	}
	return b.String()
}

func formatChatRecord(rec *store.ChatRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Chat: %s\n", rec.Chat.ID)
	fmt.Fprintf(&b, "Title: %s\n", rec.Chat.Title)
	fmt.Fprintf(&b, "Status: %s%s%s\n\n", statusColor(rec.Chat.Status), rec.Chat.Status, colorReset)

	for i, rr := range rec.Runs {
		fmt.Fprintf(&b, "--- Run %d ---\n", i+1)
		b.WriteString(formatRunRecord(&rr))
		b.WriteString("\n")
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func formatScheduledRunList(scheduledRuns []assistant.ScheduledRun) string {
	if len(scheduledRuns) == 0 {
		return "No scheduled runs found.\n"
	}
	var b strings.Builder
	for _, scheduledRun := range scheduledRuns {
		fmt.Fprintf(&b, "[%s] %s  %s\n", scheduledRun.Status, scheduledRun.ID, scheduledRun.ScheduledFor.Local().Format(time.DateTime))
		if strings.TrimSpace(scheduledRun.CronExpr) != "" {
			fmt.Fprintf(&b, "      cron: %s\n", scheduledRun.CronExpr)
		}
		fmt.Fprintf(&b, "      %s\n", truncate(scheduledRun.UserRequestRaw, 80))
	}
	return b.String()
}

func formatScheduledRun(scheduledRun assistant.ScheduledRun) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s\n", scheduledRun.Status, scheduledRun.ID)
	fmt.Fprintf(&b, "  Chat:      %s\n", scheduledRun.ChatID)
	fmt.Fprintf(&b, "  Parent:    %s\n", scheduledRun.ParentRunID)
	fmt.Fprintf(&b, "  Scheduled: %s\n", scheduledRun.ScheduledFor.Local().Format(time.DateTime))
	if strings.TrimSpace(scheduledRun.CronExpr) != "" {
		fmt.Fprintf(&b, "  Cron:      %s\n", scheduledRun.CronExpr)
	}
	fmt.Fprintf(&b, "  Created:   %s\n", scheduledRun.CreatedAt.Local().Format(time.DateTime))
	if scheduledRun.TriggeredAt != nil {
		fmt.Fprintf(&b, "  Triggered: %s\n", scheduledRun.TriggeredAt.Local().Format(time.DateTime))
	}
	if scheduledRun.RunID != "" {
		fmt.Fprintf(&b, "  Run:       %s\n", scheduledRun.RunID)
	}
	if scheduledRun.ErrorMessage != "" {
		fmt.Fprintf(&b, "  Error:     %s\n", scheduledRun.ErrorMessage)
	}
	fmt.Fprintf(&b, "  Prompt:    %s\n", truncate(scheduledRun.UserRequestRaw, 160))
	return b.String()
}
