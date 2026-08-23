import { useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchVideoFacets, type ChannelFacet, type ContentSource } from "../api.ts";

/** How many topic chips show before the rest fold behind "More". */
const TOPIC_LIMIT = 8;

interface FacetProps {
  source: ContentSource;
  channel: string;
  topic: string;
  onChange: (next: { channel: string; topic: string }) => void;
}

/* One request feeds both the rail and the chips: same key, same cache entry. */
function useFacets(source: ContentSource) {
  return useQuery({
    queryKey: ["facets", source],
    queryFn: () => fetchVideoFacets(source),
    staleTime: 60_000,
    retry: false,
  });
}

/* One-shot channels fold behind "Others" so the list carries the channels worth
   navigating to. A selected one-shot stays visible while folded. */
function foldChannels(channels: ChannelFacet[], selected: string, open: boolean) {
  const main = channels.filter((c) => c.count > 1);
  const single = channels.filter((c) => c.count <= 1);
  const shown =
    open || main.length === 0
      ? [...main, ...single]
      : [...main, ...single.filter((c) => c.channel === selected)];
  return { shown, single, foldable: main.length > 0 && single.length > 0 };
}

/** One entry of the channel strip: artwork (or the initial as a stand-in),
 * name, count. Clicking toggles the filter. */
function ChannelTile({
  facet,
  selected,
  onClick,
}: {
  facet: ChannelFacet;
  selected: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className="channel-tile"
      aria-pressed={selected}
      title={`${facet.channel} · ${facet.count} in library`}
      onClick={onClick}
    >
      {facet.thumbnail_url ? (
        // no-referrer: yt3.googleusercontent answers a CORB-blocked non-image
        // when the request carries a foreign referrer (observed 2026-08-23).
        <img
          className="channel-art"
          src={facet.thumbnail_url}
          alt=""
          loading="lazy"
          referrerPolicy="no-referrer"
        />
      ) : (
        <span className="channel-art channel-art-initial" aria-hidden>
          {facet.channel.slice(0, 1).toUpperCase()}
        </span>
      )}
      <span className="channel-name">{facet.channel}</span>
      <span className="channel-count num">{facet.count}</span>
    </button>
  );
}

/**
 * The library's left column from 1280px up: the inbox teaser, then the channel
 * rail as a card. It is a sibling of the feed rather than a child of the filter
 * bar, because the Shelf system gives the rail a column of its own instead of
 * the fixed position in the page margin the Modernist rail had — at
 * `--reading-w: 1120px` that margin is 80px and no longer holds a 244px card.
 *
 * Below 1280px `.rail-col` is display:none and `ChannelStrip` takes over.
 */
export function ChannelRail({
  source,
  channel,
  topic,
  onChange,
  header,
}: FacetProps & {
  /** Rendered above the rail, in the same column — the inbox teaser. */
  header?: ReactNode;
}) {
  const [othersOpen, setOthersOpen] = useState(false);
  const facets = useFacets(source);

  const channels = facets.data?.channels ?? [];
  if (channels.length < 2) return null;

  const { shown, single, foldable } = foldChannels(channels, channel, othersOpen);

  return (
    <div className="rail-col">
      {header}
      <aside className="channel-rail" role="group" aria-label="Filter by channel">
        <div className="kick" style={{ marginBottom: 8 }}>
          Channels
        </div>
        <button
          type="button"
          className="channel-row"
          aria-pressed={channel === ""}
          onClick={() => onChange({ channel: "", topic })}
        >
          <span className="channel-row-art" aria-hidden>
            ∗
          </span>
          <span className="channel-row-name">All channels</span>
        </button>
        {shown.map((c) => (
          <button
            key={c.channel}
            type="button"
            className="channel-row"
            aria-pressed={channel === c.channel}
            title={`${c.channel} · ${c.count} in library`}
            onClick={() => onChange({ channel: channel === c.channel ? "" : c.channel, topic })}
          >
            {c.thumbnail_url ? (
              <img
                className="channel-row-art"
                src={c.thumbnail_url}
                alt=""
                loading="lazy"
                referrerPolicy="no-referrer"
              />
            ) : (
              <span className="channel-row-art" aria-hidden>
                {c.channel.slice(0, 1).toUpperCase()}
              </span>
            )}
            <span className="channel-row-name">{c.channel}</span>
            <span className="channel-row-count num">{c.count}</span>
          </button>
        ))}
        {foldable ? (
          <button
            type="button"
            className="channel-row channel-row-others"
            aria-expanded={othersOpen}
            onClick={() => setOthersOpen((o) => !o)}
          >
            <span className="channel-row-art" aria-hidden>
              {othersOpen ? "−" : "+"}
            </span>
            <span className="channel-row-name">
              {othersOpen ? "Fewer" : `Others (${single.length})`}
            </span>
          </button>
        ) : null}
      </aside>
    </div>
  );
}

