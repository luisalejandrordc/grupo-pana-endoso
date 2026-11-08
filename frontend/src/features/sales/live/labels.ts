import { ApiError } from "../../../shared/api/client";
import type {
  CancelCategory,
  HistoryEntry,
  SaleStateValue,
  Temperature,
} from "./api";

export const stateLabels: Record<SaleStateValue, string> = {
  active: "En curso",
  completed: "Completada",
  cancelled: "Cancelada",
};

export const temperatureLabels: Record<Temperature, string> = {
  hot: "Alta",
  warm: "Media",
  cold: "Baja",
};

export const cancelCategories: CancelCategory[] = [
  "customer_withdrew",
  "payment_or_financing_failed",
  "unit_unavailable",
  "due_diligence_finding",
  "registration_error",
  "other",
];

export const cancelLabels: Record<CancelCategory, string> = {
  customer_withdrew: "El cliente desistió de la compra",
  payment_or_financing_failed: "Falló el pago o el financiamiento",
  unit_unavailable: "La unidad ya no está disponible",
  due_diligence_finding: "Hallazgo en la debida diligencia",
  registration_error: "Error en el registro de datos",
  other: "Otro motivo",
};

export const phaseLabels: Record<string, string> = {
  early: "Anticipada",
  in_approaching: "Dentro del plazo",
  overdue: "Con retraso",
  recurring_overdue: "Con retraso recurrente",
  not_applicable: "Sin plazo definido",
};

/** The 13 task names, as shown in the workspace table (the detail reads the names from the API). */
export const TASK_NAMES: string[] = [
  "Cotización",
  "Debida diligencia del cliente",
  "Liquidación de la unidad",
  "Pago del cliente a Grupo Pana",
  "Pago a Toyota",
  "Facturación",
  "Documentos endosados",
  "Importación (DAM)",
  "Liberación del vehículo",
  "Inscripción y placa",
  "Traslado a PDS",
  "Equipamiento",
  "Entrega al cliente",
];

/** The message to show for any failure: the server's own Spanish text when there is one. */
export function errorText(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  if (error instanceof Error && error.message) return error.message;
  return "Ocurrió un error. Inténtalo nuevamente.";
}

/** Field errors from a refused request, by field name. */
export function fieldErrors(error: unknown): Record<string, string> {
  const out: Record<string, string> = {};
  if (error instanceof ApiError && error.fields) {
    for (const f of error.fields) if (!out[f.field]) out[f.field] = f.message;
  }
  return out;
}

export function shortId(id: string) {
  return id.slice(0, 8).toUpperCase();
}

/** Labels for the keys that appear in the history of changes. */
const changeLabels: Record<string, string> = {
  notes: "Notas",
  initialToyotaAmount: "Monto inicial a Toyota",
  initialCommission: "Comisión inicial",
  accessoryAmount: "Accesorios",
  releaseProcessingAmount: "Trámite de liberación",
  signerDocumentType: "Tipo de documento del firmante",
  signerDocumentNumber: "Documento del firmante",
  signerFirstName: "Nombre del firmante",
  signerLastName: "Apellido del firmante",
  signerNameSource: "Origen del nombre",
  pepStatus: "Estado PEP",
  ddcRegime: "Régimen de debida diligencia",
  vehicleSeries: "Serie",
  vin: "VIN",
  finalToyotaAmount: "Monto final a Toyota",
  bank: "Banco",
  paymentDate: "Fecha de pago",
  transactionNumber: "N.º de operación",
  depositedAmount: "Monto depositado",
  mismatchReason: "Motivo de la diferencia",
  damNumber: "DAM",
  customsOffice: "Aduana",
  controlChannel: "Canal de control",
  personInCharge: "Responsable",
  licensePlate: "Placa",
  registrationDate: "Fecha de inscripción",
  transferDate: "Fecha de traslado",
  serviceOrderNumber: "Orden de servicio",
  completionDate: "Fecha de término",
  surquilloTransferDate: "Traslado a Surquillo",
  deliveryDate: "Fecha de entrega",
  fileName: "Archivo",
  sizeBytes: "Tamaño",
  category: "Categoría",
  from: "Antes",
  to: "Después",
};

