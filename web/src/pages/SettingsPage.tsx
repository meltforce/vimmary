import { useState, ReactNode } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchWebhookInfo,
  fetchFeedInfo,
  fetchKarakeepStatus,
  setKarakeepAPIKey,
  importKarakeepBookmarks,
  fetchSummaryPrompts,
  setSummaryPrompt,
  fetchProviders,
  fetchModels,
  setModel,
  listPodcastFeeds,
  setPodcastSubscription,
  backfillPodcastFeed,
} from "../api.ts";
import type {
  ContentSource,
  ModelInfo,
  ModelsResponse,
  PodcastFeed,
} from "../api.ts";
import LoadingSkeleton from "../components/LoadingSkeleton.tsx";
import { MicIcon } from "../components/SourceBadge.tsx";

function CopyButton({ text, label = "Copy" }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      onClick={() => {
        navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      }}
      className="vim-btn ghost"
      style={{ padding: "6px 12px", fontSize: 12 }}
    >
      {copied ? "Copied ✓" : label}
    </button>
  );
}

function Section({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: ReactNode;
}) {
  return (
    <section style={{ marginBottom: 40 }}>
      <div className="vim-grid-settings">
        <div>
          <h3
            style={{
              fontFamily: "var(--font-serif)",
              fontSize: 20,
              fontWeight: 500,
              margin: "0 0 6px",
              letterSpacing: "-0.01em",
              color: "var(--vim-ink)",
            }}
          >
            {title}
          </h3>
          <p
            style={{
              fontSize: 12.5,
              color: "var(--vim-ink-3)",
              margin: 0,
              lineHeight: 1.5,
            }}
          >
            {subtitle}
          </p>
        </div>
        <div
          style={{
            background: "var(--vim-surface)",
            borderRadius: 12,
            border: "1px solid var(--vim-line-soft)",
            padding: "4px 20px",
          }}
        >
          {children}
        </div>
      </div>
    </section>
  );
}

function Row({
  label,
  value,
  mono = false,
  truncate = true,
  isLast = false,
  children,
}: {
  label: string;
  value?: ReactNode;
  mono?: boolean;
  truncate?: boolean;
  isLast?: boolean;
  children?: ReactNode;
}) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        padding: "16px 0",
        borderBottom: isLast ? "none" : "1px solid var(--vim-line-soft)",
        gap: 16,
      }}
    >
      <div style={{ minWidth: 0, flex: 1 }}>
        <div style={{ fontSize: 13, color: "var(--vim-ink-3)", marginBottom: 3 }}>
          {label}
        </div>
        {value && (
          <div
            style={{
              fontFamily: mono ? "var(--font-mono)" : undefined,
              fontSize: mono ? 12.5 : 14,
              color: "var(--vim-ink)",
              overflow: truncate ? "hidden" : undefined,
              textOverflow: truncate ? "ellipsis" : undefined,
              whiteSpace: truncate ? "nowrap" : undefined,
            }}
          >
            {value}
          </div>
        )}
      </div>
      {children && <div style={{ flexShrink: 0 }}>{children}</div>}
    </div>
  );
}

