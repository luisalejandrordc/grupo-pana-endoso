import { describe, expect, it } from "vitest";
import { formatDay, formatSize, whatsappLink } from "./format";

describe("formatDay", () => {
  it("keeps a calendar date on the day it names", () => {
    expect(formatDay("2026-03-01")).toBe("1 de marzo de 2026");
    expect(formatDay("2026-12-31")).toBe("31 de diciembre de 2026");
  });

  it("leaves what it cannot read as it came", () => {
    expect(formatDay("pronto")).toBe("pronto");
  });
});

describe("formatSize", () => {
  it("says bytes, kilobytes and megabytes plainly", () => {
    expect(formatSize(512)).toBe("512 B");
    expect(formatSize(2048)).toBe("2 KB");
    expect(formatSize(3 * 1024 * 1024)).toBe("3 MB");
    expect(formatSize(1.5 * 1024 * 1024)).toBe("1.5 MB");
  });
});

describe("whatsappLink", () => {
  it("links a Peruvian mobile and adds the country code", () => {
    expect(whatsappLink("987 654 321")).toBe("https://wa.me/51987654321");
  });

  it("links a number that already has its country code", () => {
    expect(whatsappLink("+57 300 123 4567")).toBe("https://wa.me/573001234567");
    expect(whatsappLink("51987654321")).toBe("https://wa.me/51987654321");
  });

  it("offers no link rather than a wrong one", () => {
    expect(whatsappLink("(01) 234 5678")).toBeNull();
    expect(whatsappLink("")).toBeNull();
  });
});
