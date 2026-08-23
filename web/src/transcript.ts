// Pure transcript-player logic, kept out of the components so it is testable
// the way display.ts is.

import type { TranscriptSegment } from "./api.ts";

/** Index of the segment playing at `time`: the last segment whose start is at
 * or before it, -1 before the first. Binary search — the caller polls this
 * several times a second over thousands of segments. */
export function findActiveIndex(segments: TranscriptSegment[], time: number): number {
  let lo = 0;
  let hi = segments.length - 1;
  let best = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (segments[mid].s <= time) {
      best = mid;
      lo = mid + 1;
    } else {
      hi = mid - 1;
    }
  }
  return best;
}

/** Indices of the segments whose text contains the query, case-insensitive.
 * An empty or whitespace query matches nothing rather than everything. */
export function searchSegments(segments: TranscriptSegment[], query: string): number[] {
  const q = query.trim().toLowerCase();
  if (!q) return [];
  const hits: number[] = [];
  for (let i = 0; i < segments.length; i++) {
    if (segments[i].t.toLowerCase().includes(q)) hits.push(i);
  }
  return hits;
}

/** `m:ss` under an hour, `h:mm:ss` above — the tabular form the .cue time
 * column expects, unlike formatDuration's prose form ("1h 2m"). */
export function formatTimestamp(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const ss = String(s).padStart(2, "0");
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${ss}`;
  return `${m}:${ss}`;
}
