import { ReactNode } from "react";
import { Link, NavLink } from "react-router-dom";
import { usePodcastsEnabled } from "../features.ts";
import { useIsDesktop } from "../hooks/useMediaQuery.ts";

export default function Layout({ children }: { children: ReactNode }) {
  const podcasts = usePodcastsEnabled();
  const isDesktop = useIsDesktop();

  // Without cast2md there is no second content type, so the entry is absent
  // rather than disabled. The tab bar takes its column count from the list.
  const items = [
    { to: "/", label: "Videos", end: true },
    ...(podcasts ? [{ to: "/podcasts", label: "Podcasts", end: false }] : []),
    { to: "/stats", label: "Stats", end: false },
  ];

  return (
    <div className="min-h-screen flex flex-col">
      {isDesktop ? (
        <nav className="nav">
          {/* A plain Link: the brand points at the video list but must not
              carry the active mark, which belongs to the Videos item. */}
          <Link to="/" className="nav-brand">
            vimmary
            <span className="house">meltforce</span>
          </Link>
          {items.map((item) => (
            <NavLink key={item.to} to={item.to} end={item.end}>
              {item.label}
            </NavLink>
          ))}
          <NavLink
            to="/settings"
            className="ml-auto"
            style={{ color: "var(--color-neutral-600)" }}
          >
            Settings
          </NavLink>
        </nav>
      ) : null}

      <main className="flex-1 flex flex-col">{children}</main>

      {!isDesktop ? (
        <>
          {/* Reserves the tab bar's height so the last row is not covered. */}
          <div aria-hidden className="h-[76px]" />
          <nav className="tabs">
            {items.map((item) => (
              <NavLink key={item.to} to={item.to} end={item.end}>
                {item.label}
              </NavLink>
            ))}
            <NavLink to="/settings">More</NavLink>
          </nav>
        </>
      ) : null}
    </div>
  );
}
