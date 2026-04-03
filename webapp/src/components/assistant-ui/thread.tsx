"use client";

import {
  ArrowUpIcon,
  CheckIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  ClipboardListIcon,
  CopyIcon,
  DownloadIcon,
  FlaskConicalIcon,
  LoaderIcon,
  PencilIcon,
  RefreshCwIcon,
  SquareIcon,
  WandSparklesIcon,
  ThumbsDownIcon,
  ThumbsUpIcon,
} from "lucide-react";
import {
  ActionBarPrimitive,
  AuiIf,
  BranchPickerPrimitive,
  ComposerPrimitive,
  ErrorPrimitive,
  MessagePrimitive,
  ThreadPrimitive,
  useAuiState,
} from "@assistant-ui/react";
import "@assistant-ui/react-markdown/styles/dot.css";

import {
  ComposerAddAttachment,
  ComposerAttachments,
  UserMessageAttachments,
} from "@/components/assistant-ui/attachment";
import { MarkdownText } from "@/components/assistant-ui/markdown-text";
import { Reasoning, ReasoningGroup } from "@/components/assistant-ui/reasoning";
import { ToolFallback } from "@/components/assistant-ui/tool-fallback";
import { ToolGroup } from "@/components/assistant-ui/tool-group";
import { TooltipIconButton } from "@/components/assistant-ui/tooltip-icon-button";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useEffect, useMemo, useState, type ComponentType } from "react";

type ThreadPhaseStatus =
  | "queued"
  | "gating"
  | "answering"
  | "selecting_project"
  | "planning"
  | "contracting"
  | "generating"
  | "evaluating"
  | "scheduling"
  | "reporting"
  | "waiting"
  | "completed"
  | "failed"
  | "exhausted"
  | "cancelled"
  | null;

type ThreadPhaseEvent =
  | "queued"
  | "gating"
  | "answering"
  | "selecting_project"
  | "planning"
  | "contracting"
  | "generating"
  | "evaluating"
  | "scheduling"
  | "reporting"
  | "waiting"
  | "completed"
  | "failed"
  | "exhausted"
  | "cancelled";

const WORKFLOW_PHASES = [
  { key: "queued", label: "Queued" },
  { key: "gating", label: "Gate" },
  { key: "selecting_project", label: "Project" },
  { key: "planning", label: "Planner" },
  { key: "contracting", label: "Contract" },
  { key: "generating", label: "Generator" },
  { key: "evaluating", label: "Evaluator" },
  { key: "completed", label: "Complete" },
] as const;

const ANSWER_PHASES = [
  { key: "queued", label: "Queued" },
  { key: "gating", label: "Gate" },
  { key: "answering", label: "Answer" },
  { key: "completed", label: "Complete" },
] as const;

type PhaseTrack = "workflow" | "answer";

type AssistantAgentRole = "gate" | "answer" | "project_selector" | "planner" | "contractor" | "generator" | "evaluator";

