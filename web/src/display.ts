/**
 * Presentation helpers shared by the list, detail and stats screens. Status is
 * carried by weight and outline, never by hue — the palette is mono, so there
 * is no green/amber/red here.
 */

/** The `.status` modifier for a video row's processing state. */
export function statusClass(status: string): string {
  switch (status) {
    case "completed":
      return "status-done";
    case "processing":
      return "status-running";
    case "pending":
      return "status-queued";
    default:
      // failed, no_captions and anything the server adds later.
      return "status-failed";
  }
}

export function statusLabel(status: string): string {
  switch (status) {
    case "completed":
      return "done";
    case "processing":
      return "running";
    case "pending":
      return "queued";
    case "no_captions":
      return "no captions";
    case "failed":
      return "failed";
    default:
      return status;
  }
}

/** True while the row is still moving and the list should keep polling. */
export function isInFlight(status: string): boolean {
  return status === "pending" || status === "processing";
}

const DAY = 86_400_000;

/** The key rows group by: one band per calendar day. */
export function dayKey(iso: string): string {
  const d = new Date(iso);
  return `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`;
}

/** `Today`, `Yesterday`, then the date itself. */
export function dayLabel(iso: string): string {
  const d = new Date(iso);
  const midnight = new Date();
  midnight.setHours(0, 0, 0, 0);
  const delta = midnight.getTime() - new Date(d).setHours(0, 0, 0, 0);
  if (delta === 0) return "Today";
  if (delta === DAY) return "Yesterday";
  return d.toLocaleDateString(undefined, {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: d.getFullYear() === midnight.getFullYear() ? undefined : "numeric",
  });
}

export function shortDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    day: "numeric",
    month: "short",
  });
}

export function longDate(d: Date): string {
  return d.toLocaleDateString(undefined, {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
  });
}

export function clock(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

/**
 * The first prose of a summary, for a feed row. Strips the Markdown scaffolding
 * and the restated title most summaries open with, so the excerpt starts at the
 * first sentence that says something.
 *
 * The paragraph is returned whole and the CSS clamp does the truncating, so the
 * line count follows the column width rather than a guessed character count.
 */
export function excerpt(summary: string | undefined, title: string): string {
  if (!summary) return "";

  // Fenced blocks go first: a fence may contain anything, including lines that
  // would otherwise read as a heading or as prose. Tracked as state rather than
  // matched as one expression, because an unclosed fence runs to the end.
  const lines: string[] = [];
  let fenced = false;
  for (const line of summary.split("\n")) {
    if (/^ {0,3}(?:```|~~~)/.test(line)) {
      fenced = !fenced;
      continue;
    }
    if (!fenced) lines.push(line);
  }

  const heading = (line: string) => /^ {0,3}#{1,6}\s/.test(line);

  const needle = title.trim().toLowerCase();
  const restatesTitle = (line: string) => {
    const l = line.trim().toLowerCase().replace(/[*_`#]/g, "").trim();
    if (!l || !needle) return false;
    return l.includes(`summary of ${needle}`) || l.startsWith(needle);
  };

  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.trim() === "" || heading(line) || restatesTitle(line)) {
      i++;
      continue;
    }
    break;
  }

  const paragraph: string[] = [];
  for (; i < lines.length && lines[i].trim() !== ""; i++) {
    paragraph.push(
      lines[i]
        // A leading list bullet, ordinal or quote mark is scaffolding too.
        .replace(/^\s*(?:[-*+]|\d+\.)\s+/, "")
        .replace(/^\s*>\s?/, ""),
    );
  }

  return paragraph
    .join(" ")
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
    .replace(/`([^`]+)`/g, "$1")
    .replace(/(\*\*|__)(.+?)\1/g, "$2")
    .replace(/(\*|_)(?=\S)(.+?\S)\1/g, "$2")
    .replace(/\s+/g, " ")
    .trim();
}

/** Splits an ordered list into day bands without reordering it. */
export function groupByDay<T>(items: T[], at: (item: T) => string) {
  const groups: { key: string; label: string; items: T[] }[] = [];
  for (const item of items) {
    const key = dayKey(at(item));
    const last = groups[groups.length - 1];
    if (last && last.key === key) last.items.push(item);
    else groups.push({ key, label: dayLabel(at(item)), items: [item] });
  }
  return groups;
}
