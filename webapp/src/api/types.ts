export type BootstrapResponse = {
  product_name: string;
  product_tagline: string;
  default_model: string;
  default_max_generation_attempts: number;
  chats_path: string;
  runs_path: string;
};

export type RunStatus =
  | "queued"
  | "gating"
  | "answering"
  | "selecting_project"
  | "planning"
  | "contracting"
  | "generating"
  | "evaluating"
  | "scheduling"
  | "wiki_ingesting"
  | "reporting"
  | "waiting"
  | "completed"
  | "failed"
  | "exhausted"
  | "cancelled";

export type ScheduledRunStatus = "pending" | "triggered" | "cancelled" | "failed";

export type AgentRole =
  | "gate"
  | "answer"
  | "project_selector"
  | "planner"
  | "contractor"
  | "generator"
  | "evaluator"
  | "scheduler"
  | "wiki_ingest"
  | "reporter";

export type RunRecord = {
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
    role: AgentRole;
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
  wait_requests: Array<{
    id: string;
    kind: string;
    title: string;
    prompt: string;
    risk_summary?: string;
    created_at: string;
  }>;
  scheduled_runs: ScheduledRun[];
};

export type ScheduledRun = {
  id: string;
  chat_id: string;
  parent_run_id: string;
  user_request_raw: string;
  max_generation_attempts: number;
  cron_expr?: string;
  scheduled_for: string;
  status: ScheduledRunStatus;
  run_id?: string;
  error_message?: string;
  created_at: string;
  triggered_at?: string;
};

export type ChatSummary = {
  id: string;
  root_run_id: string;
  latest_run_id: string;
  title: string;
  status: RunStatus;
  created_at: string;
  updated_at: string;
};

export type ChatRecord = {
  chat: ChatSummary;
  runs: RunRecord[];
};

export type LiveRunEvent = {
  id: string;
  run_id: string;
  type: string;
  phase: string;
  summary: string;
  created_at: string;
  data?: Record<string, unknown>;
};

export type ProjectSummary = {
  slug: string;
  name: string;
  description: string;
  workspace_dir: string;
  wiki_enabled: boolean;
  wiki_page_count: number;
  updated_at?: string;
};

export type ProjectRunStats = {
  active_runs: number;
  waiting_runs: number;
  scheduled_runs: number;
  completed_runs: number;
  stopped_runs: number;
  wiki_page_count: number;
};

export type ProjectDetailResponse = {
  project: ProjectSummary;
  stats: ProjectRunStats;
  recent_runs: Array<RunRecord["run"]>;
  latest_log_entries?: string[];
};

export type Pagination = {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
};

export type ProjectRunsResponse = {
  runs: Array<RunRecord["run"]>;
  pagination: Pagination;
  run_records?: RunRecord[];
};

export type WikiPageMeta = {
  path: string;
  title: string;
  page_type: string;
  updated_at?: string;
  status?: string;
  confidence?: string;
  source_refs?: string[];
  related?: string[];
};

export type WikiPageResponse = {
  meta: WikiPageMeta;
  path: string;
  content: string;
};
