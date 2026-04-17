import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { apiClient } from "@/api/client";
import type { ProjectDetailResponse, RunRecord, RunStatus, WikiPageMeta } from "@/api/types";

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

type RunColumnKey = "queued" | "working" | "waiting" | "scheduled" | "completed" | "stopped";

const runColumns: Array<{ key: RunColumnKey; title: string }> = [
  { key: "queued", title: "Queued" },
  { key: "working", title: "Working" },
  { key: "waiting", title: "Waiting" },
  { key: "scheduled", title: "Scheduled" },
  { key: "completed", title: "Completed" },
  { key: "stopped", title: "Stopped" },
];

const runStatHelp = {
  active: "Queued or currently working runs, including planning, generating, evaluating, scheduling, wiki ingesting, and reporting.",
  waiting: "Runs paused until user input, approval, or missing context is provided.",
  scheduled: "Future follow-up runs that have been queued by schedule.",
  completed: "Runs that finished successfully.",
  stopped: "Runs that failed, exhausted retry attempts, or were cancelled.",
  wikiPages: "Markdown wiki pages tracked for this project.",
} as const;

const wikiPageTypeOptions = ["all", ...pageTypeOrder] as const;
type WikiPageTypeFilter = (typeof wikiPageTypeOptions)[number];

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
                  <dt title={runStatHelp.wikiPages}>Wiki Pages</dt>
                  <dd>{project.wiki_page_count}</dd>
                </div>
                <div>
                  <dt title={runStatHelp.active}>Active</dt>
                  <dd>{detail?.stats.active_runs ?? "—"}</dd>
                </div>
                <div>
                  <dt title={runStatHelp.waiting}>Waiting</dt>
                  <dd>{detail?.stats.waiting_runs ?? "—"}</dd>
                </div>
                <div>
                  <dt title={runStatHelp.completed}>Completed</dt>
                  <dd>{detail?.stats.completed_runs ?? "—"}</dd>
                </div>
                <div>
                  <dt title={runStatHelp.stopped}>Stopped</dt>
                  <dd>{detail?.stats.stopped_runs ?? "—"}</dd>
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
            <div><dt title={runStatHelp.active}>Active</dt><dd>{detailQuery.data.stats.active_runs}</dd></div>
            <div><dt title={runStatHelp.waiting}>Waiting</dt><dd>{detailQuery.data.stats.waiting_runs}</dd></div>
            <div><dt title={runStatHelp.scheduled}>Scheduled</dt><dd>{detailQuery.data.stats.scheduled_runs}</dd></div>
            <div><dt title={runStatHelp.completed}>Completed</dt><dd>{detailQuery.data.stats.completed_runs}</dd></div>
            <div><dt title={runStatHelp.stopped}>Stopped</dt><dd>{detailQuery.data.stats.stopped_runs}</dd></div>
            <div><dt title={runStatHelp.wikiPages}>Wiki Pages</dt><dd>{detailQuery.data.stats.wiki_page_count}</dd></div>
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

  const detailQuery = useQuery({
    queryKey: ["project-detail", slug],
    queryFn: () => apiClient.getProjectDetail(slug),
    staleTime: 20_000,
  });

  const pageQuery = useQuery({
    queryKey: ["wiki-page", slug, selectedPath],
    queryFn: () => apiClient.getWikiPage(slug, selectedPath),
    staleTime: 20_000,
  });

  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState<WikiPageTypeFilter>("all");
  const [statusFilter, setStatusFilter] = useState("all");

  const allPages = pagesQuery.data?.pages ?? [];

  const pageMetaByPath = useMemo(() => {
    const map = new Map<string, WikiPageMeta>();
    for (const page of allPages) map.set(page.path, page);
    return map;
  }, [allPages]);

  const selectedMeta = pageMetaByPath.get(selectedPath) ?? pageQuery.data?.meta;
  const wikiStatusOptions = useMemo(() => {
    const statuses = new Set<string>();
    for (const page of allPages) {
      if (page.status) statuses.add(page.status);
    }
    return ["all", ...Array.from(statuses).sort()];
  }, [allPages]);
  const filteredPages = useMemo(
    () => filterWikiPages(allPages, {
      query: searchQuery,
      pageType: typeFilter,
      status: statusFilter,
    }),
    [allPages, searchQuery, statusFilter, typeFilter],
  );
  const grouped = useMemo(() => groupPagesByType(filteredPages), [filteredPages]);
  const pageTypeCounts = useMemo(() => countBy(allPages, (page) => page.page_type || "overview"), [allPages]);
  const reportCount = pageTypeCounts.get("report") ?? 0;

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

        <section className="wiki-health-summary" aria-label="Project health summary">
          <div>
            <strong>{allPages.length}</strong>
            <span>Pages</span>
          </div>
          <div title={runStatHelp.active}>
            <strong>{detailQuery.data?.stats.active_runs ?? "—"}</strong>
            <span>Active</span>
          </div>
          <div title={runStatHelp.waiting}>
            <strong>{detailQuery.data?.stats.waiting_runs ?? "—"}</strong>
            <span>Waiting</span>
          </div>
          <div title={runStatHelp.stopped}>
            <strong>{detailQuery.data?.stats.stopped_runs ?? "—"}</strong>
            <span>Stopped</span>
          </div>
        </section>

        {reportCount > 0 && (
          <p className="wiki-report-note">
            {reportCount} run reports are available. Use filters to keep the navigation focused.
          </p>
        )}

        <div className="wiki-filters" aria-label="Wiki filters">
          <label>
            <span>Search</span>
            <input
              type="search"
              value={searchQuery}
              onChange={(event) => setSearchQuery(event.target.value)}
              placeholder="Title, path, status..."
            />
          </label>
          <div className="wiki-filter-row">
            <label>
              <span>Type</span>
              <select value={typeFilter} onChange={(event) => setTypeFilter(event.target.value as WikiPageTypeFilter)}>
                {wikiPageTypeOptions.map((option) => (
                  <option key={option} value={option}>
                    {option === "all" ? "All types" : `${humanize(option)} (${pageTypeCounts.get(option) ?? 0})`}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span>Status</span>
              <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
                {wikiStatusOptions.map((option) => (
                  <option key={option} value={option}>
                    {option === "all" ? "All statuses" : humanize(option)}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <p className="wiki-filter-count">
            Showing {filteredPages.length} of {allPages.length} pages
          </p>
        </div>

        {pagesQuery.isLoading && <p className="projects-note">Loading pages…</p>}
        {pagesQuery.isError && <p className="projects-note">Failed to load pages.</p>}

        <div className="wiki-tree">
          {grouped.map((group) => (
            <details key={group.pageType} open className="wiki-group">
              <summary>
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
          title={page.title}
        >
          <span>{compactWikiTitle(page)}</span>
          {page.status && page.status !== "active" && <small>{humanize(page.status)}</small>}
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
  const [statusFilter, setStatusFilter] = useState<RunColumnKey | "all">("all");
  const [dateRange, setDateRange] = useState<"24h" | "7d" | "30d" | "all">("7d");
  const [selectedRunID, setSelectedRunID] = useState("");

  useEffect(() => {
    setSelectedRunID("");
  }, [slug]);

  useEffect(() => {
    if (!selectedRunID) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setSelectedRunID("");
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [selectedRunID]);

  const runsQuery = useQuery({
    queryKey: ["project-runs-board", slug],
    queryFn: () => apiClient.listProjectRuns(slug, { page: 1, pageSize: 200, includeDetails: true }),
    staleTime: 5_000,
    refetchInterval: 5_000,
    refetchIntervalInBackground: true,
  });

  const scheduledQuery = useQuery({
    queryKey: ["scheduled-runs", "pending"],
    queryFn: () => apiClient.listScheduledRuns({ status: "pending" }),
    staleTime: 10_000,
    refetchInterval: 10_000,
    refetchIntervalInBackground: true,
  });

  const runs = runsQuery.data?.runs ?? [];
  const runRecords = runsQuery.data?.run_records ?? [];
  const recordByRunID = useMemo(() => {
    const map = new Map<string, RunRecord>();
    for (const record of runRecords) map.set(record.run.id, record);
    return map;
  }, [runRecords]);

  const projectByParentRunID = useMemo(() => {
    const map = new Map<string, string>();
    for (const run of runs) map.set(run.id, run.project.slug);
    for (const record of runRecords) map.set(record.run.id, record.run.project.slug);
    return map;
  }, [runRecords, runs]);

  const pendingScheduled = scheduledQuery.data?.scheduled_runs ?? [];
  const unknownScheduledParentRunIDs = useMemo(() => {
    const missing = new Set<string>();
    for (const scheduledRun of pendingScheduled) {
      if (projectByParentRunID.has(scheduledRun.parent_run_id)) continue;
      if (!scheduledRun.parent_run_id) continue;
      missing.add(scheduledRun.parent_run_id);
    }
    return Array.from(missing);
  }, [pendingScheduled, projectByParentRunID]);

  const parentRunQueries = useQueries({
    queries: unknownScheduledParentRunIDs.map((runID) => ({
      queryKey: ["run-record", runID],
      queryFn: () => apiClient.getRunRecord(runID),
      staleTime: 15_000,
      refetchInterval: 15_000,
      refetchIntervalInBackground: true,
    })),
  });

  const fallbackRecordByRunID = useMemo(() => {
    const map = new Map<string, RunRecord>();
    for (const query of parentRunQueries) {
      if (query.data) map.set(query.data.run.id, query.data);
    }
    return map;
  }, [parentRunQueries]);

  const pendingScheduledForProject = useMemo(() => {
    return pendingScheduled.filter((scheduledRun) => {
      const knownSlug = projectByParentRunID.get(scheduledRun.parent_run_id);
      if (knownSlug) return knownSlug === slug;
      const parent = fallbackRecordByRunID.get(scheduledRun.parent_run_id);
      return parent?.run.project.slug === slug;
    });
  }, [fallbackRecordByRunID, pendingScheduled, projectByParentRunID, slug]);

  const filteredRuns = useMemo(() => {
    return runs
      .filter((run) => matchesDateRange(run.updated_at || run.created_at, dateRange))
      .sort((left, right) => +new Date(right.updated_at || right.created_at) - +new Date(left.updated_at || left.created_at));
  }, [dateRange, runs]);

  const runsByColumn = useMemo(() => {
    const grouped: Record<Exclude<RunColumnKey, "scheduled">, typeof filteredRuns> = {
      queued: [],
      working: [],
      waiting: [],
      completed: [],
      stopped: [],
    };
    for (const run of filteredRuns) grouped[statusToColumn(run.status)].push(run);
    return grouped;
  }, [filteredRuns]);

  const filteredScheduled = useMemo(() => {
    return pendingScheduledForProject
      .filter((scheduledRun) => matchesDateRange(scheduledRun.scheduled_for || scheduledRun.created_at, dateRange))
      .sort((left, right) => +new Date(left.scheduled_for) - +new Date(right.scheduled_for));
  }, [dateRange, pendingScheduledForProject]);

  const selectedRunFromBoard = selectedRunID
    ? (recordByRunID.get(selectedRunID) ?? fallbackRecordByRunID.get(selectedRunID))
    : null;
  const selectedRunQuery = useQuery({
    queryKey: ["run-record", selectedRunID],
    queryFn: () => apiClient.getRunRecord(selectedRunID),
    enabled: Boolean(selectedRunID) && !selectedRunFromBoard,
    staleTime: 10_000,
    refetchInterval: selectedRunID ? 5_000 : false,
    refetchIntervalInBackground: true,
  });
  const selectedRun = selectedRunFromBoard ?? selectedRunQuery.data ?? null;

  const visibleColumns = statusFilter === "all" ? runColumns : runColumns.filter((column) => column.key === statusFilter);
  const waitingForProjectLookups = parentRunQueries.some((query) => query.isLoading);

  return (
    <section className="runs-board-page">
      <header className="runs-board-header">
        <div>
          <p className="projects-kicker">Project Runs</p>
          <h2>{slug}</h2>
          <p className="project-overview-subtitle">Track live execution and inspect full run provenance without leaving the board.</p>
        </div>
        <div className="project-overview-links">
          <Link to="/projects/$slug" params={{ slug }}>Overview</Link>
          <Link to="/projects/$slug/wiki" params={{ slug }}>Wiki</Link>
        </div>
      </header>

      <div className="runs-filter-bar">
        <label>
          Status
          <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value as RunColumnKey | "all")}>
            <option value="all">All Columns</option>
            {runColumns.map((column) => (
              <option key={column.key} value={column.key}>{column.title}</option>
            ))}
          </select>
        </label>
        <label>
          Window
          <select value={dateRange} onChange={(event) => setDateRange(event.target.value as "24h" | "7d" | "30d" | "all")}>
            <option value="24h">Last 24h</option>
            <option value="7d">Last 7d</option>
            <option value="30d">Last 30d</option>
            <option value="all">All time</option>
          </select>
        </label>
        <button type="button" onClick={() => void runsQuery.refetch()} disabled={runsQuery.isFetching}>
          {runsQuery.isFetching ? "Refreshing…" : "Refresh"}
        </button>
      </div>

      {runsQuery.isError && <p className="projects-note">Failed to load runs: {(runsQuery.error as Error).message}</p>}
      {scheduledQuery.isError && <p className="projects-note">Failed to load scheduled runs: {(scheduledQuery.error as Error).message}</p>}
      {waitingForProjectLookups && <p className="projects-note">Resolving scheduled run project ownership…</p>}

      <div className="runs-board-grid">
        {visibleColumns.map((column) => {
          const isScheduled = column.key === "scheduled";
          const runsForColumn = isScheduled ? [] : runsByColumn[column.key];
          const count = isScheduled ? filteredScheduled.length : runsForColumn.length;
          return (
            <section key={column.key} className="runs-column" data-column={column.key}>
              <header className="runs-column-header">
                <h3>{column.title}</h3>
                <span>{count}</span>
              </header>

              <div className="runs-column-body">
                {isScheduled &&
                  filteredScheduled.map((scheduledRun) => {
                    const parentRecord = recordByRunID.get(scheduledRun.parent_run_id) ?? fallbackRecordByRunID.get(scheduledRun.parent_run_id);
                    return (
                      <button
                        key={scheduledRun.id}
                        type="button"
                        className="run-card run-card-scheduled"
                        onClick={() => parentRecord && setSelectedRunID(parentRecord.run.id)}
                        disabled={!parentRecord}
                      >
                        <p className="run-card-goal">{truncate(scheduledRun.user_request_raw || "Scheduled follow-up", 120)}</p>
                        <p className="run-card-summary">Scheduled for {formatDateTime(scheduledRun.scheduled_for)}</p>
                        <div className="run-card-meta">
                          <span className="status-pill" data-status="scheduling">Scheduled</span>
                          <span>{relativeTime(scheduledRun.created_at)}</span>
                        </div>
                      </button>
                    );
                  })}

                {!isScheduled &&
                  runsForColumn.map((run) => {
                    const record = recordByRunID.get(run.id);
                    const changedPages = extractChangedPages(record);
                    const outcome = runOutcomeSummary(record);
                    return (
                      <button key={run.id} type="button" className="run-card" onClick={() => setSelectedRunID(run.id)}>
                        <p className="run-card-goal">{truncate(run.task_spec.goal || run.user_request_raw, 120)}</p>
                        <p className="run-card-summary">{outcome}</p>
                        <div className="run-card-meta">
                          <span className="status-pill" data-status={run.status}>{statusLabel[run.status]}</span>
                          <span>{relativeTime(run.updated_at || run.created_at)}</span>
                          {run.waiting_for && <span className="run-pill waiting">Waiting</span>}
                          {typeof run.latest_evaluation?.score === "number" && <span className="run-pill">Score {run.latest_evaluation.score}</span>}
                          <span className="run-pill">{record?.artifacts.length ?? 0} artifacts</span>
                          {changedPages.length > 0 && <span className="run-pill">{changedPages.length} wiki pages</span>}
                        </div>
                      </button>
                    );
                  })}

                {count === 0 && <p className="runs-column-empty">No runs match this filter.</p>}
              </div>
            </section>
          );
        })}
      </div>

      {selectedRunID && (
        <div className="run-drawer-layer" role="presentation">
          <button type="button" className="run-drawer-backdrop" aria-label="Close run detail" onClick={() => setSelectedRunID("")} />
          <aside className="run-drawer" role="dialog" aria-modal="true" aria-label="Run details">
            <header className="run-drawer-header">
              <div>
                <p className="projects-kicker">Run Detail</p>
                <h3>{selectedRun?.run.id ?? selectedRunID}</h3>
              </div>
              <button type="button" className="run-drawer-close" onClick={() => setSelectedRunID("")}>Close</button>
            </header>

            {selectedRunQuery.isLoading && !selectedRun && <p className="projects-note">Loading run details…</p>}
            {selectedRunQuery.isError && !selectedRun && (
              <p className="projects-note">Failed to load run details: {(selectedRunQuery.error as Error).message}</p>
            )}

            {selectedRun && (
              <div className="run-drawer-content">
                <section className="run-detail-section">
                  <h4>{truncate(selectedRun.run.task_spec.goal || selectedRun.run.user_request_raw, 140)}</h4>
                  <div className="run-detail-meta">
                    <span className="status-pill" data-status={selectedRun.run.status}>{statusLabel[selectedRun.run.status]}</span>
                    <span>Phase: {humanize(selectedRun.run.phase)}</span>
                    <span>Created: {formatDateTime(selectedRun.run.created_at)}</span>
                    <span>Updated: {formatDateTime(selectedRun.run.updated_at)}</span>
                    {selectedRun.run.completed_at && <span>Completed: {formatDateTime(selectedRun.run.completed_at)}</span>}
                  </div>
                  <p className="run-detail-user-request">{selectedRun.run.user_request_raw}</p>
                  <p className="run-card-summary">{runOutcomeSummary(selectedRun)}</p>
                </section>

                <section className="run-detail-section">
                  <h5>Evaluation</h5>
                  {selectedRun.evaluations.length === 0 && <p className="projects-note">No evaluations recorded.</p>}
                  {selectedRun.evaluations.map((evaluation) => (
                    <article key={evaluation.id} className="run-detail-entry">
                      <p><strong>Score:</strong> {evaluation.score} ({evaluation.passed ? "passed" : "needs follow-up"})</p>
                      <p>{evaluation.summary}</p>
                      {evaluation.missing_requirements.length > 0 && (
                        <ul>
                          {evaluation.missing_requirements.map((item) => <li key={item}>{item}</li>)}
                        </ul>
                      )}
                    </article>
                  ))}
                </section>

                <section className="run-detail-section">
                  <h5>Artifacts</h5>
                  {selectedRun.artifacts.length === 0 && <p className="projects-note">No artifacts.</p>}
                  {selectedRun.artifacts.map((artifact) => (
                    <article key={artifact.id} className="run-detail-entry">
                      <p><strong>{artifact.title || artifact.kind}</strong> ({artifact.kind})</p>
                      {artifact.url && <a href={artifact.url} target="_blank" rel="noreferrer">Open artifact</a>}
                      {artifact.source_url && <a href={artifact.source_url} target="_blank" rel="noreferrer">Source URL</a>}
                    </article>
                  ))}
                </section>

                <section className="run-detail-section">
                  <h5>Wiki Ingest</h5>
                  {extractChangedPages(selectedRun).length === 0 && <p className="projects-note">No changed wiki pages captured.</p>}
                  {extractChangedPages(selectedRun).length > 0 && (
                    <ul>
                      {extractChangedPages(selectedRun).map((path) => <li key={path}>{path}</li>)}
                    </ul>
                  )}
                </section>

                <section className="run-detail-section">
                  <h5>Attempts</h5>
                  {selectedRun.attempts.length === 0 && <p className="projects-note">No attempts.</p>}
                  {selectedRun.attempts.map((attempt) => (
                    <article key={attempt.id} className="run-detail-entry">
                      <p><strong>{humanize(attempt.role)}</strong> · {formatDateTime(attempt.started_at)}</p>
                      <p>{attempt.output_summary || attempt.input_summary}</p>
                    </article>
                  ))}
                </section>

                <section className="run-detail-section">
                  <h5>Timeline</h5>
                  {selectedRun.events.length === 0 && <p className="projects-note">No timeline events.</p>}
                  {selectedRun.events.map((event) => (
                    <article key={event.id} className="run-detail-entry">
                      <p><strong>{humanize(event.type)}</strong> · {formatDateTime(event.created_at)}</p>
                      <p>{event.summary}</p>
                    </article>
                  ))}
                </section>

                <section className="run-detail-section">
                  <h5>Evidence</h5>
                  {selectedRun.evidence.length === 0 && <p className="projects-note">No evidence records.</p>}
                  {selectedRun.evidence.map((evidence) => (
                    <article key={evidence.id} className="run-detail-entry">
                      <p><strong>{humanize(evidence.kind)}</strong></p>
                      <p>{evidence.summary}</p>
                    </article>
                  ))}
                </section>

                <section className="run-detail-section">
                  <h5>Tool Calls</h5>
                  {selectedRun.tool_calls.length === 0 && <p className="projects-note">No tool calls.</p>}
                  {selectedRun.tool_calls.map((toolCall) => (
                    <article key={toolCall.id} className="run-detail-entry">
                      <p><strong>{toolCall.tool_name}</strong></p>
                      <p>{toolCall.output_summary || toolCall.input_summary}</p>
                    </article>
                  ))}
                </section>

                <section className="run-detail-section">
                  <h5>Web Steps</h5>
                  {selectedRun.web_steps.length === 0 && <p className="projects-note">No web automation steps.</p>}
                  {selectedRun.web_steps.map((step) => (
                    <article key={step.id} className="run-detail-entry">
                      <p><strong>{step.title}</strong> · {formatDateTime(step.occurred_at)}</p>
                      <p>{step.summary}</p>
                    </article>
                  ))}
                </section>

                <section className="run-detail-section">
                  <h5>Wait Requests</h5>
                  {selectedRun.wait_requests.length === 0 && <p className="projects-note">No wait requests.</p>}
                  {selectedRun.wait_requests.map((request) => (
                    <article key={request.id} className="run-detail-entry">
                      <p><strong>{request.title || humanize(request.kind)}</strong></p>
                      <p>{request.prompt}</p>
                    </article>
                  ))}
                </section>

                <section className="run-detail-section">
                  <h5>Scheduled Follow-ups</h5>
                  {selectedRun.scheduled_runs.length === 0 && <p className="projects-note">No scheduled follow-ups.</p>}
                  {selectedRun.scheduled_runs.map((scheduledRun) => (
                    <article key={scheduledRun.id} className="run-detail-entry">
                      <p><strong>{scheduledRun.user_request_raw}</strong></p>
                      <p>Status: {scheduledRun.status} · {formatDateTime(scheduledRun.scheduled_for)}</p>
                    </article>
                  ))}
                </section>

                <section className="run-detail-section">
                  <h5>Raw Events</h5>
                  <pre className="run-detail-raw">{JSON.stringify(selectedRun.events, null, 2)}</pre>
                </section>
              </div>
            )}
          </aside>
        </div>
      )}
    </section>
  );
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

function filterWikiPages(
  pages: WikiPageMeta[],
  filters: { query: string; pageType: WikiPageTypeFilter; status: string },
): WikiPageMeta[] {
  const query = filters.query.trim().toLowerCase();
  return pages.filter((page) => {
    if (filters.pageType !== "all" && (page.page_type || "overview") !== filters.pageType) return false;
    if (filters.status !== "all" && page.status !== filters.status) return false;
    if (!query) return true;

    const searchable = [
      page.title,
      page.path,
      page.page_type,
      page.status,
      page.confidence,
      ...(page.source_refs ?? []),
      ...(page.related ?? []),
    ].join(" ").toLowerCase();
    return searchable.includes(query);
  });
}

function countBy<T>(items: T[], keyFor: (item: T) => string): Map<string, number> {
  const counts = new Map<string, number>();
  for (const item of items) {
    const key = keyFor(item);
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  return counts;
}

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

function compactWikiTitle(page: WikiPageMeta): string {
  const title = page.title || page.path;
  if (page.page_type === "report") {
    const match = page.path.match(/run_(\d{8}T\d{6}Z)_([a-z0-9]+)/);
    if (match) return `Run ${formatRunTimestamp(match[1])} ${match[2].slice(0, 8)}`;
  }
  return truncate(title, page.page_type === "topic" ? 92 : 72);
}

function formatRunTimestamp(value: string): string {
  const match = value.match(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z$/);
  if (!match) return value;
  return `${match[1]}-${match[2]}-${match[3]} ${match[4]}:${match[5]}Z`;
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

function statusToColumn(status: RunStatus): Exclude<RunColumnKey, "scheduled"> {
  if (status === "queued") return "queued";
  if (status === "waiting") return "waiting";
  if (status === "completed") return "completed";
  if (status === "failed" || status === "exhausted" || status === "cancelled") return "stopped";
  return "working";
}

function matchesDateRange(value: string | undefined, range: "24h" | "7d" | "30d" | "all"): boolean {
  if (range === "all") return true;
  if (!value) return false;
  const timestamp = +new Date(value);
  if (!Number.isFinite(timestamp)) return false;
  const windowMs =
    range === "24h"
      ? 24 * 60 * 60 * 1000
      : range === "7d"
        ? 7 * 24 * 60 * 60 * 1000
        : 30 * 24 * 60 * 60 * 1000;
  return Date.now() - timestamp <= windowMs;
}

function extractChangedPages(record: RunRecord | undefined): string[] {
  if (!record) return [];
  const changed = new Set<string>();
  for (const event of record.events) {
    const raw = event.data?.changed_pages;
    if (Array.isArray(raw)) {
      for (const value of raw) {
        if (typeof value === "string" && value.trim()) changed.add(value.trim());
      }
    }
  }
  return Array.from(changed).sort((left, right) => left.localeCompare(right));
}

function runOutcomeSummary(record: RunRecord | undefined): string {
  if (!record) return "Run details are still loading.";
  const latestEvaluation = [...record.evaluations].sort(
    (left, right) => +new Date(right.created_at) - +new Date(left.created_at),
  )[0];
  if (latestEvaluation?.summary) return truncate(latestEvaluation.summary, 140);
  const latestEvent = [...record.events].sort(
    (left, right) => +new Date(right.created_at) - +new Date(left.created_at),
  )[0];
  if (latestEvent?.summary) return truncate(latestEvent.summary, 140);
  return "No outcome summary recorded yet.";
}

function formatDateTime(value: string | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(+date)) return "—";
  return date.toLocaleString();
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
