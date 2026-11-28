/**
 * What each of the 13 tasks collects, as the API names it. The form draws itself from these definitions, and
 * only the fields that changed are sent. The server stays the authority: these definitions only decide what to
 * show and which fields are locked once later tasks are completed (the same table the backend uses).
 */

import { LIMITS } from "../../shared/lib/limits";

/** `hidden` is data the form carries and sends but never draws (task 2's name source). */
export type FieldKind = "text" | "money" | "date" | "select" | "hidden";

export interface FieldDef {
  key: string;
  label: string;
  kind: FieldKind;
  options?: { value: string; label: string }[];
  hint?: string;
  placeholder?: string;
  /** Calculated by the server: shown, never sent. */
  readOnly?: boolean;
  /** Spans two columns. */
  wide?: boolean;
  /** The most characters the server accepts; the box shows a counter. */
  limit?: number;
}

export interface FieldGroup {
  title?: string;
  /** Fields of a nested object, such as an invoice: their keys are "prefix.field". */
  prefix?: string;
  fields: FieldDef[];
}

const bankOptions = [
  { value: "bcp", label: "BCP" },
  { value: "bbva", label: "BBVA" },
  { value: "scotiabank", label: "Scotiabank" },
  { value: "interbank", label: "Interbank" },
];

const payment = (expectedLabel: string): FieldGroup[] => [
  {
    fields: [
      { key: "bank", label: "Banco", kind: "select", options: bankOptions },
      { key: "paymentDate", label: "Fecha de la operación", kind: "date" },
      {
        key: "transactionNumber",
        label: "N.º de operación",
        kind: "text",
        placeholder: "Ej. 004581237",
      },
      {
        key: "expectedAmount",
        label: expectedLabel,
        kind: "money",
        readOnly: true,
      },
      {
        key: "depositedAmount",
        label: "Monto depositado",
        kind: "money",
        placeholder: "0.00",
      },
      {
        key: "mismatchReason",
        label: "Motivo de la diferencia",
        kind: "text",
        wide: true,
        limit: LIMITS.reason,
        hint: "Obligatorio cuando el monto depositado no coincide con el esperado; se guarda junto con el monto.",
      },
    ],
  },
];

const invoice = (
  prefix: string,
  title: string,
  expectedLabel: string,
): FieldGroup => ({
  title,
  prefix,
  fields: [
    {
      key: "number",
      label: "N.º de factura",
      kind: "text",
      placeholder: "F001-000123",
    },
    { key: "date", label: "Fecha de emisión", kind: "date" },
    {
      key: "expectedAmount",
      label: expectedLabel,
      kind: "money",
      readOnly: true,
    },
    {
      key: "amount",
      label: "Monto facturado",
      kind: "money",
      placeholder: "0.00",
    },
    {
      key: "mismatchReason",
      label: "Motivo de la diferencia",
      kind: "text",
      wide: true,
      limit: LIMITS.reason,
    },
  ],
});

