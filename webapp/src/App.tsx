import { useQuery } from "@tanstack/react-query";
import { Link, Outlet } from "@tanstack/react-router";
import { apiClient } from "@/api/client";

export function AppShell() {
  const { data: bootstrap } = useQuery({
    queryKey: ["bootstrap"],
    queryFn: apiClient.bootstrap,
    staleTime: 30_000,
  });

  return (
    <div className="app-shell">
      <main className="chat-main">
        <header className="app-header">
          <Link to="/" className="app-brand" aria-label="Go to home">
            <img className="app-brand-logo" src="/logo.svg" alt="" aria-hidden="true" />
            <span className="app-brand-title">{bootstrap?.product_name ?? "Codex"}</span>
          </Link>
        </header>
        <div className="app-content">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
