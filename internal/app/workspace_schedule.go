package app

import (
	"context"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

const (
	workspaceWikiManagementRootRunID = "run_workspace_wiki_management_root"
	workspaceWikiManagementChatID    = "chat_workspace_wiki_management"
	workspaceWikiManagementSchedule  = "scheduled_workspace_wiki_management_daily"
	workspaceWikiManagementCronExpr  = "0 0 * * *"
)

const workspaceWikiManagementPrompt = `Perform the daily workspace wiki management pass for all projects.

Goals:
1. Keep the workspace structurally clean across all projects.
2. Keep each project wiki focused on durable knowledge rather than operational residue.
3. Preserve important historical material by archiving it instead of deleting it when appropriate.

Required workflow:
1. Run cva workspace lint first and treat every reported failure as a required maintenance task.
2. For each project that needs maintenance, read PROJECT.md, AGENTS.md, wiki/overview.md, wiki/index.md, and wiki/log.md before making changes.
3. Because this run happens once per day, focus only on documents and paths that were created or updated since the previous daily wiki management pass, unless cva workspace lint reports a broader structural problem.
4. Inspect each affected project's root, wiki/, raw/, scripts/, runs/, and .cache/ only within that recent-change window, except where broader inspection is required to fix a lint failure safely.
5. Fix safe structural drift when possible.
6. If a fix would be risky or behavior-changing, do not force it; record the issue in that project's wiki instead.

Maintenance rules:
- Keep .browser-profile in the project root when that project's policy says it is an intentional reusable project asset.
- Keep durable knowledge in wiki/.
- Keep immutable source material in raw/.
- Keep reusable code in scripts/.
- Keep run-specific evidence, temporary outputs, and archives in runs/.
- Do not allow prompt-like procedural task pages in wiki/topics/.
- Do not allow run-history dump pages in wiki/reports/.
- Keep wiki/log.md compressed as a timeline index. Do not expand it into a full execution dump.
- Archive historical detail under runs/archive/ when needed instead of deleting it casually.

Expected checks per affected project:
- root clutter or forbidden runtime residue introduced or updated since the last daily wiki management pass
- executable files incorrectly placed in raw/imports
- procedural topic pages newly added to wiki/topics
- run-history dump pages newly added to wiki/reports
- obvious wiki/index.md or wiki/overview.md drift relative to the current filesystem
- stale references introduced by recent edits, such as evidence/ or artifacts/ where runs/evidence/ or runs/artifacts/ should be used

Expected outputs:
- Apply safe structural cleanup where needed.
- Update wiki/index.md, wiki/overview.md, or wiki/open-questions.md only when the structure summary or project understanding has materially changed.
- Append one concise maintenance entry to wiki/log.md only for projects where a material change was made.
- If no material changes are needed for a project, leave it unchanged.

Constraints:
- Use cva workspace lint as the authoritative first-pass detector.
- Prefer preserving history via runs/archive/ over deletion.
- Do not create new prompt-derived wiki pages.
- Do not create new run-dump reports in wiki/reports.
- Do not mutate raw source documents unless correction is clearly necessary.
- Keep the run bounded, conservative, and maintenance-focused.`

func ensureWorkspaceWikiManagementSchedule(ctx context.Context, repo *store.SQLiteRepository, cfg config.Config, now time.Time) error {
	if repo == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}

	if err := ensureWorkspaceWikiManagementRootRun(ctx, repo, cfg, now); err != nil {
		return err
	}

	nextScheduledFor, err := assistant.NextCronOccurrence(workspaceWikiManagementCronExpr, now.In(time.Local))
	if err != nil {
		return err
	}

	createdAt := now.UTC()
	runID := ""
	errorMessage := ""
	var triggeredAt *time.Time
	if existing, err := repo.GetScheduledRun(ctx, workspaceWikiManagementSchedule); err == nil {
		createdAt = existing.CreatedAt
		runID = existing.RunID
		errorMessage = strings.TrimSpace(existing.ErrorMessage)
		triggeredAt = existing.TriggeredAt
	}

	return repo.SaveScheduledRun(ctx, assistant.ScheduledRun{
		ID:                    workspaceWikiManagementSchedule,
		ChatID:                workspaceWikiManagementChatID,
		ParentRunID:           workspaceWikiManagementRootRunID,
		UserRequestRaw:        workspaceWikiManagementPrompt,
		MaxGenerationAttempts: max(1, cfg.MaxGenerationAttempts),
		CronExpr:              workspaceWikiManagementCronExpr,
		ScheduledFor:          nextScheduledFor,
		Status:                assistant.ScheduledRunStatusPending,
		RunID:                 runID,
		ErrorMessage:          errorMessage,
		CreatedAt:             createdAt,
		TriggeredAt:           triggeredAt,
	})
}

func ensureWorkspaceWikiManagementRootRun(ctx context.Context, repo *store.SQLiteRepository, cfg config.Config, now time.Time) error {
	if _, err := repo.GetRun(ctx, workspaceWikiManagementRootRunID); err == nil {
		return nil
	}

	run := assistant.NewRun("System root run for the recurring workspace wiki management schedule.", now.UTC(), max(1, cfg.MaxGenerationAttempts))
	run.ID = workspaceWikiManagementRootRunID
	run.ChatID = workspaceWikiManagementChatID
	run.Status = assistant.RunStatusCompleted
	run.Phase = assistant.RunPhaseCompleted
	run.GateRoute = assistant.RunRouteWorkflow
	run.GateReason = "Bootstrap parent for the daily workspace wiki management schedule."
	completedAt := now.UTC()
	run.CompletedAt = &completedAt
	run.UpdatedAt = completedAt
	return repo.SaveRun(ctx, run)
}
