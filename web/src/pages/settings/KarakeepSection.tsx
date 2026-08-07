import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchWebhookInfo,
  fetchKarakeepStatus,
  setKarakeepAPIKey,
  importKarakeepBookmarks,
} from "../../api.ts";
import { CopyButton, Row, Section, SectionError, SectionLoading } from "./primitives.tsx";

/**
 * KarakeepSection holds the per-user Karakeep API key, the webhook Karakeep
 * posts to, and the one-off bookmark import. The key is per user
 * (users.karakeep_api_key), unlike the service-wide LLM keys in LLMSection.
 */
export default function KarakeepSection() {
  const queryClient = useQueryClient();
  const [apiKey, setApiKey] = useState("");
  const [showApiKey, setShowApiKey] = useState(false);

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
      setShowApiKey(false);
    },
  });

  const importBookmarks = useMutation({
    mutationFn: importKarakeepBookmarks,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["videos"] }),
  });

  const error = (webhookError ?? statusError) as Error | undefined;
  const webhookURL = `${window.location.origin}/webhook/karakeep`;

  return (
    <Section title="Karakeep" subtitle="Keep Vimmary and Karakeep in sync.">
      {(webhookLoading || statusLoading) && <SectionLoading what="Karakeep settings" />}
      {error && <SectionError error={error} />}
      {!webhookLoading && !statusLoading && !error && (
        <>
          <Row
            label="API key"
            value={
              status?.configured ? (
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 12.5 }}>
                  ••••••••••••
                </span>
              ) : (
                <span style={{ color: "var(--vim-ink-3)" }}>Not configured</span>
              )
            }
          >
            {showApiKey ? (
              <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
                <input
                  type="password"
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  placeholder="Paste key"
                  className="vim-input"
                  style={{ width: 200, padding: "7px 10px", fontSize: 12 }}
                  autoFocus
                />
                <button
                  onClick={() => saveKey.mutate(apiKey)}
                  disabled={!apiKey || saveKey.isPending}
                  className="vim-btn primary"
                  style={{ padding: "6px 12px", fontSize: 12 }}
                >
                  {saveKey.isPending ? "Saving…" : "Save"}
                </button>
                <button
                  onClick={() => {
                    setShowApiKey(false);
                    setApiKey("");
                  }}
                  className="vim-btn ghost"
                  style={{ padding: "6px 12px", fontSize: 12 }}
                >
                  Cancel
                </button>
              </div>
            ) : (
              <button
                onClick={() => setShowApiKey(true)}
                className="vim-btn ghost"
                style={{ padding: "6px 12px", fontSize: 12 }}
              >
                {status?.configured ? "Change" : "Set"}
              </button>
            )}
          </Row>
          {saveKey.isError && (
            <p style={{ fontSize: 12, color: "var(--vim-err)", margin: "0 0 8px" }}>
              {(saveKey.error as Error).message}
            </p>
          )}
          <Row label="Webhook URL" value={webhookURL} mono>
            <CopyButton text={webhookURL} />
          </Row>
          <Row label="Bearer token" value={webhook?.token ?? ""} mono>
            <CopyButton text={webhook?.token ?? ""} />
          </Row>
          {status?.configured && (
            <Row
              label="Bulk import"
              value="Pull every YouTube bookmark you've ever starred."
              truncate={false}
              isLast
            >
              <button
                onClick={() => importBookmarks.mutate()}
                disabled={importBookmarks.isPending}
                className="vim-btn primary"
                style={{ padding: "8px 14px", fontSize: 12 }}
              >
                {importBookmarks.isPending ? "Importing…" : "Import"}
              </button>
            </Row>
          )}
          {!status?.configured && (
            <Row label="Bulk import" value="Configure API key to enable." isLast />
          )}
          {importBookmarks.isSuccess && importBookmarks.data && (
            <p
              style={{
                fontSize: 12,
                color: "var(--vim-ok)",
                padding: "0 0 12px",
                margin: 0,
              }}
            >
              Found {importBookmarks.data.total} videos · imported{" "}
              {importBookmarks.data.imported} · skipped {importBookmarks.data.skipped}
            </p>
          )}
          {importBookmarks.isError && (
            <p
              style={{
                fontSize: 12,
                color: "var(--vim-err)",
                padding: "0 0 12px",
                margin: 0,
              }}
            >
              {(importBookmarks.error as Error).message}
            </p>
          )}
        </>
      )}
    </Section>
  );
}
