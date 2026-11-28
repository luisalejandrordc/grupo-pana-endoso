import { describe, expect, it } from "vitest";
import {
  allFields,
  amountsDiffer,
  applyChange,
  applyLookup,
  isDirty,
  lockedKeys,
  taskForms,
  toDraft,
  toPayload,
} from "./taskForms";

describe("task form definitions", () => {
  it("define fields for every task and no key twice", () => {
    for (let number = 1; number <= 13; number++) {
      expect(taskForms[number]).toBeDefined();
      const keys = allFields(number).map((f) => f.key);
      expect(new Set(keys).size).toBe(keys.length);
    }
    expect(taskForms[7]).toEqual([]);
  });

  it("use the keys the API uses", () => {
    expect(allFields(1).map((f) => f.key)).toContain("quotedFinalPrice");
    expect(allFields(6).map((f) => f.key)).toContain(
      "releaseProcessing.mismatchReason",
    );
    expect(allFields(10).map((f) => f.key)).toContain("licensePlate");
    expect(allFields(13).map((f) => f.key)).toEqual([
      "surquilloTransferDate",
      "deliveryDate",
    ]);
  });
});

describe("toDraft", () => {
  it("turns the task data into text, with empty for no value", () => {
    const draft = toDraft(1, {
      initialToyotaAmount: "30000.00",
      initialCommission: null,
      quotedFinalPrice: "35000.00",
    });
    expect(draft.initialToyotaAmount).toBe("30000.00");
    expect(draft.initialCommission).toBe("");
    expect(draft.accessoryAmount).toBe("");
    expect(draft.quotedFinalPrice).toBe("35000.00");
  });

  it("reads the nested invoices of task 6", () => {
    const draft = toDraft(6, {
      toyota: {
        number: "F001-000123",
        amount: "30000.00",
        expectedAmount: "30000.00",
      },
      commission: { number: null },
    });
    expect(draft["toyota.number"]).toBe("F001-000123");
    expect(draft["toyota.expectedAmount"]).toBe("30000.00");
    expect(draft["commission.number"]).toBe("");
    expect(draft["accessories.amount"]).toBe("");
  });

  it("copes with no data at all", () => {
    expect(toDraft(9, undefined).personInCharge).toBe("");
  });
});

describe("toPayload", () => {
  const saved = toDraft(1, {
    initialToyotaAmount: "30000.00",
    initialCommission: "3000.00",
    accessoryAmount: "1000.00",
    releaseProcessingAmount: "1000.00",
    quotedFinalPrice: "35000.00",
  });

  it("sends nothing when nothing changed", () => {
    expect(toPayload(1, saved, { ...saved })).toEqual({});
    expect(isDirty(1, saved, { ...saved })).toBe(false);
  });

  it("sends only what changed, trimmed", () => {
    const payload = toPayload(1, saved, {
      ...saved,
      accessoryAmount: " 1200 ",
    });
    expect(payload).toEqual({ accessoryAmount: "1200" });
    expect(isDirty(1, saved, { ...saved, accessoryAmount: "1200" })).toBe(true);
  });

  it("sends null to clear a value that existed", () => {
    expect(toPayload(1, saved, { ...saved, initialCommission: "  " })).toEqual({
      initialCommission: null,
    });
  });

  it("does not send an empty field that was already empty", () => {
    const empty = toDraft(9, {});
    expect(toPayload(9, empty, { ...empty, personInCharge: "   " })).toEqual(
      {},
    );
  });

  it("never sends the calculated fields", () => {
    expect(toPayload(1, saved, { ...saved, quotedFinalPrice: "1" })).toEqual(
      {},
    );
    const payment = toDraft(4, { expectedAmount: "35000.00" });
    expect(toPayload(4, payment, { ...payment, expectedAmount: "9" })).toEqual(
      {},
    );
  });

  it("nests the fields of an invoice", () => {
    const base = toDraft(6, {});
    const payload = toPayload(6, base, {
      ...base,
      "toyota.number": "F001-000123",
      "toyota.amount": "29999",
      "toyota.mismatchReason": "Descuento del banco",
      "commission.date": "2026-03-05",
    });
    expect(payload).toEqual({
      toyota: {
        number: "F001-000123",
        amount: "29999",
        mismatchReason: "Descuento del banco",
      },
      commission: { date: "2026-03-05" },
    });
  });
});

