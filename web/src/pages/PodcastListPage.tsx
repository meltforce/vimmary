import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { fetchFeedInfo, fetchStats, listVideos, searchVideos } from "../api.ts";
import PageHeader from "../components/PageHeader.tsx";
import FeedList, { type Row } from "../components/FeedList.tsx";
import FacetFilters from "../components/FacetFilters.tsx";
import { Skel } from "../components/LoadingSkeleton.tsx";
import { AlertIcon, SearchIcon } from "../components/icons.tsx";
import { useIsDesktop } from "../hooks/useMediaQuery.ts";
import { isInFlight, longDate } from "../display.ts";

const PAGE_SIZE = 20;

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
  const channel = params.get("channel") ?? "";
  const topic = params.get("topic") ?? "";
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
    queryKey: ["podcasts", channel, topic, offset],
    queryFn: () =>
      listVideos({
        source: "podcast",
        channelExact: channel || undefined,
        topic: topic || undefined,
        limit: PAGE_SIZE,
        offset,
      }),
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
        source: m.source,
        summary: m.summary,
        topics: m.metadata?.topics,
        score: m.score,
      }))
    : list.data?.videos.map((v) => ({
        id: v.id,
        title: v.title,
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
      }));

  const loading = searching ? search.isLoading : list.isLoading;
  const error = (searching ? search.error : list.error) as Error | null;
  const totalPages = list.data ? Math.ceil(list.data.total / PAGE_SIZE) : 1;

  const setPage = (n: number) => {
    const next = new URLSearchParams(params);
    next.set("page", String(n));
    setParams(next);
  };

  const hours = (stats.data?.total_duration_seconds ?? 0) / 3600;

  return (
    <div className="feed-page">
      <PageHeader kicker={longDate(new Date())} title="Podcasts" />

      {/* Two cells, side by side at every width — the strip's default is four. */}
      <div className="hero" style={{ gridTemplateColumns: "1fr 1fr" }}>
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
          style={{ flex: isDesktop ? "0 1 320px" : "0 0 100%" }}
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

      {!searching ? (
        <FacetFilters
          source="podcast"
          channel={channel}
          topic={topic}
          onChange={(next) => {
            const p = new URLSearchParams(params);
            if (next.channel) p.set("channel", next.channel);
            else p.delete("channel");
            if (next.topic) p.set("topic", next.topic);
            else p.delete("topic");
            p.delete("page");
            setParams(p);
          }}
        />
      ) : null}

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
      ) : (
        <FeedList
          rows={rows}
          loading={loading}
          searching={searching}
          variant="podcast"
          lead={page === 1 && !searching && channel === "" && topic === ""}
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
    </div>
  );
}
