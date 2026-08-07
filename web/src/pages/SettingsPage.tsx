import KarakeepSection from "./settings/KarakeepSection.tsx";
import LLMSection from "./settings/LLMSection.tsx";
import SummariesSection from "./settings/SummariesSection.tsx";
import RSSSection from "./settings/RSSSection.tsx";
import PodcastSection from "./settings/PodcastSection.tsx";

/**
 * The Settings page is five unrelated concerns stacked in one column, and each
 * one owns its queries, its state and its own error rendering.
 *
 * That ownership is the point of the split. The page used to bundle four
 * queries into a single isLoading/errorObj pair — a hard conjunction, so any
 * one failing backend replaced the entire page with an error box. LLMSection
 * already had to opt out of that chain, because the server answers 404 to a
 * non-admin and the whole page would have gone with it. Every section now
 * behaves the way LLMSection did: a failing Karakeep call costs the Karakeep
 * card, not the page.
 */
export default function SettingsPage() {
  return (
    <div className="vim-page-narrower">
      <div className="vim-kicker" style={{ marginBottom: 10 }}>
        — Preferences
      </div>
      <h1 className="vim-h1-stats-settings" style={{ marginBottom: 36 }}>
        Settings
      </h1>

      <KarakeepSection />
      {/* Primary user only; renders nothing for everyone else. */}
      <LLMSection />
      <SummariesSection />
      <RSSSection />
      {/* Renders nothing when cast2md is not configured. */}
      <PodcastSection />
    </div>
  );
}
