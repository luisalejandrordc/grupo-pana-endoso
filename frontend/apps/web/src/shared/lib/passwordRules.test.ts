import { describe, expect, it } from "vitest";
import { isPasswordAcceptable, passwordRules } from "./passwordRules";

describe("passwordRules", () => {
  it("starts with nothing met", () => {
    expect(passwordRules("", "").map((rule) => rule.met)).toEqual([
      false,
      false,
    ]);
  });

  it("counts length and match separately", () => {
    expect(passwordRules("corta", "corta").map((rule) => rule.met)).toEqual([
      false,
      true,
    ]);
    expect(
      passwordRules("una-larga-clave", "otra").map((rule) => rule.met),
    ).toEqual([true, false]);
  });

  it("accepts exactly the minimum length, and counts characters rather than bytes", () => {
    expect(isPasswordAcceptable("1234567890", "1234567890")).toBe(true);
    expect(isPasswordAcceptable("123456789", "123456789")).toBe(false);
    expect(isPasswordAcceptable("contraseña", "contraseña")).toBe(true);
  });

  it("never treats two empty fields as a match", () => {
    expect(passwordRules("", "")[1].met).toBe(false);
    expect(isPasswordAcceptable("", "")).toBe(false);
  });
});
