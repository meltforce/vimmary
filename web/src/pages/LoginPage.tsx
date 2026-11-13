import { useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import PageHeader from "../components/PageHeader.tsx";
import { ApiError, fetchLoginMethods, fetchSession, login } from "../api.ts";

/** The `error` codes pkg/auth puts on the redirect back to this page. */
const ERRORS: Record<string, string> = {
  invalid_state: "The sign-in took too long or was started in another tab. Try again.",
  oidc_unavailable: "The sign-in provider cannot be reached right now.",
  oidc_denied: "The sign-in provider refused the request.",
  oidc_failed: "The sign-in provider's answer could not be verified. Try again.",
  no_account: "There is no vimmary account for this login, or it is disabled. Ask the admin for access.",
  identity_in_use: "That login is already linked to another vimmary account.",
  login_failed: "Signing in failed. Try again.",
};

/** Only a local path is a valid return target, never another origin. */
function safeNext(next: string | null): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/login")) return "/";
  return next;
}

/**
 * The login page renders outside Layout: the nav would fetch the inbox count,
 * which answers 401 without a session.
 */
export default function LoginPage() {
  const [params] = useSearchParams();
  const next = safeNext(params.get("next"));
  const errorCode = params.get("error");

  const methods = useQuery({ queryKey: ["auth", "methods"], queryFn: fetchLoginMethods, retry: false });
  const session = useQuery({ queryKey: ["auth", "me"], queryFn: fetchSession, retry: false });

  return (
    <div className="min-h-screen flex flex-col">
      <nav className="nav">
        <a href="/" className="nav-brand">
          vimmary
        </a>
      </nav>
      <main className="flex-1 flex flex-col">
        <PageHeader kicker="vimmary" title="Sign in" />
        <div className="page-x" style={{ width: "100%", paddingBottom: 40 }}>
          <div className="card" style={{ maxWidth: 440, padding: "24px 28px" }}>
            {errorCode ? (
              <p className="field-error" role="alert" style={{ margin: "0 0 18px", fontSize: 13 }}>
                {ERRORS[errorCode] ?? `Signing in failed (${errorCode}).`}
              </p>
            ) : null}
            {session.data ? (
              <SignedIn login={session.data.login} next={next} />
            ) : methods.isPending ? (
              <p className="field-hint">Loading…</p>
            ) : methods.error ? (
              <p className="field-error">{(methods.error as Error).message}</p>
            ) : methods.data === null ? (
              <NotConfigured />
            ) : (
              <Methods methods={methods.data} next={next} />
            )}
          </div>
        </div>
      </main>
    </div>
  );
}

function SignedIn({ login, next }: { login: string; next: string }) {
  return (
    <>
      <p style={{ fontSize: 14, margin: "0 0 16px" }}>
        Signed in as <strong>{login}</strong>.
      </p>
      <a className="btn btn-primary" href={next}>
        Continue
      </a>
    </>
  );
}

function NotConfigured() {
  return (
    <>
      <p style={{ fontSize: 14, margin: "0 0 16px" }}>
        This instance has no login. It identifies you through Tailscale.
      </p>
      <a className="btn btn-primary" href="/">
        Open vimmary
      </a>
    </>
  );
}

function Methods({ methods, next }: { methods: Awaited<ReturnType<typeof fetchLoginMethods>> & object; next: string }) {
  const oidc = methods.filter((m) => m.type === "oidc");
  const password = methods.some((m) => m.type === "password");
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 18 }}>
      {oidc.map((m) => (
        <a
          key={m.start_url}
          className="btn btn-primary btn-block btn-center"
          href={`${m.start_url}?next=${encodeURIComponent(next)}`}
        >
          Sign in with {m.display_name}
        </a>
      ))}
      {oidc.length > 0 && password ? <div className="hr" aria-hidden="true" /> : null}
      {password ? <PasswordForm next={next} primary={oidc.length === 0} /> : null}
    </div>
  );
}

function PasswordForm({ next, primary }: { next: string; primary: boolean }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await login(username, password);
      // A full load, so every query starts over with the new session.
      window.location.assign(next);
    } catch (err) {
      const status = err instanceof ApiError ? err.status : 0;
      setError(
        status === 401
          ? "Wrong username or password."
          : status === 403
            ? "This account is disabled or has no access."
            : status === 429
              ? "Too many failed attempts. Wait a few minutes and try again."
              : (err as Error).message,
      );
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      <div className="field">
        <label htmlFor="login-username">Username</label>
        <input
          id="login-username"
          className="input"
          autoComplete="username"
          autoCapitalize="none"
          spellCheck={false}
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          required
          autoFocus
        />
      </div>
      <div className="field">
        <label htmlFor="login-password">Password</label>
        <input
          id="login-password"
          className="input"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
        />
        {error ? (
          <p className="field-error" role="alert">
            {error}
          </p>
        ) : null}
      </div>
      <button
        type="submit"
        className={`btn ${primary ? "btn-primary" : "btn-secondary"} btn-block btn-center`}
        disabled={busy || !username || !password}
      >
        {busy ? "Signing in…" : "Sign in"}
      </button>
    </form>
  );
}
