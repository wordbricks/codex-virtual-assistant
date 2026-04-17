import { useMemo, useState, type FormEvent } from "react";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { apiClient } from "@/api/client";
import type { ProjectDetailResponse, ProjectSummary, RunStatus, WikiPageMeta } from "@/api/types";
import { LegacyChatPage } from "@/legacy/LegacyChatPage";

const statusLabel: Record<RunStatus, string> = {
  queued: "Queued",
  gating: "Gating",
  answering: "Answering",
  selecting_project: "Selecting Project",
  planning: "Planning",
  contracting: "Contracting",
  generating: "Generating",
  evaluating: "Evaluating",
  scheduling: "Scheduling",
  wiki_ingesting: "Wiki Ingesting",
  reporting: "Reporting",
  waiting: "Waiting",
  completed: "Completed",
  failed: "Failed",
  exhausted: "Exhausted",
  cancelled: "Cancelled",
};

const pageTypeOrder = [
  "overview",
  "topic",
  "entity",
  "decision",
  "playbook",
  "source",
  "question",
  "report",
] as const;

const pageTypeIcons: Record<string, string> = {
  overview: "📘",
  topic: "🧩",
  entity: "🏷️",
  decision: "✅",
  playbook: "🛠️",
  source: "🔗",
  question: "❓",
  report: "📝",
};

export function ProjectsHomePlaceholder() {
  const projectsQuery = useQuery({
    queryKey: ["projects"],
    queryFn: apiClient.listProjects,
    staleTime: 30_000,
  });

  const projects = projectsQuery.data?.projects ?? [];
  const detailQueries = useQueries({
    queries: projects.map((project) => ({
      queryKey: ["project-detail", project.slug],
      queryFn: () => apiClient.getProjectDetail(project.slug),
      staleTime: 20_000,
      enabled: project.slug !== "no_project",
    })),
  });

  const detailBySlug = useMemo(() => {
    const bySlug = new Map<string, ProjectDetailResponse>();
    projects.forEach((project, index) => {
      const query = detailQueries[index];
      if (query?.data) bySlug.set(project.slug, query.data);
    });
    return bySlug;
  }, [detailQueries, projects]);

  const inbox = projects.find((project) => project.slug === "no_project") ?? null;
  const namedProjects = projects.filter((project) => project.slug !== "no_project");

  return (
    <section className="projects-home">
      <header className="projects-home-header">
        <div>
          <p className="projects-kicker">Project Console</p>
          <h2 className="projects-title">Projects</h2>
          <p className="projects-subtitle">Choose a project to review wiki memory, recent runs, and current status.</p>
        </div>
      </header>

      {projectsQuery.isLoading && <p className="projects-note">Loading projects…</p>}
      {projectsQuery.isError && <p className="projects-note">Failed to load projects: {(projectsQuery.error as Error).message}</p>}

      {inbox && (
        <section className="projects-inbox">
          <h3>Inbox / Unsorted</h3>
          <p>{inbox.description || "Quick one-off work without durable project memory."}</p>
        </section>
      )}

      <div className="project-grid">
        {namedProjects.map((project) => {
          const detail = detailBySlug.get(project.slug);
          return (
            <Link key={project.slug} to="/projects/$slug" params={{ slug: project.slug }} className="project-card">
              <div className="project-card-head">
                <h3>{project.name}</h3>
                <span>{project.slug}</span>
              </div>
              <p className="project-card-desc">{project.description || "No description available."}</p>
              <dl className="project-card-stats">
                <div>
                  <dt>Wiki Pages</dt>
                  <dd>{project.wiki_page_count}</dd>
                </div>
                <div>
                  <dt>Active</dt>
                  <dd>{detail?.stats.active_runs ?? "—"}</dd>
                </div>
                <div>
                  <dt>Waiting</dt>
                  <dd>{detail?.stats.waiting_runs ?? "—"}</dd>
                </div>
                <div>
                  <dt>Completed</dt>
                  <dd>{detail?.stats.completed_runs ?? "—"}</dd>
                </div>
              </dl>
              {detail?.recent_runs?.[0] && (
                <p className="project-card-run">
                  Latest run: {truncate(detail.recent_runs[0].task_spec.goal || detail.recent_runs[0].user_request_raw, 84)}
                </p>
              )}
            </Link>
          );
        })}
      </div>

      {!projectsQuery.isLoading && !projectsQuery.isError && namedProjects.length === 0 && (
        <p className="projects-note">No named projects found yet.</p>
      )}
    </section>
  );
}

