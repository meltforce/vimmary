import { useQuery } from "@tanstack/react-query";
import { fetchFeedInfo } from "../../api.ts";
import { usePodcastsEnabled } from "../../features.ts";
import { CopyButton, Section, SectionError, SectionLoading } from "./primitives.tsx";

/**
 * RSSSection shows the feed URLs. There are three separate subscriptions rather
 * than one feed with a filter, because an RSS reader cannot filter — the split
 * has to happen in the URL. The 32-byte token in the path is the only access
 * control on those routes; they are mounted outside the Tailscale middleware
 * because a reader cannot authenticate over Tailscale.
 */
export default function RSSSection() {
  const podcastsEnabled = usePodcastsEnabled();
  const { data: feedInfo, isLoading, error } = useQuery({
    queryKey: ["settings", "feed"],
    queryFn: fetchFeedInfo,
  });

  const subtitle = podcastsEnabled
    ? "Subscribe to your own feeds of summaries."
    : "Subscribe to your own feed of summaries.";

  if (isLoading)
    return (
      <Section title="RSS" subtitle={subtitle}>
        <SectionLoading what="feed token" />
      </Section>
    );
  if (error)
    return (
      <Section title="RSS" subtitle={subtitle}>
        <SectionError error={error as Error} />
      </Section>
    );

  const feedBase = feedInfo ? `${window.location.origin}/feed/atom/${feedInfo.token}` : "";
  const truncatedFeedToken = feedInfo ? `${feedInfo.token.slice(0, 8)}…` : "—";
  const feedVariants: { label: string; suffix: string; hint: string }[] = podcastsEnabled
    ? [
        { label: "Videos only", suffix: "", hint: "The original feed. Existing subscriptions keep this content." },
        { label: "Podcasts only", suffix: "/podcasts", hint: "Podcast episode summaries." },
        { label: "Everything", suffix: "/all", hint: "Both kinds; each entry is tagged with its type." },
      ]
    : [{ label: "Your personal feed URL", suffix: "", hint: "" }];

  return (
    <Section title="RSS" subtitle={subtitle}>
      {feedInfo &&
      feedVariants.map((variant, i) => (
        <div
          key={variant.suffix}
          style={{
            padding: "16px 0",
            borderBottom:
              i === feedVariants.length - 1 ? "none" : "1px solid var(--vim-line-soft)",
          }}
        >
          <div style={{ fontSize: 13, color: "var(--vim-ink-3)", marginBottom: 3 }}>
            {variant.label}
          </div>
          {variant.hint && (
            <div style={{ fontSize: 12, color: "var(--vim-ink-4)", marginBottom: 8 }}>
              {variant.hint}
            </div>
          )}
          <div
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: 12.5,
              padding: "12px 14px",
              background: "var(--vim-surface-2)",
              borderRadius: 6,
              color: "var(--vim-ink-2)",
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
              gap: 12,
            }}
          >
            <span
              style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
            >
              {window.location.origin}/feed/atom/
              <span style={{ color: "var(--vim-accent-ink)" }}>{truncatedFeedToken}</span>
              {variant.suffix}
            </span>
            <CopyButton text={feedBase + variant.suffix} />
          </div>
        </div>
      ))}
  </Section>
  );
}
