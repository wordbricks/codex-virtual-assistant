import type { BootstrapResponse, ChatRecord, ChatSummary, ProjectSummary, RunRecord, RunStatus } from "@/api/types";

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
