import { useQuery } from "@tanstack/react-query";
import { fetchFeatures } from "./api.ts";
import type { Features } from "./api.ts";

/**
 * useFeatures reports which optional integrations the server has configured.
 *
 * vimmary runs standalone without cast2md, and cast2md runs standalone without
 * vimmary. When an integration is off, its half of the UI is not shown at all
 * rather than shown in a disabled or error state — an installation that never
 * heard of podcasts should look like one.
 *
 * The answer changes only on a server restart, so it is cached for the session.
 */
function useFeaturesQuery() {
  return useQuery({
    queryKey: ["features"],
    queryFn: fetchFeatures,
    staleTime: Infinity,
    gcTime: Infinity,
    retry: false,
  });
}

export function useFeatures(): Features {
  const { data } = useFeaturesQuery();
  // Default to off while loading. Showing the podcast UI and then removing it
  // would be worse than showing it a moment late on a deployment that has it.
  return (
    data ?? {
      podcasts: false,
      cast2md_url: "",
      is_admin: false,
      user_id: 0,
      login: "",
      display_name: "",
    }
  );
}

/**
 * useFeaturesResolved reports whether the answer has arrived — with data or
 * with an error, since `retry: false` makes a failure final.
 *
 * Anything that *removes* UI on the default-off answer has to wait for this.
 * The routes are the case that matters: App renders the podcast routes only
 * when `podcasts` is true, so on a direct hit of /podcasts or /podcasts/new the
 * first render has no such route, the catch-all redirects to / with `replace`,
 * and the deep link is gone before the flags arrive. Rendering the podcast UI a
 * moment late is fine; redirecting away from it a moment early is not.
 */
export function useFeaturesResolved(): boolean {
  return !useFeaturesQuery().isPending;
}

export function usePodcastsEnabled(): boolean {
  return useFeatures().podcasts;
}

/**
 * useIsAdmin reports whether the signed-in user is the primary user. It gates
 * the service-wide LLM settings, which everyone else does not see at all — the
 * server answers 404 on those routes for them, so rendering the section would
 * only produce a broken panel.
 */
export function useIsAdmin(): boolean {
  return useFeatures().is_admin;
}

/**
 * useIdentity reports which account the server resolved this request to.
 *
 * It is shown rather than merely used because vimmary is multi-user and the
 * identity arrives from Tailscale without any visible sign-in: on a shared
 * device nothing on screen said whose library was being displayed, and every
 * row hangs off this user ID.
 */
export function useIdentity(): { userId: number; login: string; displayName: string } {
  const f = useFeatures();
  return { userId: f.user_id, login: f.login, displayName: f.display_name };
}
