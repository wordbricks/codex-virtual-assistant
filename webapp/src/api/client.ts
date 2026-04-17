import type {
  BootstrapResponse,
  ChatRecord,
  ChatSummary,
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

export async function fetchJSON<T>(url: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(url, {
    headers: {
      Accept: "application/json",
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...(options.headers ?? {}),
    },
    ...options,
  });

  const text = await response.text();
  const payload = text ? (JSON.parse(text) as T) : (null as T);
  if (!response.ok) {
    throw new Error(typeof payload === "string" ? payload : `request failed ${response.status}`);
  }
  return payload;
}

export const apiClient = {
  bootstrap: () => fetchJSON<BootstrapResponse>("/api/v1/bootstrap"),
  listProjects: () => fetchJSON<{ projects: ProjectSummary[] }>("/api/v1/projects"),
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