export function ProjectOverviewPlaceholder() {
  const { slug } = useParams({ from: "/projects/$slug" });
  const queryClient = useQueryClient();

  const bootstrapQuery = useQuery({
    queryKey: ["bootstrap"],
    queryFn: apiClient.bootstrap,
    staleTime: 30_000,
  });

  const detailQuery = useQuery({
    queryKey: ["project-detail", slug],
    queryFn: () => apiClient.getProjectDetail(slug),
    staleTime: 20_000,
  });

  const overviewQuery = useQuery({
    queryKey: ["wiki-page", slug, "overview.md"],
    queryFn: () => apiClient.getWikiPage(slug, "overview.md"),
    staleTime: 20_000,
  });

  const openQuestionsQuery = useQuery({
    queryKey: ["wiki-page", slug, "open-questions.md"],
    queryFn: () => apiClient.getWikiPage(slug, "open-questions.md"),
    staleTime: 20_000,
  });

  const [requestText, setRequestText] = useState("");
  const [maxAttempts, setMaxAttempts] = useState(bootstrapQuery.data?.default_max_generation_attempts ?? 3);
  const [runMessage, setRunMessage] = useState<string>("");

  const createRunMutation = useMutation({
    mutationFn: async () => {
      const text = requestText.trim();
      if (!text) throw new Error("Run request is required.");
      return apiClient.createRun("/api/v1/runs", {
        user_request_raw: text,
        max_generation_attempts: maxAttempts,
        project_slug: slug,
      });
    },
    onSuccess: (result) => {
      setRunMessage(`Run ${result.run.id} queued for project ${slug}.`);
      setRequestText("");
      void queryClient.invalidateQueries({ queryKey: ["project-detail", slug] });
    },
    onError: (error) => {
      setRunMessage((error as Error).message);
    },
  });

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    createRunMutation.mutate();
  };

  return (
    <section className="project-overview">
      {detailQuery.isLoading && <p className="projects-note">Loading project…</p>}
      {detailQuery.isError && <p className="projects-note">Failed to load project: {(detailQuery.error as Error).message}</p>}

      {detailQuery.data && (
        <>
          <header className="project-overview-header">
            <div>
              <p className="projects-kicker">Project</p>
              <h2>{detailQuery.data.project.name}</h2>
              <p className="project-overview-subtitle">{detailQuery.data.project.description || "No description available."}</p>
            </div>
            <div className="project-overview-links">
              <Link to="/projects/$slug/wiki" params={{ slug }}>Open Wiki</Link>
              <Link to="/projects/$slug/runs" params={{ slug }}>Open Runs</Link>
            </div>
          </header>

          <dl className="project-overview-stats">
            <div><dt>Active</dt><dd>{detailQuery.data.stats.active_runs}</dd></div>
            <div><dt>Waiting</dt><dd>{detailQuery.data.stats.waiting_runs}</dd></div>
            <div><dt>Scheduled</dt><dd>{detailQuery.data.stats.scheduled_runs}</dd></div>
            <div><dt>Completed</dt><dd>{detailQuery.data.stats.completed_runs}</dd></div>
            <div><dt>Stopped</dt><dd>{detailQuery.data.stats.stopped_runs}</dd></div>
            <div><dt>Wiki Pages</dt><dd>{detailQuery.data.stats.wiki_page_count}</dd></div>
          </dl>

          <div className="project-overview-grid">
            <article className="project-panel">
              <h3>Overview</h3>
              <WikiMarkdown slug={slug} currentPath="overview.md" markdown={stripFrontmatter(overviewQuery.data?.content ?? "No overview content yet.")} />
            </article>

            <article className="project-panel">
              <h3>Recent Log Entries</h3>
              <ul className="project-log-list">
                {(detailQuery.data.latest_log_entries ?? []).map((entry) => (
                  <li key={entry}>{entry}</li>
                ))}
                {(detailQuery.data.latest_log_entries ?? []).length === 0 && <li>No recent log entries.</li>}
              </ul>
            </article>

            <article className="project-panel">
              <h3>Open Questions</h3>
              <WikiMarkdown slug={slug} currentPath="open-questions.md" markdown={stripFrontmatter(openQuestionsQuery.data?.content ?? "No open questions yet.")} />
            </article>

            <article className="project-panel">
              <h3>Recent Runs</h3>
              <ul className="project-runs-list">
                {detailQuery.data.recent_runs.map((run) => (
                  <li key={run.id}>
                    <div>
                      <strong>{truncate(run.task_spec.goal || run.user_request_raw, 90)}</strong>
                      <p>{relativeTime(run.updated_at || run.created_at)}</p>
                    </div>
                    <span className="status-pill" data-status={run.status}>{statusLabel[run.status]}</span>
                  </li>
                ))}
                {detailQuery.data.recent_runs.length === 0 && <li>No runs yet.</li>}
              </ul>
            </article>
          </div>

          <section className="project-composer">
            <h3>Start a New Run</h3>
            <form onSubmit={onSubmit}>
              <textarea
                value={requestText}
                onChange={(event) => setRequestText(event.target.value)}
                placeholder="Describe the next task for this project…"
                rows={4}
              />
              <div className="project-composer-row">
                <label>
                  Max attempts
                  <input
                    type="number"
                    min={1}
                    max={9}
                    value={maxAttempts}
                    onChange={(event) => setMaxAttempts(Number.parseInt(event.target.value, 10) || 1)}
                  />
                </label>
                <button type="submit" disabled={createRunMutation.isPending}>
                  {createRunMutation.isPending ? "Starting…" : "Start Run"}
                </button>
              </div>
            </form>
            {runMessage && <p className="project-run-message">{runMessage}</p>}
          </section>
        </>
      )}
    </section>
  );
}

