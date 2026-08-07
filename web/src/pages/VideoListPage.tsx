import { Fragment, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
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
import { Skel } from "../components/LoadingSkeleton.tsx";
import { AlertIcon, SearchIcon } from "../components/icons.tsx";
import { useIsDesktop } from "../hooks/useMediaQuery.ts";
import { formatDuration } from "../utils.ts";
import {
  clock,
  groupByDay,
  isInFlight,
  longDate,
  statusClass,
  statusLabel,
} from "../display.ts";

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

/** The list and the search results are rendered by one table. */
interface Row {
  id: string;
  title: string;
  channel: string;
  created_at: string;
  source: string;
  status?: string;
  detail_level?: string;
  duration_seconds?: number;
  error_message?: string;
  score?: number;
}

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
  };
}

function matchToRow(m: HybridMatch): Row {
  return {
    id: m.id,
    title: m.title || m.youtube_id,
    channel: m.channel,
    created_at: m.created_at,
    source: m.source,
    score: m.score,
  };
}

export default function VideoListPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
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
    <>
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
          style={{ flex: isDesktop ? "0 1 320px" : "1 1 100%" }}
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
      ) : isDesktop ? (
        <RowTable rows={rows} loading={loading} searching={searching} onOpen={(r) => navigate(detailPath(r))} />
      ) : (
        <RowList rows={rows} loading={loading} searching={searching} />
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
    </>
  );
}

function detailPath(r: Row): string {
  return r.source === "podcast" ? `/podcast/${r.id}` : `/video/${r.id}`;
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

/* Desktop. The head, the rules and the day bands paint before the data does;
   only the values are placeholders, so nothing reflows on arrival. */
function RowTable({
  rows,
  loading,
  searching,
  onOpen,
}: {
  rows?: Row[];
  loading: boolean;
  searching: boolean;
  onOpen: (row: Row) => void;
}) {
  const groups = rows ? groupByDay(rows, (r) => r.created_at) : [];

  return (
    <table className="table">
      <thead>
        <tr>
          <th>Video</th>
          <th style={{ width: 200 }}>Channel</th>
          <th style={{ width: 96 }}>Detail</th>
          <th style={{ width: 130 }}>{searching ? "Match" : "Status"}</th>
          <th className="right" style={{ width: 90 }}>Length</th>
          <th className="right" style={{ width: 110 }}>{searching ? "Score" : "Added"}</th>
        </tr>
      </thead>
      <tbody>
        {loading
          ? Array.from({ length: 8 }, (_, i) => (
              <tr key={i}>
                <td><Skel w={`${72 - (i % 4) * 9}%`} /></td>
                <td><Skel w={110} /></td>
                <td><Skel w={48} /></td>
                <td><Skel w={64} h={18} /></td>
                <td className="right"><Skel w={44} /></td>
                <td className="right"><Skel w={62} /></td>
              </tr>
            ))
          : groups.map((g) => (
              <Fragment key={g.key}>
                <tr className="grp">
                  <td colSpan={6} className="kick">{g.label}</td>
                </tr>
                {g.items.map((r) => (
                  <tr key={r.id} style={{ cursor: "pointer" }} onClick={() => onOpen(r)}>
                    <td style={{ fontWeight: 500 }}>
                      <Link to={detailPath(r)} style={{ color: "inherit" }}>{r.title}</Link>
                      {r.error_message ? (
                        <div style={{ font: "400 11.5px var(--font-body)", color: "var(--color-neutral-600)", marginTop: 3 }}>
                          {r.error_message}
                        </div>
                      ) : null}
                    </td>
                    <td style={{ color: "var(--color-neutral-700)" }}>{r.channel}</td>
                    <td>{r.detail_level ? <span className="tag tag-neutral">{r.detail_level}</span> : null}</td>
                    <td>
                      {r.status ? (
                        <span className={`status ${statusClass(r.status)}`}>{statusLabel(r.status)}</span>
                      ) : (
                        <span className="tag tag-neutral">{r.source}</span>
                      )}
                    </td>
                    <td className="num right">
                      {r.duration_seconds ? formatDuration(r.duration_seconds) : ""}
                    </td>
                    <td className="num right" style={{ fontSize: 12.5, color: "var(--color-neutral-600)" }}>
                      {r.score !== undefined ? r.score.toFixed(2) : clock(r.created_at)}
                    </td>
                  </tr>
                ))}
              </Fragment>
            ))}
      </tbody>
    </table>
  );
}

/* Phone. One row is one target: the whole row opens the detail page, and the
   per-row actions live there rather than as five buttons under a thumbnail. */
function RowList({ rows, loading, searching }: { rows?: Row[]; loading: boolean; searching: boolean }) {
  if (loading) {
    return (
      <div style={{ borderTop: "var(--rule-strong)" }}>
        {Array.from({ length: 6 }, (_, i) => (
          <div key={i} className="row">
            <div style={{ flex: 1, minWidth: 0 }}>
              <Skel w={`${74 - (i % 3) * 11}%`} />
              <div style={{ marginTop: 5 }}><Skel w={128} h={11} /></div>
            </div>
            <Skel w={52} h={18} />
          </div>
        ))}
      </div>
    );
  }

  const groups = rows ? groupByDay(rows, (r) => r.created_at) : [];

  return (
    <div style={{ borderTop: "var(--rule-strong)" }}>
      {groups.map((g) => (
        <Fragment key={g.key}>
          <div className="kick row-group">{g.label}</div>
          {g.items.map((r) => (
            <Link key={r.id} to={detailPath(r)} className="row" style={{ color: "inherit", minHeight: 44 }}>
              <span style={{ flex: 1, minWidth: 0 }}>
                <span className="row-title" style={{ display: "block" }}>{r.title}</span>
                <span className="row-meta" style={{ display: "block" }}>
                  {r.error_message ??
                    [
                      r.channel,
                      r.duration_seconds ? formatDuration(r.duration_seconds) : null,
                      r.detail_level,
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                </span>
              </span>
              {r.status ? (
                <span className={`status ${statusClass(r.status)}`}>{statusLabel(r.status)}</span>
              ) : searching && r.score !== undefined ? (
                <span className="num row-value" style={{ fontSize: 12.5, color: "var(--color-neutral-600)" }}>
                  {r.score.toFixed(2)}
                </span>
              ) : null}
            </Link>
          ))}
        </Fragment>
      ))}
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
