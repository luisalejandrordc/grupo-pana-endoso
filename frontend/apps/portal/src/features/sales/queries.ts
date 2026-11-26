import { useQuery } from "@tanstack/react-query";
import { api } from "../../shared/api/client";

export const saleKeys = {
  all: ["sales"] as const,
  list: () => [...saleKeys.all, "list"] as const,
  detail: (id: string) => [...saleKeys.all, "detail", id] as const,
};

/**
 * Progress is what customers come back to see, so these are always re-read when a screen opens or the
 * window regains focus, never served from a minute-old cache.
 */
const fresh = { staleTime: 0, refetchOnWindowFocus: true } as const;

export function useSales() {
  return useQuery({
    queryKey: saleKeys.list(),
    queryFn: () => api.listSales(),
    ...fresh,
  });
}

export function useSale(id: string) {
  return useQuery({
    queryKey: saleKeys.detail(id),
    queryFn: () => api.getSale(id),
    ...fresh,
  });
}
