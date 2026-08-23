import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchVideoFacets, type ContentSource } from "../api.ts";

/** How many topic chips show before the rest fold behind "More". */
const TOPIC_LIMIT = 8;

/**
 * The library's navigation row: a channel select and the LLM topic chips,
 * fed by the facets endpoint. The values are the stored column values, so the
 * caller filters with `channelExact` — never the ILIKE partial match.
 *
 * The row renders nothing while loading, on error, or when there is only one
 * value to choose from: facets are navigation sugar, not content, and a bar
 * with one option is noise.
 */
export default function FacetFilters({
  source,
  channel,
  topic,
  onChange,
}: {
  source: ContentSource;
  channel: string;
  topic: string;
  onChange: (next: { channel: string; topic: string }) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const facets = useQuery({
    queryKey: ["facets", source],
    queryFn: () => fetchVideoFacets(source),
    staleTime: 60_000,
    retry: false,
  });

  const channels = facets.data?.channels ?? [];
  const topics = facets.data?.topics ?? [];
  if (channels.length < 2 && topics.length < 2) return null;

  // The selected topic stays visible even when it sits below the fold.
  const visibleTopics = expanded ? topics : topics.slice(0, TOPIC_LIMIT);
  const selectedHidden = topic && !visibleTopics.some((t) => t.topic === topic);
  const shownTopics = selectedHidden
    ? [...visibleTopics, ...topics.filter((t) => t.topic === topic)]
    : visibleTopics;

  return (
    <div className="filters" style={{ flexWrap: "wrap", rowGap: 8 }}>
      {channels.length >= 2 ? (
        <select
          className="select"
          style={{ width: "auto", maxWidth: 260 }}
          value={channel}
          aria-label="Filter by channel"
          onChange={(e) => onChange({ channel: e.target.value, topic })}
        >
          <option value="">All channels</option>
          {channels.map((c) => (
            <option key={c.channel} value={c.channel}>
              {c.channel} ({c.count})
            </option>
          ))}
        </select>
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
