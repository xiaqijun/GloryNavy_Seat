import {useQuery} from "@tanstack/react-query";
import * as api from "./api";
export function useGrowthStatus(
  corp: string,
  char: string,
  kind: string,
  version?: string,
) {
  return useQuery({
    queryKey: ["welfare", "growth-check", corp, char, kind, version],
    queryFn: ({ signal }) => api.growthCheck(corp, char, kind, signal),
    enabled: kind.startsWith("growth_") && !!char,
    refetchInterval: 30000,
  });
}
