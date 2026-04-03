import {
  AssistantRuntimeProvider,
  useExternalStoreRuntime,
  type AppendMessage,
  type ExternalStoreAdapter,
  type ThreadMessage,
} from "@assistant-ui/react";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Thread } from "@/components/assistant-ui/thread";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

type BootstrapResponse = {
  product_name: string;
  product_tagline: string;
  default_model: string;
  default_max_generation_attempts: number;
  chats_path: string;
  runs_path: string;
};

type RunStatus =
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

type RunRecord = {
  run: {
    id: string;
    chat_id: string;
    parent_run_id?: string;
    status: RunStatus;
    phase: RunStatus;
    gate_route?: "answer" | "workflow" | "";
    gate_reason?: string;
    gate_decided_at?: string | null;
    project: {
      slug: string;
      name: string;
      description: string;
      workspace_dir: string;
    };
    user_request_raw: string;
    created_at: string;
    updated_at: string;
    attempt_count: number;
    max_generation_attempts: number;
    latest_evaluation?: { summary?: string; score: number };
    waiting_for?: {
      title?: string;
      kind?: string;
      prompt?: string;
      risk_summary?: string;
    } | null;
    task_spec: { goal: string; deliverables?: string[] };
    completed_at?: string | null;
  };
  events: Array<{ id?: string; type: string; phase: string; summary: string; created_at: string; data?: Record<string, unknown> }>;
  attempts: Array<{
    id: string;
    role: "gate" | "answer" | "project_selector" | "planner" | "contractor" | "generator" | "evaluator";
    input_summary: string;
    output_summary: string;
    critique?: string;
    started_at: string;
    finished_at?: string | null;
  }>;
  artifacts: Array<{
    id: string;
    kind: string;
    title: string;
    mime_type: string;
    path?: string;
    url?: string;
    content?: string;
    source_url?: string;
    created_at: string;
  }>;
  evidence: Array<{
    id: string;
    kind: string;
    summary: string;
    detail?: string;
    created_at: string;
  }>;
  evaluations: Array<{
    id: string;
    passed: boolean;
    score: number;
    summary: string;
    missing_requirements: string[];
    evidence_checked: string[];
    next_action_for_generator: string;
    created_at: string;
  }>;
  tool_calls: Array<{
    id: string;
    attempt_id: string;
    tool_name: string;
    input_summary: string;
    output_summary: string;
    started_at: string;
    finished_at: string;
  }>;
  web_steps: Array<{
    id: string;
    title: string;
    url?: string;
    summary: string;
    occurred_at: string;
  }>;
};

type ChatSummary = {
  id: string;
  root_run_id: string;
  latest_run_id: string;
  title: string;
  status: RunStatus;
  created_at: string;
  updated_at: string;
};

type ChatRecord = {
  chat: ChatSummary;
  runs: RunRecord[];
};

type LiveRunEvent = {
  id: string;
  run_id: string;
  type: string;
  phase: string;
  summary: string;
  created_at: string;
  data?: Record<string, unknown>;
};

type LiveReasoningPart = {
  id: string;
  attemptId: string;
  role: "gate" | "answer" | "project_selector" | "planner" | "contractor" | "generator" | "evaluator";
  text: string;
  updatedAt: string;
};

type LiveToolPart = {
  id: string;
  attemptId: string;
  role: "gate" | "answer" | "project_selector" | "planner" | "contractor" | "generator" | "evaluator";
  toolName: string;
  argsText?: string;
  result?: string;
  status: "running" | "complete" | "incomplete";
  updatedAt: string;
};

type LiveTelemetry = {
  reasoning: Record<string, LiveReasoningPart>;
  tools: Record<string, LiveToolPart>;
};

type AgentRole = "gate" | "answer" | "project_selector" | "planner" | "contractor" | "generator" | "evaluator";

type HistoryEntry = {
  id: string;
  title: string;
  status: RunStatus;
  updatedAt: string;
};
const TERMINAL_STATUSES: RunStatus[] = ["completed", "failed", "exhausted", "cancelled"];

const statusLabel: Record<RunStatus, string> = {
  queued: "Queued",
  gating: "Gating",
  answering: "Answering",
  selecting_project: "Selecting Project",
  planning: "Planning",
  contracting: "Contracting",
  generating: "Working",
  evaluating: "Checking",
  scheduling: "Scheduling",
  reporting: "Reporting",
  waiting: "Waiting",
  completed: "Done",
  failed: "Failed",
  exhausted: "Exhausted",
  cancelled: "Cancelled",
};

const statusMessage: Record<RunStatus, string> = {
  queued: "Task accepted, starting soon…",
  gating: "Deciding whether this run should answer directly or execute workflow…",
  answering: "Preparing a read-oriented answer from available context…",
  selecting_project: "Choosing the right project workspace…",
  planning: "Turning your request into a plan…",
  contracting: "Locking the acceptance contract before work starts…",
  generating: "Working on the task…",
  evaluating: "Checking whether the result is complete…",
  scheduling: "Scheduling follow-up or deferred work…",
  reporting: "Preparing the final report…",
  waiting: "Waiting for your input to continue.",
  completed: "Task completed.",
  failed: "Stopped — could not continue safely.",
  exhausted: "Stopped after reaching the retry limit.",
  cancelled: "Task was cancelled.",
};

const LIVE_EVENT_TYPES = new Set([
  "reasoning",
  "tool_call_started",
  "tool_call_completed",
]);

