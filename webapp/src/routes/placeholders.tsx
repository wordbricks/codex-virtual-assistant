import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { apiClient } from "@/api/client";
import { LegacyChatPage } from "@/legacy/LegacyChatPage";

export function ProjectsHomePlaceholder() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["projects"],
    queryFn: apiClient.listProjects,
    staleTime: 30_000,
  });

  return (
    <section className="chat-stage" style={{ padding: "2rem" }}>
      <header className="chat-header">
        <h2 className="chat-title">Projects</h2>
      </header>

      {isLoading && <p>Loading projects…</p>}
      {isError && <p>Failed to load projects: {(error as Error).message}</p>}

      {!isLoading && !isError && (
        <ul style={{ display: "grid", gap: "0.75rem", marginTop: "1rem" }}>
          {(data?.projects ?? []).map((project) => (
            <li key={project.slug} style={{ listStyle: "none" }}>
              <Link to="/projects/$slug" params={{ slug: project.slug }}>
                {project.name} <span style={{ opacity: 0.7 }}>({project.slug})</span>
              </Link>
            </li>
          ))}
          {(data?.projects ?? []).length === 0 && <li style={{ listStyle: "none" }}>No projects found.</li>}
        </ul>
      )}
    </section>
  );
}

export function ProjectOverviewPlaceholder() {
  const { slug } = useParams({ from: "/projects/$slug" });

  return (
    <section className="chat-stage" style={{ padding: "2rem" }}>
      <header className="chat-header">
        <h2 className="chat-title">Project: {slug}</h2>
      </header>
      <p>Project overview route scaffold is ready for Milestone 5.</p>
      <p>
        <Link to="/projects/$slug/wiki" params={{ slug }}>Open wiki</Link>
        {" · "}
        <Link to="/projects/$slug/runs" params={{ slug }}>Open runs</Link>
      </p>
    </section>
  );
}

export function ProjectWikiPlaceholder() {
  const { slug } = useParams({ from: "/projects/$slug/wiki" });

  return (
    <section className="chat-stage" style={{ padding: "2rem" }}>
      <header className="chat-header">
        <h2 className="chat-title">Wiki: {slug}</h2>
      </header>
      <p>Wiki route scaffold is ready for Milestone 5.</p>
    </section>
  );
}

export function ProjectWikiPagePlaceholder() {
  const { slug, _splat } = useParams({ from: "/projects/$slug/wiki/$" });

  return (
    <section className="chat-stage" style={{ padding: "2rem" }}>
      <header className="chat-header">
        <h2 className="chat-title">Wiki Page: {slug}</h2>
      </header>
      <p>Requested path: {_splat || "(root)"}</p>
    </section>
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