export const taskForms: Record<number, FieldGroup[]> = {
  1: [
    {
      fields: [
        {
          key: "initialToyotaAmount",
          label: "Monto inicial a pagar a Toyota",
          kind: "money",
          placeholder: "0.00",
        },
        {
          key: "initialCommission",
          label: "Comisión inicial de Grupo Pana",
          kind: "money",
          placeholder: "0.00",
        },
        {
          key: "accessoryAmount",
          label: "Monto de accesorios",
          kind: "money",
          placeholder: "0.00",
        },
        {
          key: "releaseProcessingAmount",
          label: "Trámite de liberación del vehículo",
          kind: "money",
          placeholder: "0.00",
        },
        {
          key: "quotedFinalPrice",
          label: "Precio final cotizado",
          kind: "money",
          readOnly: true,
          hint: "Se calcula con los montos anteriores.",
        },
      ],
    },
  ],
  2: [
    {
      fields: [
        {
          key: "signerDocumentType",
          label: "Tipo de documento del firmante",
          kind: "select",
          options: [
            { value: "DNI", label: "DNI" },
            { value: "CE", label: "Carné de extranjería" },
            { value: "PAS", label: "Pasaporte" },
            { value: "CDI", label: "Carné diplomático (CDI)" },
          ],
        },
        {
          key: "signerDocumentNumber",
          label: "Número de documento",
          kind: "text",
          hint: "Se guarda junto con el tipo.",
        },
        { key: "signerFirstName", label: "Nombres del firmante", kind: "text" },
        {
          key: "signerLastName",
          label: "Apellidos del firmante",
          kind: "text",
        },
        {
          key: "signerNameSource",
          label: "Origen del nombre del firmante",
          kind: "hidden",
        },
        {
          key: "pepStatus",
          label: "Condición PEP",
          kind: "select",
          options: [
            { value: "no_pep", label: "No es PEP" },
            { value: "pep", label: "PEP" },
            { value: "related_to_pep", label: "Familiar o asociado de un PEP" },
          ],
        },
        {
          key: "ddcRegime",
          label: "Régimen de debida diligencia",
          kind: "select",
          options: [
            { value: "general", label: "General" },
            { value: "reinforced", label: "Reforzado" },
          ],
        },
      ],
    },
  ],
  3: [
    {
      fields: [
        {
          key: "vehicleSeries",
          label: "Número de serie",
          kind: "text",
          placeholder: "Ej. SER123456",
        },
        {
          key: "vin",
          label: "VIN",
          kind: "text",
          placeholder: "17 caracteres",
          hint: "Sin las letras I, O ni Q.",
        },
        {
          key: "finalToyotaAmount",
          label: "Monto final a pagar a Toyota",
          kind: "money",
          placeholder: "0.00",
        },
        {
          key: "finalCommission",
          label: "Comisión final",
          kind: "money",
          readOnly: true,
          hint: "Se calcula y debe ser mayor que cero.",
        },
      ],
    },
  ],
  4: payment("Precio final cotizado (esperado)"),
  5: payment("Monto final a Toyota (esperado)"),
  6: [
    invoice("toyota", "Factura de Toyota", "Monto final a Toyota (esperado)"),
    invoice("commission", "Factura de comisión", "Comisión final (esperada)"),
    invoice("accessories", "Factura de accesorios", "Accesorios (esperado)"),
    invoice(
      "releaseProcessing",
      "Factura del trámite de liberación",
      "Trámite de liberación (esperado)",
    ),
  ],
  7: [],
  8: [
    {
      fields: [
        { key: "damNumber", label: "N.º de DAM", kind: "text" },
        {
          key: "customsOffice",
          label: "Intendencia de aduana",
          kind: "select",
          options: [
            { value: "callao_maritima", label: "Callao Marítima" },
            { value: "chancay", label: "Chancay" },
          ],
        },
        {
          key: "controlChannel",
          label: "Canal de control",
          kind: "select",
          options: [
            { value: "green", label: "Verde" },
            { value: "orange", label: "Naranja" },
            { value: "red", label: "Rojo" },
          ],
        },
      ],
    },
  ],
  9: [
    {
      fields: [
        {
          key: "personInCharge",
          label: "Responsable de la liberación",
          kind: "text",
        },
      ],
    },
  ],
  10: [
    {
      fields: [
        {
          key: "licensePlate",
          label: "Placa",
          kind: "text",
          placeholder: "6 caracteres",
        },
        {
          key: "registrationDate",
          label: "Fecha de inmatriculación",
          kind: "date",
        },
      ],
    },
  ],
  11: [
    {
      fields: [
        {
          key: "pdsTransferDate",
          label: "Fecha de traslado a PDS",
          kind: "date",
        },
      ],
    },
  ],
  12: [
    {
      fields: [
        {
          key: "serviceOrderNumber",
          label: "N.º de orden de servicio",
          kind: "text",
        },
        {
          key: "equipmentCompletionDate",
          label: "Fecha de término del equipamiento",
          kind: "date",
        },
      ],
    },
  ],
  13: [
    {
      fields: [
        {
          key: "surquilloTransferDate",
          label: "Fecha de traslado a Surquillo",
          kind: "date",
          hint: "Opcional.",
        },
        {
          key: "deliveryDate",
          label: "Fecha de entrega del vehículo",
          kind: "date",
        },
      ],
    },
  ],
};

export type Draft = Record<string, string>;

function fullKey(group: FieldGroup, field: FieldDef) {
  return group.prefix ? `${group.prefix}.${field.key}` : field.key;
}

/** Every field of a task, with its full key. */
export function allFields(
  number: number,
): { key: string; def: FieldDef; group: FieldGroup }[] {
  return (taskForms[number] ?? []).flatMap((group) =>
    group.fields.map((def) => ({ key: fullKey(group, def), def, group })),
  );
}

/**
 * The task's data from the API as flat text values keyed by full key; empty means no value. Amounts the server
 * works out (the quoted price, the expected deposit, the final commission) arrive apart from the data, in
 * `calculated`, and fill the read-only fields.
 */
