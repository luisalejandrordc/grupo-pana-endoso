import { describe, expect, it } from "vitest";
import { ApiError } from "../../shared/api/client";
import {
  createErrorsFrom,
  emptyModelForm,
  normalizePrice,
  priceProblem,
  toCreateModelData,
  validateCreateModel,
} from "./form";
import { imageProblem, MAX_IMAGE_BYTES } from "./images";
import { formatPrice } from "./labels";

describe("priceProblem", () => {
  it.each(["35000", "35000.5", "35000.50", "0.01", "9999999999.99"])(
    "accepts %s",
    (price) => {
      expect(priceProblem(price)).toBeNull();
    },
  );

  it.each([
    "",
    " ",
    "0",
    "0.00",
    "-5",
    "+5",
    "10.999",
    "abc",
    "1e5",
    "1,000",
    "12345678901",
    ".5",
    "5.",
  ])("rejects %j", (price) => {
    expect(priceProblem(price)).not.toBeNull();
  });
});

describe("normalizePrice", () => {
  it("always has two decimals, like the API returns", () => {
    expect(normalizePrice("35000")).toBe("35000.00");
    expect(normalizePrice(" 35000.5 ")).toBe("35000.50");
    expect(normalizePrice("0.01")).toBe("0.01");
  });
});

describe("formatPrice", () => {
  it("shows whole amounts without decimals and fractional ones with two", () => {
    expect(formatPrice("35000.00")).toMatch(/35[\s,.]?000$/);
    expect(formatPrice("19500.50")).toMatch(/19[\s,.]?500[.,]50$/);
  });
});

describe("create model form", () => {
  const valid = {
    ...emptyModelForm(),
    name: " Hilux ",
    description: " Pick-up ",
    categories: ["camioneta" as const],
    versionName: "SR",
    referencePrice: "35000",
    colorName: "Blanco",
  };

  it("lists every missing field at once, including the image", () => {
    expect(
      Object.keys(validateCreateModel(emptyModelForm(), false)).sort(),
    ).toEqual(
      [
        "categories",
        "colorName",
        "image",
        "name",
        "referencePrice",
        "versionName",
      ].sort(),
    );
    expect(validateCreateModel(valid, true)).toEqual({});
  });

  it("builds the API body trimmed, with the price normalized and no empty description", () => {
    expect(toCreateModelData(valid)).toEqual({
      name: "Hilux",
      description: "Pick-up",
      categories: ["camioneta"],
      version: { name: "SR", referencePrice: "35000.00" },
      color: { name: "Blanco" },
    });
    expect(
      toCreateModelData({ ...valid, description: "  " }),
    ).not.toHaveProperty("description");
  });

  it("maps the API's dotted field names onto the form", () => {
    const error = new ApiError("Revise", "VALIDATION_FAILED", 422, [
      {
        field: "version.referencePrice",
        code: "INVALID_PRICE",
        message: "Precio inválido.",
      },
      { field: "color.name", code: "REQUIRED", message: "Falta el color." },
      { field: "unknown", code: "X", message: "ignored" },
    ]);
    expect(createErrorsFrom(error)).toEqual({
      referencePrice: "Precio inválido.",
      colorName: "Falta el color.",
    });
  });
});

describe("imageProblem", () => {
  const file = (type: string, size: number) => {
    const f = new File(["x"], "a", { type });
    Object.defineProperty(f, "size", { value: size });
    return f;
  };

  it("accepts JPEG, PNG and WebP up to 5 MB", () => {
    for (const type of ["image/jpeg", "image/png", "image/webp"]) {
      expect(imageProblem(file(type, 1024))).toBeNull();
    }
    expect(imageProblem(file("image/png", MAX_IMAGE_BYTES))).toBeNull();
  });

  it("rejects other types, empty files and files over 5 MB", () => {
    expect(imageProblem(file("image/gif", 1024))).toMatch(/JPEG, PNG o WebP/);
    expect(imageProblem(file("application/pdf", 1024))).not.toBeNull();
    expect(imageProblem(file("image/png", MAX_IMAGE_BYTES + 1))).toMatch(
      /5 MB/,
    );
    expect(imageProblem(file("image/png", 0))).not.toBeNull();
  });
});
