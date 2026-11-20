import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { catalogApi, type ModelListParams } from "./api";

export const catalogKeys = {
  all: ["catalog"] as const,
  list: (params: ModelListParams) => ["catalog", "list", params] as const,
  detail: (id: string) => ["catalog", "detail", id] as const,
};

export function useModelList(params: ModelListParams) {
  return useQuery({
    queryKey: catalogKeys.list(params),
    queryFn: () => catalogApi.list(params),
    placeholderData: keepPreviousData,
  });
}

export function useModelTotal(status: "all" | "active" | "archived") {
  const params: ModelListParams = { status, page: 1, pageSize: 1 };
  return useQuery({
    queryKey: catalogKeys.list(params),
    queryFn: () => catalogApi.list(params),
    select: (result) => result.total,
  });
}

/** The full model, always read fresh from the server so versions, colors and images are current. */
export function useModel(id: string | null) {
  return useQuery({
    queryKey: catalogKeys.detail(id ?? ""),
    queryFn: () => catalogApi.get(id as string),
    enabled: id !== null,
    staleTime: 0,
  });
}

/**
 * Wraps any catalog write so that, once it succeeds, every catalog list and detail is read again. The
 * catalog is small and edited rarely, so refetching beats patching the cache by hand.
 */
export function useCatalogMutation<TVariables, TResult>(
  run: (variables: TVariables) => Promise<TResult>,
) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: run,
    onSuccess: () => client.invalidateQueries({ queryKey: catalogKeys.all }),
  });
}
