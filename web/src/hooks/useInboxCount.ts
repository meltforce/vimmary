import { useQuery } from "@tanstack/react-query";
import { listInbox } from "../api.ts";

/**
 * Items still awaiting triage. Two surfaces need the figure — the nav badge and
 * the library's dashboard line — so the query lives here rather than in either
 * of them, and both read one cache entry.
 *
 * `limit: 1` because only `total` is used; the endpoint reports it for the
 * whole inbox regardless of the page size.
 */
export function useInboxCount(): number {
  const { data } = useQuery({
    queryKey: ["inbox", "count"],
    queryFn: () => listInbox({ limit: 1 }),
    staleTime: 60_000,
  });
  return data?.total ?? 0;
}
