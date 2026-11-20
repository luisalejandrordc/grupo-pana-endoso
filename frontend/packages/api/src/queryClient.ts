import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "./client";

/**
 * One cache for the whole app. A request the server refused (4xx) is a final answer, so it is not retried;
 * a network failure or a 5xx is tried once more.
 */
export function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        refetchOnWindowFocus: false,
        retry: (failureCount, error) => {
          if (error instanceof ApiError && error.status < 500) return false;
          return failureCount < 1;
        },
      },
      mutations: { retry: false },
    },
  });
}
