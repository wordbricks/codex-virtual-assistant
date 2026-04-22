import type {
  AuthStatus,
  BootstrapResponse,
  ChatRecord,
  ChatSummary,
  CreateProjectResponse,
  ProjectDetailResponse,
  ProjectRunsResponse,
  ProjectSummary,
  RunRecord,
  RunStatus,
  ScheduledRun,
  ScheduledRunStatus,
  WikiPageMeta,
  WikiPageResponse,
} from "@/api/types";

let csrfToken = "";

export function setCSRFToken(token: string | undefined) {
  csrfToken = token ?? "";
}

export async function fetchJSON<T>(url: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set("Accept", "application/json");
  if (options.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const method = (options.method ?? "GET").toUpperCase();
  if (csrfToken && !["GET", "HEAD", "OPTIONS"].includes(method)) {
    headers.set("X-CVA-CSRF", csrfToken);
  }

  const response = await fetch(url, {
    ...options,
    credentials: options.credentials ?? "same-origin",
    headers,
  });

  const text = await response.text();
  let payload: T | string | null = null;
  if (text) {
    try {
      payload = JSON.parse(text) as T;
    } catch {
      payload = text;
    }
  }
  if (!response.ok) {
    if (response.status === 401) setCSRFToken(undefined);
    if (payload && typeof payload === "object" && "error" in payload && typeof payload.error === "string") {
      throw new Error(payload.error);
    }
    throw new Error(typeof payload === "string" ? payload.trim() : `request failed ${response.status}`);
  }
  return payload as T;
}

export const apiClient = {
  authStatus: async () => {
    const status = await fetchJSON<AuthStatus>("/api/v1/auth/status");
    setCSRFToken(status.csrf_token);
    return status;
  },
  login: async (body: { user_id: string; password: string }) => {
    const status = await fetchJSON<AuthStatus>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify(body),
    });
    setCSRFToken(status.csrf_token);
    return status;
  },
  logout: async () => {
    const status = await fetchJSON<AuthStatus>("/api/v1/auth/logout", { method: "POST" });
    setCSRFToken(status.csrf_token);
    return status;
  },
  bootstrap: () => fetchJSON<BootstrapResponse>("/api/v1/bootstrap"),
  listProjects: () => fetchJSON<{ projects: ProjectSummary[] }>("/api/v1/projects"),
  createProject: (body: { slug?: string; name: string; description?: string }) =>
    fetchJSON<CreateProjectResponse>("/api/v1/projects", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  getProjectDetail: (slug: string) =>
    fetchJSON<ProjectDetailResponse>(`/api/v1/projects/${encodeURIComponent(slug)}`),
  listWikiPages: (slug: string) =>
    fetchJSON<{ pages: WikiPageMeta[] }>(`/api/v1/projects/${encodeURIComponent(slug)}/wiki/pages`),
  getWikiPage: (slug: string, path: string) =>
    fetchJSON<WikiPageResponse>(
      `/api/v1/projects/${encodeURIComponent(slug)}/wiki/page?path=${encodeURIComponent(path)}`,
    ),
  listProjectRuns: (
    slug: string,
    options: {
      status?: RunStatus;
      page?: number;
      pageSize?: number;
      includeDetails?: boolean;
    } = {},
  ) => {
    const params = new URLSearchParams();
    if (options.status) params.set("status", options.status);
    if (options.page) params.set("page", String(options.page));
    if (options.pageSize) params.set("page_size", String(options.pageSize));
    if (typeof options.includeDetails === "boolean") params.set("include_details", String(options.includeDetails));
    const query = params.toString();
    const suffix = query ? `?${query}` : "";
    return fetchJSON<ProjectRunsResponse>(`/api/v1/projects/${encodeURIComponent(slug)}/runs${suffix}`);
  },
  getRunRecord: (runId: string) => fetchJSON<RunRecord>(`/api/v1/runs/${encodeURIComponent(runId)}`),
  getScheduledRun: (scheduledRunId: string) =>
    fetchJSON<ScheduledRun>(`/api/v1/scheduled/${encodeURIComponent(scheduledRunId)}`),
  listScheduledRuns: (options: { chatId?: string; status?: ScheduledRunStatus } = {}) => {
    const params = new URLSearchParams();
    if (options.chatId) params.set("chat_id", options.chatId);
    if (options.status) params.set("status", options.status);
    const query = params.toString();
    const suffix = query ? `?${query}` : "";
    return fetchJSON<{ scheduled_runs: ScheduledRun[] }>(`/api/v1/scheduled${suffix}`);
  },
  listChats: (path: string) => fetchJSON<{ chats: ChatSummary[] }>(path),
  getChat: (path: string, chatId: string) => fetchJSON<ChatRecord>(`${path}/${encodeURIComponent(chatId)}`),
  createRun: (
    path: string,
    body: {
      user_request_raw: string;
      max_generation_attempts: number;
      parent_run_id?: string;
      project_slug?: string;
    },
  ) =>
    fetchJSON<{ run: { id: string; chat_id: string; status: RunStatus; updated_at: string } }>(path, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  cancelRun: (runId: string) => fetchJSON<RunRecord>(`/api/v1/runs/${encodeURIComponent(runId)}/cancel`, { method: "POST" }),
};
