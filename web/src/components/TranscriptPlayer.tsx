import { useEffect, useMemo, useRef, useState } from "react";
import type { TranscriptSegment } from "../api.ts";
import { findActiveIndex, formatTimestamp, searchSegments } from "../transcript.ts";
import { useYouTubePlayer } from "../hooks/useYouTubePlayer.ts";

/** How long a manual scroll suppresses the follow-along auto-scroll. */
const SCROLL_GRACE_MS = 3000;

/** Wraps the query matches in <mark> so a hit is visible inside its line. */
function highlight(text: string, query: string) {
  const q = query.trim();
  if (!q) return text;
  const lower = text.toLowerCase();
  const needle = q.toLowerCase();
  const parts: React.ReactNode[] = [];
  let pos = 0;
  for (let at = lower.indexOf(needle); at !== -1; at = lower.indexOf(needle, pos)) {
    if (at > pos) parts.push(text.slice(pos, at));
    parts.push(<mark key={at}>{text.slice(at, at + needle.length)}</mark>);
    pos = at + needle.length;
  }
  if (pos === 0) return text;
  parts.push(text.slice(pos));
  return parts;
}

export default function TranscriptPlayer({
  youtubeId,
  segments,
  seekTarget,
  onSeekHandled,
}: {
  youtubeId: string;
  segments: TranscriptSegment[];
  /** Seconds another view (the chapters tab) wants the player to jump to. */
  seekTarget: number | null;
  onSeekHandled: () => void;
}) {
  const { containerRef, ready, playing, currentTime, embedError, seekTo } =
    useYouTubePlayer(youtubeId);

  const [query, setQuery] = useState("");
  const [hitPos, setHitPos] = useState(0);
  const paneRef = useRef<HTMLDivElement>(null);
  const rowRefs = useRef<(HTMLDivElement | null)[]>([]);
  const userScrolledAt = useRef(0);

  const activeIndex = findActiveIndex(segments, currentTime);
  const hits = useMemo(() => searchSegments(segments, query), [segments, query]);
  const currentHit = hits.length ? hits[Math.min(hitPos, hits.length - 1)] : -1;

  useEffect(() => setHitPos(0), [query]);

  // Follow playback unless the reader is scrolling on their own.
  useEffect(() => {
    if (!playing || activeIndex < 0) return;
    if (Date.now() - userScrolledAt.current < SCROLL_GRACE_MS) return;
    rowRefs.current[activeIndex]?.scrollIntoView({ block: "nearest" });
  }, [activeIndex, playing]);

  useEffect(() => {
    if (seekTarget === null || !ready) return;
    seekTo(seekTarget);
    onSeekHandled();
  }, [seekTarget, ready, seekTo, onSeekHandled]);

  const jumpToHit = (pos: number) => {
    if (!hits.length) return;
    const next = ((pos % hits.length) + hits.length) % hits.length;
    setHitPos(next);
    userScrolledAt.current = 0;
    rowRefs.current[hits[next]]?.scrollIntoView({ block: "center" });
  };

  const markScrolled = () => {
    userScrolledAt.current = Date.now();
  };

  return (
    <div className="player-grid">
      {/* Video and search pin together while the transcript scrolls under
          them. */}
      <div className="player-head">
        {embedError ? (
          <div className="empty" style={{ padding: "32px 0" }}>
            <div className="kick">Player unavailable</div>
            <h3>This video cannot be embedded.</h3>
            <p>The creator disabled external playback, or the player failed to load.</p>
            <a
              className="btn btn-secondary"
              href={`https://youtube.com/watch?v=${youtubeId}`}
              target="_blank"
              rel="noopener noreferrer"
            >
              Watch on YouTube
            </a>
          </div>
        ) : (
          <div className="player-frame">
            <div ref={containerRef} />
          </div>
        )}

      </div>

      {/* The transcript column: its own label, its own search field, then the
          cues. Side by side with the video above 1100px, stacked under it
          below — either way the search field stays with the transcript it
          searches and never scrolls out of reach. */}
      <div className="player-side">
        {/* The search field belongs to the transcript, not to the video, so it
            sits on the paper ground under the transcript's own label rather
            than inside the dark player block. */}
        <div className="player-searchhead">
          <span className="kick">Transcript</span>
          <span className="count">{segments.length.toLocaleString()} lines</span>
        </div>

        <form
          className="player-search"
          onSubmit={(e) => {
            e.preventDefault();
            jumpToHit(hitPos + (hits.length && currentHit >= 0 ? 1 : 0));
          }}
        >
          <input
            className="input"
            type="search"
            placeholder="Search transcript"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") {
                e.preventDefault();
                jumpToHit(hitPos + 1);
              } else if (e.key === "ArrowUp") {
                e.preventDefault();
                jumpToHit(hitPos - 1);
              }
            }}
          />
          {query.trim() ? (
            <span className="mono player-hits">
              {hits.length ? `${Math.min(hitPos + 1, hits.length)}/${hits.length}` : "0/0"}
            </span>
          ) : null}
        </form>

      <div className="transcript player-pane" ref={paneRef} onWheel={markScrolled} onTouchMove={markScrolled}>
          {segments.map((seg, i) => (
            <div
              key={i}
              ref={(el) => {
                rowRefs.current[i] = el;
              }}
              className={`cue${i === activeIndex ? " cue-active" : ""}${i === currentHit ? " cue-hit" : ""}`}
              onClick={() => {
                if (embedError) return;
                seekTo(seg.s);
              }}
            >
              {embedError ? (
                <time>
                  <a
                    href={`https://youtube.com/watch?v=${youtubeId}&t=${Math.floor(seg.s)}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    style={{ color: "inherit" }}
                  >
                    {formatTimestamp(seg.s)}
                  </a>
                </time>
              ) : (
                <time>{formatTimestamp(seg.s)}</time>
              )}
              <div className="player-cue-text">{highlight(seg.t, query)}</div>
            </div>
          ))}
      </div>
      </div>
    </div>
  );
}
