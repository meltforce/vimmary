import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  changePassword,
  fetchAccount,
  fetchLoginMethods,
  revokeSession,
  unlinkIdentity,
  type AuthSession,
} from "../../api.ts";
import { shortDate, clock } from "../../display.ts";
import { useAuth } from "../../features.ts";
import { useSignOut } from "../../hooks/useSignOut.ts";
import { Row, Section, SectionError, SectionLoading } from "./primitives.tsx";
import TokensSection from "./TokensSection.tsx";

/** Browser and system from a user agent, enough to tell sessions apart. */
function describeAgent(ua: string): string {
  if (!ua) return "Unknown client";
  const browser = /Edg\//.test(ua)
    ? "Edge"
    : /Firefox\//.test(ua)
      ? "Firefox"
      : /Chrome\//.test(ua)
        ? "Chrome"
        : /Safari\//.test(ua)
          ? "Safari"
          : ua.split(/[ /]/)[0];
  const os = /iPhone|iPad/.test(ua)
    ? "iOS"
    : /Android/.test(ua)
      ? "Android"
      : /Mac OS X/.test(ua)
        ? "macOS"
        : /Windows/.test(ua)
          ? "Windows"
          : /Linux/.test(ua)
            ? "Linux"
            : "";
  return os ? `${browser} on ${os}` : browser;
}

function when(iso: string): string {
  return `${shortDate(iso)}, ${clock(iso)}`;
}

/**
 * The signed-in user's own logins: password, linked OIDC identities and the
 * sessions of every browser. Present only when application login is
 * configured.
 */
