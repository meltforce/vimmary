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
