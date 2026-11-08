import { describe, expect, it } from "vitest";
import { ApiError } from "../../../shared/api/client";
import type { HistoryEntry } from "./api";
import {
  describeChanges,
  describeEvent,
  errorText,
  fieldErrors,
  shortId,
} from "./labels";

const entry = (over: Partial<HistoryEntry>): HistoryEntry => ({
  id: "1",
  actorKind: "employee",
  entityType: "task_instance",
  entityId: "t1",
  action: "UPDATED",
  createdAt: "2026-03-02T10:00:00Z",
  ...over,
});

describe("errors", () => {
  it("shows the server's message", () => {
    expect(
      errorText(
        new ApiError("La tarea no está en curso.", "TASK_NOT_IN_PROGRESS", 409),
      ),
    ).toBe("La tarea no está en curso.");
    expect(errorText(new Error("Fallo de red"))).toBe("Fallo de red");
    expect(errorText("???")).toMatch(/error/i);
  });

  it("maps field errors by field, keeping the first message", () => {
    const error = new ApiError(
      "Revise los datos ingresados.",
      "VALIDATION_FAILED",
      422,
      [
        { field: "vin", code: "VIN_INVALID", message: "VIN inválido" },
        { field: "vin", code: "OTHER", message: "otro" },
        {
          field: "documents.quotation_pdf",
          code: "DOCUMENT_REQUIRED",
          message: "Falta el documento",
        },
      ],
    );
    expect(fieldErrors(error)).toEqual({
      vin: "VIN inválido",
      "documents.quotation_pdf": "Falta el documento",
    });
    expect(fieldErrors(new Error("x"))).toEqual({});
  });
});

describe("shortId", () => {
  it("is the first eight characters in capitals", () => {
    expect(shortId("0198abcd-1234-7000-8000-000000000001")).toBe("0198ABCD");
  });
});

describe("describeChanges", () => {
  it("shows before and after for business fields", () => {
    const lines = describeChanges({
      accessoryAmount: { before: "1000.00", after: "1200.00" },
      bank: { before: null, after: "bcp" },
    });
    expect(lines).toContain("Accesorios: 1000.00 → 1200.00");
    expect(lines).toContain("Banco: vacío → bcp");
  });

  it("never shows personal data, only that it changed", () => {
    expect(describeChanges({ signerFirstName: "changed" })).toEqual([
      "Nombre del firmante: modificado",
    ]);
  });

  it("names invoice fields", () => {
    expect(
      describeChanges({ "toyota.number": { before: null, after: "F001-1" } }),
    ).toEqual(["Factura Toyota · número: vacío → F001-1"]);
  });

  it("leaves out the keys that only identify the event", () => {
    expect(
      describeChanges({
        taskNumber: 3,
        slot: "quotation_pdf",
        automatic: true,
      }),
    ).toEqual([]);
    expect(describeChanges(null)).toEqual([]);
    expect(describeChanges(undefined)).toEqual([]);
  });
});

describe("describeEvent", () => {
  it("titles task events with the task", () => {
    expect(
      describeEvent(
        entry({ action: "COMPLETED" }),
        "Tarea 2 · Debida diligencia",
        undefined,
      ).title,
    ).toBe("Tarea 2 · Debida diligencia completada");
    expect(
      describeEvent(entry({ action: "CORRECTED" }), undefined, undefined).title,
    ).toBe("Tarea: datos corregidos");
  });

  it("titles sale and document events", () => {
    expect(
      describeEvent(
        entry({ entityType: "sale_register", action: "CREATED" }),
        undefined,
        undefined,
      ).title,
    ).toBe("Venta registrada");
    expect(
      describeEvent(
        entry({ entityType: "document", action: "REPLACED" }),
        undefined,
        "Cotización en PDF",
      ).title,
    ).toBe("Documento reemplazado: Cotización en PDF");
  });

  it("falls back to the raw names for events it does not know", () => {
    expect(
      describeEvent(
        entry({ entityType: "other", action: "ZAP" }),
        undefined,
        undefined,
      ).title,
    ).toBe("other · ZAP");
  });
});