export function ProjectWikiPlaceholder() {
  const { slug } = useParams({ from: "/projects/$slug/wiki" });
  return <ProjectWikiReader slug={slug} selectedPath="index.md" />;
}

export function ProjectWikiPagePlaceholder() {
  const { slug, _splat } = useParams({ from: "/projects/$slug/wiki/$" });
  const selectedPath = (_splat && _splat.trim()) || "index.md";
  return <ProjectWikiReader slug={slug} selectedPath={selectedPath} />;
}

function ProjectWikiReader({ slug, selectedPath }: { slug: string; selectedPath: string }) {
  const pagesQuery = useQuery({
    queryKey: ["wiki-pages", slug],
    queryFn: () => apiClient.listWikiPages(slug),
    staleTime: 20_000,
  });

  const pageQuery = useQuery({
    queryKey: ["wiki-page", slug, selectedPath],
    queryFn: () => apiClient.getWikiPage(slug, selectedPath),
    staleTime: 20_000,
  });

  const navigate = useNavigate();

  const pageMetaByPath = useMemo(() => {
    const map = new Map<string, WikiPageMeta>();
    for (const page of pagesQuery.data?.pages ?? []) map.set(page.path, page);
    return map;
  }, [pagesQuery.data?.pages]);

  const selectedMeta = pageMetaByPath.get(selectedPath) ?? pageQuery.data?.meta;
  const grouped = useMemo(() => groupPagesByType(pagesQuery.data?.pages ?? []), [pagesQuery.data?.pages]);

  const openWikiPath = (path: string) => {
    if (path === "index.md") {
      void navigate({ to: "/projects/$slug/wiki", params: { slug } });
      return;
    }
    void navigate({ to: "/projects/$slug/wiki/$", params: { slug, _splat: path } });
  };

  return (
    <section className="wiki-layout">
      <aside className="wiki-sidebar">
        <div className="wiki-sidebar-header">
          <p className="projects-kicker">Wiki</p>
          <h3>{slug}</h3>
        </div>

        {pagesQuery.isLoading && <p className="projects-note">Loading pages…</p>}
        {pagesQuery.isError && <p className="projects-note">Failed to load pages.</p>}

        <div className="wiki-tree">
          {grouped.map((group) => (
            <details key={group.pageType} open className="wiki-group">
              <summary>
                <span>{pageTypeIcons[group.pageType] ?? "📄"}</span>
                <span>{humanize(group.pageType)}</span>
                <span className="wiki-group-count">{group.pages.length}</span>
              </summary>
              <FolderBranch
                branch={group.branch}
                selectedPath={selectedPath}
                onOpenPath={openWikiPath}
              />
            </details>
          ))}
          {grouped.length === 0 && <p className="projects-note">No wiki pages found.</p>}
        </div>
      </aside>

      <article className="wiki-document">
        <WikiBreadcrumb slug={slug} path={selectedPath} title={selectedMeta?.title} />

        <header className="wiki-document-header">
          <h2>{selectedMeta?.title ?? selectedPath}</h2>
          <div className="wiki-meta-row">
            {selectedMeta?.status && <span className="wiki-meta-pill">status: {selectedMeta.status}</span>}
            {selectedMeta?.confidence && <span className="wiki-meta-pill">confidence: {selectedMeta.confidence}</span>}
            {(selectedMeta?.source_refs ?? []).map((source) => (
              <span key={source} className="wiki-meta-pill">{source}</span>
            ))}
            {(selectedMeta?.related ?? []).map((related) => (
              <button
                key={related}
                type="button"
                className="wiki-meta-pill wiki-meta-link"
                onClick={() => openWikiPath(normalizeWikiPath(related))}
              >
                {related}
              </button>
            ))}
          </div>
        </header>

        {pageQuery.isLoading && <p className="projects-note">Loading page…</p>}
        {pageQuery.isError && <p className="projects-note">Failed to load page.</p>}

        {pageQuery.data && (
          <WikiMarkdown
            slug={slug}
            currentPath={selectedPath}
            markdown={stripFrontmatter(pageQuery.data.content)}
          />
        )}
      </article>
    </section>
  );
}

