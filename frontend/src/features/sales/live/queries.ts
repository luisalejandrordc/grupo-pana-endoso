import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  documentsApi,
  pickersApi,
  salesApi,
  tasksApi,
  type SaleListParams,
} from "./api";

/** Every sale key starts with "sales", so one invalidation refreshes the list, the detail, tasks, files and history. */
export const saleKeys = {
  all: ["sales"] as const,
  list: (params: SaleListParams) => ["sales", "list", params] as const,
  detail: (id: string) => ["sales", id, "detail"] as const,
  tasks: (id: string) => ["sales", id, "tasks"] as const,
  task: (id: string, number: number) => ["sales", id, "task", number] as const,
  documents: (id: string) => ["sales", id, "documents"] as const,
  history: (id: string, page: number) =>
    ["sales", id, "history", page] as const,
};

export function useSaleList(params: SaleListParams) {
  return useQuery({
    queryKey: saleKeys.list(params),
    queryFn: () => salesApi.list(params),
    placeholderData: keepPreviousData,
  });
}

/** Totals for the summary cards: one tiny request per filter. */
export function useSaleTotal(
  filter: Pick<SaleListParams, "state" | "temperature">,
) {
  const params: SaleListParams = { ...filter, page: 1, pageSize: 1 };
  return useQuery({
    queryKey: saleKeys.list(params),
    queryFn: () => salesApi.list(params),
    select: (result) => result.total,
  });
}

export function useSale(id: string) {
  return useQuery({
    queryKey: saleKeys.detail(id),
    queryFn: () => salesApi.get(id),
    staleTime: 0,
  });
}

export function useTasks(saleId: string) {
  return useQuery({
    queryKey: saleKeys.tasks(saleId),
    queryFn: () => tasksApi.list(saleId),
    staleTime: 0,
    select: (result) => result.items,
  });
}

export function useTask(saleId: string, number: number) {
  return useQuery({
    queryKey: saleKeys.task(saleId, number),
    queryFn: () => tasksApi.get(saleId, number),
    staleTime: 0,
  });
}

export function useDocuments(saleId: string) {
  return useQuery({
    queryKey: saleKeys.documents(saleId),
    queryFn: () => documentsApi.list(saleId),
    staleTime: 0,
    select: (result) => result.items,
  });
}

export function useHistory(saleId: string, page: number, pageSize: number) {
  return useQuery({
    queryKey: saleKeys.history(saleId, page),
    queryFn: () => salesApi.history(saleId, page, pageSize),
    staleTime: 0,
    placeholderData: keepPreviousData,
  });
}

export function useSelectableCatalog() {
  return useQuery({
    queryKey: ["catalog", "selectable"] as const,
    queryFn: () => pickersApi.selectableCatalog(),
    staleTime: 0,
    select: (result) => result.items,
  });
}

export function useAdvisors(enabled: boolean) {
  return useQuery({
    queryKey: ["staff", "advisors"] as const,
    queryFn: () => pickersApi.advisors(),
    enabled,
    select: (result) => result.items,
  });
}

/** Wraps any sale write so that, once it succeeds, everything about sales is read again from the server. */
export function useSaleMutation<TVariables, TResult>(
  run: (variables: TVariables) => Promise<TResult>,
) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: run,
    onSuccess: () => client.invalidateQueries({ queryKey: saleKeys.all }),
  });
}