export function Thread({
  phaseStatus = null,
  phaseEvents = [],
  generatorAttempts = 0,
  maxGenerationAttempts = 0,
  waitingFor = null,
  waitingKey = null,
  onWaitingSubmit,
  onWaitingCancel,
}: {
  phaseStatus?: ThreadPhaseStatus;
  phaseEvents?: ThreadPhaseEvent[];
  generatorAttempts?: number;
  maxGenerationAttempts?: number;
  waitingFor?: {
    title?: string;
    kind?: string;
    prompt?: string;
    risk_summary?: string;
  } | null;
  waitingKey?: string | null;
  onWaitingSubmit?: (input: { approved: boolean; response?: string }) => Promise<void>;
  onWaitingCancel?: () => Promise<void>;
}) {
  const messageComponents = useMemo(() => ({
    UserMessage,
    EditComposer,
    AssistantMessage: function AssistantMessageWithWaiting() {
      return (
        <AssistantMessage
          waitingFor={waitingFor}
          waitingKey={waitingKey}
          onWaitingSubmit={onWaitingSubmit}
          onWaitingCancel={onWaitingCancel}
        />
      );
    },
  }), [onWaitingCancel, onWaitingSubmit, waitingFor, waitingKey]);

  return (
    <ThreadPrimitive.Root
      className="flex h-full flex-col bg-background text-sm"
      style={
        {
          "--thread-max-width": "48rem",
          "--accent-color": "#1c1c1e",
          "--accent-foreground": "#ffffff",
        } as React.CSSProperties
      }
    >
      <ThreadPrimitive.Viewport
        turnAnchor="top"
        className="relative flex flex-1 flex-col overflow-x-auto overflow-y-scroll scroll-smooth px-4 pt-4"
      >
        <AuiIf condition={(s) => s.thread.isEmpty}>
          <ThreadWelcome />
        </AuiIf>

        <ThreadPrimitive.Messages
          components={messageComponents}
        />

        <ThreadPrimitive.ViewportFooter className="sticky bottom-0 mx-auto mt-auto flex w-full max-w-[var(--thread-max-width)] flex-col gap-4 overflow-visible rounded-t-3xl bg-background pb-4">
          <GANPolicyDiagram
            phaseStatus={phaseStatus}
            phaseEvents={phaseEvents}
            generatorAttempts={generatorAttempts}
            maxGenerationAttempts={maxGenerationAttempts}
          />
          <Composer />
        </ThreadPrimitive.ViewportFooter>
      </ThreadPrimitive.Viewport>
    </ThreadPrimitive.Root>
  );
}