function ModelSelector() {
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery<ModelsResponse>({
    queryKey: ["models"],
    queryFn: () => fetchModels(),
  });

  const [selected, setSelected] = useState<string | null>(null);

  const currentKey =
    data?.selected_provider && data?.selected_model
      ? `${data.selected_provider}:${data.selected_model}`
      : "";
  const displaySelected = selected ?? currentKey;

  const save = useMutation({
    mutationFn: (key: string) => {
      if (!key) return setModel("", "");
      const [provider, ...rest] = key.split(":");
      return setModel(provider, rest.join(":"));
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["models"] });
      queryClient.invalidateQueries({ queryKey: ["providers"] });
    },
  });

  const hasChanges = displaySelected !== currentKey;

  if (isLoading)
    return <div style={{ fontSize: 13, color: "var(--vim-ink-3)" }}>Loading models…</div>;
  if (!data?.models?.length)
    return <div style={{ fontSize: 13, color: "var(--vim-ink-3)" }}>No models available</div>;

  const byProvider = new Map<string, ModelInfo[]>();
  const seen = new Set<string>();
  for (const m of data.models as ModelInfo[]) {
    const k = `${m.provider}:${m.id}`;
    if (seen.has(k)) continue;
    seen.add(k);
    const list = byProvider.get(m.provider) || [];
    list.push(m);
    byProvider.set(m.provider, list);
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      <div style={{ display: "flex", gap: 8 }}>
        <select
          value={displaySelected}
          onChange={(e) => setSelected(e.target.value)}
          className="vim-input"
          style={{ flex: 1, fontSize: 13 }}
        >
          <option value="">Provider default</option>
          {[...byProvider.entries()].map(([provider, models]) => (
            <optgroup
              key={provider}
              label={provider.charAt(0).toUpperCase() + provider.slice(1)}
            >
              {models.map((m) => (
                <option key={`${provider}:${m.id}`} value={`${provider}:${m.id}`}>
                  {m.display_name || m.id}
                </option>
              ))}
            </optgroup>
          ))}
        </select>
        <button
          onClick={() => save.mutate(displaySelected)}
          disabled={!hasChanges || save.isPending}
          className="vim-btn primary"
          style={{ padding: "8px 14px", fontSize: 12 }}
        >
          {save.isPending ? "Saving…" : "Save"}
        </button>
      </div>
      {save.isError && (
        <p style={{ fontSize: 12, color: "var(--vim-err)", margin: 0 }}>
          {(save.error as Error).message}
        </p>
      )}
    </div>
  );
}

