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
import { usePodcastsEnabled } from "../../features.ts";
import ConfirmDialog from "../../components/ConfirmDialog.tsx";
import { Section, SectionError, SectionLoading, Switch } from "./primitives.tsx";

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

function PodcastFeedRow({ feed }: { feed: PodcastFeed }) {
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
  const rowError = (subscribe.error ?? backfill.error ?? summarizeAll.error ?? transcribeAll.error) as
    | Error
    | undefined;

  return (
    <div className="set-row" style={{ alignItems: "flex-start" }}>
      <div className="kick" style={{ paddingTop: 4 }}>
        <Switch
          checked={feed.subscribed}
          disabled={subscribe.isPending}
          label={`Subscribe to ${feed.title}`}
          onChange={(enabled) => subscribe.mutate({ enabled, level: feed.detail_level })}
        />
      </div>

      <div className="val">
        <div style={{ fontSize: 14, fontWeight: 500 }}>{feed.title}</div>
        <div style={{ font: "400 11.5px var(--font-body)", color: "var(--color-neutral-600)", marginTop: 3 }}>
          {feed.completed_count} of {feed.episode_count} transcribed in cast2md ·{" "}
          {feed.summarized_count} summarized here
          {feed.subscribed ? ` · ${formatPolled(feed.last_polled_at)}` : ""}
        </div>

        {/* The first poll runs when the feed is switched on, so this only
            appears when that attempt failed and the poller has to retry. */}
        {feed.subscribed && !feed.initialized ? (
          <div style={{ font: "400 11.5px var(--font-body)", color: "var(--color-neutral-600)", marginTop: 4 }}>
            The first poll has not completed. The poller retries within a poll interval.
          </div>
        ) : null}
        {feed.last_error ? (
          <div style={{ font: "400 11.5px var(--font-body)", color: "var(--color-accent-700)", marginTop: 4 }}>
            {feed.last_error}
          </div>
        ) : null}

        <div style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 10, flexWrap: "wrap" }}>
          <select
            className="select"
            style={{ width: "auto" }}
            value={feed.detail_level}
            disabled={subscribe.isPending}
            aria-label="Detail level for this feed"
            onChange={(e) => subscribe.mutate({ enabled: feed.subscribed, level: e.target.value })}
          >
            <option value="medium">medium</option>
            <option value="deep">deep</option>
          </select>
          <select
            className="select"
            style={{ width: "auto" }}
            value={feed.initial_backfill}
            disabled={subscribe.isPending}
            aria-label="How many recent episodes are summarized when this feed is switched on"
            onChange={(e) =>
              subscribe.mutate({
                enabled: feed.subscribed,
                level: feed.detail_level,
                initialBackfill: parseInt(e.target.value, 10),
              })
            }
          >
            <option value={0}>on subscribe: none</option>
            {[1, 3, 5, 10, 25].map((n) => (
              <option key={n} value={n}>on subscribe: last {n}</option>
            ))}
          </select>
          <select
            className="select"
            style={{ width: "auto" }}
            value={backfillLimit}
            aria-label="Backfill size"
            onChange={(e) => setBackfillLimit(parseInt(e.target.value, 10))}
          >
            {[5, 10, 25, 50].map((n) => (
              <option key={n} value={n}>last {n}</option>
            ))}
          </select>
          <button
            type="button"
            className="btn btn-secondary"
            disabled={backfill.isPending || busy}
            title="Summarize the newest completed episodes without moving the watermark"
            onClick={() => backfill.mutate()}
          >
            {backfill.isPending ? "Queueing…" : "Backfill"}
          </button>
          <button
            type="button"
            className="btn btn-secondary"
            disabled={busy || feed.completed_count === 0}
            onClick={() => setConfirm("summarize")}
          >
            Summarize all ({feed.completed_count})
          </button>
          <button
            type="button"
            className="btn btn-secondary"
            disabled={busy || feed.transcribable_count === 0}
            onClick={() => setConfirm("transcribe")}
          >
            Transcribe all ({feed.transcribable_count})
          </button>
        </div>

        {backfill.isSuccess ? (
          <p className="field-hint">
            {backfill.data.queued} queued · {backfill.data.skipped} skipped
          </p>
        ) : null}
        {summarizeAll.isSuccess ? (
          <p className="field-hint">
            {summarizeAll.data.queued} queued · {summarizeAll.data.skipped} already done
          </p>
        ) : null}
        {transcribeAll.isSuccess ? (
          <p className="field-hint">
            {transcribeAll.data.queued} queued in cast2md · {transcribeAll.data.skipped} skipped
          </p>
        ) : null}
        {rowError ? <p className="field-error">{rowError.message}</p> : null}
      </div>

      <ConfirmDialog
        open={confirm !== null}
        title={confirm === "summarize" ? "Summarize the whole feed?" : "Transcribe the whole feed?"}
        body={
          confirm === "summarize"
            ? `${feed.completed_count} ${feed.completed_count === 1 ? "episode has" : "episodes have"} a transcript in cast2md. Each one costs a model call.`
            : `cast2md downloads and transcribes ${feed.transcribable_count} ${feed.transcribable_count === 1 ? "episode" : "episodes"}. They arrive here as they finish${feed.subscribed ? "" : " — but only once this feed is subscribed"}.`
        }
        confirmLabel="Go ahead"
        busy={busy}
        onConfirm={() => (confirm === "summarize" ? summarizeAll.mutate() : transcribeAll.mutate())}
        onCancel={() => setConfirm(null)}
      />
    </div>
  );
}

export default function PodcastSection() {
  const enabled = usePodcastsEnabled();
  const { data, isLoading, error } = useQuery({
    queryKey: ["podcast-feeds"],
    queryFn: listPodcastFeeds,
    // Only ask when the server says it has cast2md; a 503 is not worth retrying
    // either way.
    enabled,
    retry: false,
  });

  // Without cast2md there is no section — not a disabled one, not an empty one.
  // This installation simply does not do podcasts.
  if (!enabled) return null;

  return (
    <Section
      title="Podcasts"
      subtitle="Which shows get summarized as cast2md finishes transcribing them. Switching a feed on summarizes its most recent episodes right away and takes the newest of them as the watermark, so the batch is never fetched twice."
    >
      {isLoading ? <SectionLoading /> : null}
      {error ? <SectionError error={error as Error} /> : null}
      {data && data.feeds.length === 0 ? (
        <p style={{ padding: "15px 0", fontSize: 13.5, color: "var(--color-neutral-700)" }}>
          cast2md has no feeds yet.
        </p>
      ) : null}
      {data?.feeds.map((feed) => (
        <PodcastFeedRow key={feed.feed_id} feed={feed} />
      ))}
    </Section>
  );
}
