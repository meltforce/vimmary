import { useQuery } from "@tanstack/react-query";
import { fetchFeedInfo } from "../../api.ts";
import { usePodcastsEnabled } from "../../features.ts";
import { CopyButton, Row, Section, SectionError, SectionLoading } from "./primitives.tsx";

/**
 * The feed URLs. There are three separate subscriptions rather than one feed
 * with a filter, because an RSS reader cannot filter — the split has to happen
 * in the URL. The 32-byte token in the path is the only access control on those
 * routes; they are mounted outside the Tailscale middleware because a reader
 * cannot authenticate over Tailscale.
 */
export default function RSSSection() {
  const podcastsEnabled = usePodcastsEnabled();
  const { data: feed, isLoading, error } = useQuery({
    queryKey: ["settings", "feed"],
    queryFn: fetchFeedInfo,
  });

  const subtitle =
    "Atom, authenticated by the token in the path — anyone holding the URL can read the feed, so treat it as the secret it is.";

  const variants = feed
    ? podcastsEnabled
      ? [
          { label: "Videos", url: feed.urls.videos, hint: "The original feed. Existing subscriptions keep this content." },
          { label: "Podcasts", url: feed.urls.podcasts, hint: "Episode summaries only." },
          { label: "Everything", url: feed.urls.all, hint: "Both kinds; each entry is tagged with its type." },
        ]
      : [{ label: "Feed URL", url: feed.urls.videos, hint: "" }]
    : [];

  return (
    <Section title="Feed" subtitle={subtitle}>
      {isLoading ? <SectionLoading /> : null}
      {error ? <SectionError error={error as Error} /> : null}
      {variants.map((v) => (
        <Row key={v.label} label={v.label}>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <span className="mono" style={{ flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
              {v.url}
            </span>
            <CopyButton text={v.url} />
          </div>
          {v.hint ? (
            <p style={{ font: "400 12px var(--font-body)", color: "var(--color-neutral-600)", margin: "6px 0 0" }}>
              {v.hint}
            </p>
          ) : null}
        </Row>
      ))}
    </Section>
  );
}
