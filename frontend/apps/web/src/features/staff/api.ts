import { queryString, type Operations, type Schemas } from "@endosoflow/api";
import { api } from "../../shared/api/client";

/** Every type here comes from the generated contract, so a change in the API shows up as a type error. */
export type Employee = Schemas["Employee"];
export type EmployeeList = Schemas["EmployeeList"];
export type CreateEmployeeInput = Schemas["CreateEmployeeRequest"];
export type UpdateEmployeeInput = Schemas["UpdateEmployeeRequest"];
export type CreateEmployeeResult = Schemas["CreateEmployeeResult"];
export type EmployeeListParams = NonNullable<
  Operations["listEmployees"]["parameters"]["query"]
>;

export const staffApi = {
  list: (params: EmployeeListParams) =>
    api.request<EmployeeList>(`/employees${queryString({ ...params })}`),
  get: (id: string) => api.request<Employee>(`/employees/${id}`),
  create: (input: CreateEmployeeInput) =>
    api.request<CreateEmployeeResult>("/employees", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  /** Only the fields sent change; `version` is required and a stale one answers VERSION_CONFLICT. */
  update: (id: string, input: UpdateEmployeeInput) =>
    api.request<Employee>(`/employees/${id}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  /** Ends the person's sessions and stops them from signing in. Their history stays. */
  deactivate: (id: string) =>
    api.request<Employee>(`/employees/${id}/deactivate`, { method: "POST" }),
  reactivate: (id: string) =>
    api.request<Employee>(`/employees/${id}/reactivate`, { method: "POST" }),
  /** A new activation link, for someone who has not set a password yet. */
  resendActivation: (id: string) =>
    api.request<{ status: "accepted" }>(`/employees/${id}/resend-activation`, {
      method: "POST",
    }),
};