function emptyLiveTelemetry(): LiveTelemetry {
  return { reasoning: {}, tools: {} };
}

export function App() {
  const [bootstrap, setBootstrap] = useState<BootstrapResponse | null>(null);
  const [selectedChatId, setSelectedChatId] = useState<string>(readChatIDFromHash());
  const [currentChat, setCurrentChat] = useState<ChatRecord | null>(null);
  const [messages, setMessages] = useState<ThreadMessage[]>([]);
  const [liveTelemetry, setLiveTelemetry] = useState<LiveTelemetry>(emptyLiveTelemetry);
  const [chatHistory, setChatHistory] = useState<HistoryEntry[]>([]);
  const [attempts, setAttempts] = useState(3);
  const [isRunning, setIsRunning] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [threadKey, setThreadKey] = useState("draft");
  const [isReportOpen, setIsReportOpen] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);
  const pollTimerRef = useRef<number | null>(null);
  const previousRunIDRef = useRef("");
  const previousIsRunningRef = useRef(false);

  const closeEventStream = useCallback(() => {
    eventSourceRef.current?.close();
    eventSourceRef.current = null;
  }, []);

  const stopPolling = useCallback(() => {
    if (pollTimerRef.current !== null) {
      window.clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  }, []);

  const mergeChatHistoryEntry = useCallback((entry: HistoryEntry) => {
    setChatHistory((prev) => {
      const next = [entry, ...prev.filter((chat) => chat.id !== entry.id)];
      next.sort((a, b) => +new Date(b.updatedAt) - +new Date(a.updatedAt));
      return next;
    });
  }, []);

  const replaceChatHistory = useCallback((chats: ChatSummary[]) => {
    setChatHistory(
      chats.map((chat) => ({
        id: chat.id,
        title: truncate(chat.title, 48),
        status: chat.status,
        updatedAt: chat.updated_at,
      })),
    );
  }, []);

  const hydrateChat = useCallback(
    (record: ChatRecord | null) => {
      setCurrentChat(record);
      if (!record) {
        setMessages([]);
        setIsRunning(false);
        return;
      }
      const latestRun = latestRunFromChat(record);
      setIsRunning(Boolean(latestRun && isActiveStatus(latestRun.run.status)));
      mergeChatHistoryEntry({
        id: record.chat.id,
        title: truncate(record.chat.title, 48),
        status: record.chat.status,
        updatedAt: record.chat.updated_at,
      });
    },
    [mergeChatHistoryEntry],
  );

  useEffect(() => {
    if (!currentChat) return;
    setMessages(chatToMessages(currentChat, liveTelemetry));
  }, [currentChat, liveTelemetry]);

  const refreshChatList = useCallback(
    async () => {
      if (!bootstrap) return;
      const payload = await fetchJSON<{ chats: ChatSummary[] }>(bootstrap.chats_path);
      replaceChatHistory(payload.chats);
      return payload.chats;
    },
    [bootstrap, replaceChatHistory],
  );

  const refreshChat = useCallback(
    async (chatID: string) => {
      if (!bootstrap) return;
      const record = await fetchJSON<ChatRecord>(`${bootstrap.chats_path}/${encodeURIComponent(chatID)}`);
      hydrateChat(record);
      return record;
    },
    [bootstrap, hydrateChat],
  );

  const syncPolling = useCallback(
    (chatID: string, status: RunStatus) => {
      stopPolling();
      if (!isActiveStatus(status)) return;
      pollTimerRef.current = window.setTimeout(() => void refreshChat(chatID).catch(() => undefined), 4000);
    },
    [refreshChat, stopPolling],
  );

  const connectEventStream = useCallback(
    (chatID: string, runID: string, status: RunStatus) => {
      closeEventStream();
      if (!isActiveStatus(status)) return;
      const source = new EventSource(`${bootstrap?.runs_path}/${encodeURIComponent(runID)}/events`);
      source.addEventListener("run_event", (event) => {
        const payload = parseLiveRunEvent(event);
        if (payload?.run_id === runID && LIVE_EVENT_TYPES.has(payload.type)) {
          setLiveTelemetry((prev) => applyLiveRunEvent(prev, payload));
          return;
        }

        void refreshChat(chatID).then((record) => {
          const latestRun = record ? latestRunFromChat(record) : null;
          if (record && latestRun) {
            syncPolling(chatID, latestRun.run.status);
            if (!isActiveStatus(latestRun.run.status)) closeEventStream();
          }
        });
      });
      source.onerror = () => syncPolling(chatID, status);
      eventSourceRef.current = source;
    },
    [bootstrap?.runs_path, closeEventStream, refreshChat, syncPolling],
  );

  const syncRealtimeForChat = useCallback(
    (record: ChatRecord) => {
      closeEventStream();
      stopPolling();
      const latestRun = latestRunFromChat(record);
      if (latestRun && isActiveStatus(latestRun.run.status)) {
        connectEventStream(record.chat.id, latestRun.run.id, latestRun.run.status);
        syncPolling(record.chat.id, latestRun.run.status);
      }
    },
    [closeEventStream, connectEventStream, stopPolling, syncPolling],
  );

  const loadChat = useCallback(
    async (chatID: string) => {
      setIsLoading(true);
      try {
        const record = await refreshChat(chatID);
        if (record) syncRealtimeForChat(record);
      } finally {
        setIsLoading(false);
      }
    },
    [refreshChat, syncRealtimeForChat],
  );

  useEffect(() => {
    void fetchJSON<BootstrapResponse>("/api/v1/bootstrap").then((p) => {
      setBootstrap(p);
      setAttempts(p.default_max_generation_attempts);
      void fetchJSON<{ chats: ChatSummary[] }>(p.chats_path)
        .then((payload) => replaceChatHistory(payload.chats))
        .finally(() => setIsLoading(false));
    });
  }, [replaceChatHistory]);

  useEffect(() => {
    const onHashChange = () => setSelectedChatId(readChatIDFromHash());
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  useEffect(() => {
    if (!bootstrap) return;
    if (!selectedChatId) {
      closeEventStream();
      stopPolling();
      hydrateChat(null);
      setLiveTelemetry(emptyLiveTelemetry());
      setThreadKey(`draft-${Date.now()}`);
      return;
    }
    setLiveTelemetry(emptyLiveTelemetry());
    setThreadKey(selectedChatId);
    void loadChat(selectedChatId);
    return () => { closeEventStream(); stopPolling(); };
  }, [bootstrap, closeEventStream, hydrateChat, loadChat, selectedChatId, stopPolling]);

  const onNew = useCallback(
    async (message: AppendMessage) => {
      if (!bootstrap) return;
      const text = getMessageText(message);
      if (!text) return;

      const createdAt = new Date();
      const latestRun = currentChat ? latestRunFromChat(currentChat) : null;
      const optimisticHistory = currentChat ? chatToMessages(currentChat, emptyLiveTelemetry()) : [];
      setMessages([...optimisticHistory, buildUserMessage(text, createdAt), buildPendingAssistantMessage(createdAt)]);
      setIsRunning(true);
      if (!currentChat) {
        setCurrentChat(null);
      }
      setLiveTelemetry(emptyLiveTelemetry());

      const parentRunId =
        latestRun && isFollowUpSourceStatus(latestRun.run.status)
          ? latestRun.run.id
          : "";

      const created = await fetchJSON<{ run: { id: string; chat_id: string; status: RunStatus; updated_at: string } }>(
        bootstrap.runs_path,
        {
          method: "POST",
          body: JSON.stringify({
            user_request_raw: text,
            max_generation_attempts: attempts,
            ...(parentRunId ? { parent_run_id: parentRunId } : {}),
          }),
        },
      );

      await refreshChatList();
      setSelectedChatId(created.run.chat_id);
      setThreadKey(created.run.chat_id);
      updateHash(created.run.chat_id);
      await loadChat(created.run.chat_id);
    },
    [attempts, bootstrap, currentChat, loadChat, refreshChatList],
  );

  const onCancel = useCallback(async () => {
    if (!bootstrap || !selectedChatId || !currentChat) return;
    const latestRun = latestRunFromChat(currentChat);
    if (!latestRun) return;
    await fetchJSON<RunRecord>(`${bootstrap.runs_path}/${encodeURIComponent(latestRun.run.id)}/cancel`, { method: "POST" });
    const record = await refreshChat(selectedChatId);
    if (record) {
      syncRealtimeForChat(record);
      await refreshChatList();
    }
  }, [bootstrap, currentChat, refreshChat, refreshChatList, selectedChatId, syncRealtimeForChat]);

  const resumeWaitingRun = useCallback(
    async ({ approved, response }: { approved: boolean; response?: string }) => {
      const latestRun = currentChat ? latestRunFromChat(currentChat) : null;
      if (!latestRun) return;
      const input: Record<string, string> = {};
      if (response?.trim()) input.response = response.trim();
      if (approved) input.approval = "approved";
      await fetchJSON<{ ok: boolean }>(
        `/api/v1/runs/${encodeURIComponent(latestRun.run.id)}/resume`,
        {
          method: "POST",
          body: Object.keys(input).length ? JSON.stringify({ input }) : undefined,
        },
      );
      const record = await refreshChat(latestRun.run.chat_id);
      if (record) {
        syncRealtimeForChat(record);
        await refreshChatList();
      }
    },
    [currentChat, refreshChat, refreshChatList, syncRealtimeForChat],
  );

  const cancelWaitingRun = useCallback(async () => {
    const latestRun = currentChat ? latestRunFromChat(currentChat) : null;
    if (!latestRun) return;
    await fetchJSON<RunRecord>(
      `/api/v1/runs/${encodeURIComponent(latestRun.run.id)}/cancel`,
      { method: "POST" },
    );
    const record = await refreshChat(latestRun.run.chat_id);
    if (record) {
      syncRealtimeForChat(record);
      await refreshChatList();
    }
  }, [currentChat, refreshChat, refreshChatList, syncRealtimeForChat]);

  const runtimeAdapter = useMemo<ExternalStoreAdapter<ThreadMessage>>(
    () => ({ isLoading, isRunning, messages, onNew, onCancel }),
    [isLoading, isRunning, messages, onCancel, onNew],
  );

  const runtime = useExternalStoreRuntime(runtimeAdapter);
  const latestRun = currentChat ? latestRunFromChat(currentChat) : null;
  const status = latestRun?.run.status ?? null;
  const showReport = Boolean(latestRun && status && TERMINAL_STATUSES.includes(status));

  useEffect(() => {
    if (!latestRun) {
      setIsReportOpen(false);
      previousRunIDRef.current = "";
      previousIsRunningRef.current = isRunning;
      return;
    }

    const runChanged = previousRunIDRef.current !== latestRun.run.id;

    if (runChanged) {
      setIsReportOpen(false);
    } else if (previousIsRunningRef.current && !isRunning && TERMINAL_STATUSES.includes(latestRun.run.status)) {
      setIsReportOpen(true);
    }

    previousRunIDRef.current = latestRun.run.id;
    previousIsRunningRef.current = isRunning;
  }, [isRunning, latestRun]);

  return (
    <TooltipProvider>
      <AssistantRuntimeProvider key={threadKey} runtime={runtime}>
        <div className="app-shell">
          <aside className="sidebar">
            <div className="sidebar-brand">
              <span className="sidebar-brand-icon">C</span>
              <div>
                <p className="sidebar-brand-label">Workspace</p>
                <h1>{bootstrap?.product_name ?? "Codex"}</h1>
              </div>
            </div>

            <button
              type="button"
              className="sidebar-new-btn"
              onClick={() => { setSelectedChatId(""); updateHash(""); }}
            >
              + New chat
            </button>

            <nav className="sidebar-history">
              {chatHistory.length === 0 ? (
                <p className="sidebar-empty">No recent chats.</p>
              ) : (
                chatHistory.map((entry) => (
                  <button
                    key={entry.id}
                    type="button"
                    className={`sidebar-item${entry.id === selectedChatId ? " is-active" : ""}`}
                    onClick={() => { setSelectedChatId(entry.id); updateHash(entry.id); }}
                  >
                    <span className="sidebar-item-title">{entry.title}</span>
                    <span className="sidebar-item-meta">{statusLabel[entry.status]} · {relativeTime(entry.updatedAt)}</span>
                  </button>
                ))
              )}
            </nav>

            <div className="sidebar-footer">
              <div className="sidebar-footer-row">
                <span>Model</span>
                <span>{bootstrap?.default_model ?? "—"}</span>
              </div>
              <div className="sidebar-footer-row">
                <span>Retries</span>
                <input
                  className="sidebar-retries-input"
                  type="number"
                  min={1}
                  max={9}
                  value={attempts}
                  onChange={(e) => setAttempts(Number.parseInt(e.target.value, 10) || 1)}
                />
              </div>
            </div>
          </aside>

          <main className="chat-main">
            <header className="chat-header">
              <h2 className="chat-title">
                {currentChat?.chat.title || "How can Codex help today?"}
              </h2>
              {status && (
                <span className="status-pill" data-status={status}>
                  {statusLabel[status]}
                </span>
              )}
            </header>

            <div className="chat-stage">
              <div className="chat-content">
                <div className="thread-shell">
                  <Thread
                    phaseStatus={status}
                    phaseEvents={latestRun?.events.map((event) => event.phase as RunStatus) ?? []}
                    generatorAttempts={latestRun?.attempts.filter((attempt) => attempt.role === "generator").length ?? 0}
                    maxGenerationAttempts={latestRun?.run.max_generation_attempts ?? 0}
                    waitingFor={latestRun?.run.waiting_for ?? null}
                    waitingKey={latestRun ? `${latestRun.run.id}:${latestRun.run.updated_at}` : null}
                    onWaitingSubmit={resumeWaitingRun}
                    onWaitingCancel={cancelWaitingRun}
                  />
                </div>
              </div>

              {showReport && latestRun && (
                <SupervisorReport
                  record={latestRun}
                  isOpen={isReportOpen}
                  onToggle={() => setIsReportOpen((open) => !open)}
                />
              )}
            </div>
          </main>
        </div>
      </AssistantRuntimeProvider>
    </TooltipProvider>
  );
}

function SupervisorReport({
  record,
  isOpen,
  onToggle,
}: {
  record: RunRecord;
  isOpen: boolean;
  onToggle: () => void;
}) {
  const videoArtifact = record.artifacts.find((artifact) => artifact.mime_type.startsWith("video/") && artifact.url);
  const screenshots = record.artifacts.filter((artifact) => artifact.mime_type.startsWith("image/") && artifact.url);
  const videoPoster = screenshots[0]?.url;
  const deliverableArtifacts = record.artifacts.filter((artifact) => !artifact.mime_type.startsWith("video/") && !artifact.mime_type.startsWith("image/"));
  const latestContentArtifact = [...deliverableArtifacts].reverse().find((artifact) => artifact.content?.trim());
  const outcome = reportOutcome(record.run.status);
  const completedAt = record.run.completed_at || record.run.updated_at;
  const [activeScreenshotIndex, setActiveScreenshotIndex] = useState(0);
  const activeScreenshot = screenshots[activeScreenshotIndex] ?? screenshots[0];

  useEffect(() => {
    setActiveScreenshotIndex(0);
  }, [record.run.id]);

  useEffect(() => {
    if (activeScreenshotIndex >= screenshots.length) {
      setActiveScreenshotIndex(Math.max(0, screenshots.length - 1));
    }
  }, [activeScreenshotIndex, screenshots.length]);

  const showPreviousScreenshot = () => {
    setActiveScreenshotIndex((current) => (current - 1 + screenshots.length) % screenshots.length);
  };

  const showNextScreenshot = () => {
    setActiveScreenshotIndex((current) => (current + 1) % screenshots.length);
  };

  return (
    <section className={`report-overlay${isOpen ? " is-open" : ""}`}>
      {isOpen && (
        <button
          type="button"
          className="report-backdrop"
          onClick={onToggle}
          aria-label="Close supervisor report"
        />
      )}

      <button
        type="button"
        className="report-launcher"
        onClick={onToggle}
        aria-expanded={isOpen}
        aria-label={isOpen ? "Hide supervisor report" : "Show supervisor report"}
      >
        {isOpen ? "Hide report" : "Supervisor report"}
      </button>

      <aside className="report-panel" aria-hidden={!isOpen}>
        <div className="report-panel-body">
          <div className="report-panel-topbar">
            <div>
              <p className="report-kicker">Supervisor Report</p>
              <h3 className="report-drawer-title">Run summary</h3>
            </div>
            <button
              type="button"
              className="report-close-btn"
              onClick={onToggle}
              aria-label="Close supervisor report"
            >
              Close
            </button>
          </div>

          <div className="report-hero">
            <div>
              <h3 className="report-title">{record.run.task_spec.goal || record.run.user_request_raw}</h3>
              <p className="report-summary">{latestAssistantText(record)}</p>
              <div className="report-summary-meta">
                {record.run.gate_route && (
                  <span>
                    Route: {record.run.gate_route === "answer" ? "Answer" : "Workflow"}
                  </span>
                )}
                {record.run.gate_reason && <span>Gate: {record.run.gate_reason}</span>}
                {record.run.parent_run_id && (
                  <span>
                    Follow-up from {record.run.parent_run_id}
                  </span>
                )}
              </div>
            </div>
            <div className="report-meta">
              <span className={`report-outcome is-${record.run.status}`}>{outcome}</span>
              <span>Finished {relativeTime(completedAt)}</span>
            </div>
          </div>

          <div className="report-grid">
            <article className="report-card">
              <h4>What CVA Delivered</h4>
              <ul className="report-list">
                {record.run.task_spec.deliverables?.length ? (
                  record.run.task_spec.deliverables.map((item) => <li key={item}>{item}</li>)
                ) : (
                  <li>{record.run.user_request_raw}</li>
                )}
              </ul>
              {record.run.latest_evaluation && (
                <p className="report-note">
                  Evaluation score {record.run.latest_evaluation.score}/100. {record.run.latest_evaluation.summary}
                </p>
              )}
            </article>

            <article className="report-card">
              <h4>How CVA Worked</h4>
              <div className="report-timeline">
                {record.attempts.map((attempt) => (
                  <div className="report-step" key={attempt.id}>
                    <div className="report-step-header">
                      <strong>{humanize(attempt.role)}</strong>
                      <span>{relativeTime(attempt.finished_at || attempt.started_at)}</span>
                    </div>
                    <p>{attempt.output_summary || attempt.input_summary}</p>
                  </div>
                ))}
              </div>
            </article>
          </div>

          {videoArtifact && (
            <article className="report-card report-video-card">
              <div className="report-section-head">
                <div>
                  <h4>Browser Recording</h4>
                  <p>Supervisors can review the captured browser replay directly in the report.</p>
                </div>
                <a className="report-link" href={videoArtifact.url} target="_blank" rel="noreferrer">
                  Open video
                </a>
              </div>
              <div
                className="report-video-shell"
                style={videoPoster ? { backgroundImage: `url("${videoPoster}")` } : undefined}
              >
                {videoPoster && (
                  <img
                    className="report-video-poster"
                    src={videoPoster}
                    alt=""
                    aria-hidden="true"
                  />
                )}
                <video
                  className="report-video"
                  controls
                  playsInline
                  preload="metadata"
                  src={videoArtifact.url}
                  poster={videoPoster}
                >
                  Your browser could not play the embedded recording.{" "}
                  <a className="report-link" href={videoArtifact.url} target="_blank" rel="noreferrer">
                    Open video
                  </a>
                </video>
              </div>
            </article>
          )}

          {screenshots.length > 0 && (
            <article className="report-card">
              <div className="report-section-head">
                <div>
                  <h4>Key Screens</h4>
                  <p>
                    {activeScreenshotIndex + 1} / {screenshots.length}
                  </p>
                </div>
                {activeScreenshot && (
                  <a className="report-link" href={activeScreenshot.url} target="_blank" rel="noreferrer">
                    Open image
                  </a>
                )}
              </div>
              <div className="report-shot-carousel">
                {screenshots.length > 1 && (
                  <button
                    type="button"
                    className="report-shot-nav report-shot-nav-prev"
                    onClick={showPreviousScreenshot}
                    aria-label="Show previous screen"
                  >
                    Previous
                  </button>
                )}
                {activeScreenshot && (
                  <a
                    className="report-shot report-shot-active"
                    href={activeScreenshot.url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    <img
                      src={activeScreenshot.url}
                      alt={activeScreenshot.title}
                      loading="lazy"
                    />
                    <span>{activeScreenshot.title}</span>
                  </a>
                )}
                {screenshots.length > 1 && (
                  <button
                    type="button"
                    className="report-shot-nav report-shot-nav-next"
                    onClick={showNextScreenshot}
                    aria-label="Show next screen"
                  >
                    Next
                  </button>
                )}
              </div>
              {screenshots.length > 1 && (
                <div className="report-shot-dots" aria-label="Choose screen">
                  {screenshots.map((artifact, index) => (
                    <button
                      type="button"
                      key={artifact.id}
                      className={`report-shot-dot${index === activeScreenshotIndex ? " is-active" : ""}`}
                      onClick={() => setActiveScreenshotIndex(index)}
                      aria-label={`Show screen ${index + 1}`}
                      aria-pressed={index === activeScreenshotIndex}
                    />
                  ))}
                </div>
              )}
            </article>
          )}

          {(latestContentArtifact || deliverableArtifacts.length > 0) && (
            <article className="report-card">
              <div className="report-section-head">
                <div>
                  <h4>Outputs</h4>
                  <p>Generated deliverables and artifacts from the completed run.</p>
                </div>
              </div>
              {latestContentArtifact?.content && (
                <pre className="report-output">{latestContentArtifact.content.trim()}</pre>
              )}
              <div className="report-artifacts">
                {deliverableArtifacts.map((artifact) => (
                  <div className="report-artifact" key={artifact.id}>
                    <div>
                      <strong>{artifact.title}</strong>
                      <p>{artifact.mime_type}</p>
                    </div>
                    {artifact.url ? (
                      <a className="report-link" href={artifact.url} target="_blank" rel="noreferrer">
                        Open
                      </a>
                    ) : (
                      <span className="report-empty">Inline only</span>
                    )}
                  </div>
                ))}
              </div>
            </article>
          )}
        </div>
      </aside>
    </section>
  );
}

// ── data helpers ──────────────────────────────────────────────────────────────

function latestRunFromChat(record: ChatRecord): RunRecord | null {
  return record.runs.at(-1) ?? null;
}

function chatToMessages(record: ChatRecord, liveTelemetry: LiveTelemetry): ThreadMessage[] {
  return record.runs.flatMap((runRecord, index) =>
    recordToMessages(
      runRecord,
      index === record.runs.length - 1 ? liveTelemetry : emptyLiveTelemetry(),
    ),
  );
}

function recordToMessages(record: RunRecord, liveTelemetry: LiveTelemetry): ThreadMessage[] {
  const user = buildUserMessage(record.run.user_request_raw, new Date(record.run.created_at), `${record.run.id}-user`);
  const assistants = buildAssistantMessages(record, liveTelemetry);
  if (assistants.length === 0) return [user];
  return [user, ...assistants];
}

function buildUserMessage(text: string, createdAt: Date, id = "draft-user"): ThreadMessage {
  return {
    id,
    role: "user",
    createdAt,
    content: [{ type: "text", text }],
    attachments: [],
    status: { type: "complete", reason: "stop" },
    metadata: { custom: {} },
  };
}

function buildPendingAssistantMessage(createdAt: Date, id = "draft-assistant"): ThreadMessage {
  return buildAssistantPhaseMessage(
    {
      id,
      createdAt: createdAt.toISOString(),
      content: [{ type: "text", text: statusMessage.queued }],
      agentRole: "gate",
    },
    { type: "running" },
  );
}

function latestAssistantText(record: RunRecord): string {
  const candidates = [
    record.run.latest_evaluation?.summary,
    record.run.waiting_for?.prompt,
    [...record.events].sort((a, b) => +new Date(b.created_at) - +new Date(a.created_at))[0]?.summary,
  ].filter(Boolean);
  return candidates[0] ?? statusMessage[record.run.status];
}

function buildAssistantMessages(record: RunRecord, liveTelemetry: LiveTelemetry): ThreadMessage[] {
  const attempts = record.attempts
    .slice()
    .sort((a, b) => +new Date(a.started_at) - +new Date(b.started_at));

  const persistedAttemptIDs = new Set(attempts.map((attempt) => attempt.id));

  const persistedMessages = attempts.map((attempt) =>
    buildAssistantPhaseMessage({
      id: `${record.run.id}-assistant-${attempt.id}`,
      createdAt: attempt.finished_at || attempt.started_at,
      content: buildAttemptContent(record, attempt),
      agentRole: attempt.role,
    }),
  );

  const liveAttemptIDs = Array.from(new Set([
    ...Object.values(liveTelemetry.reasoning).map((part) => part.attemptId),
    ...Object.values(liveTelemetry.tools).map((part) => part.attemptId),
  ]))
    .filter((attemptId) => attemptId && !persistedAttemptIDs.has(attemptId));

  const liveMessages = liveAttemptIDs
    .map((attemptId) => {
      const content = buildLiveAttemptContent(record, liveTelemetry, attemptId);
      const updatedAt = latestLiveAttemptTimestamp(liveTelemetry, attemptId) ?? record.run.updated_at;
      return buildAssistantPhaseMessage({
        id: `${record.run.id}-assistant-live-${attemptId}`,
        createdAt: updatedAt,
        content,
        agentRole: liveAttemptRole(liveTelemetry, attemptId),
      });
    })
    .filter((message) => message.content.length > 0);

  const placeholder = buildCurrentPhasePlaceholder(record, attempts, liveAttemptIDs);
  const messages = [...persistedMessages, ...liveMessages, ...(placeholder ? [placeholder] : [])];

  if (messages.length === 0) {
    return [
      buildAssistantPhaseMessage({
        id: `${record.run.id}-assistant`,
        createdAt: record.run.updated_at,
        content: [{ type: "text", text: latestAssistantText(record) }],
        agentRole: attemptRoleForPhase(record.run.phase),
      }, runStatusToMessageStatus(record.run.status)),
    ];
  }

  return messages.map((message, index) => ({
    ...message,
    status: index === messages.length - 1
      ? runStatusToMessageStatus(record.run.status)
      : { type: "complete", reason: "stop" as const },
  }));
}

function buildAttemptContent(
  record: RunRecord,
  attempt: RunRecord["attempts"][number],
): ThreadMessage["content"] {
  const reasoningSections = [
    attempt.input_summary?.trim(),
    attempt.critique?.trim(),
  ].filter(Boolean);

  const reasoningParts = reasoningSections.length > 0
    ? [{
        type: "reasoning" as const,
        parentId: `${record.run.id}-${attempt.id}`,
        text: `### ${humanize(attempt.role)}\n\n${reasoningSections.join("\n\n")}`,
      }]
    : [];

  const toolParts = record.tool_calls
    .filter((tool) => tool.attempt_id === attempt.id)
    .sort((a, b) => +new Date(a.started_at) - +new Date(b.started_at))
    .map((tool) => ({
      type: "tool-call" as const,
      toolCallId: tool.id,
      toolName: tool.tool_name,
      argsText: tool.input_summary?.trim() || "",
      result: tool.output_summary?.trim() || "Completed",
      status: { type: "complete" as const },
    }));

  const text = attempt.output_summary?.trim();
  return [
    ...reasoningParts,
    ...toolParts,
    ...(text ? [{ type: "text" as const, text }] : []),
  ];
}

function buildLiveAttemptContent(
  record: RunRecord,
  liveTelemetry: LiveTelemetry,
  attemptId: string,
): ThreadMessage["content"] {
  const reasoningParts = Object.values(liveTelemetry.reasoning)
    .filter((part) => part.attemptId === attemptId && isMeaningfulReasoningText(part.text))
    .sort((a, b) => +new Date(a.updatedAt) - +new Date(b.updatedAt))
    .map((part) => ({
      type: "reasoning" as const,
      parentId: `${record.run.id}-${attemptId}`,
      text: `### ${humanize(part.role)}\n\n${part.text.trim()}`,
    }));

  const toolParts = Object.values(liveTelemetry.tools)
    .filter((tool) => tool.attemptId === attemptId)
    .sort((a, b) => +new Date(a.updatedAt) - +new Date(b.updatedAt))
    .map((tool) => ({
      type: "tool-call" as const,
      toolCallId: tool.id,
      toolName: tool.toolName,
      argsText: tool.argsText || "",
      result: tool.result,
      status:
        tool.status === "running"
          ? ({ type: "running" } as const)
          : tool.status === "incomplete"
            ? ({ type: "incomplete", reason: "error" } as const)
            : ({ type: "complete" } as const),
    }));

  return [...reasoningParts, ...toolParts];
}

function buildAssistantPhaseMessage(
  input: { id: string; createdAt: string; content: ThreadMessage["content"]; agentRole: AgentRole },
  status: ThreadMessage["status"] = { type: "complete", reason: "stop" },
): ThreadMessage {
  return {
    id: input.id,
    role: "assistant",
    createdAt: new Date(input.createdAt),
    content: input.content,
    status,
    metadata: {
      unstable_state: null,
      unstable_annotations: [],
      unstable_data: [],
      steps: [],
      custom: {
        agentRole: input.agentRole,
        agentName: agentName(input.agentRole),
      },
    },
  };
}

function buildCurrentPhasePlaceholder(
  record: RunRecord,
  attempts: RunRecord["attempts"],
  liveAttemptIDs: string[],
): ThreadMessage | null {
  if (!isActiveStatus(record.run.status)) return null;
  if (liveAttemptIDs.length > 0) return null;

  const expectedRole = attemptRoleForPhase(record.run.phase);
  const latestAttempt = attempts.at(-1);
  if (latestAttempt?.role === expectedRole) return null;

  const text = latestPhaseSummary(record);
  if (!text) return null;

  return buildAssistantPhaseMessage(
    {
      id: `${record.run.id}-assistant-phase-${record.run.phase}`,
      createdAt: record.run.updated_at,
      content: [{ type: "text", text }],
      agentRole: expectedRole,
    },
    { type: "running" },
  );
}

function latestLiveAttemptTimestamp(liveTelemetry: LiveTelemetry, attemptId: string): string | null {
  const timestamps = [
    ...Object.values(liveTelemetry.reasoning)
      .filter((part) => part.attemptId === attemptId)
      .map((part) => part.updatedAt),
    ...Object.values(liveTelemetry.tools)
      .filter((part) => part.attemptId === attemptId)
      .map((part) => part.updatedAt),
  ].filter(Boolean);

  if (timestamps.length === 0) return null;
  return timestamps.sort((a, b) => +new Date(b) - +new Date(a))[0] ?? null;
}

function liveAttemptRole(liveTelemetry: LiveTelemetry, attemptId: string): AgentRole {
  const reasoningRole = Object.values(liveTelemetry.reasoning).find((part) => part.attemptId === attemptId)?.role;
  if (reasoningRole) return reasoningRole;
  const toolRole = Object.values(liveTelemetry.tools).find((part) => part.attemptId === attemptId)?.role;
  if (toolRole) return toolRole;
  return "generator";
}

function runStatusToMessageStatus(status: RunStatus): ThreadMessage["status"] {
  switch (status) {
    case "queued":
    case "gating":
    case "answering":
    case "selecting_project":
    case "planning":
    case "contracting":
    case "generating":
    case "evaluating":
    case "scheduling":
    case "reporting":
      return { type: "running" };
    case "waiting":
      return { type: "requires-action", reason: "interrupt" };
    case "completed":
      return { type: "complete", reason: "stop" };
    case "failed": case "exhausted":
      return { type: "incomplete", reason: "error" };
    case "cancelled":
      return { type: "incomplete", reason: "cancelled" };
  }
}

function reportOutcome(status: RunStatus) {
  switch (status) {
    case "completed":
      return "Completed";
    case "failed":
      return "Failed";
    case "exhausted":
      return "Retries exhausted";
    case "cancelled":
      return "Cancelled";
    default:
      return humanize(status);
  }
}

function latestPhaseSummary(record: RunRecord): string {
  const candidates = [...record.events]
    .filter((event) => event.phase === record.run.phase)
    .sort((a, b) => +new Date(b.created_at) - +new Date(a.created_at))
    .map((event) => event.summary?.trim())
    .filter(Boolean);

  return candidates[0] ?? statusMessage[record.run.status];
}

function attemptRoleForPhase(phase: RunStatus): AgentRole {
  switch (phase) {
    case "queued":
    case "gating":
      return "gate";
    case "answering":
      return "answer";
    case "selecting_project":
      return "project_selector";
    case "planning":
      return "planner";
    case "contracting":
      return "contractor";
    case "evaluating":
      return "evaluator";
    default:
      return "generator";
  }
}

function agentName(role: AgentRole) {
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

function parseLiveRunEvent(event: Event): LiveRunEvent | null {
  if (!(event instanceof MessageEvent) || typeof event.data !== "string" || !event.data.trim()) {
    return null;
  }
  try {
    return JSON.parse(event.data) as LiveRunEvent;
  } catch {
    return null;
  }
}

function applyLiveRunEvent(state: LiveTelemetry, event: LiveRunEvent): LiveTelemetry {
  const data = event.data ?? {};

  if (event.type === "reasoning") {
    const id = stringField(data, "item_id") || event.id;
    const attemptId = stringField(data, "attempt_id") || id;
    const role = roleField(data, "attempt_role");
    const text = stringField(data, "text") || event.summary;
    if (!isMeaningfulReasoningText(text)) return state;

    return {
      ...state,
      reasoning: {
        ...state.reasoning,
        [id]: { id, attemptId, role, text, updatedAt: event.created_at },
      },
    };
  }

  if (event.type === "tool_call_started" || event.type === "tool_call_completed") {
    const id = stringField(data, "tool_call_id") || stringField(data, "item_id") || event.id;
    const attemptId = stringField(data, "attempt_id") || id;
    const toolName = stringField(data, "tool_name") || "tool";
    const rawStatus = stringField(data, "status").toLowerCase();
    const status =
      event.type === "tool_call_started"
        ? "running"
        : rawStatus === "failed" || rawStatus === "declined"
          ? "incomplete"
          : "complete";

    return {
      ...state,
      tools: {
        ...state.tools,
        [id]: {
          id,
          attemptId,
          role: roleField(data, "attempt_role"),
          toolName,
          argsText: stringField(data, "input_summary") || state.tools[id]?.argsText,
          result: stringField(data, "output_summary") || state.tools[id]?.result,
          status,
          updatedAt: event.created_at,
        },
      },
    };
  }

  return state;
}

// ── utilities ─────────────────────────────────────────────────────────────────

function isActiveStatus(s: RunStatus) {
  return !["waiting", "completed", "failed", "exhausted", "cancelled"].includes(s);
}

function isFollowUpSourceStatus(s: RunStatus) {
  return s === "completed";
}

function getMessageText(msg: AppendMessage) {
  return msg.content.filter((p) => p.type === "text").map((p) => p.text).join("\n").trim();
}

function truncate(s: string, n: number) {
  s = s.replace(/\s+/g, " ").trim();
  return s.length <= n ? s : `${s.slice(0, n - 1).trimEnd()}…`;
}

function stringField(source: Record<string, unknown>, key: string) {
  const value = source[key];
  return typeof value === "string" ? value : "";
}

function isMeaningfulReasoningText(value: string) {
  switch (value.trim()) {
    case "":
    case "[]":
    case "{}":
    case "null":
    case `""`:
      return false;
    default:
      return true;
  }
}

function roleField(source: Record<string, unknown>, key: string): AgentRole {
  const value = stringField(source, key);
  if (value === "gate") return value;
  if (value === "answer") return value;
  if (value === "project_selector") return value;
  if (value === "contractor") return value;
  if (value === "planner" || value === "evaluator") return value;
  return "generator";
}

function humanize(s: string) {
  return s.replace(/[_-]+/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

function relativeTime(value: string) {
  const diff = Math.max(0, Math.round((Date.now() - +new Date(value)) / 60000));
  if (diff < 1) return "just now";
  if (diff < 60) return `${diff}m ago`;
  const h = Math.round(diff / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.round(h / 24)}d ago`;
}

function readChatIDFromHash() {
  const h = location.hash.replace(/^#/, "").trim();
  if (!h) return "";
  if (h.startsWith("chat=")) return decodeURIComponent(h.slice(5));
  if (h.startsWith("run=")) return decodeURIComponent(h.slice(4));
  return decodeURIComponent(h);
}

function updateHash(id: string) {
  location.hash = id ? `chat=${encodeURIComponent(id)}` : "";
}

async function fetchJSON<T>(url: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(url, {
    headers: { Accept: "application/json", ...(options.body ? { "Content-Type": "application/json" } : {}), ...(options.headers ?? {}) },
    ...options,
  });
  const text = await res.text();
  const payload = text ? (JSON.parse(text) as T) : (null as T);
  if (!res.ok) throw new Error(typeof payload === "string" ? payload : `request failed ${res.status}`);
  return payload;
}
