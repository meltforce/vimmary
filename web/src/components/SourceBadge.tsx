import type { ContentSource } from "../api.ts";

// The monitor-with-play-triangle used for video rows, also used as the submit
// field's icon on the video list.
export function PlayIcon({ size = 16 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="var(--vim-ink-3)"
      strokeWidth="1.6"
    >
      <rect x="3" y="6" width="18" height="12" rx="3" />
      <path d="M10 9v6l5-3z" fill="var(--vim-ink-3)" />
    </svg>
  );
}

// The microphone used for podcast rows.
export function MicIcon({ size = 16, color = "currentColor" }: { size?: number; color?: string }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke={color}
      strokeWidth="1.6"
      strokeLinecap="round"
    >
      <rect x="9" y="2.5" width="6" height="11" rx="3" />
      <path d="M5.5 11a6.5 6.5 0 0 0 13 0" />
      <path d="M12 17.5V21" />
    </svg>
  );
}

/**
 * SourceBadge is the visible type marker every summary carries. Videos and
 * podcasts share one list, one search and one combined feed, so the kind has to
 * be readable from the row itself rather than inferred from the page it is on.
 *
 * The podcast badge uses the accent ink rather than a second colour family, so
 * it reads as a distinction inside the existing palette.
 */
export default function SourceBadge({
  source,
  compact = false,
}: {
  source: ContentSource;
  compact?: boolean;
}) {
  const isPodcast = source === "podcast";
  const color = isPodcast ? "var(--vim-accent-ink)" : "var(--vim-ink-3)";

  return (
    <span
      className="vim-status"
      title={isPodcast ? "Podcast episode" : "YouTube video"}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 4,
        color,
        background: `color-mix(in oklch, ${color} 8%, transparent)`,
        border: `1px solid color-mix(in oklch, ${color} 22%, transparent)`,
      }}
    >
      {isPodcast ? (
        <MicIcon size={12} color={color} />
      ) : (
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="1.8">
          <rect x="3" y="6" width="18" height="12" rx="3" />
          <path d="M10 9v6l5-3z" fill={color} stroke="none" />
        </svg>
      )}
      {!compact && (isPodcast ? "podcast" : "video")}
    </span>
  );
}
