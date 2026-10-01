import { useQuery } from "@tanstack/react-query";
import { getSession } from "./api";

export function useSession(enabled = true) {
  return useQuery({
    queryKey: ["identity", "session"],
    queryFn: ({ signal }) => getSession(signal),
    enabled,
    // The interval still renews the session every minute. A short fresh window
    // prevents nested page components from repeating the same check on mount.
    staleTime: 30_000,
    refetchOnWindowFocus: true,
    refetchInterval: 60_000,
  });
}
