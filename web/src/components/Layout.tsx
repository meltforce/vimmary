import { ReactNode, useEffect, useRef } from "react";
import { Link, NavLink } from "react-router-dom";
import { usePodcastsEnabled } from "../features.ts";
import { useIsDesktop } from "../hooks/useMediaQuery.ts";
import { useInboxCount } from "../hooks/useInboxCount.ts";

/**
 * Publishes the nav's height as `--nav-h` so anything that pins below it — the
 * player's video-and-search head, the reader's rail — can offset itself
 * without a hard-coded number. Measured rather than assumed: the nav's height
 * follows its font size and padding, and a stale constant would either hide a
 * strip of video under the nav or leave a gap the transcript scrolls through.
 */
function useNavHeight() {
  const ref = useRef<HTMLElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const publish = () =>
      document.documentElement.style.setProperty("--nav-h", `${el.offsetHeight}px`);
    publish();
    const observer = new ResizeObserver(publish);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return ref;
}

/**
 * One pill nav across both breakpoint trees. The phone's bottom tab bar is
 * gone with the Shelf redesign: a segmented pill row reads the same on both
 * viewports and scrolls horizontally when it does not fit, which the tab bar's
 * fixed column count could not do once Channels joined the nav.
 */
export default function Layout({ children }: { children: ReactNode }) {
  const podcasts = usePodcastsEnabled();
  const isDesktop = useIsDesktop();
  const inbox = useInboxCount();
  const navRef = useNavHeight();

  // Without cast2md there is no second content type, so the entry is absent
  // rather than disabled.
  const items = [
    { to: "/", label: "Videos", end: true },
    // Inbox sits next to Videos: both are YouTube surfaces, and triage feeds
    // the video list.
    { to: "/inbox", label: "Inbox", end: false, badge: inbox },
    ...(podcasts ? [{ to: "/podcasts", label: "Podcasts", end: false }] : []),
    // Promoted out of Settings by the redesign: following channels is browsing,
    // not configuration.
    { to: "/channels", label: "Channels", end: false },
    { to: "/stats", label: "Stats", end: false },
    { to: "/settings", label: "Settings", end: false },
  ];

  return (
    <div className="min-h-screen flex flex-col">
      <nav className="nav" ref={navRef}>
        {/* A plain Link: the brand points at the video list but must not carry
            the active mark, which belongs to the Videos item. */}
        <Link to="/" className="nav-brand">
          vimmary
          {/* The house label costs the pill row the width of a whole item on a
              phone, and the row is what has to stay reachable. */}
          {isDesktop ? <span className="house">meltforce</span> : null}
        </Link>
        <div className="pillnav">
          {items.map((item) => (
            <NavLink key={item.to} to={item.to} end={item.end}>
              {item.label}
              {item.badge ? <span className="badge">{item.badge}</span> : null}
            </NavLink>
          ))}
        </div>
      </nav>

      <main className="flex-1 flex flex-col">{children}</main>
    </div>
  );
}
