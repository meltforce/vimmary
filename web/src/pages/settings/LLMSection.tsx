import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { fetchLLMSettings, updateLLMSettings } from "../../api.ts";
import { useIsAdmin } from "../../features.ts";
import { Row, Section } from "./primitives.tsx";

/**
 * LLMSection holds the service-wide API keys and the summary provider. They
 * used to come from setec at startup; resolving them over the network was what
 * left vimmary dead for six hours on 2026-08-07, so they now live in the
 * database and are entered here.
 *
 * This was the first section to own its query instead of joining the page's
 * isLoading/errorObj chain, because for a non-admin the server answers 404 and
 * that conjunction would have blanked the whole Settings page. Every section
 * works that way now; the chain is gone.
 *
 * It renders nothing at all for a non-admin — not a disabled card, not an
 * explanation. Service-wide keys are the primary user's business.
 */
export default function LLMSection() {
  const queryClient = useQueryClient();
  const isAdmin = useIsAdmin();
  const [editing, setEditing] = useState<"mistral" | "anthropic" | null>(null);
  const [draft, setDraft] = useState("");

  const { data: llm } = useQuery({
    queryKey: ["settings", "llm"],
    queryFn: fetchLLMSettings,
    enabled: isAdmin,
    retry: false,
  });

  const save = useMutation({
    mutationFn: updateLLMSettings,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["settings", "llm"] });
      // The key decides which providers exist and which models can be listed,
      // so both of those have to be refetched rather than left stale.
      queryClient.invalidateQueries({ queryKey: ["providers"] });
      queryClient.invalidateQueries({ queryKey: ["models"] });
      setEditing(null);
      setDraft("");
    },
  });

  if (!isAdmin || !llm) return null;

  const keyRow = (
    which: "mistral" | "anthropic",
    label: string,
    configured: boolean,
    clearable: boolean
  ) => (
    <Row
      key={which}
      label={label}
      value={
        configured ? (
          <span style={{ fontFamily: "var(--font-mono)", fontSize: 12.5 }}>
            ••••••••••••
          </span>
        ) : (
          <span style={{ color: "var(--vim-ink-3)" }}>Not configured</span>
        )
      }
    >
      {editing === which ? (
        <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
          <input
            type="password"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder="Paste key"
            className="vim-input"
            style={{ width: 200, padding: "7px 10px", fontSize: 12 }}
            autoFocus
          />
          <button
            onClick={() =>
              save.mutate(
                which === "mistral"
                  ? { mistral_api_key: draft }
                  : { anthropic_api_key: draft }
              )
            }
            disabled={!draft || save.isPending}
            className="vim-btn primary"
            style={{ padding: "6px 12px", fontSize: 12 }}
          >
            {save.isPending ? "Saving…" : "Save"}
          </button>
          <button
            onClick={() => {
              setEditing(null);
              setDraft("");
            }}
            className="vim-btn ghost"
            style={{ padding: "6px 12px", fontSize: 12 }}
          >
            Cancel
          </button>
        </div>
      ) : (
        <div style={{ display: "flex", gap: 6 }}>
          <button
            onClick={() => {
              setEditing(which);
              setDraft("");
            }}
            className="vim-btn ghost"
            style={{ padding: "6px 12px", fontSize: 12 }}
          >
            {configured ? "Replace" : "Set key"}
          </button>
          {configured && clearable && (
            <button
              onClick={() =>
                save.mutate(
                  which === "mistral"
                    ? { mistral_api_key: "" }
                    : { anthropic_api_key: "" }
                )
              }
              disabled={save.isPending}
              className="vim-btn ghost"
              style={{ padding: "6px 12px", fontSize: 12 }}
            >
              Remove
            </button>
          )}
        </div>
      )}
    </Row>
  );

  return (
    <Section
      title="LLM providers"
      subtitle="Service-wide. Only you can see this."
    >
      {keyRow("mistral", "Mistral API key", llm.mistral_configured, false)}
      {keyRow("anthropic", "Anthropic API key", llm.anthropic_configured, true)}
      <Row label="Summary provider" value={llm.provider || "—"} isLast>
        <select
          className="vim-input"
          style={{ padding: "6px 10px", fontSize: 12 }}
          value={llm.provider}
          disabled={save.isPending}
          onChange={(e) => save.mutate({ provider: e.target.value })}
        >
          {llm.mistral_configured && <option value="mistral">Mistral</option>}
          {llm.anthropic_configured && <option value="claude">Claude</option>}
        </select>
      </Row>
      {save.error && (
        <p style={{ color: "var(--vim-danger)", fontSize: 12, marginTop: 8 }}>
          {(save.error as Error).message}
        </p>
      )}
      <p style={{ color: "var(--vim-ink-3)", fontSize: 12, marginTop: 8 }}>
        The Mistral key also serves embeddings and podcast transcription, so
        replacing it changes those too. The Anthropic key may be left empty.
      </p>
    </Section>
  );
}