function FolderBranch({
  branch,
  selectedPath,
  onOpenPath,
}: {
  branch: FolderNode;
  selectedPath: string;
  onOpenPath: (path: string) => void;
}) {
  return (
    <div className="wiki-branch">
      {branch.folders.map((folder) => (
        <details key={folder.key} open className="wiki-folder">
          <summary>{folder.name}</summary>
          <FolderBranch branch={folder} selectedPath={selectedPath} onOpenPath={onOpenPath} />
        </details>
      ))}
      {branch.pages.map((page) => (
        <button
          key={page.path}
          type="button"
          className={`wiki-page-link${page.path === selectedPath ? " is-active" : ""}`}
          onClick={() => onOpenPath(page.path)}
        >
          {page.title}
        </button>
      ))}
    </div>
  );
}

function WikiBreadcrumb({ slug, path, title }: { slug: string; path: string; title?: string }) {
  const segments = path.split("/").filter(Boolean);
  return (
    <nav className="wiki-breadcrumb" aria-label="Breadcrumb">
      <Link to="/">Projects</Link>
      <span>/</span>
      <Link to="/projects/$slug" params={{ slug }}>{slug}</Link>
      <span>/</span>
      <Link to="/projects/$slug/wiki" params={{ slug }}>wiki</Link>
      {segments.length > 1 && (
        <>
          {segments.slice(0, -1).map((segment) => (
            <span key={segment}>/ {segment}</span>
          ))}
        </>
      )}
      <span>/ {title ?? segments[segments.length - 1] ?? "index.md"}</span>
    </nav>
  );
}

function WikiMarkdown({ slug, currentPath, markdown }: { slug: string; currentPath: string; markdown: string }) {
  const navigate = useNavigate();

  const components = useMemo<Components>(
    () => ({
      a: ({ href, children, ...props }) => {
        const resolved = resolveWikiLink(currentPath, href ?? "");
        if (resolved.type === "internal") {
          const destination = resolved.path === "index.md" ? `/projects/${slug}/wiki` : `/projects/${slug}/wiki/${resolved.path}`;
          return (
            <a
              href={destination}
              onClick={(event) => {
                event.preventDefault();
                if (resolved.path === "index.md") {
                  void navigate({ to: "/projects/$slug/wiki", params: { slug } });
                } else {
                  void navigate({ to: "/projects/$slug/wiki/$", params: { slug, _splat: resolved.path } });
                }
              }}
              {...props}
            >
              {children}
            </a>
          );
        }

        const external = /^https?:\/\//.test(href ?? "");
        return (
          <a href={href} target={external ? "_blank" : undefined} rel={external ? "noreferrer" : undefined} {...props}>
            {children}
          </a>
        );
      },
    }),
    [currentPath, navigate, slug],
  );

  return (
    <div className="wiki-markdown">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {markdown}
      </ReactMarkdown>
    </div>
  );
}

export function ProjectRunsPlaceholder() {
  const { slug } = useParams({ from: "/projects/$slug/runs" });

  return (
    <section className="chat-stage" style={{ padding: "2rem" }}>
      <header className="chat-header">
        <h2 className="chat-title">Runs: {slug}</h2>
      </header>
      <p>Runs board route scaffold is ready for Milestone 6.</p>
    </section>
  );
}

export function LegacyChatRoute() {
  return <LegacyChatPage />;
}

type FolderNode = {
  name: string;
  key: string;
  pages: WikiPageMeta[];
  folders: FolderNode[];
};

type PageTypeGroup = {
  pageType: string;
  pages: WikiPageMeta[];
  branch: FolderNode;
};