function GANPolicyDiagram({
  phaseStatus,
  phaseEvents,
  generatorAttempts,
  maxGenerationAttempts,
}: {
  phaseStatus: ThreadPhaseStatus;
  phaseEvents: ThreadPhaseEvent[];
  generatorAttempts: number;
  maxGenerationAttempts: number;
}) {
  const track = resolvePhaseTrack(phaseStatus, phaseEvents);
  const phases = track === "answer" ? ANSWER_PHASES : WORKFLOW_PHASES;
  const displayPhase = resolveDisplayPhase(phaseStatus, phaseEvents, track);
  const activeIndex = phaseStatusToIndex(displayPhase, phases);
  const isFailure = phaseStatus === "failed" || phaseStatus === "cancelled" || phaseStatus === "exhausted";

  return (
    <div className="rounded-[2rem] border border-border/70 bg-card/95 px-4 py-3 shadow-sm backdrop-blur">
      <div className="mb-3 flex items-center justify-end gap-2">
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "rounded-full px-2.5 py-1 text-[11px] font-medium",
              isFailure
                ? "bg-destructive/10 text-destructive"
                : activeIndex === phases.length - 1
                  ? "bg-emerald-500/12 text-emerald-700 dark:text-emerald-300"
                  : "bg-[color:var(--accent-color)]/12 text-[color:var(--accent-color)]",
            )}
          >
            {phaseStatusPill(phaseStatus)}
          </span>
          <span className="rounded-full bg-muted px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
            Attempts {generatorAttempts}/{Math.max(maxGenerationAttempts, generatorAttempts, 1)}
          </span>
        </div>
      </div>

      <div
        className="grid items-start gap-2 sm:gap-3"
        style={{ gridTemplateColumns: `repeat(${phases.length}, minmax(0, 1fr))` }}
      >
        {phases.map((phase, index) => {
          const state = phaseVisualState(index, activeIndex, isFailure);

          return (
            <div key={phase.key} className="flex min-w-0 flex-col items-center gap-2">
              <div className="flex w-full items-center gap-2">
                {index > 0 && (
                  <div
                    className={cn(
                      "h-px flex-1 transition-colors",
                      state.connectorClass,
                    )}
                  />
                )}
                <div
                  className={cn(
                    "relative flex size-8 shrink-0 items-center justify-center rounded-full border text-[11px] font-semibold transition-all",
                    state.nodeClass,
                  )}
                >
                  {index + 1}
                  {state.isActive && !isFailure && (
                    <span className="absolute inset-0 rounded-full border border-[color:var(--accent-color)]/50 animate-ping" />
                  )}
                </div>
                {index < phases.length - 1 && (
                  <div
                    className={cn(
                      "h-px flex-1 transition-colors",
                      index < activeIndex ? (isFailure && activeIndex === phases.length - 1 ? "bg-destructive/35" : "bg-[color:var(--accent-color)]/35") : "bg-border/70",
                    )}
                  />
                )}
              </div>
              <div className="text-center">
                <p className={cn("text-[11px] font-medium", state.labelClass)}>{phase.label}</p>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function ThreadWelcome() {
  return (
    <div className="mx-auto my-auto flex w-full max-w-[var(--thread-max-width)] flex-grow flex-col">
      <div className="flex w-full flex-grow flex-col items-center justify-center">
        <div className="flex size-full flex-col justify-center px-8">
          <div className="text-2xl font-semibold">Hello there!</div>
          <div className="text-2xl text-muted-foreground/65">
            How can I help you today?
          </div>
        </div>
      </div>
    </div>
  );
}

function Composer() {
  return (
    <ComposerPrimitive.Root className="relative flex w-full flex-col">
      <ComposerPrimitive.AttachmentDropzone className="flex w-full flex-col rounded-3xl border border-input bg-background px-1 pt-2 outline-none transition-shadow has-[textarea:focus-visible]:border-ring has-[textarea:focus-visible]:ring-2 has-[textarea:focus-visible]:ring-ring/20 data-[dragging=true]:border-ring data-[dragging=true]:border-dashed data-[dragging=true]:bg-accent/50">
        <ComposerAttachments />
        <ComposerPrimitive.Input
          placeholder="Send a message..."
          className="mb-1 max-h-32 min-h-14 w-full resize-none bg-transparent px-4 pt-2 pb-3 text-sm outline-none placeholder:text-muted-foreground focus-visible:ring-0"
          rows={1}
          autoFocus
          aria-label="Message input"
        />
        <ComposerAction />
      </ComposerPrimitive.AttachmentDropzone>
    </ComposerPrimitive.Root>
  );
}

function ComposerAction() {
  return (
    <div className="relative mx-2 mb-2 flex items-center justify-between">
      <ComposerAddAttachment />

      <AuiIf condition={(s) => !s.thread.isRunning}>
        <ComposerPrimitive.Send asChild>
          <TooltipIconButton
            tooltip="Send message"
            side="bottom"
            variant="default"
            size="icon"
            className="size-8 rounded-full"
            style={{
              backgroundColor: "var(--accent-color)",
              color: "var(--accent-foreground)",
            }}
            aria-label="Send message"
          >
            <ArrowUpIcon className="size-4" />
          </TooltipIconButton>
        </ComposerPrimitive.Send>
      </AuiIf>

      <AuiIf condition={(s) => s.thread.isRunning}>
        <ComposerPrimitive.Cancel asChild>
          <Button
            type="button"
            variant="default"
            size="icon"
            className="size-8 rounded-full"
            style={{
              backgroundColor: "var(--accent-color)",
              color: "var(--accent-foreground)",
            }}
            aria-label="Stop generating"
          >
            <SquareIcon className="size-3 fill-current" />
          </Button>
        </ComposerPrimitive.Cancel>
      </AuiIf>
    </div>
  );
}

function UserMessage() {
  return (
    <MessagePrimitive.Root
      className="mx-auto grid w-full max-w-[var(--thread-max-width)] auto-rows-auto grid-cols-[minmax(72px,1fr)_auto] content-start gap-y-2 px-2 py-3 fade-in slide-in-from-bottom-1 animate-in duration-150"
      data-role="user"
    >
      <UserMessageAttachments />

      <div className="relative col-start-2 min-w-0">
        <div className="rounded-3xl bg-muted px-4 py-2.5 break-words text-foreground">
          <MessagePrimitive.Parts />
        </div>
        <div className="absolute top-1/2 left-0 -translate-x-full -translate-y-1/2 pr-2">
          <UserActionBar />
        </div>
      </div>

      <BranchPicker className="col-span-full col-start-1 row-start-3 -mr-1 justify-end" />
    </MessagePrimitive.Root>
  );
}

function UserActionBar() {
  return (
    <ActionBarPrimitive.Root
      hideWhenRunning
      autohide="not-last"
      className="flex flex-col items-end"
    >
      <ActionBarPrimitive.Edit asChild>
        <TooltipIconButton tooltip="Edit" className="p-4">
          <PencilIcon />
        </TooltipIconButton>
      </ActionBarPrimitive.Edit>
    </ActionBarPrimitive.Root>
  );
}

function EditComposer() {
  return (
    <MessagePrimitive.Root className="mx-auto flex w-full max-w-[var(--thread-max-width)] flex-col px-2 py-3">
      <ComposerPrimitive.Root className="ml-auto flex w-full max-w-[85%] flex-col rounded-3xl bg-muted">
        <ComposerPrimitive.Input
          className="min-h-14 w-full resize-none bg-transparent p-4 text-sm text-foreground outline-none"
          autoFocus
        />
        <div className="mx-3 mb-3 flex items-center gap-2 self-end">
          <ComposerPrimitive.Cancel asChild>
            <Button variant="ghost" size="sm">
              Cancel
            </Button>
          </ComposerPrimitive.Cancel>
          <ComposerPrimitive.Send asChild>
            <Button size="sm">Update</Button>
          </ComposerPrimitive.Send>
        </div>
      </ComposerPrimitive.Root>
    </MessagePrimitive.Root>
  );
}

function AssistantMessage({
  waitingFor,
  waitingKey,
  onWaitingSubmit,
  onWaitingCancel,
}: {
  waitingFor?: {
    title?: string;
    kind?: string;
    prompt?: string;
    risk_summary?: string;
  } | null;
  waitingKey?: string | null;
  onWaitingSubmit?: (input: { approved: boolean; response?: string }) => Promise<void>;
  onWaitingCancel?: () => Promise<void>;
}) {
  const partComponents = useMemo(() => ({
    Text: MarkdownText,
    Reasoning,
    ReasoningGroup,
    ToolGroup,
    tools: { Fallback: ToolFallback },
  }), []);
  const rawAgentRole = useAuiState((s) => {
    const custom = (s.message.metadata?.custom ?? {}) as Record<string, unknown>;
    const role = custom.agentRole;
    return role === "gate" || role === "answer" || role === "project_selector" || role === "planner" || role === "contractor" || role === "evaluator" || role === "generator" ? role : "generator";
  });
  const rawAgentName = useAuiState((s) => {
    const custom = (s.message.metadata?.custom ?? {}) as Record<string, unknown>;
    const name = custom.agentName;
    return typeof name === "string" ? name : "";
  });
  const agentRole = rawAgentRole;
  const agentNameLabel = rawAgentName.trim() ? rawAgentName : defaultAgentName(agentRole);
  const AgentIcon = agentIcon(agentRole);

  return (
    <MessagePrimitive.Root
      className="relative mx-auto w-full max-w-[var(--thread-max-width)] py-3 fade-in slide-in-from-bottom-1 animate-in duration-150"
      data-role="assistant"
    >
      <div className="flex items-center gap-2 pb-3">
        <div className={cn(
          "flex size-8 shrink-0 items-center justify-center rounded-full",
          agentBadgeClass(agentRole),
        )}>
          <AgentIcon className="size-4" />
        </div>
        <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
          {agentNameLabel}
        </span>
      </div>

      <div className="pl-10">
        <div className="min-w-0 break-words leading-relaxed text-foreground">
          <div className={cn(
            "min-w-0",
          )}>
            <MessagePrimitive.Parts components={partComponents} />
            <MessageError />
            <AuiIf condition={(s) => s.thread.isRunning && s.message.content.length === 0}>
              <div className="flex items-center gap-2 text-muted-foreground">
                <LoaderIcon className="size-4 animate-spin" />
                <span className="text-sm">Thinking...</span>
              </div>
            </AuiIf>
            <AuiIf condition={(s) => s.message.isLast}>
              {waitingFor && onWaitingSubmit && onWaitingCancel && (
                <WaitingPrompt
                  waitingFor={waitingFor}
                  waitingKey={waitingKey}
                  onSubmit={onWaitingSubmit}
                  onCancel={onWaitingCancel}
                />
              )}
            </AuiIf>

            <div className="mt-1 flex min-h-6 items-center">
              <BranchPicker />
              <AssistantActionBar />
            </div>
          </div>
        </div>
      </div>
    </MessagePrimitive.Root>
  );
}

function WaitingPrompt({
  waitingFor,
  waitingKey,
  onSubmit,
  onCancel,
}: {
  waitingFor: {
    title?: string;
    kind?: string;
    prompt?: string;
    risk_summary?: string;
  };
  waitingKey?: string | null;
  onSubmit: (input: { approved: boolean; response?: string }) => Promise<void>;
  onCancel: () => Promise<void>;
}) {
  const [value, setValue] = useState("");
  const [feedback, setFeedback] = useState("");
  const [isDismissed, setIsDismissed] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const needsApproval = waitingFor.kind === "approval" || waitingFor.kind === "authentication";

  useEffect(() => {
    setValue("");
    setFeedback("");
    setIsDismissed(false);
    setIsSubmitting(false);
  }, [waitingKey]);

  const handleSubmit = async (approved: boolean) => {
    setIsSubmitting(true);
    setFeedback(approved ? "Approving…" : "Sending…");

    try {
      await onSubmit({
        approved,
        response: value.trim() || undefined,
      });
      setIsDismissed(true);
    } catch (error) {
      setFeedback(error instanceof Error ? error.message : "Request failed.");
      setIsSubmitting(false);
    }
  };

  const handleCancel = async () => {
    setIsSubmitting(true);
    setFeedback("Cancelling…");

    try {
      await onCancel();
      setIsDismissed(true);
    } catch (error) {
      setFeedback(error instanceof Error ? error.message : "Cancel failed.");
      setIsSubmitting(false);
    }
  };

  if (isDismissed) {
    return null;
  }

  return (
    <div className="mt-4 rounded-[1.4rem] border border-amber-200/80 bg-amber-50/70 p-4 shadow-sm">
      <div className="flex flex-col gap-1">
        <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-amber-700">
          {humanize(waitingFor.kind ?? "action-needed")}
        </p>
        <h3 className="text-sm font-semibold text-foreground">
          {waitingFor.title ?? "The assistant is waiting"}
        </h3>
        {waitingFor.prompt && (
          <p className="text-sm text-muted-foreground">
            {waitingFor.prompt}
          </p>
        )}
        {waitingFor.risk_summary && (
          <p className="text-xs text-muted-foreground/80">
            {waitingFor.risk_summary}
          </p>
        )}
      </div>

      <div className="mt-3 flex flex-col gap-3">
        <textarea
          className="min-h-28 w-full resize-y rounded-2xl border border-border bg-background px-3 py-2.5 text-sm outline-none transition focus:border-[color:var(--accent-color)] focus:ring-2 focus:ring-[color:var(--accent-color)]/15"
          value={value}
          onChange={(event) => setValue(event.target.value)}
          rows={4}
          placeholder="Type your reply, approval, or additional context…"
          disabled={isSubmitting}
        />

        <div className="flex flex-wrap items-center gap-2">
          <Button type="button" size="sm" onClick={() => void handleSubmit(false)} disabled={isSubmitting}>
            Continue
          </Button>
          {needsApproval && (
            <Button type="button" size="sm" variant="secondary" onClick={() => void handleSubmit(true)} disabled={isSubmitting}>
              Approve
            </Button>
          )}
          <Button type="button" size="sm" variant="ghost" onClick={() => void handleCancel()} disabled={isSubmitting}>
            Cancel
          </Button>
          {feedback && <span className="text-xs text-muted-foreground">{feedback}</span>}
        </div>
      </div>
    </div>
  );
}

function agentIcon(role: AssistantAgentRole): ComponentType<{ className?: string }> {
  switch (role) {
    case "gate":
      return ClipboardListIcon;
    case "answer":
      return WandSparklesIcon;
    case "project_selector":
      return ClipboardListIcon;
    case "planner":
      return ClipboardListIcon;
    case "contractor":
      return ClipboardListIcon;
    case "evaluator":
      return FlaskConicalIcon;
    default:
      return WandSparklesIcon;
  }
}

function defaultAgentName(role: AssistantAgentRole) {
  switch (role) {
    case "gate":
      return "Gate";
    case "answer":
      return "Answer";
    case "project_selector":
      return "Project";
    case "planner":
      return "Planner";
    case "contractor":
      return "Contract";
    case "evaluator":
      return "Evaluator";
    default:
      return "Generator";
  }
}

function agentBadgeClass(role: AssistantAgentRole) {
  switch (role) {
    case "planner":
      return "bg-muted text-muted-foreground";
    case "contractor":
      return "bg-muted text-muted-foreground";
    case "evaluator":
      return "bg-muted text-muted-foreground";
    default:
      return "bg-muted text-muted-foreground";
  }
}

function MessageError() {
  return (
    <MessagePrimitive.Error>
      <ErrorPrimitive.Root className="mt-2 rounded-md border border-destructive bg-destructive/10 p-3 text-sm text-destructive dark:bg-destructive/5 dark:text-red-200">
        <ErrorPrimitive.Message className="line-clamp-2" />
      </ErrorPrimitive.Root>
    </MessagePrimitive.Error>
  );
}

function AssistantActionBar() {
  return (
    <ActionBarPrimitive.Root
      hideWhenRunning
      autohide="not-last"
      className="-ml-1 flex gap-1 text-muted-foreground"
    >
      <ActionBarPrimitive.Copy asChild>
        <TooltipIconButton tooltip="Copy">
          <AuiIf condition={(s) => s.message.isCopied}>
            <CheckIcon />
          </AuiIf>
          <AuiIf condition={(s) => !s.message.isCopied}>
            <CopyIcon />
          </AuiIf>
        </TooltipIconButton>
      </ActionBarPrimitive.Copy>
      <ActionBarPrimitive.ExportMarkdown asChild>
        <TooltipIconButton tooltip="Export as Markdown">
          <DownloadIcon />
        </TooltipIconButton>
      </ActionBarPrimitive.ExportMarkdown>
      <ActionBarPrimitive.Reload asChild>
        <TooltipIconButton tooltip="Refresh">
          <RefreshCwIcon />
        </TooltipIconButton>
      </ActionBarPrimitive.Reload>
      <ActionBarPrimitive.FeedbackPositive asChild>
        <TooltipIconButton tooltip="Good response">
          <ThumbsUpIcon />
        </TooltipIconButton>
      </ActionBarPrimitive.FeedbackPositive>
      <ActionBarPrimitive.FeedbackNegative asChild>
        <TooltipIconButton tooltip="Bad response">
          <ThumbsDownIcon />
        </TooltipIconButton>
      </ActionBarPrimitive.FeedbackNegative>
    </ActionBarPrimitive.Root>
  );
}

function BranchPicker({
  className,
  ...rest
}: React.ComponentProps<typeof BranchPickerPrimitive.Root>) {
  return (
    <BranchPickerPrimitive.Root
      hideWhenSingleBranch
      className={cn(
        "mr-2 -ml-2 inline-flex items-center text-xs text-muted-foreground",
        className,
      )}
      {...rest}
    >
      <BranchPickerPrimitive.Previous asChild>
        <TooltipIconButton tooltip="Previous">
          <ChevronLeftIcon />
        </TooltipIconButton>
      </BranchPickerPrimitive.Previous>
      <span className="font-medium">
        <BranchPickerPrimitive.Number /> / <BranchPickerPrimitive.Count />
      </span>
      <BranchPickerPrimitive.Next asChild>
        <TooltipIconButton tooltip="Next">
          <ChevronRightIcon />
        </TooltipIconButton>
      </BranchPickerPrimitive.Next>
    </BranchPickerPrimitive.Root>
  );
}

function humanize(s: string) {
  return s.replace(/[_-]+/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

function phaseStatusToIndex(
  status: ThreadPhaseStatus,
  phases: ReadonlyArray<{ key: string; label: string }>,
) {
  if (status === "completed" || status === "failed" || status === "exhausted" || status === "cancelled") {
    return phases.length - 1;
  }
  const index = phases.findIndex((phase) => phase.key === status);
  return index >= 0 ? index : 0;
}

function resolvePhaseTrack(status: ThreadPhaseStatus, events: ThreadPhaseEvent[]): PhaseTrack {
  if (status === "answering") return "answer";
  for (let index = events.length - 1; index >= 0; index -= 1) {
    if (events[index] === "answering") return "answer";
  }
  return "workflow";
}

function resolveDisplayPhase(status: ThreadPhaseStatus, events: ThreadPhaseEvent[], track: PhaseTrack): ThreadPhaseStatus {
  if (status === "scheduling" || status === "reporting") {
    return track === "answer" ? "answering" : "evaluating";
  }

  if (status !== "waiting") return status;

  const candidates: ThreadPhaseEvent[] = track === "answer"
    ? ["answering", "gating", "queued"]
    : ["evaluating", "generating", "contracting", "planning", "selecting_project", "gating", "queued"];

  for (let index = events.length - 1; index >= 0; index -= 1) {
    const phase = events[index];
    if (candidates.includes(phase)) {
      return phase;
    }
  }

  return track === "answer" ? "answering" : "generating";
}

function phaseStatusPill(status: ThreadPhaseStatus) {
  switch (status) {
    case "queued":
      return "Queued";
    case "gating":
      return "Gating";
    case "answering":
      return "Answering";
    case "selecting_project":
      return "Selecting Project";
    case "planning":
      return "Planning";
    case "contracting":
      return "Contracting";
    case "generating":
      return "Working";
    case "evaluating":
      return "Checking";
    case "scheduling":
      return "Scheduling";
    case "reporting":
      return "Reporting";
    case "waiting":
      return "Waiting";
    case "completed":
      return "Done";
    case "failed":
      return "Failed";
    case "exhausted":
      return "Exhausted";
    case "cancelled":
      return "Cancelled";
    default:
      return "Idle";
  }
}

function phaseVisualState(index: number, activeIndex: number, isFailure: boolean) {
  const isPast = index < activeIndex;
  const isActive = index === activeIndex;
  const isFuture = index > activeIndex;

  if (isFailure && isActive) {
    return {
      isActive,
      connectorClass: "bg-destructive/35",
      nodeClass: "border-destructive/30 bg-destructive text-destructive-foreground",
      labelClass: "text-destructive",
    };
  }

  if (isActive) {
    return {
      isActive,
      connectorClass: "bg-[color:var(--accent-color)]/35",
      nodeClass: "border-[color:var(--accent-color)] bg-[color:var(--accent-color)] text-[color:var(--accent-foreground)] shadow-[0_0_0_6px_rgba(0,0,0,0.08)]",
      labelClass: "text-foreground",
    };
  }

  if (isPast) {
    return {
      isActive,
      connectorClass: "bg-[color:var(--accent-color)]/35",
      nodeClass: "border-[color:var(--accent-color)]/25 bg-[color:var(--accent-color)]/10 text-[color:var(--accent-color)]",
      labelClass: "text-foreground/80",
    };
  }

  if (isFuture) {
    return {
      isActive,
      connectorClass: "bg-border/70",
      nodeClass: "border-border/70 bg-background text-muted-foreground",
      labelClass: "text-muted-foreground",
    };
  }

  return {
    isActive,
    connectorClass: "bg-border/70",
    nodeClass: "border-border/70 bg-background text-muted-foreground",
    labelClass: "text-muted-foreground",
  };
}
