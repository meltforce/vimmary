import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  listPodcastFeeds,
  setPodcastSubscription,
  backfillPodcastFeed,
  summarizeAllPodcastFeed,
  transcribeAllPodcastFeed,
} from "../../api.ts";
import type { PodcastFeed } from "../../api.ts";
import { MicIcon } from "../../components/SourceBadge.tsx";
import { usePodcastsEnabled } from "../../features.ts";
import { Section } from "./primitives.tsx";

function formatPolled(iso?: string): string {
  if (!iso) return "never polled";
  return `polled ${new Date(iso).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  })}`;
}

// Both bulk actions ask first, because both are unbounded in a way the small
// backfill is not: one spends LLM calls on the whole back catalogue, the other
// starts downloads and Whisper runs in cast2md.
type PendingBulk = "summarize" | "transcribe" | null;

function PodcastFeedRow({ feed, isLast }: { feed: PodcastFeed; isLast: boolean }) {
  const queryClient = useQueryClient();
  const [backfillLimit, setBackfillLimit] = useState(5);
  const [confirm, setConfirm] = useState<PendingBulk>(null);

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["podcast-feeds"] });
    queryClient.invalidateQueries({ queryKey: ["podcasts"] });
  };

  const subscribe = useMutation({
    mutationFn: (next: { enabled: boolean; level: string; initialBackfill?: number }) =>
      setPodcastSubscription(feed.feed_id, next.enabled, next.level, next.initialBackfill),
    onSuccess: invalidate,
  });

  const backfill = useMutation({
    mutationFn: () => backfillPodcastFeed(feed.feed_id, backfillLimit),
    onSuccess: invalidate,
  });

  const summarizeAll = useMutation({
    mutationFn: () => summarizeAllPodcastFeed(feed.feed_id),
    onSuccess: () => {
      setConfirm(null);
      invalidate();
    },
  });

  const transcribeAll = useMutation({
    mutationFn: () => transcribeAllPodcastFeed(feed.feed_id),
    onSuccess: () => {
      setConfirm(null);
      invalidate();
    },
  });

  const busy = summarizeAll.isPending || transcribeAll.isPending;
  const bulkError = (summarizeAll.error ?? transcribeAll.error) as Error | undefined;

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
          {feed.completed_count} of {feed.episode_count} transcribed in cast2md ·{" "}
          {feed.summarized_count} summarized here
          {feed.subscribed && (
            <>
              {" · "}
              {formatPolled(feed.last_polled_at)}
            </>
          )}
        </div>
        {/* The first poll runs when the feed is switched on, so this only
            appears when that attempt failed and the poller has to retry. */}
        {feed.subscribed && !feed.initialized && (
          <div style={{ fontSize: 11.5, color: "var(--vim-ink-4)", marginTop: 4 }}>
            The first poll has not completed. The poller retries within a poll
            interval.
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
            title="Detail level for this feed's summaries"
          >
            <option value="medium">medium</option>
            <option value="deep">deep</option>
          </select>
          <select
            value={feed.initial_backfill}
            disabled={subscribe.isPending}
            onChange={(e) =>
              subscribe.mutate({
                enabled: feed.subscribed,
                level: feed.detail_level,
                initialBackfill: parseInt(e.target.value, 10),
              })
            }
            className="vim-input"
            style={{ width: "auto", padding: "5px 8px", fontSize: 12 }}
            title="How many recent episodes are summarized right away when this feed is switched on"
          >
            <option value={0}>on subscribe: none</option>
            {[1, 3, 5, 10, 25].map((n) => (
              <option key={n} value={n}>
                on subscribe: last {n}
              </option>
            ))}
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
            disabled={backfill.isPending || busy}
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

        {/* Whole-feed actions. Both are open-ended, so each states its count
            and asks before running. */}
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 8,
            marginTop: 8,
            flexWrap: "wrap",
          }}
        >
          {confirm === null ? (
            <>
              <button
                onClick={() => setConfirm("summarize")}
                disabled={busy || feed.completed_count === 0}
                className="vim-btn ghost"
                style={{ padding: "5px 12px", fontSize: 12 }}
                title="Summarize every episode cast2md already has a transcript for"
              >
                Summarize all ({feed.completed_count})
              </button>
              <button
                onClick={() => setConfirm("transcribe")}
                disabled={busy || feed.transcribable_count === 0}
                className="vim-btn ghost"
                style={{ padding: "5px 12px", fontSize: 12 }}
                title="Ask cast2md to download and transcribe the rest of this feed"
              >
                Transcribe all ({feed.transcribable_count})
              </button>
            </>
          ) : (
            <>
              <span style={{ fontSize: 12, color: "var(--vim-warn)" }}>
                {confirm === "summarize"
                  ? `Summarize ${feed.completed_count} episode${
                      feed.completed_count === 1 ? "" : "s"
                    }? That is ${feed.completed_count} model call${
                      feed.completed_count === 1 ? "" : "s"
                    }.`
                  : `Have cast2md download and transcribe ${feed.transcribable_count} episode${
                      feed.transcribable_count === 1 ? "" : "s"
                    }? They appear here as they finish${
                      feed.subscribed ? "" : " — but only once this feed is subscribed"
                    }.`}
              </span>
              <button
                onClick={() =>
                  confirm === "summarize" ? summarizeAll.mutate() : transcribeAll.mutate()
                }
                disabled={busy}
                className="vim-btn primary"
                style={{ padding: "5px 12px", fontSize: 12 }}
              >
                {busy ? "Queuing…" : "Yes, go ahead"}
              </button>
              <button
                onClick={() => setConfirm(null)}
                className="vim-btn ghost"
                style={{ padding: "5px 12px", fontSize: 12 }}
              >
                Cancel
              </button>
            </>
          )}
          {summarizeAll.isSuccess && (
            <span style={{ fontSize: 12, color: "var(--vim-ok)" }}>
              {summarizeAll.data.queued} queued · {summarizeAll.data.skipped} already done
            </span>
          )}
          {transcribeAll.isSuccess && (
            <span style={{ fontSize: 12, color: "var(--vim-ok)" }}>
              {transcribeAll.data.queued} queued in cast2md · {transcribeAll.data.skipped} skipped
            </span>
          )}
          {bulkError && (
            <span style={{ fontSize: 12, color: "var(--vim-err)" }}>{bulkError.message}</span>
          )}
        </div>
      </div>
    </div>
  );
}

export default function PodcastSection() {
  const enabled = usePodcastsEnabled();
  const { data, isLoading, error } = useQuery({
    queryKey: ["podcast-feeds"],
    queryFn: listPodcastFeeds,
    // Only ask when the server says it has cast2md; a 503 is not worth
    // retrying either way.
    enabled,
    retry: false,
  });

  // Without cast2md there is no section — not a disabled one, not an empty
  // one. This installation simply does not do podcasts.
  if (!enabled) return null;

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
