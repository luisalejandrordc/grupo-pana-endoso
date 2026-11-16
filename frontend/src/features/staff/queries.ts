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

export function useCreateEmployee() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateEmployeeInput) => staffApi.create(input),
    onSuccess: () => client.invalidateQueries({ queryKey: staffKeys.all }),
  });
}
