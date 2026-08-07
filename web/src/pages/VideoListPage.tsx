import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchFeedInfo,
  fetchStats,
  listVideos,
  retryAllFailed,
  searchVideos,
  submitVideo,
  transcribeAllNoCaptions,
  type HybridMatch,
  type Video,
} from "../api.ts";
import PageHeader from "../components/PageHeader.tsx";
import Toast, { useToast } from "../components/Toast.tsx";
import FeedList, { type Row } from "../components/FeedList.tsx";
import { Skel } from "../components/LoadingSkeleton.tsx";
import { AlertIcon, SearchIcon } from "../components/icons.tsx";
import { useIsDesktop } from "../hooks/useMediaQuery.ts";
import { isInFlight, longDate } from "../display.ts";

const PAGE_SIZE = 20;

/* The chips are server-side filters, so every one of them is a `status` the
   list endpoint accepts. There is no detail-level filter on the API and the
   frontend does not invent one by filtering the current page. */
const FILTERS = [
  { key: "", label: "All" },
  { key: "pending", label: "Queue" },
  { key: "failed", label: "Failed" },
  { key: "no_captions", label: "No captions" },
] as const;

function toRow(v: Video): Row {
  return {
    id: v.id,
    title: v.title || v.youtube_id,
    channel: v.channel,
    created_at: v.created_at,
    source: v.source,
    status: v.status,
    detail_level: v.detail_level,
    duration_seconds: v.duration_seconds,
    error_message: v.error_message,
    summary: v.summary,
    thumbnail_url: v.thumbnail_url,
    topics: v.metadata?.topics,
  };
}

/* Search returns neither a thumbnail nor a length, so those rows carry the
   neutral block and no duration badge. */
function matchToRow(m: HybridMatch): Row {
  return {
    id: m.id,
    title: m.title || m.youtube_id,
    channel: m.channel,
    created_at: m.created_at,
    source: m.source,
    summary: m.summary,
    topics: m.metadata?.topics,
    score: m.score,
  };
}

