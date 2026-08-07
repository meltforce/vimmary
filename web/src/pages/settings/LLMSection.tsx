import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { fetchLLMSettings, updateLLMSettings } from "../../api.ts";
import { useIsAdmin } from "../../features.ts";
import { Row, Section } from "./primitives.tsx";

/**
 * The service-wide API keys and the summary provider. They used to come from
 * setec at startup; resolving them over the network was what left vimmary dead
 * for six hours on 2026-08-07, so they now live in the database and are entered
 * here.
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
    clearable: boolean,
  ) => (
    <Row
      key={which}
      label={label}
      value={
        editing === which ? undefined : configured ? (
          <span className="mono">••••••••••••</span>
        ) : (
          <span style={{ color: "var(--color-neutral-600)" }}>Not configured</span>
        )
      }
    >
      {editing === which ? (
        <div style={{ display: "flex", gap: 6, alignItems: "center", flexWrap: "wrap" }}>
          <input
            type="password"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder="Paste key"
            className="input"
            style={{ width: 220 }}
            autoFocus
          />
          <button
            type="button"
            className="btn btn-primary"
            disabled={!draft || save.isPending}
            onClick={() =>
              save.mutate(
                which === "mistral" ? { mistral_api_key: draft } : { anthropic_api_key: draft },
              )
            }
          >
            {save.isPending ? "Saving…" : "Save"}
          </button>
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => {
              setEditing(null);
              setDraft("");
            }}
          >
            Cancel
          </button>
        </div>
      ) : (
        <div style={{ display: "flex", gap: 6 }}>
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => {
              setEditing(which);
              setDraft("");
            }}
          >
            {configured ? "Replace" : "Set key"}
          </button>
          {configured && clearable ? (
            <button
              type="button"
              className="btn btn-danger"
              disabled={save.isPending}
              onClick={() =>
                save.mutate(
                  which === "mistral" ? { mistral_api_key: "" } : { anthropic_api_key: "" },
                )
              }
            >
              Remove
            </button>
          ) : null}
        </div>
      )}
    </Row>
  );

  return (
    <Section
      title="LLM providers"
      subtitle="Service-wide and visible to the primary user only. The Mistral key also serves embeddings and podcast transcription, so replacing it changes all three; the Anthropic key may be left empty."
    >
      {keyRow("mistral", "Mistral key", llm.mistral_configured, false)}
      {keyRow("anthropic", "Anthropic key", llm.anthropic_configured, true)}
      <Row label="Summary provider">
        <select
          className="select"
          style={{ width: "auto", minWidth: 180 }}
          value={llm.provider}
          disabled={save.isPending}
          onChange={(e) => save.mutate({ provider: e.target.value })}
        >
          {llm.mistral_configured ? <option value="mistral">Mistral</option> : null}
          {llm.anthropic_configured ? <option value="claude">Claude</option> : null}
        </select>
        {save.error ? <p className="field-error">{(save.error as Error).message}</p> : null}
      </Row>
    </Section>
  );
}
