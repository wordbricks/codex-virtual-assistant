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
  runs_path: string;
};

type RunStatus =
  | "queued"
  | "selecting_project"
  | "planning"
  | "contracting"
  | "generating"
  | "evaluating"
  | "waiting"
  | "completed"
  | "failed"
  | "exhausted"
  | "cancelled";

type RunRecord = {
  run: {
    id: string;
    status: RunStatus;
    phase: RunStatus;
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
    role: "planner" | "contractor" | "generator" | "evaluator";
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
  role: "planner" | "contractor" | "generator" | "evaluator";
  text: string;
  updatedAt: string;
};

type LiveToolPart = {
  id: string;
  attemptId: string;
  role: "planner" | "contractor" | "generator" | "evaluator";
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

type AgentRole = "planner" | "contractor" | "generator" | "evaluator";

type HistoryEntry = {
  id: string;
  title: string;
  status: RunStatus;
  updatedAt: string;
};

const RUN_HISTORY_KEY = "cva.run-history";
const MAX_HISTORY_ITEMS = 20;
const TERMINAL_STATUSES: RunStatus[] = ["completed", "failed", "exhausted", "cancelled"];

const statusLabel: Record<RunStatus, string> = {
  queued: "Queued",
  selecting_project: "Selecting Project",
  planning: "Planning",
  contracting: "Contracting",
  generating: "Working",
  evaluating: "Checking",
  waiting: "Waiting",
  completed: "Done",
  failed: "Failed",
  exhausted: "Exhausted",
  cancelled: "Cancelled",
};

const statusMessage: Record<RunStatus, string> = {
  queued: "Task accepted, starting soon…",
  selecting_project: "Choosing the right project workspace…",
  planning: "Turning your request into a plan…",
  contracting: "Locking the acceptance contract before work starts…",
  generating: "Working on the task…",
  evaluating: "Checking whether the result is complete…",
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
  const [selectedRunId, setSelectedRunId] = useState<string>(readRunIDFromHash());
  const [currentRecord, setCurrentRecord] = useState<RunRecord | null>(null);
  const [messages, setMessages] = useState<ThreadMessage[]>([]);
  const [liveTelemetry, setLiveTelemetry] = useState<LiveTelemetry>(emptyLiveTelemetry);
  const [runHistory, setRunHistory] = useState<HistoryEntry[]>(loadRunHistory);
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

  const rememberRun = useCallback((entry: HistoryEntry) => {
    setRunHistory((prev) => {
      const next = [entry, ...prev.filter((r) => r.id !== entry.id)].slice(0, MAX_HISTORY_ITEMS);
      localStorage.setItem(RUN_HISTORY_KEY, JSON.stringify(next));
      return next;
    });
  }, []);

  const hydrateRecord = useCallback(
    (record: RunRecord | null) => {
      setCurrentRecord(record);
      if (!record) {
        setMessages([]);
        setIsRunning(false);
        return;
      }
      setIsRunning(isActiveStatus(record.run.status));
      rememberRun({
        id: record.run.id,
        title: truncate(record.run.task_spec.goal || record.run.user_request_raw, 48),
        status: record.run.status,
        updatedAt: record.run.updated_at,
      });
    },
    [rememberRun],
  );

  useEffect(() => {
    if (!currentRecord) return;
    setMessages(recordToMessages(currentRecord, liveTelemetry));
  }, [currentRecord, liveTelemetry]);

  const refreshRun = useCallback(
    async (runID: string) => {
      if (!bootstrap) return;
      const record = await fetchJSON<RunRecord>(`${bootstrap.runs_path}/${encodeURIComponent(runID)}`);
      hydrateRecord(record);
      return record;
    },
    [bootstrap, hydrateRecord],
  );

  const syncPolling = useCallback(
    (runID: string, status: RunStatus) => {
      stopPolling();
      if (!isActiveStatus(status)) return;
      pollTimerRef.current = window.setTimeout(() => void refreshRun(runID).catch(() => undefined), 4000);
    },
    [refreshRun, stopPolling],
  );

  const connectEventStream = useCallback(
    (runID: string, status: RunStatus) => {
      closeEventStream();
      if (!isActiveStatus(status)) return;
      const source = new EventSource(`${bootstrap?.runs_path}/${encodeURIComponent(runID)}/events`);
      source.addEventListener("run_event", (event) => {
        const payload = parseLiveRunEvent(event);
        if (payload?.run_id === runID && LIVE_EVENT_TYPES.has(payload.type)) {
          setLiveTelemetry((prev) => applyLiveRunEvent(prev, payload));
          return;
        }

        void refreshRun(runID).then((record) => {
          if (record) {
            syncPolling(runID, record.run.status);
            if (!isActiveStatus(record.run.status)) closeEventStream();
          }
        });
      });
      source.onerror = () => syncPolling(runID, status);
      eventSourceRef.current = source;
    },
    [bootstrap?.runs_path, closeEventStream, refreshRun, syncPolling],
  );

  const syncRealtimeForRecord = useCallback(
    (record: RunRecord) => {
      closeEventStream();
      stopPolling();
      if (isActiveStatus(record.run.status)) {
        connectEventStream(record.run.id, record.run.status);
        syncPolling(record.run.id, record.run.status);
      }
    },
    [closeEventStream, connectEventStream, stopPolling, syncPolling],
  );

  const loadRun = useCallback(
    async (runID: string) => {
      setIsLoading(true);
      try {
        const record = await refreshRun(runID);
        if (record) syncRealtimeForRecord(record);
      } finally {
        setIsLoading(false);
      }
    },
    [refreshRun, syncRealtimeForRecord],
  );

  useEffect(() => {
    void fetchJSON<BootstrapResponse>("/api/v1/bootstrap").then((p) => {
      setBootstrap(p);
      setAttempts(p.default_max_generation_attempts);
      setIsLoading(false);
    });
  }, []);

  useEffect(() => {
    const onHashChange = () => setSelectedRunId(readRunIDFromHash());
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  useEffect(() => {
    if (!bootstrap) return;
    if (!selectedRunId) {
      closeEventStream();
      stopPolling();
      hydrateRecord(null);
      setLiveTelemetry(emptyLiveTelemetry());
      setThreadKey(`draft-${Date.now()}`);
      return;
    }
    setLiveTelemetry(emptyLiveTelemetry());
    setThreadKey(selectedRunId);
    void loadRun(selectedRunId);
    return () => { closeEventStream(); stopPolling(); };
  }, [bootstrap, closeEventStream, hydrateRecord, loadRun, selectedRunId, stopPolling]);

  const onNew = useCallback(
    async (message: AppendMessage) => {
      if (!bootstrap) return;
      const text = getMessageText(message);
      if (!text) return;

      setMessages([buildUserMessage(text, new Date())]);
      setIsRunning(true);
      setCurrentRecord(null);
      setLiveTelemetry(emptyLiveTelemetry());

      const created = await fetchJSON<{ run: { id: string; status: RunStatus; updated_at: string } }>(
        bootstrap.runs_path,
        { method: "POST", body: JSON.stringify({ user_request_raw: text, max_generation_attempts: attempts }) },
      );

      rememberRun({ id: created.run.id, title: truncate(text, 48), status: created.run.status, updatedAt: created.run.updated_at });
      setSelectedRunId(created.run.id);
      setThreadKey(created.run.id);
      updateHash(created.run.id);
      await loadRun(created.run.id);
    },
    [attempts, bootstrap, loadRun, rememberRun],
  );

  const onCancel = useCallback(async () => {
    if (!bootstrap || !selectedRunId) return;
    const record = await fetchJSON<RunRecord>(`${bootstrap.runs_path}/${encodeURIComponent(selectedRunId)}/cancel`, { method: "POST" });
    hydrateRecord(record);
    syncRealtimeForRecord(record);
  }, [bootstrap, hydrateRecord, selectedRunId, syncRealtimeForRecord]);

  const resumeWaitingRun = useCallback(
    async ({ approved, response }: { approved: boolean; response?: string }) => {
      if (!currentRecord) return;
      const input: Record<string, string> = {};
      if (response?.trim()) input.response = response.trim();
      if (approved) input.approval = "approved";
      await fetchJSON<{ ok: boolean }>(
        `/api/v1/runs/${encodeURIComponent(currentRecord.run.id)}/resume`,
        {
          method: "POST",
          body: Object.keys(input).length ? JSON.stringify({ input }) : undefined,
        },
      );
      const record = await fetchJSON<RunRecord>(
        `/api/v1/runs/${encodeURIComponent(currentRecord.run.id)}`,
      );
      hydrateRecord(record);
      syncRealtimeForRecord(record);
    },
    [currentRecord, hydrateRecord, syncRealtimeForRecord],
  );

  const cancelWaitingRun = useCallback(async () => {
    if (!currentRecord) return;
    const record = await fetchJSON<RunRecord>(
      `/api/v1/runs/${encodeURIComponent(currentRecord.run.id)}/cancel`,
      { method: "POST" },
    );
    hydrateRecord(record);
    syncRealtimeForRecord(record);
  }, [currentRecord, hydrateRecord, syncRealtimeForRecord]);

  const runtimeAdapter = useMemo<ExternalStoreAdapter<ThreadMessage>>(
    () => ({ isLoading, isRunning, messages, onNew, onCancel }),
    [isLoading, isRunning, messages, onCancel, onNew],
  );

  const runtime = useExternalStoreRuntime(runtimeAdapter);
  const status = currentRecord?.run.status ?? null;
  const showReport = Boolean(currentRecord && status && TERMINAL_STATUSES.includes(status));

  useEffect(() => {
    if (!currentRecord) {
      setIsReportOpen(false);
      previousRunIDRef.current = "";
      previousIsRunningRef.current = isRunning;
      return;
    }

    const runChanged = previousRunIDRef.current !== currentRecord.run.id;

    if (runChanged) {
      setIsReportOpen(false);
    } else if (previousIsRunningRef.current && !isRunning && TERMINAL_STATUSES.includes(currentRecord.run.status)) {
      setIsReportOpen(true);
    }

    previousRunIDRef.current = currentRecord.run.id;
    previousIsRunningRef.current = isRunning;
  }, [currentRecord, isRunning]);

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
              onClick={() => { setSelectedRunId(""); updateHash(""); }}
            >
              + New chat
            </button>

            <nav className="sidebar-history">
              {runHistory.length === 0 ? (
                <p className="sidebar-empty">No recent chats.</p>
              ) : (
                runHistory.map((entry) => (
                  <button
                    key={entry.id}
                    type="button"
                    className={`sidebar-item${entry.id === selectedRunId ? " is-active" : ""}`}
                    onClick={() => { setSelectedRunId(entry.id); updateHash(entry.id); }}
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
                {currentRecord?.run.task_spec.goal || "How can Codex help today?"}
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
                    phaseEvents={currentRecord?.events.map((event) => event.phase as "queued" | "selecting_project" | "planning" | "contracting" | "generating" | "evaluating" | "waiting" | "completed" | "failed" | "cancelled") ?? []}
                    generatorAttempts={currentRecord?.attempts.filter((attempt) => attempt.role === "generator").length ?? 0}
                    maxGenerationAttempts={currentRecord?.run.max_generation_attempts ?? 0}
                    waitingFor={currentRecord?.run.waiting_for ?? null}
                    waitingKey={currentRecord ? `${currentRecord.run.id}:${currentRecord.run.updated_at}` : null}
                    onWaitingSubmit={resumeWaitingRun}
                    onWaitingCancel={cancelWaitingRun}
                  />
                </div>
              </div>

              {showReport && currentRecord && (
                <SupervisorReport
                  record={currentRecord}
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

function recordToMessages(record: RunRecord, liveTelemetry: LiveTelemetry): ThreadMessage[] {
  const user = buildUserMessage(record.run.user_request_raw, new Date(record.run.created_at), `${record.run.id}-user`);
  const assistants = buildAssistantMessages(record, liveTelemetry);
  if (assistants.length === 0) return [user];
  return [user, ...assistants];
}

function buildUserMessage(text: string, createdAt: Date, id = "draft-user"): ThreadMessage {
  return { id, role: "user", createdAt, content: [{ type: "text", text }], attachments: [], metadata: { custom: {} } };
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
    case "queued": case "selecting_project": case "planning": case "contracting": case "generating": case "evaluating":
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
    case "selecting_project":
      return "planner";
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

function roleField(source: Record<string, unknown>, key: string): "planner" | "contractor" | "generator" | "evaluator" {
  const value = stringField(source, key);
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

function readRunIDFromHash() {
  const h = location.hash.replace(/^#/, "").trim();
  if (!h) return "";
  return decodeURIComponent(h.startsWith("run=") ? h.slice(4) : h);
}

function updateHash(id: string) {
  location.hash = id ? `run=${encodeURIComponent(id)}` : "";
}

function loadRunHistory(): HistoryEntry[] {
  try {
    const raw = localStorage.getItem(RUN_HISTORY_KEY);
    const parsed = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed) ? parsed : [];
  } catch { return []; }
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