export default function VideoListPage() {
  const queryClient = useQueryClient();
  const isDesktop = useIsDesktop();
  const toast = useToast();

  const [params, setParams] = useSearchParams();
  const query = params.get("q") ?? "";
  const status = params.get("status") ?? "";
  const page = Math.max(1, parseInt(params.get("page") ?? "1", 10));
  const offset = (page - 1) * PAGE_SIZE;

  const [searchInput, setSearchInput] = useState(query);
  const [url, setUrl] = useState("");

  const searching = query.length > 0;

  const search = useQuery({
    queryKey: ["search", query],
    queryFn: () => searchVideos(query),
    enabled: searching,
  });

  const list = useQuery({
    queryKey: ["videos", status, offset],
    queryFn: () => listVideos({ status: status || undefined, limit: PAGE_SIZE, offset }),
    enabled: !searching,
    // A queue that is moving is polled three times as often as one that is not.
    refetchInterval: (q) =>
      q.state.data?.videos.some((v) => isInFlight(v.status)) ? 3000 : 10000,
  });

  const stats = useQuery({ queryKey: ["stats"], queryFn: () => fetchStats(), refetchInterval: 10000 });
  const feed = useQuery({ queryKey: ["settings", "feed"], queryFn: fetchFeedInfo });

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["videos"] });
    queryClient.invalidateQueries({ queryKey: ["stats"] });
  };

  const submit = useMutation({
    mutationFn: (u: string) => submitVideo(u),
    onSuccess: () => {
      setUrl("");
      invalidate();
      toast.show("Queued. It appears in the list once the transcript is in.");
    },
  });

  const retryAll = useMutation({
    mutationFn: retryAllFailed,
    onSuccess: (r) => {
      invalidate();
      toast.show(`${r.retried} ${r.retried === 1 ? "video" : "videos"} queued for retry.`);
    },
  });

  const transcribeAll = useMutation({
    mutationFn: transcribeAllNoCaptions,
    onSuccess: (r) => {
      invalidate();
      toast.show(
        `${r.transcribing} ${r.transcribing === 1 ? "video" : "videos"} queued for Voxtral.`,
      );
    },
  });

  const rows: Row[] | undefined = searching
    ? search.data?.results.map(matchToRow)
    : list.data?.videos.map(toRow);
  const loading = searching ? search.isLoading : list.isLoading;
  const error = (searching ? search.error : list.error) as Error | null;

  const byStatus = stats.data?.by_status ?? {};
  const failedCount = byStatus.failed ?? 0;
  const noCaptionsCount = byStatus.no_captions ?? 0;
  const queuedCount = (byStatus.pending ?? 0) + (byStatus.processing ?? 0);

  /* The stats endpoint returns a daily series; the week is a sum over it rather
     than a second request. */
  const lastWeek = (() => {
    const days = stats.data?.daily_activity;
    if (!days) return undefined;
    const since = Date.now() - 7 * 86_400_000;
    return days
      .filter((d) => new Date(d.date).getTime() >= since)
      .reduce((n, d) => n + d.count, 0);
  })();

  const setParam = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    // Any change to what is listed puts the reader back on the first page.
    if (key !== "page") next.delete("page");
    setParams(next);
  };

  const totalPages = list.data ? Math.ceil(list.data.total / PAGE_SIZE) : 1;
  const isEmpty = !loading && rows && rows.length === 0;

  return (
    <div className="feed-page">
      <PageHeader kicker={longDate(new Date())} title="Videos" />

      <form
        className="cmdline page-x"
        style={{ paddingBottom: 18, maxWidth: isDesktop ? 640 + 80 : undefined }}
        onSubmit={(e) => {
          e.preventDefault();
          const t = url.trim();
          if (t) submit.mutate(t);
        }}
      >
        <input
          className="input"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="Paste a YouTube URL"
          aria-label="YouTube URL"
        />
        <button type="submit" className="btn btn-primary" disabled={submit.isPending || !url.trim()}>
          {submit.isPending ? "Adding…" : "Summarize"}
        </button>
      </form>

      {submit.isError ? (
        <div className="banner">
          <AlertIcon />
          <span>{(submit.error as Error).message}</span>
        </div>
      ) : null}

      <div className="hero">
        <HeroCell label="Videos" value={stats.data?.total_count} />
        <HeroCell label="Last 7 days" value={lastWeek} />
        <HeroCell label="Queued" value={queuedCount} loaded={!!stats.data} />
        <HeroCell label="Failed" value={failedCount} loaded={!!stats.data} accent={failedCount > 0} />
      </div>

      <div className="filters">
        <form
          className="search"
          /* Below 768px the bar does not wrap, so the field must refuse to
             shrink — otherwise four chips squeeze it to a stub. It takes the
             full width and the chips scroll off the right edge. */
          style={{ flex: isDesktop ? "0 1 320px" : "0 0 100%" }}
          onSubmit={(e) => {
            e.preventDefault();
            setParam("q", searchInput.trim());
          }}
        >
          <SearchIcon size={15} />
          <input
            className="input"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            placeholder="Search all summaries"
            aria-label="Search summaries"
          />
        </form>
        {FILTERS.map((f) => (
          <button
            key={f.key || "all"}
            type="button"
            className="chip"
            aria-pressed={!searching && status === f.key}
            onClick={() => {
              setSearchInput("");
              const next = new URLSearchParams();
              if (f.key) next.set("status", f.key);
              setParams(next);
            }}
          >
            {f.label}
          </button>
        ))}
        {searching ? (
          <span className="spacer note" style={{ font: "400 11.5px var(--font-body)", color: "var(--color-neutral-600)" }}>
            {search.data?.results.length ?? 0} for “{query}”
          </span>
        ) : null}
      </div>

      {error ? (
        <div className="banner">
          <AlertIcon />
          <span>{error.message}</span>
        </div>
      ) : null}

      {search.data?.warnings?.map((w) => (
        <div key={w} className="banner">
          <AlertIcon />
          <span>{w}</span>
        </div>
      ))}

      {isEmpty ? (
        <EmptyState searching={searching} query={query} filtered={status !== ""} />
      ) : (
        <FeedList
          rows={rows}
          loading={loading}
          searching={searching}
          variant="video"
          lead={page === 1 && !searching && status === ""}
        />
      )}

      {!searching && list.data && list.data.total > PAGE_SIZE ? (
        <div className="page-x flex items-center gap-4" style={{ paddingTop: 14, paddingBottom: 14 }}>
          <span className="num" style={{ font: "400 12px var(--font-body)", color: "var(--color-neutral-600)" }}>
            Page {page} of {totalPages}
          </span>
          <button
            type="button"
            className="btn btn-secondary ml-auto"
            style={{ fontSize: 12 }}
            disabled={page <= 1}
            onClick={() => setParam("page", String(page - 1))}
          >
            Previous
          </button>
          <button
            type="button"
            className="btn btn-secondary"
            style={{ fontSize: 12 }}
            disabled={offset + PAGE_SIZE >= list.data.total}
            onClick={() => setParam("page", String(page + 1))}
          >
            Next
          </button>
        </div>
      ) : null}

      <div className="footer">
        {failedCount > 0 ? (
          <button
            type="button"
            className="btn btn-ghost"
            style={{ fontSize: 12.5 }}
            disabled={retryAll.isPending}
            onClick={() => retryAll.mutate()}
          >
            Retry all {failedCount} failed →
          </button>
        ) : null}
        {noCaptionsCount > 0 ? (
          <button
            type="button"
            className="btn btn-ghost"
            style={{ fontSize: 12.5 }}
            disabled={transcribeAll.isPending}
            onClick={() => transcribeAll.mutate()}
          >
            Transcribe {noCaptionsCount} without captions →
          </button>
        ) : null}
        {feed.data ? (
          <a className="btn btn-ghost" style={{ fontSize: 12.5 }} href={feed.data.urls.videos}>
            Open the Atom feed →
          </a>
        ) : null}
        <span className="spacer note num">
          {stats.data ? `${stats.data.total_count} summaries` : ""}
        </span>
      </div>

      <Toast message={toast.message} onDismiss={toast.dismiss} />
    </div>
  );
}

