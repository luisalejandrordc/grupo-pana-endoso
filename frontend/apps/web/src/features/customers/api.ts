import { api } from "../../shared/api/client";
import { queryString } from "../../shared/api/query";
import type { Schemas } from "../../shared/api/schema";

export type ApiCustomer = Schemas["Customer"];
export type CustomerList = Schemas["CustomerList"];
export type CreateCustomerBody = Schemas["CreateCustomerRequest"];
export type UpdateCustomerBody = Schemas["UpdateCustomerRequest"];
export type CustomerLookupResult = Schemas["CustomerLookupResult"];
export type IdentityResult = Schemas["IdentityResult"];

export interface CustomerListParams {
  q?: string;
  customerType?: string;
  status?: "all" | "active" | "inactive";
  page?: number;
  pageSize?: number;
  sort?: "displayName" | "-displayName" | "createdAt" | "-createdAt";
}

export const customersApi = {
  list: (params: CustomerListParams) =>
    api.request<CustomerList>(`/customers${queryString({ ...params })}`),
  get: (id: string) => api.request<ApiCustomer>(`/customers/${id}`),
  create: (body: CreateCustomerBody) =>
    api.request<ApiCustomer>("/customers", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  update: (id: string, body: UpdateCustomerBody) =>
    api.request<ApiCustomer>(`/customers/${id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  deactivate: (id: string) =>
    api.request<ApiCustomer>(`/customers/${id}/deactivate`, { method: "POST" }),
  reactivate: (id: string) =>
    api.request<ApiCustomer>(`/customers/${id}/reactivate`, { method: "POST" }),
  remove: (id: string, reason: string) =>
    api.request<void>(`/customers/${id}${queryString({ reason })}`, {
      method: "DELETE",
    }),
  lookup: (documentType: string, documentNumber: string) =>
    api.request<CustomerLookupResult>(
      `/customers/lookup${queryString({ documentType, documentNumber })}`,
    ),
  lookupIdentity: (documentType: string, documentNumber: string) =>
    api.request<IdentityResult>(
      `/lookups/identity${queryString({ documentType, documentNumber })}`,
    ),
};
