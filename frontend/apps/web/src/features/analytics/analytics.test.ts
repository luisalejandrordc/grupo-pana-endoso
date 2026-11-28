import { describe, expect, it } from "vitest";
import {
  change,
  compactMoney,
  formatDuration,
  formatPercent,
  formatSpan,
} from "./format";
import {
  customRangeIsValid,
  limaDate,
  periodDates,
  salesQuery,
  tabsFor,
  type FilterState,
} from "./filters";

const state = (over: Partial<FilterState> = {}): FilterState => ({
  tab: "summary",
  period: "90d",
  from: "",
  to: "",
  advisor: "",
  customerType: "",
  model: "",
  temperature: "",
  ...over,
});

describe("formatting", () => {
  it("shows a missing figure as a dash", () => {
    expect(formatPercent(null)).toBe("—");
    expect(formatDuration(null)).toBe("—");
  });

  it("writes percentages and durations for people", () => {
    expect(formatPercent(0.256)).toMatch(/^25[,.]6\s%$/);
    expect(formatDuration(5.5)).toMatch(/^5[,.]5\sh$/);
    expect(formatDuration(72)).toBe("3 d");
    expect(formatSpan(0.2)).toBe("1 h");
    expect(formatSpan(100)).toBe("4 d");
  });

  it("abbreviates money for axes", () => {
    expect(compactMoney(950)).toBe("$950");
    expect(compactMoney(1500)).toMatch(/mil$/);
    expect(compactMoney(2_500_000)).toMatch(/M$/);
  });
});

describe("change against the previous period", () => {
  it("reports the direction and the size", () => {
    expect(change(120, 100)).toEqual({ trend: "up", text: "+20 %" });
    expect(change(75, 100)).toEqual({ trend: "down", text: "−25 %" });
    expect(change(100, 100)).toEqual({ trend: "flat", text: "Sin cambio" });
  });

  it("has nothing to compare when the previous figure is zero", () => {
    expect(change(0, 0)).toEqual({ trend: "flat", text: null });
    expect(change(5, 0)).toEqual({ trend: "up", text: "Nuevo" });
  });
});

describe("periods", () => {
  it("takes the date in Lima, five hours behind UTC", () => {
    expect(limaDate(new Date("2026-11-23T03:00:00Z"))).toBe("2026-11-22");
    expect(limaDate(new Date("2026-11-23T05:00:00Z"))).toBe("2026-11-23");
  });

  it("turns each preset into dates", () => {
    expect(periodDates("90d", "2026-11-22", "", "")).toEqual({});
    expect(periodDates("30d", "2026-11-22", "", "")).toEqual({
      from: "2026-10-24",
      to: "2026-11-22",
    });
    expect(periodDates("year", "2026-11-22", "", "")).toEqual({
      from: "2026-01-01",
      to: "2026-11-22",
    });
    expect(periodDates("lastYear", "2026-11-22", "", "")).toEqual({
      from: "2025-01-01",
      to: "2025-12-31",
    });
  });

  it("only passes a custom range made of real dates", () => {
    expect(periodDates("custom", "2026-11-22", "2026-01-05", "oops")).toEqual({
      from: "2026-01-05",
      to: undefined,
    });
  });

  it("refuses a custom range that is incomplete or reversed", () => {
    expect(customRangeIsValid(state({ period: "custom" }))).toBe(false);
    expect(
      customRangeIsValid(
        state({ period: "custom", from: "2026-03-10", to: "2026-03-01" }),
      ),
    ).toBe(false);
    expect(
      customRangeIsValid(
        state({ period: "custom", from: "2026-03-01", to: "2026-03-10" }),
      ),
    ).toBe(true);
    expect(customRangeIsValid(state())).toBe(true);
  });
});

describe("what a role sees", () => {
  it("gives each role its tabs", () => {
    expect(tabsFor("sales_advisor")).toEqual([
      "summary",
      "process",
      "commercial",
    ]);
    expect(tabsFor("sales_manager")).toContain("team");
    expect(tabsFor("sales_manager")).not.toContain("system");
    expect(tabsFor("system_admin")).toContain("system");
  });

  it("builds the query without empty filters", () => {
    expect(salesQuery(state(), "2026-11-22")).toEqual({
      advisorId: undefined,
      customerType: undefined,
      modelId: undefined,
      temperature: undefined,
    });
    expect(
      salesQuery(
        state({ advisor: "a-1", temperature: "hot", period: "30d" }),
        "2026-11-22",
      ),
    ).toMatchObject({
      advisorId: "a-1",
      temperature: "hot",
      from: "2026-10-24",
      to: "2026-11-22",
    });
  });
});
