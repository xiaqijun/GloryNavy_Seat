import { QueryClient } from "@tanstack/react-query";
import { APIError } from "./http";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 300_000,
      refetchOnWindowFocus: false,
      retry: (count, error) =>
        count < 1 &&
        !(
          error instanceof APIError &&
          error.status >= 400 &&
          error.status < 500
        ),
    },
  },
});