export function toDraft(
  number: number,
  data: Record<string, unknown> | undefined,
  calculated?: Record<string, unknown>,
): Draft {
  const draft: Draft = {};
  for (const { key, def, group } of allFields(number)) {
    const pick = (root: Record<string, unknown> | undefined) => {
      const source = group.prefix
        ? (root?.[group.prefix] as Record<string, unknown> | undefined)
        : root;
      return source?.[def.key];
    };
    const value = pick(data) ?? pick(calculated);
    draft[key] = value === null || value === undefined ? "" : String(value);
  }
  return draft;
}

/** Only the writable fields whose text differs from the saved one, as the API expects them (nested for invoices). */
export function toPayload(
  number: number,
  saved: Draft,
  draft: Draft,
): Record<string, unknown> {
  const payload: Record<string, unknown> = {};
  for (const { key, def, group } of allFields(number)) {
    if (def.readOnly) continue;
    const next = (draft[key] ?? "").trim();
    const before = (saved[key] ?? "").trim();
    if (next === before) continue;
    const value = next === "" ? null : next;
    if (group.prefix) {
      const nested =
        (payload[group.prefix] as Record<string, unknown> | undefined) ?? {};
      nested[def.key] = value;
      payload[group.prefix] = nested;
    } else {
      payload[def.key] = value;
    }
  }
  return payload;
}

export function isDirty(number: number, saved: Draft, draft: Draft) {
  return Object.keys(toPayload(number, saved, draft)).length > 0;
}

/** The financial source fields that later tasks depend on (mirrors the backend rule). */
const lockRules: Record<number, { keys: string[]; lockedAtTask: number }> = {
  1: {
    keys: [
      "initialToyotaAmount",
      "initialCommission",
      "accessoryAmount",
      "releaseProcessingAmount",
    ],
    lockedAtTask: 2,
  },
  3: { keys: ["finalToyotaAmount"], lockedAtTask: 4 },
  4: { keys: ["depositedAmount"], lockedAtTask: 6 },
  5: { keys: ["depositedAmount"], lockedAtTask: 6 },
};

/** Keys that cannot be corrected on a completed task because a later task is completed. */
export function lockedKeys(number: number, highestCompleted: number): string[] {
  const rule = lockRules[number];
  return rule && highestCompleted >= rule.lockedAtTask ? rule.keys : [];
}

/** Whether two amounts, as typed or stored, are both valid and different. */
export function amountsDiffer(a: string | undefined, b: string | undefined) {
  const x = Number((a ?? "").trim());
  const y = Number((b ?? "").trim());
  if (
    !(a ?? "").trim() ||
    !(b ?? "").trim() ||
    Number.isNaN(x) ||
    Number.isNaN(y)
  )
    return false;
  return Math.round(x * 100) !== Math.round(y * 100);
}

/** Task 2's lookup: only a DNI can be looked up; the other documents are always typed. */
export const SIGNER_LOOKUP_TYPE = "DNI";

const SIGNER_NAME_KEYS = ["signerFirstName", "signerLastName"];
const SIGNER_DOCUMENT_KEYS = ["signerDocumentType", "signerDocumentNumber"];

/**
 * One change to the draft. Task 2 also keeps the signer's name source honest, because the server records
 * exactly what it is sent: typing a name makes it manual (unless it is back to what the server holds), and
 * changing the document of a looked-up name clears that name, since it belonged to the old document.
 * Every other task is a plain edit.
 */
export function applyChange(
  number: number,
  saved: Draft,
  draft: Draft,
  key: string,
  value: string,
): Draft {
  const next = { ...draft, [key]: value };
  if (number !== 2) return next;
  if (SIGNER_NAME_KEYS.includes(key)) {
    const untouched =
      (next.signerFirstName ?? "") === (saved.signerFirstName ?? "") &&
      (next.signerLastName ?? "") === (saved.signerLastName ?? "");
    next.signerNameSource = untouched
      ? (saved.signerNameSource ?? "")
      : "manual";
  }
  if (
    SIGNER_DOCUMENT_KEYS.includes(key) &&
    value !== (draft[key] ?? "") &&
    draft.signerNameSource === "looked_up"
  ) {
    next.signerFirstName = "";
    next.signerLastName = "";
    next.signerNameSource = "manual";
  }
  return next;
}

/** The names a lookup found, filled into the draft as looked up. */
export function applyLookup(
  draft: Draft,
  found: { firstName?: string; lastName?: string },
): Draft {
  return {
    ...draft,
    signerFirstName: found.firstName ?? draft.signerFirstName ?? "",
    signerLastName: found.lastName ?? draft.signerLastName ?? "",
    signerNameSource: "looked_up",
  };
}
