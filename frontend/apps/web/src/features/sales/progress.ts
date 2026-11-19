import type { SaleListItem } from "./api";

/** How many of the 13 tasks are done, which the list only knows through the current task. */
export function completedTasks(sale: SaleListItem): number | null {
  if (sale.state === "completed") return 13;
  if (sale.state === "cancelled") return null;
  return sale.currentTask ? sale.currentTask.number - 1 : 0;
}
