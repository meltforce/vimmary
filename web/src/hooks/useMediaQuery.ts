import { useSyncExternalStore } from "react";

/**
 * Subscribes to a media query. The desktop and phone layouts are different
 * component trees rather than one tree reflowed, so the breakpoint has to be
 * readable in JS; this hook is the single place that reads viewport width.
 */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const mql = window.matchMedia(query);
      mql.addEventListener("change", onChange);
      return () => mql.removeEventListener("change", onChange);
    },
    () => window.matchMedia(query).matches,
    // Server-side and during hydration, assume desktop.
    () => true,
  );
}

/** The one breakpoint in the design: below this the phone layouts take over. */
export const DESKTOP_QUERY = "(min-width: 768px)";

export function useIsDesktop(): boolean {
  return useMediaQuery(DESKTOP_QUERY);
}