describe("lockedKeys", () => {
  it("follows the financial lock table", () => {
    expect(lockedKeys(1, 1)).toEqual([]);
    expect(lockedKeys(1, 2)).toContain("accessoryAmount");
    expect(lockedKeys(3, 3)).toEqual([]);
    expect(lockedKeys(3, 4)).toEqual(["finalToyotaAmount"]);
    expect(lockedKeys(4, 5)).toEqual([]);
    expect(lockedKeys(4, 6)).toEqual(["depositedAmount"]);
    expect(lockedKeys(5, 13)).toEqual(["depositedAmount"]);
    expect(lockedKeys(2, 13)).toEqual([]);
    expect(lockedKeys(6, 13)).toEqual([]);
  });
});

describe("amountsDiffer", () => {
  it("compares amounts as cents", () => {
    expect(amountsDiffer("35000", "35000.00")).toBe(false);
    expect(amountsDiffer("34999.99", "35000.00")).toBe(true);
  });

  it("is false while either side is empty or not a number", () => {
    expect(amountsDiffer("", "35000.00")).toBe(false);
    expect(amountsDiffer("35000", undefined)).toBe(false);
    expect(amountsDiffer("abc", "35000.00")).toBe(false);
  });
});

describe("task 2: the signer's name source", () => {
  const empty = toDraft(2, {});
  const looked = toDraft(2, {
    signerDocumentType: "DNI",
    signerDocumentNumber: "45871236",
    signerFirstName: "LUIS ALBERTO",
    signerLastName: "QUISPE HUAMÁN",
    signerNameSource: "looked_up",
  });

  it("carries the source in the data without drawing it", () => {
    const field = allFields(2).find((f) => f.key === "signerNameSource")!;
    expect(field.def.kind).toBe("hidden");
    expect(empty.signerNameSource).toBe("");
  });

  it("makes a typed name manual", () => {
    const next = applyChange(2, empty, empty, "signerFirstName", "Luis");
    expect(next.signerNameSource).toBe("manual");
    expect(toPayload(2, empty, next)).toEqual({
      signerFirstName: "Luis",
      signerNameSource: "manual",
    });
  });

  it("puts the source back when the names are back to what the server holds", () => {
    const edited = applyChange(
      2,
      looked,
      { ...looked, signerNameSource: "manual" },
      "signerFirstName",
      "LUIS ALBERTO X",
    );
    expect(edited.signerNameSource).toBe("manual");
    const back = applyChange(
      2,
      looked,
      edited,
      "signerFirstName",
      "LUIS ALBERTO",
    );
    expect(back.signerNameSource).toBe("looked_up");
    expect(isDirty(2, looked, back)).toBe(false);
  });

  it("clears a looked-up name when its document changes", () => {
    const type = applyChange(2, looked, looked, "signerDocumentType", "CE");
    expect(type).toMatchObject({
      signerFirstName: "",
      signerLastName: "",
      signerNameSource: "manual",
    });
    const number = applyChange(
      2,
      looked,
      looked,
      "signerDocumentNumber",
      "45871237",
    );
    expect(number.signerFirstName).toBe("");
    expect(number.signerNameSource).toBe("manual");
  });

  it("leaves typed names alone when the document changes", () => {
    const typed = applyChange(2, empty, empty, "signerFirstName", "Luis");
    const next = applyChange(
      2,
      empty,
      typed,
      "signerDocumentNumber",
      "45871236",
    );
    expect(next.signerFirstName).toBe("Luis");
    expect(next.signerNameSource).toBe("manual");
  });

  it("marks what a lookup filled as looked up", () => {
    const next = applyLookup(empty, { firstName: "LUIS", lastName: "QUISPE" });
    expect(next).toMatchObject({
      signerFirstName: "LUIS",
      signerLastName: "QUISPE",
      signerNameSource: "looked_up",
    });
    // A lookup with no names (a RUC answer) leaves what was there.
    expect(applyLookup(next, {}).signerFirstName).toBe("LUIS");
  });

  it("changes nothing special in other tasks", () => {
    const draft = toDraft(10, {});
    expect(applyChange(10, draft, draft, "licensePlate", "ABC123")).toEqual({
      ...draft,
      licensePlate: "ABC123",
    });
  });
});

describe("limits of the free-text fields", () => {
  it("count the reason for a difference, as the server does", () => {
    for (const number of [4, 5]) {
      const def = allFields(number).find(
        (f) => f.def.key === "mismatchReason",
      )!;
      expect(def.def.limit).toBe(500);
    }
    const invoices = allFields(6).filter((f) => f.def.key === "mismatchReason");
    expect(invoices).toHaveLength(4);
    for (const f of invoices) expect(f.def.limit).toBe(500);
  });
});