/**
 * The library's navigation row: the horizontal channel strip (below 1280px,
 * where the rail is hidden) and the LLM topic chips — both fed by the facets
 * endpoint. The values are the stored column values, so the caller filters with
 * `channelExact` — never the ILIKE partial match.
 *
 * Renders nothing while loading, on error, or when there is only one value to
 * choose from: facets are navigation sugar, not content, and a bar with one
 * option is noise.
 */
export default function FacetFilters({ source, channel, topic, onChange }: FacetProps) {
  const [expanded, setExpanded] = useState(false);
  const [othersOpen, setOthersOpen] = useState(false);
  const facets = useFacets(source);

  const channels = facets.data?.channels ?? [];
  const topics = facets.data?.topics ?? [];
  if (channels.length < 2 && topics.length < 2) return null;

  const { shown, single, foldable } = foldChannels(channels, channel, othersOpen);

  // The selected topic stays visible even when it sits below the fold.
  const visibleTopics = expanded ? topics : topics.slice(0, TOPIC_LIMIT);
  const selectedHidden = topic && !visibleTopics.some((t) => t.topic === topic);
  const shownTopics = selectedHidden
    ? [...visibleTopics, ...topics.filter((t) => t.topic === topic)]
    : visibleTopics;

  const toggleChannel = (name: string) =>
    onChange({ channel: channel === name ? "" : name, topic });

  return (
    <div className="filters" style={{ flexWrap: "wrap", rowGap: 8 }}>
      {channels.length >= 2 ? (
        <div className="channel-strip" role="group" aria-label="Filter by channel">
          {shown.map((c) => (
            <ChannelTile
              key={c.channel}
              facet={c}
              selected={channel === c.channel}
              onClick={() => toggleChannel(c.channel)}
            />
          ))}
          {foldable ? (
            <button
              type="button"
              className="channel-tile"
              aria-expanded={othersOpen}
              onClick={() => setOthersOpen((o) => !o)}
            >
              <span className="channel-art channel-art-initial" aria-hidden>
                {othersOpen ? "−" : `+${single.length}`}
              </span>
              <span className="channel-name">{othersOpen ? "Fewer" : "Others"}</span>
            </button>
          ) : null}
        </div>
      ) : null}

      {shownTopics.map((t) => (
        <button
          key={t.topic}
          type="button"
          className="chip"
          aria-pressed={topic === t.topic}
          onClick={() => onChange({ channel, topic: topic === t.topic ? "" : t.topic })}
        >
          {t.topic} ({t.count})
        </button>
      ))}
      {topics.length > TOPIC_LIMIT ? (
        <button
          type="button"
          className="btn btn-ghost"
          style={{ fontSize: 12 }}
          onClick={() => setExpanded((e) => !e)}
        >
          {expanded ? "Fewer" : `More (${topics.length - TOPIC_LIMIT})`}
        </button>
      ) : null}
    </div>
  );
}
