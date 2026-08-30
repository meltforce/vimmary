import { useTheme, type ThemePreference } from "../../theme.tsx";
import { useIdentity, useIsAdmin, usePodcastsEnabled } from "../../features.ts";
import { Row, Section } from "./primitives.tsx";

const THEMES: { value: ThemePreference; label: string }[] = [
  { value: "auto", label: "Auto" },
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
];

/**
 * What this browser and this account are. The appearance preference lives here
 * rather than in the topbar: it is set once and then never again, and a control
 * in the nav costs a slot on every page for that.
 */
export default function IdentitySection() {
  const { theme, setTheme } = useTheme();
  const isAdmin = useIsAdmin();
  const podcasts = usePodcastsEnabled();
  const { userId, login, displayName } = useIdentity();

  return (
    <Section
      title="Identity"
      subtitle="Who you are to vimmary, and how it looks in this browser. Access is decided by Tailscale; there is no password to set here."
    >
      {/* The account comes first: it is the answer to "whose library is this",
          and the role below only qualifies it. Falls back to the numeric ID,
          which still tells two accounts apart, when the lookup failed. */}
      <Row label="Account" value={login || (userId ? `User ${userId}` : "—")} />
      {displayName && displayName !== login && (
        <Row label="Name" value={displayName} />
      )}
      <Row label="Role" value={isAdmin ? "Primary user" : "Member"} />
      <Row
        label="Podcasts"
        value={podcasts ? "cast2md is configured" : "cast2md is not configured"}
      />
      <Row label="Appearance">
        <span className="seg">
          {THEMES.map((t) => (
            <label key={t.value} className="seg-opt">
              <input
                type="radio"
                name="theme"
                value={t.value}
                checked={theme === t.value}
                onChange={() => setTheme(t.value)}
              />
              {t.label}
            </label>
          ))}
        </span>
        <p
          style={{
            font: "400 12px var(--font-body)",
            color: "var(--color-neutral-600)",
            margin: "8px 0 0",
          }}
        >
          Auto follows the system setting. The choice is stored in this browser.
        </p>
      </Row>
    </Section>
  );
}