export default function AccountSection() {
  const queryClient = useQueryClient();
  const { method } = useAuth();
  const account = useQuery({ queryKey: ["account"], queryFn: fetchAccount, retry: false });
  const methods = useQuery({ queryKey: ["auth", "methods"], queryFn: fetchLoginMethods, retry: false });
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["account"] });

  const signOut = useSignOut();
  const revoke = useMutation({
    mutationFn: (s: AuthSession) => revokeSession(s.id),
    onSuccess: (_, s) => (s.current ? window.location.assign("/login?signed_out=1") : refresh()),
  });
  const unlink = useMutation({
    mutationFn: ({ issuer, subject }: { issuer: string; subject: string }) => unlinkIdentity(issuer, subject),
    onSuccess: refresh,
  });

  if (account.error) {
    return (
      <Section title="Account" subtitle="How you sign in to vimmary.">
        <SectionError error={account.error as Error} />
      </Section>
    );
  }
  if (!account.data) {
    return (
      <Section title="Account" subtitle="How you sign in to vimmary.">
        <SectionLoading />
      </Section>
    );
  }

  const acct = account.data;
  const oidcName = methods.data?.find((m) => m.type === "oidc")?.display_name ?? "single sign-on";
  const hasOIDC = acct.methods.includes("oidc");
  const hasPassword = acct.methods.includes("password");

  return (
    <>
      <Section
        title="Account"
        subtitle={
          method === "tailscale"
            ? "This browser has no session: vimmary recognised you through Tailscale. Signing out ends nothing here."
            : "How you sign in to vimmary. Signing out ends the session of this browser."
        }
      >
        <Row label="Login" value={acct.login} />
        <Row label="Signed in by" value={method === "oidc" ? oidcName : method === "password" ? "Password" : "Tailscale"} />
        {method !== "tailscale" ? (
          <Row label="Session">
            <button
              type="button"
              className="btn btn-secondary"
              disabled={signOut.isPending}
              onClick={() => signOut.mutate()}
            >
              {signOut.isPending ? "Signing out…" : "Sign out"}
            </button>
            {signOut.error ? <p className="field-error">{(signOut.error as Error).message}</p> : null}
          </Row>
        ) : null}
      </Section>

      {hasPassword ? (
        <Section
          title="Password"
          subtitle={
            acct.password_username
              ? `You sign in with the username ${acct.password_username}. A new password needs at least 12 characters.`
              : "This account has no password. An admin can set one under Users."
          }
        >
          {acct.password_username ? <PasswordChange /> : null}
        </Section>
      ) : null}

      {hasOIDC ? (
        <Section
          title={`Linked ${oidcName} logins`}
          subtitle={`Each login listed here signs you in to this account. Linking takes you to ${oidcName} and back.`}
        >
          {acct.identities.length === 0 ? (
            <Row label="Linked" value={<span style={{ color: "var(--color-neutral-600)" }}>None</span>} />
          ) : (
            acct.identities.map((id) => (
              <Row key={`${id.issuer} ${id.subject}`} label={id.email || id.subject}>
                <div style={{ display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap" }}>
                  <span style={{ fontSize: 13, color: "var(--color-text-2)" }}>
                    Linked {shortDate(id.created_at)} · <span className="mono">{new URL(id.issuer).host}</span>
                  </span>
                  <button
                    type="button"
                    className="btn btn-danger"
                    disabled={!id.can_unlink || unlink.isPending}
                    title={id.can_unlink ? undefined : "This is your only way to sign in."}
                    onClick={() => unlink.mutate({ issuer: id.issuer, subject: id.subject })}
                  >
                    Unlink
                  </button>
                </div>
              </Row>
            ))
          )}
          {unlink.error ? <p className="field-error">{(unlink.error as Error).message}</p> : null}
          <Row label="Add">
            {/* A form POST rather than fetch: the answer is a redirect to the
                provider, which only a navigation follows. */}
            <form method="post" action="/account/link-oidc">
              <button type="submit" className="btn btn-secondary">
                Link a {oidcName} login
              </button>
            </form>
          </Row>
        </Section>
      ) : null}

      <Section title="Sessions" subtitle="Every browser signed in to this account. Revoking one signs it out.">
        {acct.sessions.length === 0 ? (
          <Row label="Active" value={<span style={{ color: "var(--color-neutral-600)" }}>None</span>} />
        ) : (
          acct.sessions.map((s) => (
            <Row key={s.id} label={describeAgent(s.user_agent)}>
              <div style={{ display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap" }}>
                <span style={{ fontSize: 13, color: "var(--color-text-2)" }}>
                  {s.current ? <span className="tag tag-accent" style={{ marginRight: 8 }}>This browser</span> : null}
                  Last used {when(s.last_used_at)} · <span className="mono">{s.client_ip}</span>
                </span>
                <button
                  type="button"
                  className="btn btn-danger"
                  disabled={revoke.isPending}
                  onClick={() => revoke.mutate(s)}
                >
                  {s.current ? "Sign out" : "Revoke"}
                </button>
              </div>
            </Row>
          ))
        )}
        {revoke.error ? <p className="field-error">{(revoke.error as Error).message}</p> : null}
      </Section>

      <TokensSection />
    </>
  );
}

function PasswordChange() {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [repeat, setRepeat] = useState("");
  const change = useMutation({
    mutationFn: () => changePassword(current, next),
    onSuccess: () => {
      setCurrent("");
      setNext("");
      setRepeat("");
    },
  });
  const mismatch = repeat !== "" && repeat !== next;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!mismatch) change.mutate();
  };

  return (
    <form onSubmit={submit}>
      <Row label="Current">
        <input
          className="input"
          type="password"
          autoComplete="current-password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          style={{ maxWidth: 320 }}
          required
        />
      </Row>
      <Row label="New">
        <input
          className="input"
          type="password"
          autoComplete="new-password"
          minLength={12}
          value={next}
          onChange={(e) => setNext(e.target.value)}
          style={{ maxWidth: 320 }}
          required
        />
      </Row>
      <Row label="Repeat">
        <input
          className="input"
          type="password"
          autoComplete="new-password"
          value={repeat}
          onChange={(e) => setRepeat(e.target.value)}
          style={{ maxWidth: 320 }}
          required
        />
        {mismatch ? <p className="field-error">The passwords do not match.</p> : null}
      </Row>
      <Row label="">
        <button
          type="submit"
          className="btn btn-primary"
          disabled={change.isPending || !current || !next || mismatch}
        >
          {change.isPending ? "Saving…" : "Change password"}
        </button>
        {change.error ? <p className="field-error">{(change.error as Error).message}</p> : null}
        {change.isSuccess ? <p className="field-hint">Password changed.</p> : null}
      </Row>
    </form>
  );
}
