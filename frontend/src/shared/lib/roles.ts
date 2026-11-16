import { type Role } from "../types";

export const roleOptions: Role[] = [
  "system_admin",
  "sales_manager",
  "sales_advisor",
];

export const roleLabel: Record<Role, string> = {
  system_admin: "Administrador del sistema",
  sales_manager: "Gerente de ventas",
  sales_advisor: "Asesor de ventas",
};

export const roleShortLabel: Record<Role, string> = {
  system_admin: "Admin",
  sales_manager: "Gerencia",
  sales_advisor: "Asesor",
};
