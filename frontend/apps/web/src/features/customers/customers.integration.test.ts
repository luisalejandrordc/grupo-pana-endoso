/**
 * Runs the real API client against a real backend. Skipped unless INTEGRATION_API is set, for example:
 *   INTEGRATION_API=1 VITE_API_BASE_URL=http://127.0.0.1:8080/api/v1 \
 *   INTEGRATION_ADMIN=DNI:12345678 INTEGRATION_MANAGER=DNI:... INTEGRATION_ADVISOR=DNI:... \
 *   INTEGRATION_PASSWORD=... npx vitest run src/features/customers/customers.integration.test.ts
 */
import { describe, expect, it } from "vitest";

// Vitest runs this file in Node; the app's own code never touches `process`.
declare const process: { env: Record<string, string | undefined> };
import { ApiClient, ApiError } from "../../shared/api/client";
import { customersApi } from "./api";
import { api } from "../../shared/api/client";
import {
  fieldErrorsFrom,
  toCreateBody,
  toUpdateBody,
  emptyForm,
  formFromCustomer,
} from "./form";

const enabled = Boolean(process.env.INTEGRATION_API);
const password = process.env.INTEGRATION_PASSWORD ?? "";

async function signIn(spec: string | undefined) {
  const [documentType, documentNumber] = (spec ?? ":").split(":");
  await api.login({
    documentType: documentType as "DNI",
    documentNumber,
    password,
    rememberMe: false,
  });
}

const unique = () => String(Math.floor(Math.random() * 1e6)).padStart(6, "0");

