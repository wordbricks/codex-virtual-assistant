import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet } from "@tanstack/react-router";
import { LogOut, ShieldCheck } from "lucide-react";
import { apiClient } from "@/api/client";

export function AppShell() {
  const queryClient = useQueryClient();
  const authQuery = useQuery({
    queryKey: ["auth-status"],
    queryFn: apiClient.authStatus,
    retry: false,
    staleTime: 30_000,
  });
  const { data: bootstrap } = useQuery({
    queryKey: ["bootstrap"],
    queryFn: apiClient.bootstrap,
    staleTime: 30_000,
  });

  const logoutMutation = useMutation({
    mutationFn: apiClient.logout,
    onSuccess: (status) => {
      queryClient.setQueryData(["auth-status"], status);
      queryClient.removeQueries({ predicate: (query) => query.queryKey[0] !== "bootstrap" && query.queryKey[0] !== "auth-status" });
    },
  });

  const authReady = Boolean(authQuery.data) || authQuery.isError;
  const authEnabled = authQuery.data?.enabled ?? false;
  const authenticated = !authEnabled || authQuery.data?.authenticated;
  if (!authReady) {
    return (
      <div className="app-shell">
        <main className="chat-main">
          <header className="app-header">
            <div className="app-brand" aria-label="Codex Virtual Assistant">
              <img className="app-brand-logo" src="/logo.svg" alt="" aria-hidden="true" />
              <span className="app-brand-title">{bootstrap?.product_name ?? "Codex"}</span>
            </div>
          </header>
          <div className="auth-loading">Checking session</div>
        </main>
      </div>
    );
  }

  if (authEnabled && !authenticated) {
    return <LoginScreen productName={bootstrap?.product_name ?? "Codex Virtual Assistant"} />;
  }

  return (
    <div className="app-shell">
      <main className="chat-main">
        <header className="app-header">
          <Link to="/" className="app-brand" aria-label="Go to home">
            <img className="app-brand-logo" src="/logo.svg" alt="" aria-hidden="true" />
            <span className="app-brand-title">{bootstrap?.product_name ?? "Codex"}</span>
          </Link>
          {authEnabled && (
            <button
              className="app-header-action"
              type="button"
              onClick={() => logoutMutation.mutate()}
              disabled={logoutMutation.isPending}
              aria-label="Sign out"
            >
              <LogOut size={16} aria-hidden="true" />
              <span>Sign out</span>
            </button>
          )}
        </header>
        <div className="app-content">
          <Outlet />
        </div>
      </main>
    </div>
  );
}

function LoginScreen({ productName }: { productName: string }) {
  const queryClient = useQueryClient();
  const [userID, setUserID] = useState("");
  const [password, setPassword] = useState("");

  const loginMutation = useMutation({
    mutationFn: apiClient.login,
    onSuccess: (status) => {
      queryClient.setQueryData(["auth-status"], status);
      void queryClient.invalidateQueries();
    },
  });

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    loginMutation.mutate({ user_id: userID.trim(), password });
  };

  return (
    <div className="auth-screen">
      <form className="auth-panel" onSubmit={submit}>
        <div className="auth-mark" aria-hidden="true">
          <ShieldCheck size={22} />
        </div>
        <div>
          <p className="auth-kicker">{productName}</p>
          <h1 className="auth-title">Sign in</h1>
        </div>
        <label className="auth-field">
          <span>ID</span>
          <input
            autoComplete="username"
            autoFocus
            name="user_id"
            value={userID}
            onChange={(event) => setUserID(event.target.value)}
            required
          />
        </label>
        <label className="auth-field">
          <span>Password</span>
          <input
            autoComplete="current-password"
            name="password"
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            required
          />
        </label>
        {loginMutation.isError && <p className="auth-error">{(loginMutation.error as Error).message}</p>}
        <button className="auth-submit" type="submit" disabled={loginMutation.isPending}>
          {loginMutation.isPending ? "Signing in" : "Sign in"}
        </button>
      </form>
    </div>
  );
}
