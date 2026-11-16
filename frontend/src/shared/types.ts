export type Role = "system_admin" | "sales_manager" | "sales_advisor";
export type DocumentType = "DNI" | "CE" | "PAS";

export interface EmployeeProfile {
  id: string;
  documentType: DocumentType;
  documentNumber: string;
  firstName: string;
  lastName: string;
  businessEmail: string;
  role: Role;
  hasPhoto: boolean;
}

export interface Employee extends EmployeeProfile {
  nationality: string;
  gender: "M" | "F";
  birthdate: string;
  hireDate: string;
  businessPhone: string;
  personalPhone?: string | null;
  personalEmail?: string | null;
  isActive: boolean;
  activatedAt?: string | null;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface SessionResponse {
  accessToken: string;
  expiresAt: string;
  employee: EmployeeProfile;
}
