import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchWebhookInfo,
  fetchKarakeepStatus,
  setKarakeepAPIKey,
  importKarakeepBookmarks,
} from "../../api.ts";
import { CopyButton, Row, Section, SectionError, SectionLoading } from "./primitives.tsx";
import { clock } from "../../display.ts";

/**
 * The per-user Karakeep API key, the webhook Karakeep posts to, and the one-off
 * bookmark import. The key is per user (users.karakeep_api_key), unlike the
 * service-wide LLM keys in LLMSection.
 */
export default function KarakeepSection() {
  const queryClient = useQueryClient();
  const [apiKey, setApiKey] = useState("");
  const [editing, setEditing] = useState(false);

  const { data: webhook, isLoading: webhookLoading, error: webhookError } = useQuery({
    queryKey: ["settings", "webhook"],
    queryFn: fetchWebhookInfo,
  });
  const { data: status, isLoading: statusLoading, error: statusError } = useQuery({
    queryKey: ["settings", "karakeep"],
    queryFn: fetchKarakeepStatus,
  });

  const saveKey = useMutation({
    mutationFn: (key: string) => setKarakeepAPIKey(key),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["settings", "karakeep"] });
      setApiKey("");
      setEditing(false);
    },
  });

  const importBookmarks = useMutation({
    mutationFn: importKarakeepBookmarks,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["videos"] }),
  });

  const error = (webhookError ?? statusError) as Error | undefined;
  const webhookURL = `${window.location.origin}/webhook/karakeep`;
  const loading = webhookLoading || statusLoading;

  return (
    <Section
      title="Karakeep"
      subtitle="A bookmark tagged in Karakeep arrives here through the webhook; the API key lets vimmary write the summary back and pull older bookmarks in bulk."
    >
      {loading ? <SectionLoading /> : null}
      {error ? <SectionError error={error} /> : null}

      {!loading && !error ? (
        <>
          <Row
            label="API key"
            value={
              editing ? undefined : status?.configured ? (
                <span className="mono">••••••••••••</span>
              ) : (
                <span style={{ color: "var(--color-neutral-600)" }}>Not configured</span>
              )
            }
          >
            {editing ? (
              <div style={{ display: "flex", gap: 6, alignItems: "center", flexWrap: "wrap" }}>
                <input
                  type="password"
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  placeholder="Paste key"
                  className="input"
                  style={{ width: 220 }}
                  autoFocus
                />
                <button
                  type="button"
                  className="btn btn-primary"
                  disabled={!apiKey || saveKey.isPending}
                  onClick={() => saveKey.mutate(apiKey)}
                >
                  {saveKey.isPending ? "Saving…" : "Save"}
                </button>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => {
                    setEditing(false);
                    setApiKey("");
                  }}
                >
                  Cancel
                </button>
              </div>
            ) : (
              <button type="button" className="btn btn-secondary" onClick={() => setEditing(true)}>
                {status?.configured ? "Change" : "Set"}
              </button>
            )}
            {saveKey.isError ? (
              <p className="field-error">{(saveKey.error as Error).message}</p>
            ) : null}
          </Row>

          <Row label="Webhook URL" value={webhookURL} mono>
            <CopyButton text={webhookURL} />
          </Row>

          <Row label="Bearer token" value={webhook?.token ?? ""} mono>
            <CopyButton text={webhook?.token ?? ""} />
          </Row>

          <Row label="Bulk import">
            {status?.configured ? (
              <>
                <p
                  style={{
                    font: "400 13px var(--font-body)",
                    color: "var(--color-neutral-700)",
                    margin: "0 0 10px",
                    maxWidth: "58ch",
                  }}
                >
                  Pulls every YouTube bookmark in Karakeep. Already-summarized ones are skipped,
                  and the queue spaces the InnerTube calls out by itself.
                </p>
                <button
                  type="button"
                  className="btn btn-primary"
                  disabled={importBookmarks.isPending}
                  onClick={() => importBookmarks.mutate()}
                >
                  {importBookmarks.isPending ? "Importing…" : "Import bookmarks"}
                </button>
              </>
            ) : (
              <span style={{ color: "var(--color-neutral-600)", fontSize: 14 }}>
                Set the API key to enable it.
              </span>
            )}
          </Row>

          {importBookmarks.isSuccess || importBookmarks.isError ? (
            <div className="log">
              {importBookmarks.isSuccess && importBookmarks.data ? (
                <div className="log-line" style={{ paddingLeft: 0, paddingRight: 0 }}>
                  <time>{clock(new Date().toISOString())}</time>
                  <span>
                    {importBookmarks.data.total} found · {importBookmarks.data.imported} imported ·{" "}
                    {importBookmarks.data.skipped} skipped
                  </span>
                </div>
              ) : null}
              {importBookmarks.isError ? (
                <div className="log-line err" style={{ paddingLeft: 0, paddingRight: 0 }}>
                  <time>{clock(new Date().toISOString())}</time>
                  <span>{(importBookmarks.error as Error).message}</span>
                </div>
              ) : null}
            </div>
          ) : null}
        </>
      ) : null}
    </Section>
  );
}