describe.skipIf(!enabled)("customers against the real backend", () => {
  it("covers create, list, edit, lookup, deactivate and delete with the real contract", async () => {
    // ---- as a manager: create, read, edit ----
    await signIn(process.env.INTEGRATION_MANAGER);
    const tag = unique();
    const form = {
      ...emptyForm(),
      documentNumber: `00${tag}`,
      firstName: `Lucía${tag}`,
      lastName: "Quispe",
      gender: "F" as const,
      nationality: "Peruana",
      phone: "999111222",
      email: `lucia-${tag}@test.pe`,
    };
    const created = await customersApi.create(toCreateBody(form));
    expect(Object.keys(created).sort()).toEqual(
      [
        "contactFirstName",
        "contactJobTitle",
        "contactLastName",
        "createdAt",
        "customerType",
        "displayName",
        "documentNumber",
        "documentType",
        "email",
        "firstName",
        "gender",
        "id",
        "isActive",
        "lastName",
        "legalName",
        "nameSource",
        "nationality",
        "phone",
        "portalStatus",
        "updatedAt",
        "version",
      ].sort(),
    );
    expect(created).toMatchObject({
      documentNumber: `00${tag}`,
      version: 1,
      isActive: true,
      portalStatus: "not_invited",
    });

    const list = await customersApi.list({
      q: tag,
      status: "all",
      page: 1,
      pageSize: 20,
    });
    expect(list.total).toBe(1);
    expect(list.items[0].id).toBe(created.id);

    const change = toUpdateBody(created, {
      ...formFromCustomer(created),
      phone: "999000111",
    });
    expect(change.body).toEqual({ version: 1, phone: "999000111" });
    const updated = await customersApi.update(created.id, change.body);
    expect(updated).toMatchObject({ phone: "999000111", version: 2 });

    // a stale version is a conflict with the documented code
    await expect(
      customersApi.update(created.id, change.body),
    ).rejects.toMatchObject({
      code: "VERSION_CONFLICT",
      status: 409,
    });

    // validation problems arrive per field and map onto the form's fields
    const bad = await customersApi
      .create(
        toCreateBody({
          ...form,
          documentNumber: `01${tag}`,
          email: "no-es-correo",
          phone: "x",
        }),
      )
      .catch((e) => e);
    expect(bad).toBeInstanceOf(ApiError);
    expect(bad.code).toBe("VALIDATION_FAILED");
    expect(Object.keys(fieldErrorsFrom(bad)).sort()).toEqual([
      "email",
      "phone",
    ]);

    // the same document again is the documented conflict
    await expect(customersApi.create(toCreateBody(form))).rejects.toMatchObject(
      { code: "CUSTOMER_ALREADY_EXISTS", status: 409 },
    );

    // existence lookup, full access for a manager
    const found = await customersApi.lookup("DNI", `00${tag}`);
    expect(found).toMatchObject({ exists: true, access: "full" });
    expect(found.customer?.id).toBe(created.id);
    expect((await customersApi.lookup("DNI", "11112222")).exists).toBe(false);

    // Codart is not configured in this environment: the documented fallback code
    await expect(
      customersApi.lookupIdentity("DNI", "45678901"),
    ).rejects.toMatchObject({ code: "LOOKUP_UNAVAILABLE", status: 503 });
    await expect(
      customersApi.lookupIdentity("CE", "AB1234567"),
    ).rejects.toMatchObject({ code: "LOOKUP_NOT_SUPPORTED" });
    // a customer we already have is answered from our database, not the provider
    const own = await customersApi.lookupIdentity("DNI", `00${tag}`);
    expect(own).toMatchObject({ source: "database", firstName: `Lucía${tag}` });

    // an organization
    const org = await customersApi.create({
      customerType: "embajada",
      documentType: "RUC",
      documentNumber: `20${tag}00000`.slice(0, 11),
      legalName: `Embajada ${tag}`,
      nationality: "Chilena",
      phone: "014440000",
      email: `emb-${tag}@test.pe`,
      contactFirstName: "Pedro",
      contactLastName: "Soto",
      contactJobTitle: "Secretario",
    });
    expect(org).toMatchObject({
      firstName: null,
      gender: null,
      legalName: `Embajada ${tag}`,
      displayName: `Embajada ${tag}`,
    });

    // deactivate and reactivate
    expect((await customersApi.deactivate(created.id)).isActive).toBe(false);
    expect(
      (await customersApi.list({ q: tag, status: "inactive", pageSize: 20 }))
        .total,
    ).toBe(1);
    expect((await customersApi.reactivate(created.id)).isActive).toBe(true);

    // a manager cannot delete
    await expect(customersApi.remove(created.id, "x")).rejects.toMatchObject({
      code: "FORBIDDEN",
      status: 403,
    });

    // ---- as an advisor: scope and the name-only answer ----
    await signIn(process.env.INTEGRATION_ADVISOR);
    expect((await customersApi.list({ q: tag, pageSize: 20 })).total).toBe(0);
    await expect(customersApi.get(created.id)).rejects.toMatchObject({
      code: "CUSTOMER_NOT_FOUND",
      status: 404,
    });
    const restricted = await customersApi.lookup("DNI", `00${tag}`);
    expect(restricted).toMatchObject({ exists: true, access: "name_only" });
    expect(restricted.customer).toBeUndefined();
    expect(JSON.stringify(restricted)).not.toContain(`lucia-${tag}@test.pe`);
    await expect(customersApi.deactivate(created.id)).rejects.toMatchObject({
      code: "FORBIDDEN",
    });

    const mine = await customersApi.create(
      toCreateBody({
        ...form,
        documentNumber: `02${tag}`,
        email: `mine-${tag}@test.pe`,
      }),
    );
    expect((await customersApi.list({ q: tag, pageSize: 20 })).total).toBe(1); // the advisor sees what they created
    expect((await customersApi.get(mine.id)).id).toBe(mine.id);

    // ---- as an admin: delete with a reason ----
    await signIn(process.env.INTEGRATION_ADMIN);
    await expect(customersApi.remove(created.id, " ")).rejects.toMatchObject({
      code: "REASON_REQUIRED",
    });
    for (const id of [created.id, org.id, mine.id]) {
      await customersApi.remove(id, "prueba de integración");
    }
    await expect(customersApi.get(created.id)).rejects.toMatchObject({
      code: "CUSTOMER_NOT_FOUND",
    });

    const quota = await api.request<{
      limit: number;
      used: number;
      remaining: number;
    }>("/lookups/identity/quota");
    expect(quota.limit).toBe(110);
    await api.logout();
  });

  it("is signed out after logout", async () => {
    const client = new ApiClient();
    await expect(
      client.request("/customers", { skipRefresh: true }),
    ).rejects.toMatchObject({ status: 401 });
  });
});
