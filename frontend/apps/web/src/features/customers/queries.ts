import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  customersApi,
  type CreateCustomerBody,
  type CustomerListParams,
  type UpdateCustomerBody,
} from "./api";

export const customerKeys = {
  all: ["customers"] as const,
  list: (params: CustomerListParams) => ["customers", "list", params] as const,
  detail: (id: string) => ["customers", "detail", id] as const,
};

export function useCustomerList(params: CustomerListParams) {
  return useQuery({
    queryKey: customerKeys.list(params),
    queryFn: () => customersApi.list(params),
    placeholderData: keepPreviousData,
  });
}

/** Totals for the summary cards: one tiny request per status. */
export function useCreateCustomer() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateCustomerBody) => customersApi.create(body),
    onSuccess: () => client.invalidateQueries({ queryKey: customerKeys.all }),
  });
}

export function useUpdateCustomer(id: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (body: UpdateCustomerBody) => customersApi.update(id, body),
    onSuccess: () => client.invalidateQueries({ queryKey: customerKeys.all }),
  });
}

export function useResendCustomerActivation() {
  return useMutation({
    mutationFn: (id: string) => customersApi.resendActivation(id),
  });
}

export function useCustomerStatus(id: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (active: boolean) =>
      active ? customersApi.reactivate(id) : customersApi.deactivate(id),
    onSuccess: () => client.invalidateQueries({ queryKey: customerKeys.all }),
  });
}

export function useDeleteCustomer(id: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (reason: string) => customersApi.remove(id, reason),
    onSuccess: () => client.invalidateQueries({ queryKey: customerKeys.all }),
  });
}
