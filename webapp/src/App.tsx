import { useQuery } from "@tanstack/react-query";
import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import { apiClient } from "@/api/client";

const navigationLinks = [
  { to: "/", label: "Projects" },
] as const;

export function AppShell() {
  const { data: bootstrap } = useQuery({
    queryKey: ["bootstrap"],
    queryFn: apiClient.bootstrap,
    staleTime: 30_000,
  });

  const pathname = useRouterState({ select: (state) => state.location.pathname });

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <span className="sidebar-brand-icon">C</span>
          <div>
            <p className="sidebar-brand-label">Workspace</p>
            <h1>{bootstrap?.product_name ?? "Codex"}</h1>
          </div>
        </div>

        <nav className="sidebar-history" aria-label="Primary">
          {navigationLinks.map((link) => (
            <Link
              key={link.to}
              to={link.to}
              className={`sidebar-item${pathname === link.to ? " is-active" : ""}`}
            >
              <span className="sidebar-item-title">{link.label}</span>
            </Link>
          ))}
        </nav>
      </aside>

      <main className="chat-main">
        <Outlet />
      </main>
    </div>
  );
}
