// @vitest-environment node
/**
 * Runs the real API client against the real backend through the whole life of a sale: creating it, working its first
 * task with real documents, correcting, cancelling, reopening and deleting it, and the profile picture. Skipped
 * unless INTEGRATION_API is set. Against a database filled by `go run ./cmd/seed`:
 *   INTEGRATION_API=1 VITE_API_BASE_URL=http://127.0.0.1:8080/api/v1 INTEGRATION_PASSWORD='Demo-12345!' \
 *   INTEGRATION_MANAGER=DNI:40123002 INTEGRATION_ADMIN=DNI:40123001 INTEGRATION_ADVISOR=DNI:40123003 \
 *   npx vitest run src/features/sales/sales.integration.test.ts
 */
import { describe, expect, it } from "vitest";

declare const process: { env: Record<string, string | undefined> };
import { ApiError } from "@endosoflow/api";
import { api } from "../../shared/api/client";
import { customersApi } from "../customers/api";
import { documentsApi, pickersApi, salesApi, tasksApi } from "./api";
import { toDraft } from "./taskForms";

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

const pdf = () =>
  new File(
    [new TextEncoder().encode("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n")],
    "cotizacion.pdf",
    { type: "application/pdf" },
  );
async function xlsx() {
  // A minimal Office Open XML archive: three stored entries, which is all the type check looks for.
  const names = [
    "[Content_Types].xml",
    "xl/workbook.xml",
    "xl/worksheets/sheet1.xml",
  ];
  const enc = new TextEncoder();
  const parts: Uint8Array[] = [];
  const central: Uint8Array[] = [];
  let offset = 0;
  const u16 = (n: number) => new Uint8Array([n & 255, (n >> 8) & 255]);
  const u32 = (n: number) =>
    new Uint8Array([n & 255, (n >> 8) & 255, (n >> 16) & 255, (n >> 24) & 255]);
  const cat = (...a: Uint8Array[]) =>
    a.reduce((x, y) => {
      const z = new Uint8Array(x.length + y.length);
      z.set(x);
      z.set(y, x.length);
      return z;
    }, new Uint8Array());
  for (const name of names) {
    const body = enc.encode("<x/>");
    const crc = (() => {
      let c = ~0;
      for (const b of body) {
        c ^= b;
        for (let k = 0; k < 8; k++) c = (c >>> 1) ^ (0xedb88320 & -(c & 1));
      }
      return ~c >>> 0;
    })();
    const n = enc.encode(name);
    const local = cat(
      u32(0x04034b50),
      u16(20),
      u16(0),
      u16(0),
      u16(0),
      u16(0),
      u32(crc),
      u32(body.length),
      u32(body.length),
      u16(n.length),
      u16(0),
      n,
      body,
    );
    central.push(
      cat(
        u32(0x02014b50),
        u16(20),
        u16(20),
        u16(0),
        u16(0),
        u16(0),
        u16(0),
        u32(crc),
        u32(body.length),
        u32(body.length),
        u16(n.length),
        u16(0),
        u16(0),
        u16(0),
        u16(0),
        u32(0),
        u32(offset),
        n,
      ),
    );
    parts.push(local);
    offset += local.length;
  }
  const dir = cat(...central);
  const end = cat(
    u32(0x06054b50),
    u16(0),
    u16(0),
    u16(names.length),
    u16(names.length),
    u32(dir.length),
    u32(offset),
    u16(0),
  );
  return new File([cat(...parts, dir, end)], "cotizacion.xlsx", {
    type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  });
}

