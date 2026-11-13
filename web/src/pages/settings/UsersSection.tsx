import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createUser,
  deleteUser,
  fetchAccountPolicy,
  fetchLoginMethods,
  listUsers,
  saveAccountPolicy,
  setUserPassword,
  updateUser,
  type AccountPolicy,
  type ManagedUser,
} from "../../api.ts";
import ConfirmDialog from "../../components/ConfirmDialog.tsx";
import { shortDate } from "../../display.ts";
import { useIdentity } from "../../features.ts";
import { Checkbox, Row, Section, SectionError, SectionLoading } from "./primitives.tsx";

/**
 * User management for the admin: who has an account, how they sign in, and
 * the rule for first logins through the OIDC provider. The server answers 404
 * to everyone else, and the tab is not shown to them.
 */
export default function UsersSection() {
  const methods = useQuery({ queryKey: ["auth", "methods"], queryFn: fetchLoginMethods, retry: false });
  const oidc = methods.data?.find((m) => m.type === "oidc");
  const password = methods.data?.some((m) => m.type === "password") ?? false;

  return (
    <>
      <UserList />
      {password ? <CreateUser /> : null}
      {oidc ? <PolicyForm providerName={oidc.display_name ?? "single sign-on"} /> : null}
    </>
  );
}

function methodsOf(u: ManagedUser): string {
  const parts: string[] = [];
  if (u.password_username) parts.push(`password (${u.password_username})`);
  for (const id of u.identities) parts.push(id.email || "OIDC");
  return parts.length ? parts.join(", ") : "no password or linked login";
}

function UserList() {
  const queryClient = useQueryClient();
  const me = useIdentity().userId;
  const users = useQuery({ queryKey: ["admin", "users"], queryFn: listUsers, retry: false });
  const [pwFor, setPwFor] = useState<number | null>(null);
  const [pw, setPw] = useState("");
  const [toDelete, setToDelete] = useState<ManagedUser | null>(null);
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["admin", "users"] });

  const update = useMutation({
    mutationFn: ({ id, change }: { id: number; change: { disabled?: boolean; is_admin?: boolean } }) =>
      updateUser(id, change),
    onSuccess: refresh,
  });
  const savePw = useMutation({
    mutationFn: ({ id, password }: { id: number; password: string }) => setUserPassword(id, password),
    onSuccess: () => {
      setPwFor(null);
      setPw("");
      refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (id: number) => deleteUser(id),
    onSuccess: () => {
      setToDelete(null);
      refresh();
    },
  });

  const subtitle =
    "Disabling an account signs it out everywhere and refuses its logins. Only an account without library items can be deleted.";

  if (users.error) {
    return (
      <Section title="Users" subtitle={subtitle}>
        <SectionError error={users.error as Error} />
      </Section>
    );
  }
  if (!users.data) {
    return (
      <Section title="Users" subtitle={subtitle}>
        <SectionLoading />
      </Section>
    );
  }

  const busy = update.isPending || savePw.isPending || remove.isPending;
  const actionError = update.error ?? savePw.error ?? remove.error;

  return (
    <Section title="Users" subtitle={subtitle}>
      {users.data.users.map((u) => {
        const self = u.id === me;
        const disabled = Boolean(u.disabled_at);
        return (
          <Row
            key={u.id}
            label={u.login}
            value={
              <span style={{ display: "flex", gap: 6, flexWrap: "wrap", alignItems: "center" }}>
                {u.is_admin ? <span className="tag tag-accent">Admin</span> : null}
                {disabled ? <span className="tag tag-outline">Disabled</span> : null}
                {self ? <span className="tag tag-neutral">You</span> : null}
                <span style={{ fontSize: 13, color: "var(--color-text-2)" }}>
                  {methodsOf(u)} · {u.items} items · last seen {shortDate(u.last_seen)}
                </span>
              </span>
            }
          >
            {pwFor === u.id ? (
              <form
                style={{ display: "flex", gap: 6, alignItems: "center", flexWrap: "wrap", marginTop: 8 }}
                onSubmit={(e) => {
                  e.preventDefault();
                  savePw.mutate({ id: u.id, password: pw });
                }}
              >
                <input
                  className="input"
                  type="password"
                  autoComplete="new-password"
                  placeholder="New password, 12+ characters"
                  value={pw}
                  onChange={(e) => setPw(e.target.value)}
                  style={{ width: 260 }}
                  autoFocus
                />
                <button type="submit" className="btn btn-primary" disabled={!pw || busy}>
                  {savePw.isPending ? "Saving…" : "Save"}
                </button>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => {
                    setPwFor(null);
                    setPw("");
                  }}
                >
                  Cancel
                </button>
              </form>
            ) : (
              <div style={{ display: "flex", gap: 6, flexWrap: "wrap", marginTop: 8 }}>
                <button
                  type="button"
                  className="btn btn-secondary"
                  disabled={busy}
                  onClick={() => {
                    savePw.reset();
                    setPwFor(u.id);
                    setPw("");
                  }}
                >
                  {u.password_username ? "Reset password" : "Set password"}
                </button>
                {!self ? (
                  <>
                    <button
                      type="button"
                      className="btn btn-secondary"
                      disabled={busy}
                      onClick={() => update.mutate({ id: u.id, change: { disabled: !disabled } })}
                    >
                      {disabled ? "Enable" : "Disable"}
                    </button>
                    <button
                      type="button"
                      className="btn btn-secondary"
                      disabled={busy}
                      onClick={() => update.mutate({ id: u.id, change: { is_admin: !u.is_admin } })}
                    >
                      {u.is_admin ? "Remove admin" : "Make admin"}
                    </button>
                    {/* User 1 is the built-in local user MCP stdio mode acts as. */}
                    {u.items === 0 && u.id !== 1 ? (
                      <button
                        type="button"
                        className="btn btn-danger"
                        disabled={busy}
                        onClick={() => {
                          remove.reset();
                          setToDelete(u);
                        }}
                      >
                        Delete
                      </button>
                    ) : null}
                  </>
                ) : null}
              </div>
            )}
          </Row>
        );
      })}
      {actionError ? <p className="field-error">{(actionError as Error).message}</p> : null}
      <ConfirmDialog
        open={toDelete !== null}
        title="Delete this user?"
        body={
          toDelete
            ? `${toDelete.login} loses the account, its logins, channel follows and podcast subscriptions. This cannot be undone.`
            : ""
        }
        confirmLabel="Delete user"
        danger
        busy={remove.isPending}
        onConfirm={() => toDelete && remove.mutate(toDelete.id)}
        onCancel={() => setToDelete(null)}
      />
    </Section>
  );
}

