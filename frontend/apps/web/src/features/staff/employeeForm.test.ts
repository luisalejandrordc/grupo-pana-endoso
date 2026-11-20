import { describe, expect, it } from "vitest";
import type { Employee } from "./api";
import {
  emptyEmployeeForm,
  formFromEmployee,
  toCreateBody,
  toUpdateBody,
} from "./employeeForm";

const employee: Employee = {
  id: "e1",
  documentType: "DNI",
  documentNumber: "40123003",
  firstName: "Ana",
  lastName: "Rojas",
  nameSource: "manual",
  nationality: "Peruana",
  gender: "F",
  birthdate: "1992-04-10",
  hireDate: "2023-03-01",
  personalPhone: "+51 999 111 222",
  personalEmail: null,
  businessPhone: "+51 1 555 0100",
  businessEmail: "ana.rojas@grupopana.pe",
  role: "sales_advisor",
  isActive: true,
  hasPhoto: false,
  activatedAt: "2023-03-02T10:00:00Z",
  version: 7,
  createdAt: "2023-03-01T10:00:00Z",
  updatedAt: "2023-03-01T10:00:00Z",
};

describe("formFromEmployee", () => {
  it("turns missing personal contacts into empty text", () => {
    const form = formFromEmployee(employee);
    expect(form.personalEmail).toBe("");
    expect(form.personalPhone).toBe("+51 999 111 222");
    expect(form.role).toBe("sales_advisor");
  });
});

describe("toUpdateBody", () => {
  it("is null when nothing changed, so there is nothing to save", () => {
    expect(toUpdateBody(employee, formFromEmployee(employee))).toBeNull();
  });

  it("sends the version and only the fields that changed", () => {
    const form = {
      ...formFromEmployee(employee),
      businessPhone: "+51 1 555 0199",
      role: "sales_manager" as const,
    };
    expect(toUpdateBody(employee, form)).toEqual({
      version: 7,
      businessPhone: "+51 1 555 0199",
      role: "sales_manager",
    });
  });

  it("ignores spaces around a value, so retyping a space is not a change", () => {
    const form = {
      ...formFromEmployee(employee),
      firstName: "  Ana ",
      businessEmail: " ana.rojas@grupopana.pe",
    };
    expect(toUpdateBody(employee, form)).toBeNull();
  });

  it("clears a personal contact with an empty string", () => {
    const form = { ...formFromEmployee(employee), personalPhone: "" };
    expect(toUpdateBody(employee, form)).toEqual({
      version: 7,
      personalPhone: "",
    });
  });

  it("does not treat an empty personal contact as a change when there was none", () => {
    const form = { ...formFromEmployee(employee), personalEmail: "" };
    expect(toUpdateBody(employee, form)).toBeNull();
  });

  it("adds a personal contact that was missing", () => {
    const form = {
      ...formFromEmployee(employee),
      personalEmail: "ana@example.com",
    };
    expect(toUpdateBody(employee, form)).toEqual({
      version: 7,
      personalEmail: "ana@example.com",
    });
  });

  it("never sends the name's source: the backend marks an edited name as manual", () => {
    const form = { ...formFromEmployee(employee), lastName: "Rojas Vega" };
    expect(toUpdateBody(employee, form)).toEqual({
      version: 7,
      lastName: "Rojas Vega",
    });
  });
});

describe("toCreateBody", () => {
  it("trims, and leaves out personal contacts that were not given", () => {
    const body = toCreateBody({
      ...emptyEmployeeForm,
      documentNumber: " 45871236 ",
      firstName: " Luis ",
      lastName: "Quispe",
      birthdate: "1990-01-01",
      hireDate: "2024-01-01",
      businessPhone: "+51 1 555 0100",
      businessEmail: "luis@grupopana.pe",
    });
    expect(body.documentNumber).toBe("45871236");
    expect(body.firstName).toBe("Luis");
    expect(body.personalPhone).toBeUndefined();
    expect(body.personalEmail).toBeUndefined();
    expect(body.role).toBe("sales_advisor");
  });
});
