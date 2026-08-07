import { Fragment, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { Skel } from "./LoadingSkeleton.tsx";
import { formatDuration } from "../utils.ts";
import { clock, excerpt, groupByDay, statusClass, statusLabel } from "../display.ts";

/**
 * The media feed both library screens render: one reading column, artwork on
 * every row, the summary's opening lines as the row's body.
 *
 * It replaces the table and the phone list that stood here before, and it
 * replaces both at once — a feed row is already a stacked layout, so there is
 * no desktop/mobile fork to keep in step. The table was built for a queue, but
 * these are things to watch and listen to, and the two fields that tell you
 * whether you want one were exactly the two a table row could not carry.
 */

export type FeedVariant = "video" | "podcast";

/** One record in the feed. Both pages build this shape from their own source. */
export interface Row {
  id: string;
  title: string;
  channel: string;
  created_at: string;
  source: string;
  status?: string;
  detail_level?: string;
  duration_seconds?: number;
  error_message?: string;
  summary?: string;
  thumbnail_url?: string;
  topics?: string[];
  score?: number;
}

export function detailPath(r: Row): string {
  return r.source === "podcast" ? `/podcast/${r.id}` : `/video/${r.id}`;
}

interface Props {
  rows?: Row[];
  loading: boolean;
  searching: boolean;
  variant: FeedVariant;
  /**
   * Promotes the newest record. The caller passes true only on page 1 of an
   * unfiltered, unsearched list: on page 2 the newest item is not new, and in a
   * filtered list the first hit is not more important than the second.
   */
  lead: boolean;
}

export default function FeedList({ rows, loading, searching, variant, lead }: Props) {
  if (loading) return <FeedSkeleton variant={variant} />;

  const groups = rows ? groupByDay(rows, (r) => r.created_at) : [];

  return (
    <div className="feed" data-variant={variant}>
      {groups.map((g, gi) => (
        <Fragment key={g.key}>
          <div className="feed-band">{g.label}</div>
          {g.items.map((r, ri) => (
            <FeedItem
              key={r.id}
              row={r}
              variant={variant}
              searching={searching}
              lead={lead && gi === 0 && ri === 0}
            />
          ))}
        </Fragment>
      ))}
    </div>
  );
}

function FeedItem({
  row,
  variant,
  searching,
  lead,
}: {
  row: Row;
  variant: FeedVariant;
  searching: boolean;
  lead: boolean;
}) {
  const wide = variant === "video";

  // A completed row shows no status mark — that is the norm, and repeating it
  // several hundred times is what the status column did wrong.
  const status = row.status && row.status !== "completed" ? row.status : undefined;
  const body = row.error_message ?? excerpt(row.summary, row.title);
  const topics = lead ? (row.topics ?? []) : [];

  const art = (
    <div className={`feed-art ${wide ? "is-wide" : "is-square"}${row.thumbnail_url ? "" : " is-empty"}`}>
      {row.thumbnail_url ? (
        <img src={row.thumbnail_url} alt="" loading="lazy" />
      ) : (
        <span className="kick" style={{ fontSize: 9, color: "var(--color-neutral-500)" }}>
          no art
        </span>
      )}
      {wide && row.duration_seconds ? (
        <span className="dur">{formatDuration(row.duration_seconds)}</span>
      ) : null}
    </div>
  );

  const tags =
    topics.length > 0 ? (
      <div className="feed-topics">
        {topics.map((t) => (
          <span key={t} className="tag tag-neutral">
            {t}
          </span>
        ))}
      </div>
    ) : null;

  return (
    <Link to={detailPath(row)} className={`feed-item${lead ? " feed-lead" : ""}`}>
      {art}
      <div className="feed-body">
        <Kicker row={row} variant={variant} searching={searching}>
          {/* On the lead the status joins the kicker: the row is a block or a
              grid there, so there is no right edge to align it to. */}
          {lead && status ? (
            <span className={`status ${statusClass(status)}`}>{statusLabel(status)}</span>
          ) : null}
        </Kicker>
        <h3 className="feed-title">{row.title}</h3>
        {body ? (
          <p className={`feed-excerpt${row.error_message ? " is-error" : ""}`}>{body}</p>
        ) : null}
        {variant === "video" ? tags : null}
      </div>
      {/* The podcast lead is a grid, and its tag row is a direct child so it can
          drop below the cover at 390px. */}
      {variant === "podcast" ? tags : null}
      {!lead && status ? (
        <span className={`status ${statusClass(status)}`} style={{ marginLeft: "auto", flex: "none" }}>
          {statusLabel(status)}
        </span>
      ) : null}
      {!lead && searching && row.score !== undefined ? (
        <span
          className="num"
          style={{
            marginLeft: "auto",
            flex: "none",
            fontSize: 12.5,
            color: "var(--color-neutral-600)",
          }}
        >
          {row.score.toFixed(2)}
        </span>
      ) : null}
    </Link>
  );
}

/** Channel · time or length · detail level, `·` separated. */
function Kicker({
  row,
  variant,
  searching,
  children,
}: {
  row: Row;
  variant: FeedVariant;
  searching: boolean;
  children?: ReactNode;
}) {
  const parts = [
    row.channel,
    variant === "podcast"
      ? row.duration_seconds
        ? formatDuration(row.duration_seconds)
        : null
      : clock(row.created_at),
    row.detail_level,
  ].filter((p): p is string => !!p);

  return (
    <div className="kick flex items-center gap-2" style={{ flexWrap: "wrap" }}>
      {parts.map((part, i) => (
        <Fragment key={`${i}-${part}`}>
          {i > 0 ? <span aria-hidden>·</span> : null}
          {/* The show is the strongest grouping signal on the podcast screen. */}
          <span style={i === 0 && variant === "podcast" ? { color: "var(--color-accent)" } : undefined}>
            {part}
          </span>
        </Fragment>
      ))}
      {/* Search spans both content types, so the row says which one it is. */}
      {searching ? <span className="tag tag-neutral">{row.source}</span> : null}
      {children}
    </div>
  );
}

/**
 * Placeholders keep the row geometry — the artwork block is the artwork's exact
 * size — so nothing reflows when the data lands or when a poll returns.
 */
function FeedSkeleton({ variant }: { variant: FeedVariant }) {
  const wide = variant === "video";

  return (
    <div className="feed" data-variant={variant}>
      {Array.from({ length: 6 }, (_, i) => (
        <div key={i} className="feed-item">
          <div className={`feed-art ${wide ? "is-wide" : "is-square"} is-empty`} />
          <div className="feed-body">
            <Skel w={128} h={10} />
            <div style={{ marginTop: 8 }}>
              <Skel w={`${82 - (i % 3) * 13}%`} h={18} />
            </div>
            <div style={{ marginTop: 8 }}>
              <Skel w={`${68 - (i % 2) * 14}%`} h={12} />
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
