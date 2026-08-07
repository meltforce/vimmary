import { Fragment, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { fetchFeedInfo, fetchStats, listVideos, searchVideos } from "../api.ts";
import PageHeader from "../components/PageHeader.tsx";
import { Skel } from "../components/LoadingSkeleton.tsx";
import { AlertIcon, SearchIcon } from "../components/icons.tsx";
import { useIsDesktop } from "../hooks/useMediaQuery.ts";
import { formatDuration } from "../utils.ts";
import { clock, groupByDay, isInFlight, longDate, statusClass, statusLabel } from "../display.ts";

const PAGE_SIZE = 20;

/** One shape for both the list and the search results. */
interface Row {
  id: string;
  title: string;
  channel: string;
  created_at: string;
  score?: number;
  status?: string;
  detail_level?: string;
  duration_seconds?: number;
  error_message?: string;
}

/**
 * The podcast half of the library. It mirrors the video list minus the two
 * things that do not apply: there is no URL to paste — episodes arrive from a
 * subscribed feed or from cast2md's deep link — and no bulk retry, because the
 * failure modes are cast2md's rather than YouTube's.
 */
export default function PodcastListPage() {
  const isDesktop = useIsDesktop();
  const [params, setParams] = useSearchParams();
  const query = params.get("q") ?? "";
  const page = Math.max(1, parseInt(params.get("page") ?? "1", 10));
  const offset = (page - 1) * PAGE_SIZE;
  const [searchInput, setSearchInput] = useState(query);

  const searching = query.length > 0;

  const search = useQuery({
    queryKey: ["search", "podcast", query],
    queryFn: () => searchVideos(query, 20, "podcast"),
    enabled: searching,
  });

  const list = useQuery({
    queryKey: ["podcasts", offset],
    queryFn: () => listVideos({ source: "podcast", limit: PAGE_SIZE, offset }),
    enabled: !searching,
    refetchInterval: (q) =>
      q.state.data?.videos.some((v) => isInFlight(v.status)) ? 3000 : 10000,
  });

  const stats = useQuery({
    queryKey: ["stats", "podcast"],
    queryFn: () => fetchStats("podcast"),
    refetchInterval: 10000,
  });
  const feed = useQuery({ queryKey: ["settings", "feed"], queryFn: fetchFeedInfo });

  const rows: Row[] | undefined = searching
    ? search.data?.results.map((m) => ({
        id: m.id,
        title: m.title,
        channel: m.channel,
        created_at: m.created_at,
        score: m.score,
      }))
    : list.data?.videos.map((v) => ({
        id: v.id,
        title: v.title,
        channel: v.channel,
        created_at: v.created_at,
        status: v.status,
        detail_level: v.detail_level,
        duration_seconds: v.duration_seconds,
        error_message: v.error_message,
      }));

  const loading = searching ? search.isLoading : list.isLoading;
  const error = (searching ? search.error : list.error) as Error | null;
  const groups = rows ? groupByDay(rows, (r) => r.created_at) : [];
  const totalPages = list.data ? Math.ceil(list.data.total / PAGE_SIZE) : 1;

  const setPage = (n: number) => {
    const next = new URLSearchParams(params);
    next.set("page", String(n));
    setParams(next);
  };

  const hours = (stats.data?.total_duration_seconds ?? 0) / 3600;

  return (
    <>
      <PageHeader kicker={longDate(new Date())} title="Podcasts" />

      <div className="hero">
        <div>
          <div className="kick">Episodes</div>
          <div className="value">
            {stats.data ? stats.data.total_count.toLocaleString() : <Skel w={92} h={44} />}
          </div>
        </div>
        <div>
          <div className="kick">Runtime</div>
          <div className="value">
            {stats.data ? (
              <>
                {hours >= 1 ? hours.toFixed(0) : Math.round(hours * 60)}
                <span className="unit">{hours >= 1 ? "hours" : "min"}</span>
              </>
            ) : (
              <Skel w={92} h={44} />
            )}
          </div>
        </div>
      </div>

      <div className="filters">
        <form
          className="search"
          style={{ flex: isDesktop ? "0 1 320px" : "1 1 100%" }}
          onSubmit={(e) => {
            e.preventDefault();
            const next = new URLSearchParams();
            if (searchInput.trim()) next.set("q", searchInput.trim());
            setParams(next);
          }}
        >
          <SearchIcon size={15} />
          <input
            className="input"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            placeholder="Search episode summaries"
            aria-label="Search episode summaries"
          />
        </form>
        {searching ? (
          <button
            type="button"
            className="chip"
            onClick={() => {
              setSearchInput("");
              setParams(new URLSearchParams());
            }}
          >
            Clear
          </button>
        ) : null}
      </div>

      {error ? (
        <div className="banner">
          <AlertIcon />
          <span>{error.message}</span>
        </div>
      ) : null}

      {!loading && rows && rows.length === 0 ? (
        <div className="empty">
          <div className="kick">Podcasts</div>
          <h3>{searching ? `Nothing matches “${query}”.` : "No episodes yet."}</h3>
          <p>
            {searching
              ? "Only podcast summaries are searched here; videos have their own list."
              : "Subscribe to a feed under Settings → Podcasts, or use the “Summarize in vimmary” button in cast2md."}
          </p>
          <Link
            to={searching ? "/podcasts" : "/settings?tab=podcasts"}
            className="btn btn-secondary"
          >
            {searching ? "Show all episodes" : "Open podcast settings"}
          </Link>
        </div>
      ) : isDesktop ? (
        <table className="table">
          <thead>
            <tr>
              <th>Episode</th>
              <th style={{ width: 220 }}>Show</th>
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
                    <td><Skel w={`${70 - (i % 4) * 8}%`} /></td>
                    <td><Skel w={120} /></td>
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
                      <tr key={r.id}>
                        <td style={{ fontWeight: 500 }}>
                          <Link to={`/podcast/${r.id}`} style={{ color: "inherit" }}>
                            {r.title}
                          </Link>
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
                            <span className={`status ${statusClass(r.status)}`}>
                              {statusLabel(r.status)}
                            </span>
                          ) : null}
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
      ) : (
        <div style={{ borderTop: "var(--rule-strong)" }}>
          {loading
            ? Array.from({ length: 6 }, (_, i) => (
                <div key={i} className="row">
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <Skel w={`${74 - (i % 3) * 11}%`} />
                    <div style={{ marginTop: 5 }}><Skel w={128} h={11} /></div>
                  </div>
                  <Skel w={52} h={18} />
                </div>
              ))
            : groups.map((g) => (
                <Fragment key={g.key}>
                  <div className="kick row-group">{g.label}</div>
                  {g.items.map((r) => (
                    <Link
                      key={r.id}
                      to={`/podcast/${r.id}`}
                      className="row"
                      style={{ color: "inherit", minHeight: 44 }}
                    >
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
                        <span className={`status ${statusClass(r.status)}`}>
                          {statusLabel(r.status)}
                        </span>
                      ) : null}
                    </Link>
                  ))}
                </Fragment>
              ))}
        </div>
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
            onClick={() => setPage(page - 1)}
          >
            Previous
          </button>
          <button
            type="button"
            className="btn btn-secondary"
            style={{ fontSize: 12 }}
            disabled={offset + PAGE_SIZE >= list.data.total}
            onClick={() => setPage(page + 1)}
          >
            Next
          </button>
        </div>
      ) : null}

      <div className="footer">
        <Link className="btn btn-ghost" style={{ fontSize: 12.5 }} to="/settings?tab=podcasts">
          Manage subscriptions →
        </Link>
        {feed.data ? (
          <a className="btn btn-ghost" style={{ fontSize: 12.5 }} href={feed.data.urls.podcasts}>
            Open the Atom feed →
          </a>
        ) : null}
        <span className="spacer note num">
          {stats.data ? `${stats.data.total_count} episodes` : ""}
        </span>
      </div>
    </>
  );
}
