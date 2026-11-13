import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createToken, listTokens, revokeToken, type AccessToken, type TokenScope } from "../../api.ts";
import { shortDate } from "../../display.ts";
import { Checkbox, CopyButton, MONO, Row, Section, SectionError, SectionLoading } from "./primitives.tsx";

const SUBTITLE =
  "Tokens let programs act as you without a browser: mcp for MCP clients at /mcp, api for the REST API. " +
  "A token is shown once, when it is created.";

const EXPIRY: { days: number; label: string }[] = [
  { days: 30, label: "30 days" },
  { days: 90, label: "90 days" },
  { days: 365, label: "1 year" },
  { days: 0, label: "No expiry" },
];

/** A Go zero time stands for "never" in the token fields. */
function isZero(iso: string): boolean {
  return !iso || iso.startsWith("0001-01-01");
}

function describe(t: AccessToken): string {
  const parts = [`Created ${shortDate(t.created_at)}`];
  parts.push(isZero(t.last_used_at) ? "never used" : `last used ${shortDate(t.last_used_at)}`);
  if (t.expired) parts.push(`expired ${shortDate(t.expires_at)}`);
  else parts.push(isZero(t.expires_at) ? "no expiry" : `expires ${shortDate(t.expires_at)}`);
  return parts.join(" · ");
}

/** The signed-in user's personal access tokens: list, create, revoke. */
export default function TokensSection() {
  const queryClient = useQueryClient();
  const tokens = useQuery({ queryKey: ["account", "tokens"], queryFn: listTokens, retry: false });
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["account", "tokens"] });
  // A token revoked here no longer shows its secret below the form.
  const [revoked, setRevoked] = useState<number[]>([]);
  const revoke = useMutation({
    mutationFn: (id: number) => revokeToken(id),
    onSuccess: (_, id) => {
      setRevoked((r) => [...r, id]);
      refresh();
    },
  });

  if (tokens.error) {
    return (
      <Section title="Access tokens" subtitle={SUBTITLE}>
        <SectionError error={tokens.error as Error} />
      </Section>
    );
  }
  if (!tokens.data) {
    return (
      <Section title="Access tokens" subtitle={SUBTITLE}>
        <SectionLoading />
      </Section>
    );
  }

  return (
    <Section title="Access tokens" subtitle={SUBTITLE}>
      {tokens.data.tokens.length === 0 ? (
        <Row label="Tokens" value={<span style={{ color: "var(--color-neutral-600)" }}>None</span>} />
      ) : (
        tokens.data.tokens.map((t) => (
          <Row key={t.id} label={t.name}>
            <div style={{ display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap" }}>
              {t.scopes.map((s) => (
                <span key={s} className="tag tag-neutral">{s}</span>
              ))}
              <span style={{ fontSize: 13, color: t.expired ? "var(--color-accent-700)" : "var(--color-text-2)" }}>
                {describe(t)}
              </span>
              <button
                type="button"
                className="btn btn-danger"
                disabled={revoke.isPending}
                onClick={() => revoke.mutate(t.id)}
              >
                Revoke
              </button>
            </div>
          </Row>
        ))
      )}
      {revoke.error ? <p className="field-error">{(revoke.error as Error).message}</p> : null}
      <NewToken onCreated={refresh} revoked={revoked} />
    </Section>
  );
}

function NewToken({ onCreated, revoked }: { onCreated: () => void; revoked: number[] }) {
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<TokenScope[]>(["mcp"]);
  const [days, setDays] = useState(90);
  const create = useMutation({
    mutationFn: () => createToken(name.trim(), scopes, days),
    onSuccess: () => {
      setName("");
      onCreated();
    },
  });

  const toggle = (s: TokenScope, on: boolean) =>
    setScopes((cur) => (on ? [...cur.filter((x) => x !== s), s] : cur.filter((x) => x !== s)));

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (name.trim() && scopes.length > 0) create.mutate();
  };

  return (
    <form onSubmit={submit}>
      <Row label="New token">
        <div style={{ display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap" }}>
          <input
            className="input"
            placeholder="Name, e.g. laptop MCP"
            aria-label="Token name"
            maxLength={128}
            value={name}
            onChange={(e) => setName(e.target.value)}
            style={{ maxWidth: 240 }}
          />
          {(["mcp", "api"] as TokenScope[]).map((s) => (
            <label key={s} style={{ display: "inline-flex", gap: 6, alignItems: "center", fontSize: 13 }}>
              <Checkbox checked={scopes.includes(s)} onChange={(on) => toggle(s, on)} label={`Scope ${s}`} />
              {s}
            </label>
          ))}
          <select
            className="select"
            style={{ width: "auto" }}
            aria-label="Expiry"
            value={days}
            onChange={(e) => setDays(parseInt(e.target.value, 10))}
          >
            {EXPIRY.map((x) => (
              <option key={x.days} value={x.days}>{x.label}</option>
            ))}
          </select>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={create.isPending || !name.trim() || scopes.length === 0}
          >
            {create.isPending ? "Creating…" : "Create token"}
          </button>
        </div>
        {create.error ? <p className="field-error">{(create.error as Error).message}</p> : null}
      </Row>
      {create.data && !revoked.includes(create.data.id) ? (
        <Row label="Copy it now">
          <div style={{ display: "flex", gap: 10, alignItems: "center", minWidth: 0 }}>
            <code style={{ ...MONO, minWidth: 0 }} data-testid="new-token">{create.data.token}</code>
            <CopyButton text={create.data.token} />
          </div>
          <p className="field-hint">
            “{create.data.name}” is shown only this once. Send it as <span className="mono">Authorization: Bearer …</span>
          </p>
        </Row>
      ) : null}
    </form>
  );
}