function PromptEditor({
  source,
  level,
  label,
  currentPrompt,
  defaultPrompt,
}: {
  source: ContentSource;
  level: string;
  label: string;
  currentPrompt: string;
  defaultPrompt: string;
}) {
  const queryClient = useQueryClient();
  const [value, setValue] = useState(currentPrompt);
  const [open, setOpen] = useState(false);
  const isCustom = currentPrompt !== defaultPrompt;

  const save = useMutation({
    mutationFn: (prompt: string) => setSummaryPrompt(source, level, prompt),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["settings", "prompts"] }),
  });

  const reset = useMutation({
    mutationFn: () => setSummaryPrompt(source, level, ""),
    onSuccess: () => {
      setValue(defaultPrompt);
      queryClient.invalidateQueries({ queryKey: ["settings", "prompts"] });
    },
  });

  const hasChanges = value !== currentPrompt;

  return (
    <div style={{ padding: "16px 0", borderBottom: "1px solid var(--vim-line-soft)" }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 16,
          marginBottom: open ? 12 : 0,
        }}
      >
        <div>
          <div style={{ fontSize: 13, color: "var(--vim-ink-3)", marginBottom: 3 }}>
            {label}
          </div>
          <div style={{ fontSize: 14, color: "var(--vim-ink)" }}>
            {isCustom ? (
              <>
                Custom prompt{" "}
                <span
                  style={{
                    fontFamily: "var(--font-mono)",
                    fontSize: 11,
                    color: "var(--vim-accent-ink)",
                    marginLeft: 4,
                  }}
                >
                  edited
                </span>
              </>
            ) : (
              "Default prompt"
            )}
          </div>
        </div>
        <button
          onClick={() => setOpen(!open)}
          className="vim-btn ghost"
          style={{ padding: "6px 12px", fontSize: 12 }}
        >
          {open ? "Hide ↑" : "Edit ↓"}
        </button>
      </div>
      {open && (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          <textarea
            value={value}
            onChange={(e) => setValue(e.target.value)}
            rows={12}
            className="vim-input"
            style={{ fontFamily: "var(--font-mono)", fontSize: 12.5, resize: "vertical" }}
          />
          <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
            {isCustom && (
              <button
                onClick={() => reset.mutate()}
                disabled={reset.isPending}
                className="vim-btn outline danger"
                style={{ padding: "6px 12px", fontSize: 12 }}
              >
                {reset.isPending ? "Resetting…" : "Reset to default"}
              </button>
            )}
            <button
              onClick={() => save.mutate(value)}
              disabled={!hasChanges || save.isPending}
              className="vim-btn primary"
              style={{ padding: "6px 12px", fontSize: 12 }}
            >
              {save.isPending ? "Saving…" : "Save"}
            </button>
          </div>
          {save.isSuccess && (
            <p style={{ fontSize: 12, color: "var(--vim-ok)", margin: 0 }}>Prompt saved.</p>
          )}
          {save.isError && (
            <p style={{ fontSize: 12, color: "var(--vim-err)", margin: 0 }}>
              {(save.error as Error).message}
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function formatPolled(iso?: string): string {
  if (!iso) return "never polled";
  return `polled ${new Date(iso).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  })}`;
}

function PodcastFeedRow({ feed, isLast }: { feed: PodcastFeed; isLast: boolean }) {
  const queryClient = useQueryClient();
  const [backfillLimit, setBackfillLimit] = useState(5);

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["podcast-feeds"] });
    queryClient.invalidateQueries({ queryKey: ["podcasts"] });
  };

  const subscribe = useMutation({
    mutationFn: (next: { enabled: boolean; level: string }) =>
      setPodcastSubscription(feed.feed_id, next.enabled, next.level),
    onSuccess: invalidate,
  });

  const backfill = useMutation({
    mutationFn: () => backfillPodcastFeed(feed.feed_id, backfillLimit),
    onSuccess: invalidate,
  });

  return (
    <div
      style={{
        display: "flex",
        alignItems: "flex-start",
        gap: 14,
        padding: "16px 0",
        borderBottom: isLast ? "none" : "1px solid var(--vim-line-soft)",
      }}
    >
      <div
        style={{
          width: 48,
          height: 48,
          borderRadius: 8,
          overflow: "hidden",
          flexShrink: 0,
          background: "var(--vim-surface-2)",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        {feed.image_url ? (
          <img
            src={feed.image_url}
            alt=""
            style={{ width: "100%", height: "100%", objectFit: "cover" }}
          />
        ) : (
          <MicIcon size={20} color="var(--vim-ink-4)" />
        )}
      </div>

      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: 14, color: "var(--vim-ink)", marginBottom: 3 }}>{feed.title}</div>
        <div style={{ fontSize: 11.5, color: "var(--vim-ink-3)" }}>
          {feed.episode_count} episode{feed.episode_count === 1 ? "" : "s"} in cast2md ·{" "}
          {feed.summarized_count} summarized
          {feed.subscribed && (
            <>
              {" · "}
              {formatPolled(feed.last_polled_at)}
            </>
          )}
        </div>
        {feed.subscribed && !feed.initialized && (
          <div style={{ fontSize: 11.5, color: "var(--vim-ink-4)", marginTop: 4 }}>
            Waiting for the first poll. Only episodes transcribed from then on are summarized —
            use Backfill for older ones.
          </div>
        )}
        {feed.last_error && (
          <div style={{ fontSize: 11.5, color: "var(--vim-err)", marginTop: 4 }}>
            {feed.last_error}
          </div>
        )}

        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 8,
            marginTop: 10,
            flexWrap: "wrap",
          }}
        >
          <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12.5 }}>
            <input
              type="checkbox"
              checked={feed.subscribed}
              disabled={subscribe.isPending}
              onChange={(e) =>
                subscribe.mutate({ enabled: e.target.checked, level: feed.detail_level })
              }
            />
            Subscribed
          </label>
          <select
            value={feed.detail_level}
            disabled={subscribe.isPending}
            onChange={(e) => subscribe.mutate({ enabled: feed.subscribed, level: e.target.value })}
            className="vim-input"
            style={{ width: "auto", padding: "5px 8px", fontSize: 12 }}
          >
            <option value="medium">medium</option>
            <option value="deep">deep</option>
          </select>
          <select
            value={backfillLimit}
            onChange={(e) => setBackfillLimit(parseInt(e.target.value, 10))}
            className="vim-input"
            style={{ width: "auto", padding: "5px 8px", fontSize: 12 }}
          >
            {[5, 10, 25, 50].map((n) => (
              <option key={n} value={n}>
                last {n}
              </option>
            ))}
          </select>
          <button
            onClick={() => backfill.mutate()}
            disabled={backfill.isPending}
            className="vim-btn ghost"
            style={{ padding: "5px 12px", fontSize: 12 }}
            title="Summarize the newest completed episodes without moving the watermark"
          >
            {backfill.isPending ? "Queuing…" : "Backfill"}
          </button>
          {backfill.isSuccess && (
            <span style={{ fontSize: 12, color: "var(--vim-ok)" }}>
              {backfill.data.queued} queued · {backfill.data.skipped} skipped
            </span>
          )}
          {(subscribe.isError || backfill.isError) && (
            <span style={{ fontSize: 12, color: "var(--vim-err)" }}>
              {((subscribe.error ?? backfill.error) as Error).message}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}

function PodcastSection() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["podcast-feeds"],
    queryFn: listPodcastFeeds,
    // A cast2md that is off or unreachable is a normal state here, not
    // something to hammer.
    retry: false,
  });

  return (
    <Section
      title="Podcasts"
      subtitle="Pick the shows whose episodes get summarized as cast2md finishes them."
    >
      {isLoading && (
        <div style={{ padding: "16px 0", fontSize: 13, color: "var(--vim-ink-3)" }}>
          Loading feeds…
        </div>
      )}
      {error && (
        <div style={{ padding: "16px 0", fontSize: 13, color: "var(--vim-ink-3)" }}>
          cast2md is not reachable: {(error as Error).message}
        </div>
      )}
      {data && data.feeds.length === 0 && (
        <div style={{ padding: "16px 0", fontSize: 13, color: "var(--vim-ink-3)" }}>
          cast2md has no feeds yet.
        </div>
      )}
      {data?.feeds.map((feed, i) => (
        <PodcastFeedRow
          key={feed.feed_id}
          feed={feed}
          isLast={i === data.feeds.length - 1}
        />
      ))}
    </Section>
  );
}

