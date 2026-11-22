import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  staffApi,
  type CreateEmployeeInput,
  type EmployeeListParams,
  type UpdateEmployeeInput,
} from "./api";

export const staffKeys = {
  all: ["employees"] as const,
  list: (params: EmployeeListParams) => ["employees", "list", params] as const,
};

export function useEmployeeList(params: EmployeeListParams, enabled = true) {
  return useQuery({
    queryKey: staffKeys.list(params),
    queryFn: () => staffApi.list(params),
    placeholderData: keepPreviousData,
    enabled,
  });
}

/** Every change to an employee makes the lists stale: a row's state, role and name all show there. */
function useRefreshList() {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: staffKeys.all });
}

export function useCreateEmployee() {
  const refresh = useRefreshList();
  return useMutation({
    mutationFn: (input: CreateEmployeeInput) => staffApi.create(input),
    onSuccess: refresh,
  });
}

export function useUpdateEmployee() {
  const refresh = useRefreshList();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: UpdateEmployeeInput }) =>
      staffApi.update(id, body),
    onSuccess: refresh,
  });
}

export function useDeactivateEmployee() {
  const refresh = useRefreshList();
  return useMutation({
    mutationFn: (id: string) => staffApi.deactivate(id),
    onSuccess: refresh,
  });
}

export function useReactivateEmployee() {
  const refresh = useRefreshList();
  return useMutation({
    mutationFn: (id: string) => staffApi.reactivate(id),
    onSuccess: refresh,
  });
}

export function useDeleteEmployee() {
  const refresh = useRefreshList();
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      staffApi.remove(id, reason),
    onSuccess: refresh,
  });
}

export function useResendActivation() {
  return useMutation({
    mutationFn: (id: string) => staffApi.resendActivation(id),
  });
}

/** The server's current copy of one employee: what a form reloads after finding out it was out of date. */
export function useFetchEmployee() {
  const refresh = useRefreshList();
  return useMutation({
    mutationFn: (id: string) => staffApi.get(id),
    onSuccess: refresh,
  });
}
