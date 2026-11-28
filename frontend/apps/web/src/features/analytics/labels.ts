import type { CancelCategory, CustomerType, GroupBy, Temperature } from "./api";

export type TabId = "summary" | "process" | "commercial" | "team" | "system";

export const tabLabels: Record<TabId, string> = {
  summary: "Resumen",
  process: "Proceso",
  commercial: "Comercial",
  team: "Equipo",
  system: "Sistema",
};

export const temperatureLabels: Record<Temperature, string> = {
  hot: "Alta",
  warm: "Media",
  cold: "Baja",
};

export const customerTypeLabels: Record<CustomerType, string> = {
  diplomatico: "Diplomático",
  embajada: "Embajada",
  entidad_religiosa_catolica: "Entidad religiosa católica",
  entidad_religiosa_no_catolica: "Entidad religiosa no católica",
  organismo_internacional: "Organismo internacional",
};

export const cancelLabels: Record<CancelCategory, string> = {
  customer_withdrew: "El cliente desistió",
  payment_or_financing_failed: "Falló el pago o el financiamiento",
  unit_unavailable: "Unidad no disponible",
  due_diligence_finding: "Hallazgo en la debida diligencia",
  registration_error: "Error en el registro",
  other: "Otro motivo",
};

export const groupLabels: Record<GroupBy, string> = {
  model: "Modelo",
  version: "Versión",
  color: "Color",
  category: "Categoría",
};

/** The vehicle categories of the catalog; the API sends the key. */
export const categoryLabels: Record<string, string> = {
  camioneta: "Camioneta",
  comercial: "Comercial",
  familiar: "Familiar",
  hatchback: "Hatchback",
  sedan: "Sedán",
};

export const channelLabels: Record<string, string> = {
  green: "Canal verde",
  orange: "Canal naranja",
  red: "Canal rojo",
};

export const invoiceKindLabels: Record<string, string> = {
  toyota: "Toyota",
  commission: "Comisión",
  accessories: "Accesorios",
  release_processing: "Trámite de liberación",
};

export const notificationTypeLabels: Record<string, string> = {
  employee_activation: "Activación de colaborador",
  employee_activated: "Colaborador activado",
  customer_activation: "Activación de cliente",
  customer_activated: "Cliente activado",
  password_reset: "Restablecer contraseña",
  sale_created: "Venta creada",
  milestone_task_1: "Hito: tarea 1",
  milestone_task_3: "Hito: tarea 3",
  milestone_task_6: "Hito: tarea 6",
  milestone_task_7: "Hito: tarea 7",
  milestone_task_10: "Hito: tarea 10",
  milestone_task_11: "Hito: tarea 11",
  milestone_task_13: "Hito: tarea 13",
  customer_advisor_changed: "Cambio de asesor (cliente)",
  sale_assigned: "Venta asignada",
  sale_unassigned: "Venta reasignada (asesor anterior)",
  sla_warning: "Aviso de plazo",
  sla_overdue_reminder: "Recordatorio de retraso",
};

export function labelFor(
  dictionary: Record<string, string>,
  key: string,
): string {
  return dictionary[key] ?? key;
}
