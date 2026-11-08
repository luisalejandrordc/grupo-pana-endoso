import type { Role } from "../../../shared/types";
import type { ApiSale } from "./api";

export const isManagement = (role: Role) => role !== "sales_advisor";

/** The assigned advisor, a Manager or an Admin can work a sale's tasks while it is active. */
export function canWork(role: Role, userId: string, sale: ApiSale) {
  return (
    sale.state === "active" && (isManagement(role) || sale.advisorId === userId)
  );
}

/** An Advisor cancels their own sale while Tasks 1 to 3 are current, a Manager through Task 6, an Admin always. */
export function canCancel(role: Role, userId: string, sale: ApiSale) {
  if (sale.state !== "active") return false;
  const current = sale.currentTaskNumber ?? 13;
  if (role === "system_admin") return true;
  if (role === "sales_manager") return current <= 6;
  return sale.advisorId === userId && current <= 3;
}

/** A Manager reopens a sale cancelled while Tasks 1 to 6 were current; later cancellations need an Admin. */
export function canReopenSale(role: Role, sale: ApiSale) {
  if (sale.state !== "cancelled") return false;
  if (role === "system_admin") return true;
  return role === "sales_manager" && (sale.cancelledAtTaskNumber ?? 0) <= 6;
}