function keyLabel(key: string) {
  const [group, field] = key.split(".");
  const invoice: Record<string, string> = {
    toyota: "Factura Toyota",
    commission: "Factura comisión",
    accessories: "Factura accesorios",
    releaseProcessing: "Factura liberación",
  };
  if (field && invoice[group]) {
    const names: Record<string, string> = {
      number: "número",
      date: "fecha",
      amount: "monto",
      mismatchReason: "motivo de la diferencia",
    };
    return `${invoice[group]} · ${names[field] ?? field}`;
  }
  return changeLabels[key] ?? key;
}

function plain(value: unknown): string {
  if (value === null || value === undefined || value === "") return "vacío";
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean")
    return String(value);
  return JSON.stringify(value);
}

/** One readable line per field that changed in an event. */
export function describeChanges(changes: HistoryEntry["changes"]): string[] {
  if (!changes) return [];
  const lines: string[] = [];
  for (const [key, value] of Object.entries(changes)) {
    if (
      key === "taskNumber" ||
      key === "slot" ||
      key === "automatic" ||
      key === "trigger"
    )
      continue;
    if (value === "changed") {
      lines.push(`${keyLabel(key)}: modificado`);
    } else if (
      value &&
      typeof value === "object" &&
      "before" in value &&
      "after" in value
    ) {
      const change = value as { before: unknown; after: unknown };
      lines.push(
        `${keyLabel(key)}: ${plain(change.before)} → ${plain(change.after)}`,
      );
    } else if (key === "before" || key === "after") {
      continue;
    } else {
      lines.push(`${keyLabel(key)}: ${plain(value)}`);
    }
  }
  return lines;
}

export type HistoryTone = "green" | "blue" | "amber" | "red";

/** A title, icon and tone for an audit event. `taskLabel` and `slotLabel` resolve ids the event refers to. */
export function describeEvent(
  entry: HistoryEntry,
  taskLabel: string | undefined,
  slotLabel: string | undefined,
): {
  title: string;
  icon: "sales" | "check-circle" | "clock" | "file-text" | "alert";
  tone: HistoryTone;
} {
  const task = taskLabel ?? "Tarea";
  const slot = slotLabel ? `: ${slotLabel}` : "";
  const key = `${entry.entityType}:${entry.action}`;
  const map: Record<
    string,
    {
      title: string;
      icon: "sales" | "check-circle" | "clock" | "file-text" | "alert";
      tone: HistoryTone;
    }
  > = {
    "sale_register:CREATED": {
      title: "Venta registrada",
      icon: "sales",
      tone: "green",
    },
    "sale_register:UPDATED": {
      title: "Notas de la venta actualizadas",
      icon: "clock",
      tone: "blue",
    },
    "sale_register:COMPLETED": {
      title: "Venta completada",
      icon: "check-circle",
      tone: "green",
    },
    "sale_register:CANCELLED": {
      title: "Venta cancelada",
      icon: "alert",
      tone: "red",
    },
    "sale_register:REOPENED": {
      title: "Venta reabierta",
      icon: "clock",
      tone: "amber",
    },
    "sale_register:REASSIGNED": {
      title: "Venta reasignada",
      icon: "sales",
      tone: "blue",
    },
    "sale_register:DELETED": {
      title: "Venta eliminada",
      icon: "alert",
      tone: "red",
    },
    "task_instance:STARTED": {
      title: `${task} iniciada`,
      icon: "clock",
      tone: "amber",
    },
    "task_instance:UPDATED": {
      title: `${task}: datos guardados`,
      icon: "clock",
      tone: "blue",
    },
    "task_instance:COMPLETED": {
      title: `${task} completada`,
      icon: "check-circle",
      tone: "green",
    },
    "task_instance:RESTARTED": {
      title: `${task} reiniciada`,
      icon: "alert",
      tone: "amber",
    },
    "task_instance:REOPENED": {
      title: `${task} reabierta`,
      icon: "clock",
      tone: "amber",
    },
    "task_instance:CORRECTED": {
      title: `${task}: datos corregidos`,
      icon: "alert",
      tone: "amber",
    },
    "document:UPLOADED": {
      title: `Documento cargado${slot}`,
      icon: "file-text",
      tone: "blue",
    },
    "document:REPLACED": {
      title: `Documento reemplazado${slot}`,
      icon: "file-text",
      tone: "amber",
    },
    "document:REMOVED": {
      title: `Documento eliminado${slot}`,
      icon: "file-text",
      tone: "red",
    },
  };
  return (
    map[key] ?? {
      title: `${entry.entityType} · ${entry.action}`,
      icon: "clock",
      tone: "blue",
    }
  );
}
