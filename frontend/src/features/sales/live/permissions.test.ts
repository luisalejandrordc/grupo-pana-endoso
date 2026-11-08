import { describe, expect, it } from "vitest";
import type { ApiSale } from "./api";
import { canCancel, canReopenSale, canWork, isManagement } from "./permissions";

const sale = (over: Partial<ApiSale> = {}) =>
  ({
    state: "active",
    advisorId: "adv-1",
    currentTaskNumber: 1,
    cancelledAtTaskNumber: null,
    ...over,
  }) as ApiSale;

describe("canWork", () => {
  it("needs an active sale, and for an advisor their own", () => {
    expect(canWork("sales_advisor", "adv-1", sale())).toBe(true);
    expect(canWork("sales_advisor", "adv-2", sale())).toBe(false);
    expect(canWork("sales_manager", "m-1", sale())).toBe(true);
    expect(canWork("system_admin", "a-1", sale({ state: "cancelled" }))).toBe(
      false,
    );
    expect(canWork("system_admin", "a-1", sale({ state: "completed" }))).toBe(
      false,
    );
  });
  it("tells management from advisors", () => {
    expect(isManagement("sales_advisor")).toBe(false);
    expect(isManagement("sales_manager")).toBe(true);
    expect(isManagement("system_admin")).toBe(true);
  });
});

describe("canCancel", () => {
  it("lets an advisor cancel their own sale through task 3", () => {
    for (let task = 1; task <= 13; task++)
      expect(
        canCancel("sales_advisor", "adv-1", sale({ currentTaskNumber: task })),
      ).toBe(task <= 3);
    expect(canCancel("sales_advisor", "adv-2", sale())).toBe(false);
  });
  it("lets a manager cancel through task 6 and an admin always", () => {
    for (let task = 1; task <= 13; task++) {
      expect(
        canCancel("sales_manager", "m-1", sale({ currentTaskNumber: task })),
      ).toBe(task <= 6);
      expect(
        canCancel("system_admin", "a-1", sale({ currentTaskNumber: task })),
      ).toBe(true);
    }
  });
  it("never cancels a sale that is not active", () => {
    expect(canCancel("system_admin", "a-1", sale({ state: "cancelled" }))).toBe(
      false,
    );
    expect(canCancel("system_admin", "a-1", sale({ state: "completed" }))).toBe(
      false,
    );
  });
});

describe("canReopenSale", () => {
  it("lets a manager reopen up to task 6 and only an admin after", () => {
    const cancelled = (task: number) =>
      sale({ state: "cancelled", cancelledAtTaskNumber: task });
    expect(canReopenSale("sales_manager", cancelled(6))).toBe(true);
    expect(canReopenSale("sales_manager", cancelled(7))).toBe(false);
    expect(canReopenSale("system_admin", cancelled(9))).toBe(true);
    expect(canReopenSale("sales_advisor", cancelled(1))).toBe(false);
    expect(canReopenSale("system_admin", sale())).toBe(false);
  });
});
