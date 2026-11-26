import type { PortalMilestone, SaleState } from "../../shared/types";

export interface Progress {
  done: number;
  total: number;
  /** The stage being worked on now; none when the purchase is finished, cancelled or not started. */
  current: PortalMilestone | undefined;
  /** One short sentence about where the purchase stands. */
  summary: string;
}

/** Where a purchase stands, from the six milestones the server derived. The browser never recomputes them. */
export function progressOf(
  milestones: PortalMilestone[],
  state: SaleState,
): Progress {
  const done = milestones.filter((m) => m.state === "completed").length;
  const total = milestones.length;
  const current = milestones.find((m) => m.state === "in_progress");
  if (state === "cancelled")
    return {
      done,
      total,
      current: undefined,
      summary: "Esta compra fue cancelada",
    };
  if (state === "completed")
    return {
      done: total,
      total,
      current: undefined,
      summary: "Compra completada",
    };
  if (current)
    return { done, total, current, summary: `Etapa actual: ${current.name}` };
  return { done, total, current, summary: "Su compra está por comenzar" };
}