function CreateUser() {
  const queryClient = useQueryClient();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [admin, setAdmin] = useState(false);
  const create = useMutation({
    mutationFn: () => createUser(username, password, admin),
    onSuccess: () => {
      setUsername("");
      setPassword("");
      setAdmin(false);
      queryClient.invalidateQueries({ queryKey: ["admin", "users"] });
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    create.mutate();
  };

  return (
    <Section
      title="New user"
      subtitle="A password account. The username is also the login shown in vimmary; hand the password over yourself."
    >
      <form onSubmit={submit}>
        <Row label="Username">
          <input
            className="input"
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            style={{ maxWidth: 320 }}
            required
          />
        </Row>
        <Row label="Password">
          <input
            className="input"
            type="password"
            autoComplete="new-password"
            minLength={12}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            style={{ maxWidth: 320 }}
            required
          />
          <p className="field-hint">At least 12 characters.</p>
        </Row>
        <Row label="Admin">
          <label style={{ display: "inline-flex", gap: 10, alignItems: "center", fontSize: 14 }}>
            <Checkbox checked={admin} onChange={setAdmin} label="Admin" />
            May change service-wide settings and manage users
          </label>
        </Row>
        <Row label="">
          <button type="submit" className="btn btn-primary" disabled={create.isPending || !username || !password}>
            {create.isPending ? "Creating…" : "Create user"}
          </button>
          {create.error ? <p className="field-error">{(create.error as Error).message}</p> : null}
          {create.isSuccess ? <p className="field-hint">Created {create.data.login}.</p> : null}
        </Row>
      </form>
    </Section>
  );
}

function PolicyForm({ providerName }: { providerName: string }) {
  const queryClient = useQueryClient();
  const policy = useQuery({ queryKey: ["admin", "account-policy"], queryFn: fetchAccountPolicy, retry: false });
  const [draft, setDraft] = useState<{ mode: AccountPolicy["mode"]; domains: string } | null>(null);
  const save = useMutation({
    mutationFn: (p: AccountPolicy) => saveAccountPolicy(p),
    onSuccess: (p) => {
      queryClient.setQueryData(["admin", "account-policy"], p);
      setDraft(null);
    },
  });

  const subtitle = `Who gets an account on a first ${providerName} login. Accounts you create here, and logins already linked, are not affected.`;
  if (policy.error) {
    return (
      <Section title="New accounts" subtitle={subtitle}>
        <SectionError error={policy.error as Error} />
      </Section>
    );
  }
  if (!policy.data) {
    return (
      <Section title="New accounts" subtitle={subtitle}>
        <SectionLoading />
      </Section>
    );
  }

  const current = draft ?? { mode: policy.data.mode, domains: policy.data.email_domains.join(", ") };
  const dirty = draft !== null;

  return (
    <Section title="New accounts" subtitle={subtitle}>
      <Row label="Rule">
        <span className="seg">
          {(
            [
              ["closed", "Admin creates accounts"],
              ["oidc_auto", `First ${providerName} login creates one`],
            ] as const
          ).map(([mode, label]) => (
            <label key={mode} className="seg-opt">
              <input
                type="radio"
                name="account-policy"
                value={mode}
                checked={current.mode === mode}
                onChange={() => setDraft({ ...current, mode })}
              />
              {label}
            </label>
          ))}
        </span>
      </Row>
      {current.mode === "oidc_auto" ? (
        <Row label="Email domains">
          <input
            className="input"
            placeholder="example.com, example.org"
            value={current.domains}
            onChange={(e) => setDraft({ ...current, domains: e.target.value })}
            style={{ maxWidth: 420 }}
          />
          <p className="field-hint">Comma-separated. Empty admits every email address the provider vouches for.</p>
        </Row>
      ) : null}
      <Row label="">
        <button
          type="button"
          className="btn btn-primary"
          disabled={!dirty || save.isPending}
          onClick={() =>
            save.mutate({
              mode: current.mode,
              email_domains: current.domains.split(",").map((d) => d.trim()).filter(Boolean),
            })
          }
        >
          {save.isPending ? "Saving…" : "Save"}
        </button>
        {save.error ? <p className="field-error">{(save.error as Error).message}</p> : null}
      </Row>
    </Section>
  );
}
