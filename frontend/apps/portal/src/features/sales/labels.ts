import type { BadgeTone } from "@endosoflow/ui";
import type { MilestoneState, SaleState } from "../../shared/types";

export const saleStateLabels: Record<SaleState, string> = {
  active: "En curso",
  completed: "Completada",
  cancelled: "Cancelada",
};

export const saleStateTones: Record<SaleState, BadgeTone> = {
  active: "doing",
  completed: "done",
  cancelled: "neutral",
};

export const milestoneStateLabels: Record<MilestoneState, string> = {
  completed: "Completada",
  in_progress: "En curso",
  pending: "Pendiente",
};

export const milestoneStateTones: Record<MilestoneState, BadgeTone> = {
  completed: "done",
  in_progress: "doing",
  pending: "neutral",
};

export const categoryLabels: Record<string, string> = {
  camioneta: "Camioneta",
  comercial: "Comercial",
  familiar: "Familiar",
  hatchback: "Hatchback",
  sedan: "Sedán",
};

export const viewpointLabels: Record<string, string> = {
  main_view: "Vista principal",
  exterior_front: "Frente",
  exterior_left: "Lateral izquierdo",
  exterior_right: "Lateral derecho",
  exterior_rear: "Posterior",
  interior: "Interior",
};

/** The label of a category or a viewpoint; an unknown one is shown as it comes rather than hidden. */
export const labelOf = (table: Record<string, string>, key: string) =>
  table[key] ?? key;
