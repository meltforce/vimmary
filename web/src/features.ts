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
export function useFeatures(): Features {
  const { data } = useQuery({
    queryKey: ["features"],
    queryFn: fetchFeatures,
    staleTime: Infinity,
    gcTime: Infinity,
    retry: false,
  });
  // Default to off while loading. Showing the podcast UI and then removing it
  // would be worse than showing it a moment late on a deployment that has it.
  return data ?? { podcasts: false, cast2md_url: "", is_admin: false };
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