export default function SettingsPage() {
  const queryClient = useQueryClient();
  const [apiKey, setApiKey] = useState("");
  const [showApiKey, setShowApiKey] = useState(false);
  const [promptSource, setPromptSource] = useState<ContentSource>("youtube");

  const { data: webhook, isLoading: webhookLoading, error: webhookError } = useQuery({
    queryKey: ["settings", "webhook"],
    queryFn: fetchWebhookInfo,
  });
  const { data: feedInfo, isLoading: feedLoading, error: feedError } = useQuery({
    queryKey: ["settings", "feed"],
    queryFn: fetchFeedInfo,
  });
  const { data: karakeepStatus, isLoading: karakeepLoading, error: karakeepError } = useQuery({
    queryKey: ["settings", "karakeep"],
    queryFn: fetchKarakeepStatus,
  });
  const { data: prompts, isLoading: promptsLoading, error: promptsError } = useQuery({
    queryKey: ["settings", "prompts", promptSource],
    queryFn: () => fetchSummaryPrompts(promptSource),
  });
  const { data: providers } = useQuery({
    queryKey: ["providers"],
    queryFn: fetchProviders,
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

  const isLoading = webhookLoading || feedLoading || karakeepLoading || promptsLoading;
  const errorObj = webhookError || feedError || karakeepError || promptsError;

  if (isLoading)
    return (
      <div className="vim-page-narrower">
        <LoadingSkeleton count={3} />
      </div>
    );

  if (errorObj)
    return (
      <div className="vim-page-narrower">
        <div
          style={{
            padding: "12px 16px",
            borderRadius: "var(--vim-radius)",
            background: "color-mix(in oklch, var(--vim-err) 10%, transparent)",
            border: "1px solid color-mix(in oklch, var(--vim-err) 28%, transparent)",
            color: "var(--vim-err)",
            fontSize: 13,
          }}
        >
          {(errorObj as Error).message}
        </div>
      </div>
    );

  const webhookURL = `${window.location.origin}/webhook/karakeep`;
  const feedBase = feedInfo ? `${window.location.origin}/feed/atom/${feedInfo.token}` : "";
  const truncatedFeedToken = feedInfo ? `${feedInfo.token.slice(0, 8)}…` : "—";
  // Three separate subscriptions rather than one feed with a filter: an RSS
  // reader cannot filter, so the split has to happen in the URL.
  const feedVariants: { label: string; suffix: string; hint: string }[] = [
    { label: "Videos only", suffix: "", hint: "The original feed. Existing subscriptions keep this content." },
    { label: "Podcasts only", suffix: "/podcasts", hint: "Podcast episode summaries." },
    { label: "Everything", suffix: "/all", hint: "Both kinds; each entry is tagged with its type." },
  ];

  return (
    <div className="vim-page-narrower">
      <div className="vim-kicker" style={{ marginBottom: 10 }}>
        — Preferences
      </div>
      <h1 className="vim-h1-stats-settings" style={{ marginBottom: 36 }}>Settings</h1>

      {/* Karakeep */}
      <Section title="Karakeep" subtitle="Keep Vimmary and Karakeep in sync.">
        <Row
          label="API key"
          value={
            karakeepStatus?.configured ? (
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
              {karakeepStatus?.configured ? "Change" : "Set"}
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
        {karakeepStatus?.configured && (
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
        {!karakeepStatus?.configured && <Row label="Bulk import" value="Configure API key to enable." isLast />}
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
      </Section>

      {/* Summaries */}
      <Section title="Summaries" subtitle="Model and prompt configuration.">
        {providers && providers.providers.length > 0 && (
          <div style={{ padding: "16px 0", borderBottom: "1px solid var(--vim-line-soft)" }}>
            <div style={{ fontSize: 13, color: "var(--vim-ink-3)", marginBottom: 8 }}>
              Model
            </div>
            <ModelSelector />
          </div>
        )}
        <div style={{ padding: "16px 0", borderBottom: "1px solid var(--vim-line-soft)" }}>
          <div style={{ fontSize: 13, color: "var(--vim-ink-3)", marginBottom: 8 }}>
            Prompts for
          </div>
          <div style={{ display: "flex", gap: 6 }}>
            {(["youtube", "podcast"] as ContentSource[]).map((src) => (
              <button
                key={src}
                onClick={() => setPromptSource(src)}
                className={promptSource === src ? "vim-btn primary" : "vim-btn ghost"}
                style={{ padding: "6px 14px", fontSize: 12 }}
              >
                {src === "youtube" ? "Videos" : "Podcasts"}
              </button>
            ))}
          </div>
        </div>
        {prompts && (
          <>
            <PromptEditor
              key={`${promptSource}-medium`}
              source={promptSource}
              level="medium"
              label="Medium summary prompt"
              currentPrompt={prompts.medium}
              defaultPrompt={prompts.default_medium}
            />
            <PromptEditor
              key={`${promptSource}-deep`}
              source={promptSource}
              level="deep"
              label="Deep summary prompt"
              currentPrompt={prompts.deep}
              defaultPrompt={prompts.default_deep}
            />
            <div style={{ padding: "12px 0 16px", fontSize: 11.5, color: "var(--vim-ink-4)" }}>
              Placeholders:{" "}
              <code
                style={{
                  fontFamily: "var(--font-mono)",
                  background: "var(--vim-surface-2)",
                  padding: "1px 5px",
                  borderRadius: 3,
                }}
              >
                {"{{TITLE}}"}
              </code>
              ,{" "}
              <code
                style={{
                  fontFamily: "var(--font-mono)",
                  background: "var(--vim-surface-2)",
                  padding: "1px 5px",
                  borderRadius: 3,
                }}
              >
                {"{{LANGUAGE}}"}
              </code>
              ,{" "}
              <code
                style={{
                  fontFamily: "var(--font-mono)",
                  background: "var(--vim-surface-2)",
                  padding: "1px 5px",
                  borderRadius: 3,
                }}
              >
                {"{{TRANSCRIPT}}"}
              </code>
            </div>
          </>
        )}
      </Section>

      {/* Podcasts */}
      <PodcastSection />

      {/* RSS */}
      <Section title="RSS" subtitle="Subscribe to your own feeds of summaries.">
        {feedInfo &&
          feedVariants.map((variant, i) => (
            <div
              key={variant.suffix}
              style={{
                padding: "16px 0",
                borderBottom:
                  i === feedVariants.length - 1 ? "none" : "1px solid var(--vim-line-soft)",
              }}
            >
              <div style={{ fontSize: 13, color: "var(--vim-ink-3)", marginBottom: 3 }}>
                {variant.label}
              </div>
              <div style={{ fontSize: 12, color: "var(--vim-ink-4)", marginBottom: 8 }}>
                {variant.hint}
              </div>
              <div
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 12.5,
                  padding: "12px 14px",
                  background: "var(--vim-surface-2)",
                  borderRadius: 6,
                  color: "var(--vim-ink-2)",
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  gap: 12,
                }}
              >
                <span
                  style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
                >
                  {window.location.origin}/feed/atom/
                  <span style={{ color: "var(--vim-accent-ink)" }}>{truncatedFeedToken}</span>
                  {variant.suffix}
                </span>
                <CopyButton text={feedBase + variant.suffix} />
              </div>
            </div>
          ))}
      </Section>
    </div>
  );
}
