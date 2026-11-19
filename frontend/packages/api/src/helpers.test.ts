import { describe, expect, it } from "vitest";
import { ApiError } from "./client";
import { errorText, fieldErrors } from "./errors";
import { queryString } from "./query";

describe("queryString", () => {
  it("leaves out what is empty and keeps the rest", () => {
    expect(
      queryString({
        q: "ana",
        page: 2,
        state: undefined,
        tag: "",
        on: false,
        x: null,
      }),
    ).toBe("?q=ana&page=2&on=false");
    expect(queryString({})).toBe("");
  });
});

describe("errorText", () => {
  it("prefers the server's message, then any Error's, then a generic one", () => {
    expect(
      errorText(new ApiError("Documento duplicado.", "CONFLICT", 409)),
    ).toBe("Documento duplicado.");
    expect(errorText(new Error("Failed to fetch"))).toBe("Failed to fetch");
    expect(errorText("???")).toBe("Ocurrió un error. Inténtalo nuevamente.");
  });
});

describe("fieldErrors", () => {
  it("keeps the first message of each field", () => {
    const error = new ApiError("x", "VALIDATION_FAILED", 422, [
      { field: "email", code: "A", message: "Primero" },
      { field: "email", code: "B", message: "Segundo" },
      { field: "phone", code: "C", message: "Teléfono" },
    ]);
    expect(fieldErrors(error)).toEqual({ email: "Primero", phone: "Teléfono" });
    expect(fieldErrors(new Error("x"))).toEqual({});
  });
});