describe.skipIf(!enabled)("sales against the real backend", () => {
  it("covers a sale from creation to deletion with the real contract", async () => {
    await signIn(process.env.INTEGRATION_MANAGER);
    const customers = await customersApi.list({ pageSize: 1 } as never);
    const models = await pickersApi.selectableCatalog();
    const model = models.items[0];
    const version = model.versions[0];
    const color = model.colors[0];
    // the sale goes to the first advisor in the list; the advisor who signs in below must be a different one
    const advisors = (await pickersApi.advisors()).items.filter(
      (a) => a.firstName !== "Diego",
    );
    expect(advisors.length).toBeGreaterThan(0);
    expect(advisors[0]).toHaveProperty("hasPhoto");

    // ---- create ----
    const created = await salesApi.create({
      customerId: (customers as { items: { id: string }[] }).items[0].id,
      versionId: version.id,
      colorId: color.id,
      advisorId: advisors[0].id,
      temperature: "warm",
    });
    expect(created.code).toMatch(/^EF-\d{4}-\d{5}$/);
    expect(created.tasks).toHaveLength(13);
    expect(created.milestones).toHaveLength(6);
    expect(created.permissions.canCancel).toBe(true);
    expect(created.advisor.id).toBe(advisors[0].id);

    // ---- it appears in the list with its code and its first task ----
    const list = await salesApi.list({ q: created.code, pageSize: 5 });
    expect(list.items.map((s) => s.id)).toContain(created.id);
    expect(
      list.items.find((s) => s.id === created.id)!.currentTask?.number,
    ).toBe(1);

    // ---- the advisor works the first task ----
    // an advisor who does not own the sale cannot even see it
    await signIn(process.env.INTEGRATION_ADVISOR);
    await expect(tasksApi.get(created.id, 1)).rejects.toMatchObject({
      code: "SALE_NOT_FOUND",
    });
    await signIn(process.env.INTEGRATION_MANAGER);
    const started = await tasksApi.start(created.id, 1);
    expect(started.status).toBe("in_progress");
    expect(started.permissions.canComplete).toBe(true);

    // completing without data or documents names what is missing
    const refused = await tasksApi
      .complete(created.id, 1)
      .catch((e) => e as ApiError);
    expect(refused).toBeInstanceOf(ApiError);
    expect(
      (refused as ApiError).fields?.some((f) =>
        f.field.startsWith("documents."),
      ),
    ).toBe(true);

    // the form's own keys are accepted, amounts as text or numbers, with the version
    const saved = await tasksApi.saveData(created.id, 1, {
      version: started.version,
      data: {
        initialToyotaAmount: 30000,
        initialCommission: "2500.50",
        accessoryAmount: "1200",
        releaseProcessingAmount: "800",
      },
    });
    expect(saved.version).toBeGreaterThan(started.version);
    const draft = toDraft(
      1,
      saved.data as Record<string, unknown>,
      saved.calculated as Record<string, unknown>,
    );
    expect(draft.initialToyotaAmount).toBe("30000.00");
    expect(draft.quotedFinalPrice).not.toBe("");

    // a stale version is refused with the conflict code the screens rely on
    await expect(
      tasksApi.saveData(created.id, 1, {
        version: started.version,
        data: { accessoryAmount: "1" },
      }),
    ).rejects.toMatchObject({ code: "VERSION_CONFLICT" });

    // notes do not move the version
    const noted = await tasksApi.saveNotes(created.id, 1, "Llamar el lunes");
    expect(noted.notes).toBe("Llamar el lunes");
    expect(noted.version).toBe(saved.version);

    // documents: by content, not by name
    const bad = await documentsApi
      .upload(
        created.id,
        1,
        "quotation_pdf",
        new File(["MZ not a pdf"], "x.pdf", { type: "application/pdf" }),
      )
      .catch((e) => e as ApiError);
    expect((bad as ApiError).code).toBe("DOCUMENT_TYPE_NOT_ALLOWED");
    const uploaded = await documentsApi.upload(
      created.id,
      1,
      "quotation_pdf",
      pdf(),
    );
    expect(uploaded.fileName).toBe("cotizacion.pdf");
    await documentsApi.upload(created.id, 1, "quotation_excel", await xlsx());
    const link = await documentsApi.downloadUrl(uploaded.id);
    expect(link.url).toMatch(/^https?:/);

    const completed = await tasksApi.complete(created.id, 1);
    expect(completed.status).toBe("completed");

    // ---- the sale moved on; a correction needs a reason ----
    const after = await salesApi.get(created.id);
    expect(after.currentTask?.number).toBe(2);
    const detail = await tasksApi.get(created.id, 1);
    expect(detail.permissions.needsReason).toBe(true);
    await expect(
      tasksApi.saveData(created.id, 1, {
        version: detail.version,
        data: { accessoryAmount: "1300" },
      }),
    ).rejects.toMatchObject({ code: "REASON_REQUIRED" });
    const corrected = await tasksApi.saveData(created.id, 1, {
      version: detail.version,
      data: { accessoryAmount: "1300" },
      reason: "Error de digitación",
    });
    expect(
      (corrected.data as { accessoryAmount: string }).accessoryAmount,
    ).toBe("1300.00");

    // ---- history names what happened ----
    const history = await salesApi.history(created.id, 1, 50);
    const actions = history.items.map((e) => `${e.entityType}:${e.action}`);
    expect(actions.length).toBeGreaterThan(3);
    expect(history.items.some((e) => e.reason === "Error de digitación")).toBe(
      true,
    );

    // ---- cancel (category + reason), reopen, then delete with the code in the body ----
    const cancelled = await salesApi.cancel(
      created.id,
      "customer_withdrew",
      "El cliente desistió",
    );
    expect(cancelled.state).toBe("cancelled");
    expect(cancelled.cancellation?.text).toBe("El cliente desistió");
    const reopened = await salesApi.reopen(created.id, "El cliente volvió");
    expect(reopened.state).toBe("active");

    await signIn(process.env.INTEGRATION_ADMIN);
    await expect(
      salesApi.remove(created.id, "Prueba", "EF-0000-00000"),
    ).rejects.toBeInstanceOf(ApiError);
    await salesApi.remove(created.id, "Venta de prueba", created.code);
    await expect(salesApi.get(created.id)).rejects.toMatchObject({
      code: "SALE_NOT_FOUND",
    });
  }, 60_000);

  it("sets, shows and removes the signed-in employee's picture", async () => {
    await signIn(process.env.INTEGRATION_ADVISOR);
    const me = await api.me();
    const png = new File(
      [
        Uint8Array.from(
          atob(
            "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
          ),
          (c) => c.charCodeAt(0),
        ),
      ],
      "yo.png",
      { type: "image/png" },
    );
    const link = await api.setMyPhoto(png);
    expect(link.mimeType).toBe("image/png");
    expect((await api.me()).hasPhoto).toBe(true);
    expect((await api.employeePhoto(me.id)).url).toMatch(/^https?:/);
    await signIn(process.env.INTEGRATION_MANAGER); // any signed-in staff may look
    expect((await api.employeePhoto(me.id)).url).toMatch(/^https?:/);
    await signIn(process.env.INTEGRATION_ADVISOR);
    await api.removeMyPhoto();
    expect((await api.me()).hasPhoto).toBe(false);
    await expect(api.employeePhoto(me.id)).rejects.toMatchObject({
      code: "PHOTO_NOT_FOUND",
    });
  }, 30_000);
});
