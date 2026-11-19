// @vitest-environment node
/**
 * Runs the real API client against a real backend, uploading real files. Skipped unless INTEGRATION_API is
 * set; see customers.integration.test.ts for the variables. The backend must serve its images (local disk
 * storage, or R2 with a public address) so the test can fetch them back.
 */
import { describe, expect, it } from "vitest";
import { api, ApiError } from "../../shared/api/client";
import { catalogApi } from "./api";
import { createErrorsFrom } from "./form";

declare const process: { env: Record<string, string | undefined> };

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

// A real 1x1 PNG, a JPEG header, and a WebP header: the server decides the type from these bytes.
const PNG = Uint8Array.from(
  atob(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg==",
  ),
  (c) => c.charCodeAt(0),
);
const file = (bytes: Uint8Array, name: string, type: string) =>
  new File([bytes as BlobPart], name, { type });
const tag = () => String(Math.floor(Math.random() * 1e6)).padStart(6, "0");

describe.skipIf(!enabled)("catalog against the real backend", () => {
  it("creates, serves, edits, archives and deletes a model with real files", async () => {
    await signIn(process.env.INTEGRATION_MANAGER);
    const t = tag();
    const name = `Hilux ${t}`;

    // ---- create with the Main view image ----
    const created = await catalogApi.create(
      {
        name,
        description: "Pick-up de prueba",
        categories: ["camioneta", "comercial"],
        version: { name: "SR 4x2", referencePrice: "35000.50" },
        color: { name: "Blanco" },
      },
      file(PNG, "blanco.png", "image/png"),
    );
    expect(created).toMatchObject({
      name,
      isActive: true,
      version: 1,
      categories: ["camioneta", "comercial"],
    });
    expect(created.versions[0]).toMatchObject({
      name: "SR 4x2",
      referencePrice: "35000.50",
      isActive: true,
    });
    const color = created.colors[0];
    expect(color.images).toHaveLength(1);
    expect(color.images[0]).toMatchObject({
      viewpoint: "main_view",
      mimeType: "image/png",
    });

    // the image is really there, byte for byte
    const served = await fetch(color.mainImageUrl);
    expect(served.status).toBe(200);
    expect(served.headers.get("content-type")).toContain("image/png");
    expect(new Uint8Array(await served.arrayBuffer())).toEqual(PNG);

    // ---- rejections ----
    await expect(
      catalogApi.create(
        {
          name: `Falso ${t}`,
          categories: ["sedan"],
          version: { name: "X", referencePrice: "1" },
          color: { name: "Y" },
        },
        file(
          new TextEncoder().encode("esto no es una imagen"),
          "falso.png",
          "image/png",
        ),
      ),
    ).rejects.toMatchObject({ code: "IMAGE_TYPE_NOT_ALLOWED", status: 422 });
    await expect(
      catalogApi.create(
        {
          name,
          categories: ["sedan"],
          version: { name: "X", referencePrice: "1" },
          color: { name: "Y" },
        },
        file(PNG, "a.png", "image/png"),
      ),
    ).rejects.toMatchObject({ code: "MODEL_NAME_ALREADY_USED", status: 409 });
    const bad = await catalogApi
      .create(
        {
          name: `Precio ${t}`,
          categories: ["sedan"],
          version: { name: "X", referencePrice: "10.999" },
          color: { name: "Y" },
        },
        file(PNG, "a.png", "image/png"),
      )
      .catch((e) => e);
    expect(bad).toBeInstanceOf(ApiError);
    expect(bad).toMatchObject({ code: "VALIDATION_FAILED" });
    expect(createErrorsFrom(bad)).toHaveProperty("referencePrice"); // "version.referencePrice" mapped to the form

    // ---- images ----
    const jpeg = Uint8Array.from([
      0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 0x4a, 0x46, 0x49, 0x46, 0, 1, 1, 0, 0, 1,
      0, 1, 0, 0, 0xff, 0xd9,
    ]);
    let withInterior = await catalogApi.putImage(
      color.id,
      "interior",
      file(jpeg, "int.jpg", "image/jpeg"),
    );
    expect(withInterior.images.map((i) => i.viewpoint)).toEqual([
      "main_view",
      "interior",
    ]);
    expect(
      withInterior.images.find((i) => i.viewpoint === "interior"),
    ).toMatchObject({ mimeType: "image/jpeg" });

    const oldMain = color.mainImageUrl;
    withInterior = await catalogApi.putImage(
      color.id,
      "main_view",
      file(PNG, "nueva.png", "image/png"),
    );
    expect(withInterior.mainImageUrl).not.toBe(oldMain);
    expect((await fetch(withInterior.mainImageUrl)).status).toBe(200);
    expect((await fetch(oldMain)).status).toBe(404); // the replaced file was deleted after the commit

    await expect(
      catalogApi.deleteImage(color.id, "main_view"),
    ).rejects.toMatchObject({ code: "MAIN_IMAGE_REQUIRED", status: 422 });
    const interiorUrl = withInterior.images.find(
      (i) => i.viewpoint === "interior",
    )!.url;
    expect(
      (await catalogApi.deleteImage(color.id, "interior")).images,
    ).toHaveLength(1);
    expect((await fetch(interiorUrl)).status).toBe(404);

    // ---- versions and colors ----
    const second = await catalogApi.createVersion(created.id, {
      name: "SRX 4x4",
      referencePrice: "42000",
    });
    expect(second.referencePrice).toBe("42000.00");
    const edited = await catalogApi.updateVersion(second.id, {
      version: 1,
      referencePrice: "43000.5",
    });
    expect(edited).toMatchObject({ referencePrice: "43000.50", version: 2 });
    await expect(
      catalogApi.updateVersion(second.id, { version: 1, name: "x" }),
    ).rejects.toMatchObject({ code: "VERSION_CONFLICT" });
    await expect(
      catalogApi.archiveVersion(created.versions[0].id),
    ).resolves.toMatchObject({ isActive: false });
    await expect(catalogApi.archiveVersion(second.id)).rejects.toMatchObject({
      code: "LAST_ACTIVE_VERSION",
      status: 409,
    });
    await catalogApi.unarchiveVersion(created.versions[0].id);

    const red = await catalogApi.createColor(
      created.id,
      "Rojo",
      file(PNG, "rojo.png", "image/png"),
    );
    expect(red).toMatchObject({ name: "Rojo", isActive: true });
    expect(
      (
        await catalogApi.updateColor(red.id, {
          version: 1,
          name: "Rojo cereza",
        })
      ).name,
    ).toBe("Rojo cereza");
    await catalogApi.archiveColor(red.id);
    await expect(catalogApi.archiveColor(color.id)).rejects.toMatchObject({
      code: "LAST_ACTIVE_COLOR",
    });

    // ---- reading and the sale form's list ----
    const detail = await catalogApi.get(created.id);
    expect(detail.versions).toHaveLength(2);
    expect(detail.colors.map((c) => c.name).sort()).toEqual([
      "Blanco",
      "Rojo cereza",
    ]);
    const list = await catalogApi.list({ q: t, status: "all", pageSize: 12 });
    expect(list.total).toBe(1);
    expect(list.items[0]).toMatchObject({
      id: created.id,
      activeVersionCount: 2,
      activeColorCount: 1,
      minPrice: "35000.50",
    });
    const selectable = await api.request<{
      items: { id: string; versions: unknown[]; colors: unknown[] }[];
    }>("/catalog/selectable");
    const offered = selectable.items.find((m) => m.id === created.id);
    expect(offered?.versions).toHaveLength(2);
    expect(offered?.colors).toHaveLength(1); // the archived color is not offered

    // ---- an advisor: read-only, active only ----
    await signIn(process.env.INTEGRATION_ADVISOR);
    expect(
      (await catalogApi.list({ q: t, status: "all", pageSize: 12 })).total,
    ).toBe(1);
    const advisorView = await catalogApi.get(created.id);
    expect(advisorView.colors.map((c) => c.name)).toEqual(["Blanco"]); // archived color hidden
    await expect(
      catalogApi.create(
        {
          name: `Otro ${t}`,
          categories: ["sedan"],
          version: { name: "X", referencePrice: "1" },
          color: { name: "Y" },
        },
        file(PNG, "a.png", "image/png"),
      ),
    ).rejects.toMatchObject({ code: "FORBIDDEN", status: 403 });
    await expect(catalogApi.archive(created.id)).rejects.toMatchObject({
      code: "FORBIDDEN",
    });

    // ---- the manager archives: advisors lose sight of the model ----
    await signIn(process.env.INTEGRATION_MANAGER);
    expect((await catalogApi.archive(created.id)).isActive).toBe(false);
    await signIn(process.env.INTEGRATION_ADVISOR);
    await expect(catalogApi.get(created.id)).rejects.toMatchObject({
      code: "MODEL_NOT_FOUND",
      status: 404,
    });
    expect((await catalogApi.list({ q: t, pageSize: 12 })).total).toBe(0);

    // ---- an admin deletes everything, and every file goes with it ----
    await signIn(process.env.INTEGRATION_MANAGER);
    await expect(catalogApi.remove(created.id, "x")).rejects.toMatchObject({
      code: "FORBIDDEN",
    });
    await signIn(process.env.INTEGRATION_ADMIN);
    await expect(catalogApi.remove(created.id, " ")).rejects.toMatchObject({
      code: "REASON_REQUIRED",
    });
    const urls = [withInterior.mainImageUrl, red.mainImageUrl];
    await catalogApi.remove(created.id, "prueba de integración");
    await expect(catalogApi.get(created.id)).rejects.toMatchObject({
      code: "MODEL_NOT_FOUND",
    });
    for (const url of urls) expect((await fetch(url)).status).toBe(404);
    await api.logout();
  });
});