function HeroCell({
  label,
  value,
  loaded,
  accent,
}: {
  label: string;
  value?: number;
  loaded?: boolean;
  accent?: boolean;
}) {
  const known = loaded ?? value !== undefined;
  return (
    <div>
      <div className="kick">{label}</div>
      <div className="value" style={accent ? { color: "var(--color-accent)" } : undefined}>
        {known ? value?.toLocaleString() : <Skel w={92} h={44} />}
      </div>
    </div>
  );
}

function EmptyState({
  searching,
  query,
  filtered,
}: {
  searching: boolean;
  query: string;
  filtered: boolean;
}) {
  if (searching) {
    return (
      <div className="empty">
        <div className="kick">Search</div>
        <h3>Nothing matches “{query}”.</h3>
        <p>Hybrid search reads the summary text and its embedding. A narrower phrase usually helps more than a broader one.</p>
        <Link to="/" className="btn btn-secondary">Back to all videos</Link>
      </div>
    );
  }

  if (filtered) {
    return (
      <div className="empty">
        <div className="kick">Library</div>
        <h3>Nothing in this state.</h3>
        <p>No video currently carries the selected status.</p>
        <Link to="/" className="btn btn-secondary">Show all</Link>
      </div>
    );
  }

  return (
    <div className="empty">
      <div className="kick">Library</div>
      <h3>No videos yet.</h3>
      <p>
        Paste a YouTube URL above and the transcript comes back as a readable summary.
        Karakeep webhooks and bulk import live in Settings.
      </p>
      <Link to="/settings?tab=karakeep" className="btn btn-secondary">Set up Karakeep</Link>
    </div>
  );
}
