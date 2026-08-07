import type { ContentSource } from "../api.ts";

/**
 * The visible type marker every summary carries. Videos and podcasts share one
 * list, one search and one combined feed, so the kind has to be readable from
 * the row itself rather than inferred from the page it is on.
 *
 * It is a word in a tag, not an icon: the palette is mono and the design uses
 * icons only where language does not suffice.
 */
export default function SourceBadge({ source }: { source: ContentSource }) {
  return (
    <span className="tag tag-neutral">
      {source === "podcast" ? "podcast" : "video"}
    </span>
  );
}
