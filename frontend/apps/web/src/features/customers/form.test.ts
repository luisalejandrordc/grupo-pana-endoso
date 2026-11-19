import { describe, expect, it } from "vitest";
import { ApiError } from "../../shared/api/client";
import type { ApiCustomer } from "./api";
import {
  emptyForm,
  fieldErrorsFrom,
  formFromCustomer,
  toCreateBody,
  toUpdateBody,
  validateForm,
  withCustomerType,
} from "./form";
import { canLookUp, documentTypesFor, isValidDocumentNumber } from "./labels";

const person: ApiCustomer = {
  id: "c1",
  customerType: "diplomatico",
  documentType: "DNI",
  documentNumber: "00123456",
  firstName: "Lucía",
  lastName: "Quispe",
  gender: "F",
  legalName: null,
  displayName: "Lucía Quispe",
  nameSource: "looked_up",
  nationality: "Peruana",
  phone: "999111222",
  email: "lucia@test.pe",
  contactFirstName: null,
  contactLastName: null,
  contactJobTitle: null,
  portalStatus: "not_invited",
  isActive: true,
  version: 3,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

const validPerson = () => ({
  ...emptyForm(),
  documentNumber: "00123456",
  firstName: "Lucía",
  lastName: "Quispe",
  gender: "F" as const,
  nationality: "Peruana",
  phone: "999111222",
  email: "lucia@test.pe",
});

describe("document rules", () => {
  it("lets persons use DNI, CE, PAS or CDI and organizations only RUC", () => {
    expect(documentTypesFor("diplomatico")).toEqual([
      "DNI",
      "CE",
      "PAS",
      "CDI",
    ]);
    expect(documentTypesFor("embajada")).toEqual(["RUC"]);
  });

  it("validates formats like the server does", () => {
    expect(isValidDocumentNumber("DNI", "00123456")).toBe(true);
    expect(isValidDocumentNumber("DNI", "1234567")).toBe(false);
    expect(isValidDocumentNumber("RUC", "20 123 456 789")).toBe(true);
    expect(isValidDocumentNumber("CE", "ab1234567")).toBe(true);
    expect(isValidDocumentNumber("PAS", "AB1")).toBe(false);
  });

  it("only looks up DNI and RUC", () => {
    expect(canLookUp("DNI")).toBe(true);
    expect(canLookUp("RUC")).toBe(true);
    expect(canLookUp("CE")).toBe(false);
  });

  it("moves the document type along when the customer type changes", () => {
    const org = withCustomerType(validPerson(), "embajada");
    expect(org.documentType).toBe("RUC");
    const back = withCustomerType(
      { ...org, documentType: "RUC" },
      "diplomatico",
    );
    expect(back.documentType).toBe("DNI");
    const keeps = withCustomerType(
      { ...validPerson(), documentType: "CE" },
      "diplomatico",
    );
    expect(keeps.documentType).toBe("CE");
  });
});

describe("toCreateBody", () => {
  it("sends only the person fields for a person, normalized", () => {
    const body = toCreateBody({
      ...validPerson(),
      documentNumber: " 00 123 456 ",
      email: " Lucia@Test.PE ",
      legalName: "ignored",
    });
    expect(body).toEqual({
      customerType: "diplomatico",
      documentType: "DNI",
      documentNumber: "00123456",
      nationality: "Peruana",
      phone: "999111222",
      email: "lucia@test.pe",
      nameSource: "manual",
      firstName: "Lucía",
      lastName: "Quispe",
      gender: "F",
    });
  });

  it("sends only the organization fields for an organization", () => {
    const form = {
      ...withCustomerType(emptyForm(), "embajada"),
      documentNumber: "20123456789",
      legalName: " Embajada X ",
      contactFirstName: "Hiro",
      contactLastName: "Tanaka",
      contactJobTitle: "Agregado",
      nationality: "Japonesa",
      phone: "014440000",
      email: "x@y.pe",
      firstName: "ignored",
    };
    const body = toCreateBody(form);
    expect(body.legalName).toBe("Embajada X");
    expect(body.firstName).toBeUndefined();
    expect(body.contactJobTitle).toBe("Agregado");
  });
});

describe("toUpdateBody", () => {
  it("reports no change for an untouched form", () => {
    const { changed, body } = toUpdateBody(person, formFromCustomer(person));
    expect(changed).toBe(false);
    expect(body).toEqual({ version: 3 });
  });

  it("sends only what changed, with the version", () => {
    const form = {
      ...formFromCustomer(person),
      phone: "999000111",
      email: "LUCIA@test.pe ",
    };
    const { changed, body } = toUpdateBody(person, form);
    expect(changed).toBe(true);
    expect(body).toEqual({ version: 3, phone: "999000111" });
  });

  it("treats a differently-cased email or spaced document as unchanged", () => {
    const form = {
      ...formFromCustomer(person),
      email: "LUCIA@TEST.PE",
      documentNumber: "00 123 456",
    };
    expect(toUpdateBody(person, form).changed).toBe(false);
  });

  it("sends the whole new shape when the type changes", () => {
    const form = {
      ...withCustomerType(formFromCustomer(person), "embajada"),
      documentNumber: "20123456789",
      legalName: "Embajada X",
      contactFirstName: "Hiro",
      contactLastName: "Tanaka",
      contactJobTitle: "Agregado",
    };
    const { changed, body } = toUpdateBody(person, form);
    expect(changed).toBe(true);
    expect(body).toMatchObject({
      version: 3,
      customerType: "embajada",
      documentType: "RUC",
      documentNumber: "20123456789",
      legalName: "Embajada X",
      contactFirstName: "Hiro",
      contactLastName: "Tanaka",
      contactJobTitle: "Agregado",
      nationality: "Peruana",
      phone: "999111222",
    });
    expect(body).not.toHaveProperty("firstName");
  });
});

describe("validateForm", () => {
  it("accepts a complete person", () => {
    expect(validateForm(validPerson())).toEqual({});
  });

  it("lists every missing person field at once", () => {
    expect(Object.keys(validateForm(emptyForm())).sort()).toEqual(
      [
        "documentNumber",
        "email",
        "firstName",
        "gender",
        "lastName",
        "nationality",
        "phone",
      ].sort(),
    );
  });

  it("asks an organization for its own fields", () => {
    const errors = validateForm(withCustomerType(emptyForm(), "embajada"));
    expect(Object.keys(errors)).toEqual(
      expect.arrayContaining([
        "legalName",
        "contactFirstName",
        "contactLastName",
        "contactJobTitle",
      ]),
    );
    expect(errors).not.toHaveProperty("firstName");
  });

  it("rejects bad formats", () => {
    const errors = validateForm({
      ...validPerson(),
      documentNumber: "123",
      phone: "abc",
      email: "no-at",
    });
    expect(errors.documentNumber).toBeTruthy();
    expect(errors.phone).toBeTruthy();
    expect(errors.email).toBeTruthy();
  });
});

describe("fieldErrorsFrom", () => {
  it("maps API field problems and ignores unknown fields", () => {
    const error = new ApiError("Revise los datos", "VALIDATION_FAILED", 422, [
      {
        field: "email",
        code: "INVALID_EMAIL",
        message: "El correo no es válido.",
      },
      { field: "version.name", code: "X", message: "ignored" },
    ]);
    expect(fieldErrorsFrom(error)).toEqual({
      email: "El correo no es válido.",
    });
    expect(fieldErrorsFrom(new Error("boom"))).toEqual({});
  });
});