function groupPagesByType(pages: WikiPageMeta[]): PageTypeGroup[] {
  const byType = new Map<string, WikiPageMeta[]>();
  for (const page of pages) {
    const pageType = page.page_type || "overview";
    const group = byType.get(pageType) ?? [];
    group.push(page);
    byType.set(pageType, group);
  }

  const groups = Array.from(byType.entries()).map(([pageType, groupPages]) => {
    const sorted = [...groupPages].sort((left, right) => left.path.localeCompare(right.path));
    return {
      pageType,
      pages: sorted,
      branch: buildFolderTree(sorted),
    };
  });

  groups.sort((left, right) => {
    const leftIndex = pageTypeOrder.indexOf(left.pageType as (typeof pageTypeOrder)[number]);
    const rightIndex = pageTypeOrder.indexOf(right.pageType as (typeof pageTypeOrder)[number]);
    const leftRank = leftIndex === -1 ? Number.MAX_SAFE_INTEGER : leftIndex;
    const rightRank = rightIndex === -1 ? Number.MAX_SAFE_INTEGER : rightIndex;
    if (leftRank === rightRank) return left.pageType.localeCompare(right.pageType);
    return leftRank - rightRank;
  });

  return groups;
}

function buildFolderTree(pages: WikiPageMeta[]): FolderNode {
  type MutableNode = {
    name: string;
    key: string;
    pages: WikiPageMeta[];
    folders: Map<string, MutableNode>;
  };

  const root: MutableNode = {
    name: "",
    key: "root",
    pages: [],
    folders: new Map(),
  };

  for (const page of pages) {
    const segments = page.path.split("/").filter(Boolean);
    let cursor = root;
    const fileName = segments.pop() ?? page.path;

    for (const segment of segments) {
      const nextKey = `${cursor.key}/${segment}`;
      if (!cursor.folders.has(segment)) {
        cursor.folders.set(segment, {
          name: segment,
          key: nextKey,
          pages: [],
          folders: new Map(),
        });
      }
      cursor = cursor.folders.get(segment)!;
    }

    cursor.pages.push({ ...page, title: page.title || fileName });
  }

  const finalize = (node: MutableNode): FolderNode => ({
    name: node.name,
    key: node.key,
    pages: [...node.pages].sort((left, right) => left.path.localeCompare(right.path)),
    folders: Array.from(node.folders.values())
      .map(finalize)
      .sort((left, right) => left.name.localeCompare(right.name)),
  });

  return finalize(root);
}

function resolveWikiLink(currentPath: string, href: string): { type: "internal"; path: string } | { type: "external" } {
  const trimmed = href.trim();
  if (!trimmed) return { type: "external" };
  if (trimmed.startsWith("#")) return { type: "external" };
  if (/^[a-z]+:\/\//i.test(trimmed) || trimmed.startsWith("mailto:")) return { type: "external" };

  const [withoutHash] = trimmed.split("#");
  const [withoutQuery] = withoutHash.split("?");
  if (!withoutQuery.endsWith(".md")) return { type: "external" };

  const normalized = normalizeWikiPath(
    withoutQuery.startsWith("/")
      ? withoutQuery.slice(1)
      : `${dirname(currentPath)}/${withoutQuery}`,
  );
  return { type: "internal", path: normalized || "index.md" };
}

function stripFrontmatter(content: string): string {
  const trimmed = content.trim();
  if (!trimmed.startsWith("---\n")) return trimmed;
  const end = trimmed.indexOf("\n---", 4);
  if (end === -1) return trimmed;
  return trimmed.slice(end + 4).trim();
}

function normalizeWikiPath(path: string): string {
  const segments = path.split("/");
  const stack: string[] = [];
  for (const segment of segments) {
    const current = segment.trim();
    if (!current || current === ".") continue;
    if (current === "..") {
      stack.pop();
      continue;
    }
    stack.push(current);
  }
  return stack.join("/");
}

function dirname(path: string): string {
  const parts = path.split("/").filter(Boolean);
  parts.pop();
  return parts.join("/");
}

function truncate(value: string, limit: number): string {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= limit) return normalized;
  return `${normalized.slice(0, limit - 1).trimEnd()}…`;
}

function humanize(value: string): string {
  return value.replace(/[_-]+/g, " ").replace(/\b\w/g, (char) => char.toUpperCase());
}

function relativeTime(value?: string): string {
  if (!value) return "just now";
  const diff = Math.max(0, Math.round((Date.now() - +new Date(value)) / 60000));
  if (diff < 1) return "just now";
  if (diff < 60) return `${diff}m ago`;
  const hours = Math.round(diff / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}
