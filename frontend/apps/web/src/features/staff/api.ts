import { api } from "../../shared/api/client";
import { queryString } from "@endosoflow/api";
import type { DocumentType, Employee, Role } from "../../shared/types";

export interface EmployeeListParams {
  q?: string;
  role?: Role;
  status?: "all" | "active" | "inactive";
  page?: number;
  pageSize?: number;
}

export interface EmployeeList {
  items: Employee[];
  page: number;
  pageSize: number;
  total: number;
}

export interface CreateEmployeeInput {
  documentType: DocumentType;
  documentNumber: string;
  firstName: string;
  lastName: string;
  nationality: string;
  gender: "M" | "F";
  birthdate: string;
  hireDate: string;
  personalPhone?: string;
  personalEmail?: string;
  businessPhone: string;
  businessEmail: string;
  role: Role;
}

export const staffApi = {
  list: (params: EmployeeListParams) =>
    api.request<EmployeeList>(`/employees${queryString({ ...params })}`),
  create: (input: CreateEmployeeInput) =>
    api.request<{ employee: Employee; activationLinkIssued: boolean }>(
      "/employees",
      { method: "POST", body: JSON.stringify(input) },
    ),
};
