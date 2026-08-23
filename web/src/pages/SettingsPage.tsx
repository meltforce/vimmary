import { useSearchParams } from "react-router-dom";
import PageHeader from "../components/PageHeader.tsx";
import { useIsDesktop } from "../hooks/useMediaQuery.ts";
import { useIsAdmin, usePodcastsEnabled } from "../features.ts";
import IdentitySection from "./settings/IdentitySection.tsx";
import KarakeepSection from "./settings/KarakeepSection.tsx";
import LLMSection from "./settings/LLMSection.tsx";
import SummariesSection from "./settings/SummariesSection.tsx";
import RSSSection from "./settings/RSSSection.tsx";
import PodcastSection from "./settings/PodcastSection.tsx";

/**
 * Settings is six unrelated concerns, and each one owns its queries, its state
 * and its own error rendering.
 *
 * That ownership is the point of the split. The page used to bundle four
 * queries into a single isLoading/errorObj pair — a hard conjunction, so any
 * one failing backend replaced the entire page with an error box. LLMSection
 * already had to opt out of that chain, because the server answers 404 to a
 * non-admin and the whole page would have gone with it. Every section now
 * behaves the way LLMSection did: a failing Karakeep call costs one tab.
 *
 * The active tab lives in `?tab=` so a page can link straight at it — the empty
 * video list points at Karakeep, and a missing key points at LLM.
 *
 * Channels used to be a section here and is now its own nav destination
 * (`/channels`): the shelf of followed channels only reads as a shelf at full
 * page width, and following one is browsing rather than configuration.
 */
export default function SettingsPage() {
  const isDesktop = useIsDesktop();
  const isAdmin = useIsAdmin();
  const podcasts = usePodcastsEnabled();
  const [params, setParams] = useSearchParams();

  const tabs = [
    { id: "identity", label: "Identity", render: () => <IdentitySection /> },
    { id: "karakeep", label: "Karakeep", render: () => <KarakeepSection /> },
    { id: "feed", label: "Feed", render: () => <RSSSection /> },
    { id: "summaries", label: "Summaries", render: () => <SummariesSection /> },
    // Service-wide keys: the server answers 404 to everyone but the primary user.
    ...(isAdmin ? [{ id: "llm", label: "LLM", render: () => <LLMSection /> }] : []),
    // Absent rather than disabled when cast2md is not configured.
    ...(podcasts ? [{ id: "podcasts", label: "Podcasts", render: () => <PodcastSection /> }] : []),
  ];

  const requested = params.get("tab");
  const active = tabs.some((t) => t.id === requested) ? requested! : tabs[0].id;

  if (!isDesktop) {
    // One scrolling page rather than a rail: the phone has no room beside the
    // content, and the sections are short enough to read in sequence.
    return (
      <>
        <PageHeader kicker="vimmary" title="Settings" />
        <div className="page-x" style={{ display: "flex", flexDirection: "column", gap: 14, paddingBottom: 32 }}>
          {tabs.map((tab) => (
            <section key={tab.id} className="card">
              {tab.render()}
            </section>
          ))}
        </div>
      </>
    );
  }

  return (
    <>
      <PageHeader kicker="vimmary" title="Settings" />
      <div
        className="page-x"
        style={{ display: "flex", gap: 20, alignItems: "flex-start", minHeight: 560, paddingBottom: 40 }}
      >
        {/* .rail is already a card in the system. */}
        <div className="rail">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              className="rail-item"
              aria-selected={tab.id === active}
              onClick={() => setParams({ tab: tab.id })}
            >
              {tab.label}
            </button>
          ))}
        </div>
        <div className="card" style={{ flex: 1, minWidth: 0, maxWidth: 960, padding: "24px 28px" }}>
          {tabs.find((t) => t.id === active)?.render()}
        </div>
      </div>
    </>
  );
}
