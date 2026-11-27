import { describe, expect, it } from "vitest";
import {
  addDays,
  filtersToSearch,
  limaToday,
  parseFilters,
  presetRange,
  toParams,
} from "./filters";

const ADVISOR = "0198aaaa-0000-7000-8000-000000000001";

describe("limaToday", () => {
  it("is the calendar day in Lima, not in UTC", () => {
    // 03:00 UTC on the 23rd is still the evening of the 22nd in Lima.
    expect(limaToday(new Date("2026-11-23T03:00:00Z"))).toBe("2026-11-22");
    expect(limaToday(new Date("2026-11-23T05:00:00Z"))).toBe("2026-11-23");
  });
});

describe("presetRange", () => {
  it("counts the last 90 days with today included, like the backend default", () => {
    expect(presetRange("90d", "2026-11-22")).toEqual({
      from: "2026-08-25",
      to: "2026-11-22",
    });
    expect(presetRange("30d", "2026-11-22").from).toBe("2026-10-24");
  });
  it("starts the month and the year on their first day", () => {
    expect(presetRange("month", "2026-11-22").from).toBe("2026-11-01");
    expect(presetRange("year", "2026-11-22").from).toBe("2026-01-01");
  });
  it("moves dates across months and leap years without time zones getting in the way", () => {
    expect(addDays("2028-03-01", -1)).toBe("2028-02-29");
    expect(addDays("2026-12-31", 1)).toBe("2027-01-01");
  });
});

describe("parseFilters", () => {
  it("falls back to the default period and drops anything invalid", () => {
    const f = parseFilters(
      new URLSearchParams(
        "period=forever&advisor=nope&type=alien&temp=lukewarm&model=1",
      ),
    );
    expect(f).toEqual({
      preset: "90d",
      from: undefined,
      to: undefined,
      advisorId: undefined,
      customerType: undefined,
      temperature: undefined,
      modelId: undefined,
    });
  });
  it("only accepts a custom period with two valid, ordered dates", () => {
    expect(
      parseFilters(
        new URLSearchParams("period=custom&from=2026-01-01&to=2026-02-01"),
      ),
    ).toMatchObject({ preset: "custom", from: "2026-01-01", to: "2026-02-01" });
    expect(
      parseFilters(
        new URLSearchParams("period=custom&from=2026-03-01&to=2026-02-01"),
      ).preset,
    ).toBe("90d");
    expect(parseFilters(new URLSearchParams("period=custom")).preset).toBe(
      "90d",
    );
  });
});

describe("filtersToSearch", () => {
  it("leaves the default period out and keeps unrelated parameters such as the tab", () => {
    const next = filtersToSearch(
      { preset: "90d", advisorId: ADVISOR, temperature: "hot" },
      new URLSearchParams("tab=finance&period=30d"),
    );
    expect(next.get("tab")).toBe("finance");
    expect(next.has("period")).toBe(false);
    expect(next.get("advisor")).toBe(ADVISOR);
    expect(next.get("temp")).toBe("hot");
  });
  it("round-trips a custom period", () => {
    const next = filtersToSearch(
      { preset: "custom", from: "2026-01-01", to: "2026-02-01" },
      new URLSearchParams(),
    );
    expect(parseFilters(next)).toMatchObject({
      preset: "custom",
      from: "2026-01-01",
      to: "2026-02-01",
    });
  });
});

describe("toParams", () => {
  it("never sends an advisor for someone who cannot pick one", () => {
    const filters = { preset: "30d" as const, advisorId: ADVISOR };
    expect(toParams(filters, "2026-11-22", false).advisorId).toBeUndefined();
    expect(toParams(filters, "2026-11-22", true).advisorId).toBe(ADVISOR);
  });
});
