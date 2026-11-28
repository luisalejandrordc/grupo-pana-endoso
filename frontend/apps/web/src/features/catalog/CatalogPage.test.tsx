import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Role } from "../../shared/types";
import type { ApiColor, ApiImage, ApiModel, ApiVersion } from "./api";
import { CatalogPage } from "./CatalogPage";

// ---- a stateful fake of the catalog API, so these tests cover the screens without a backend ----

const NOW = "2026-01-01T00:00:00Z";
const image = (viewpoint: ApiImage["viewpoint"], url: string): ApiImage => ({
  id: `img-${viewpoint}-${url}`,
  viewpoint,
  url,
  mimeType: "image/png",
  sizeBytes: 100,
  updatedAt: NOW,
});
const version = (
  id: string,
  name: string,
  price: string,
  over: Partial<ApiVersion> = {},
): ApiVersion => ({
  id,
  modelId: "m1",
  name,
  referencePrice: price,
  isActive: true,
  version: 1,
  createdAt: NOW,
  updatedAt: NOW,
  ...over,
});
const color = (
  id: string,
  name: string,
  over: Partial<ApiColor> = {},
): ApiColor => ({
  id,
  modelId: "m1",
  name,
  isActive: true,
  mainImageUrl: `https://img.test/${id}-main.png`,
  images: [image("main_view", `https://img.test/${id}-main.png`)],
  version: 1,
  createdAt: NOW,
  updatedAt: NOW,
  ...over,
});
const hilux = (over: Partial<ApiModel> = {}): ApiModel => ({
  id: "m1",
  name: "Hilux",
  description: "Pick-up robusta",
  categories: ["camioneta", "comercial"],
  isActive: true,
  versions: [
    version("v1", "SR 4x2", "35000.00"),
    version("v2", "SRX 4x4", "42500.50"),
  ],
  colors: [color("c1", "Blanco"), color("c2", "Negro")],
  version: 1,
  createdAt: NOW,
  updatedAt: NOW,
  ...over,
});
const yaris = (): ApiModel =>
  hilux({
    id: "m2",
    name: "Yaris",
    description: null,
    categories: ["sedan"],
    isActive: false,
    versions: [version("v9", "XLi", "21000.00", { modelId: "m2" })],
    colors: [color("c9", "Gris", { modelId: "m2" })],
  });

interface Sent {
  data: Record<string, unknown> | null;
  files: Record<string, File>;
}
interface Call {
  method: string;
  path: string;
  params: URLSearchParams;
  body: Record<string, unknown> | null;
  form: Sent | null;
}

const readBlob = (blob: Blob) =>
  new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = reject;
    reader.readAsText(blob);
  });

async function readForm(form: FormData): Promise<Sent> {
  const sent: Sent = { data: null, files: {} };
  for (const [key, value] of form.entries()) {
    if (key === "data" && typeof value !== "string")
      sent.data = JSON.parse(await readBlob(value));
    else if (typeof value !== "string") sent.files[key] = value as File;
  }
  return sent;
}

let models: ApiModel[];
let calls: Call[];
let override: ((call: Call) => Response | undefined) | null;

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
const apiError = (
  status: number,
  code: string,
  message: string,
  fields?: unknown,
) => json({ error: { code, message, fields } }, status);

function summary(m: ApiModel) {
  const active = m.versions.filter((v) => v.isActive);
  return {
    id: m.id,
    name: m.name,
    description: m.description,
    categories: m.categories,
    isActive: m.isActive,
    activeVersionCount: active.length,
    activeColorCount: m.colors.filter((c) => c.isActive).length,
    minPrice: active.length
      ? active.map((v) => v.referencePrice).sort()[0]
      : null,
    mainImageUrl: m.colors.find((c) => c.isActive)?.mainImageUrl ?? null,
    version: m.version,
    createdAt: NOW,
    updatedAt: NOW,
  };
}

function findColor(id: string) {
  for (const m of models) {
    const c = m.colors.find((x) => x.id === id);
    if (c) return { m, c };
  }
  return null;
}

function installFetch() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), "http://localhost");
      const isForm = init?.body instanceof FormData;
      const call: Call = {
        method: (init?.method ?? "GET").toUpperCase(),
        path: url.pathname.replace("/api/v1", ""),
        params: url.searchParams,
        body: init?.body && !isForm ? JSON.parse(String(init.body)) : null,
        form: isForm ? await readForm(init!.body as FormData) : null,
      };
      calls.push(call);
      const custom = override?.(call);
      if (custom) return custom;

      const { method, path } = call;
      if (method === "GET" && path === "/vehicle-models") {
        const q = (call.params.get("q") ?? "").toLowerCase();
        const cat = call.params.get("category");
        const status = call.params.get("status") ?? "all";
        const filtered = models.filter(
          (m) =>
            m.name.toLowerCase().includes(q) &&
            (!cat || m.categories.includes(cat as never)) &&
            (status === "all" || (status === "active") === m.isActive),
        );
        const page = Number(call.params.get("page") ?? 1);
        const size = Number(call.params.get("pageSize") ?? 20);
        return json({
          items: filtered.slice((page - 1) * size, page * size).map(summary),
          page,
          pageSize: size,
          total: filtered.length,
        });
      }
      const one = path.match(/^\/vehicle-models\/([^/]+)$/);
      if (one && method === "GET") {
        const m = models.find((x) => x.id === one[1]);
        return m
          ? json(m)
          : apiError(404, "MODEL_NOT_FOUND", "El modelo no existe.");
      }
      if (method === "POST" && path === "/vehicle-models") {
        const d = call.form!.data as {
          name: string;
          description?: string;
          categories: never[];
          version: { name: string; referencePrice: string };
          color: { name: string };
        };
        const m = hilux({
          id: "m-new",
          name: d.name,
          description: d.description ?? null,
          categories: d.categories,
          versions: [
            version("v-new", d.version.name, d.version.referencePrice, {
              modelId: "m-new",
            }),
          ],
          colors: [color("c-new", d.color.name, { modelId: "m-new" })],
        });
        models = [m, ...models];
        return json(m, 201);
      }
      const archiveModel = path.match(
        /^\/vehicle-models\/([^/]+)\/(archive|unarchive)$/,
      );
      if (archiveModel && method === "POST") {
        const m = models.find((x) => x.id === archiveModel[1])!;
        m.isActive = archiveModel[2] === "unarchive";
        return json(m);
      }
      if (one && method === "PATCH") {
        const m = models.find((x) => x.id === one[1])!;
        Object.assign(m, call.body, { version: m.version + 1 });
        delete (m as unknown as Record<string, unknown>)["version_"];
        return json(m);
      }
      if (one && method === "DELETE") {
        models = models.filter((x) => x.id !== one[1]);
        return new Response(null, { status: 204 });
      }
      const addVersion = path.match(/^\/vehicle-models\/([^/]+)\/versions$/);
      if (addVersion && method === "POST") {
        const m = models.find((x) => x.id === addVersion[1])!;
        const v = version(
          `v-${m.versions.length + 10}`,
          String(call.body!.name),
          String(call.body!.referencePrice),
          { modelId: m.id },
        );
        m.versions.push(v);
        return json(v, 201);
      }
      const ver = path.match(
        /^\/vehicle-versions\/([^/]+)(?:\/(archive|unarchive))?$/,
      );
      if (ver) {
        const m = models.find((x) => x.versions.some((v) => v.id === ver[1]))!;
        const v = m.versions.find((x) => x.id === ver[1])!;
        if (method === "PATCH")
          Object.assign(v, call.body, { version: v.version + 1 });
        if (method === "POST") v.isActive = ver[2] === "unarchive";
        if (method === "DELETE") {
          m.versions = m.versions.filter((x) => x.id !== v.id);
          return new Response(null, { status: 204 });
        }
        return json(v);
      }
      const addColor = path.match(/^\/vehicle-models\/([^/]+)\/colors$/);
      if (addColor && method === "POST") {
        const m = models.find((x) => x.id === addColor[1])!;
        const c = color(
          `c-${m.colors.length + 10}`,
          String((call.form!.data as { name: string }).name),
          { modelId: m.id },
        );
        m.colors.push(c);
        return json(c, 201);
      }
      const col = path.match(
        /^\/vehicle-colors\/([^/]+)(?:\/(archive|unarchive))?$/,
      );
      if (col) {
        const found = findColor(col[1])!;
        if (method === "PATCH")
          Object.assign(found.c, call.body, { version: found.c.version + 1 });
        if (method === "POST") found.c.isActive = col[2] === "unarchive";
        if (method === "DELETE") {
          found.m.colors = found.m.colors.filter((x) => x.id !== found.c.id);
          return new Response(null, { status: 204 });
        }
        return json(found.c);
      }
      const img = path.match(/^\/vehicle-colors\/([^/]+)\/images\/([a-z_]+)$/);
      if (img) {
        const { c } = findColor(img[1])!;
        const vp = img[2] as ApiImage["viewpoint"];
        if (method === "PUT") {
          const file = call.form!.files["file"];
          c.images = [
            ...c.images.filter((i) => i.viewpoint !== vp),
            image(vp, `https://img.test/${c.id}-${vp}-${file.name}`),
          ];
          if (vp === "main_view")
            c.mainImageUrl = c.images.find((i) => i.viewpoint === vp)!.url;
        } else c.images = c.images.filter((i) => i.viewpoint !== vp);
        return json(c);
      }
      return apiError(404, "ROUTE_NOT_FOUND", "Ruta no encontrada.");
    }),
  );
}

function renderPage(role: Role = "sales_manager", query = "") {
  const toast = vi.fn();
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });
  // The search lives in the page's own box, kept in the address as ?q=.
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={[query ? `/?q=${encodeURIComponent(query)}` : "/"]}
      >
        <CatalogPage role={role} toast={toast} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return {
    toast,
    ...view,
    /** Types into the search box, as a person would. */
    rerenderWith: (q: string) =>
      fireEvent.change(
        screen.getByRole("searchbox", { name: "Buscar en el catálogo" }),
        { target: { value: q } },
      ),
  };
}

const png = (name = "foto.png", size = 2048, type = "image/png") => {
  const f = new File(["x"], name, { type });
  Object.defineProperty(f, "size", { value: size });
  return f;
};
const callsTo = (method: string, pattern: RegExp) =>
  calls.filter((c) => c.method === method && pattern.test(c.path));
const last = <T,>(items: T[]) => items[items.length - 1];
const listCalls = () =>
  calls.filter(
    (c) =>
      c.method === "GET" &&
      c.path === "/vehicle-models" &&
      c.params.get("pageSize") === "12",
  );

beforeEach(() => {
  models = [hilux(), yaris()];
  calls = [];
  override = null;
  installFetch();
});
afterEach(() => vi.unstubAllGlobals());

async function openDetail(
  user: ReturnType<typeof userEvent.setup>,
  name = "Hilux",
) {
  await screen.findByText(name);
  await user.click(
    screen.getByRole("button", { name: `Ver detalle de ${name}` }),
  );
  return screen.findByRole("dialog", { name });
}

// ---- listing ----

describe("listing", () => {
  it("shows active models with their categories, counts and starting price", async () => {
    renderPage();
    expect(await screen.findByText("Hilux")).toBeInTheDocument();
    expect(screen.getByText("2 versiones")).toBeInTheDocument();
    expect(screen.getByText("2 colores")).toBeInTheDocument();
    const card = within(screen.getByText("Hilux").closest("article")!);
    expect(card.getByText("Camioneta")).toBeInTheDocument(); // the label, not the raw key
    expect(card.getByText("+1")).toBeInTheDocument();
    expect(screen.getByText(/35[\s,.]?000/)).toBeInTheDocument();
    expect(screen.getByAltText("Hilux")).toHaveAttribute(
      "src",
      "https://img.test/c1-main.png",
    );
    expect(screen.queryByText("Yaris")).not.toBeInTheDocument(); // archived models are hidden by default
    expect(last(listCalls())?.params.get("status")).toBe("active");
  });

  it("lets a manager look at archived models and all of them", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("Hilux");
    await user.selectOptions(screen.getByLabelText("Estado"), "archived");
    expect(await screen.findByText("Yaris")).toBeInTheDocument();
    expect(screen.getByText("Archivado")).toBeInTheDocument();
    expect(screen.queryByText("Hilux")).not.toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText("Estado"), "all");
    expect(await screen.findByText("Hilux")).toBeInTheDocument();
    expect(screen.getByText("Yaris")).toBeInTheDocument();
  });

  it("asks the server for search, category and page", async () => {
    const user = userEvent.setup();
    models = Array.from({ length: 30 }, (_, i) =>
      hilux({ id: `m${i}`, name: `Modelo ${String(i).padStart(2, "0")}` }),
    );
    const { rerenderWith } = renderPage();
    await screen.findByText("Modelo 00");
    expect(screen.getByText("Página 1 de 3")).toBeInTheDocument();
    await user.click(screen.getByLabelText("Página siguiente"));
    await screen.findByText("Modelo 12");
    expect(last(listCalls())?.params.get("page")).toBe("2");

    rerenderWith("modelo 07");
    await waitFor(
      () => expect(last(listCalls())?.params.get("q")).toBe("modelo 07"),
      { timeout: 2000 },
    );
    expect(last(listCalls())?.params.get("page")).toBe("1");
    await screen.findByText("Modelo 07");

    rerenderWith("");
    await user.selectOptions(screen.getByLabelText("Categoría"), "sedan");
    await waitFor(() =>
      expect(last(listCalls())?.params.get("category")).toBe("sedan"),
    );
    expect(await screen.findByText("Sin resultados")).toBeInTheDocument();
  });

  it("explains an error and retries on demand", async () => {
    const user = userEvent.setup();
    let failing = true;
    override = (call) =>
      failing &&
      call.path === "/vehicle-models" &&
      call.params.get("pageSize") === "12"
        ? apiError(500, "INTERNAL_ERROR", "Ocurrió un error inesperado.")
        : undefined;
    renderPage();
    expect(
      await screen.findByText("No se pudo cargar el catálogo"),
    ).toBeInTheDocument();
    failing = false;
    await user.click(screen.getByRole("button", { name: /Reintentar/ }));
    expect(await screen.findByText("Hilux")).toBeInTheDocument();
  });

  it("shows a placeholder when a model has no image and tells an empty catalog from an empty search", async () => {
    models = [];
    const first = renderPage("sales_manager");
    expect(
      await screen.findByText("El catálogo está vacío"),
    ).toBeInTheDocument();
    expect(screen.getByText(/Crea el primer modelo/)).toBeInTheDocument();
    first.unmount();

    models = [hilux({ colors: [] })];
    renderPage("sales_advisor");
    expect(await screen.findByText("Hilux")).toBeInTheDocument();
    expect(screen.queryByAltText("Hilux")).not.toBeInTheDocument();
  });

  it("gives advisors a read-only catalog of active models", async () => {
    renderPage("sales_advisor");
    await screen.findByText("Hilux");
    expect(
      screen.queryByRole("button", { name: /Nuevo modelo/ }),
    ).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Estado")).not.toBeInTheDocument();
    expect(last(listCalls())?.params.get("status")).toBe("active");
  });
});

// ---- creating a model ----

async function openCreate(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByText("Hilux");
  await user.click(screen.getByRole("button", { name: /Nuevo modelo/ }));
  return screen.getByRole("dialog", { name: "Nuevo modelo" });
}

async function fillCreate(
  dialog: HTMLElement,
  user: ReturnType<typeof userEvent.setup>,
) {
  await user.type(
    within(dialog).getByLabelText("Nombre del modelo"),
    " Land Cruiser ",
  );
  await user.type(within(dialog).getByLabelText(/Descripción/), "Todoterreno");
  await user.click(within(dialog).getByLabelText("Familiar"));
  await user.click(within(dialog).getByLabelText("Camioneta"));
  await user.type(within(dialog).getByLabelText("Nombre de la versión"), "VX");
  await user.type(
    within(dialog).getByLabelText(/Precio de referencia/),
    "85000.5",
  );
  await user.type(within(dialog).getByLabelText("Nombre del color"), "Negro");
}

describe("creating a model", () => {
  it("sends the data and the Main view image as multipart, then opens the new model", async () => {
    const user = userEvent.setup();
    const { toast } = renderPage();
    const dialog = await openCreate(user);
    await fillCreate(dialog, user);
    await user.upload(
      within(dialog).getByLabelText("Imagen de la vista principal"),
      png("vx.png"),
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Crear modelo" }),
    );

    await waitFor(() =>
      expect(callsTo("POST", /^\/vehicle-models$/)).toHaveLength(1),
    );
    const sent = callsTo("POST", /^\/vehicle-models$/)[0].form!;
    expect(sent.data).toEqual({
      name: "Land Cruiser",
      description: "Todoterreno",
      categories: ["familiar", "camioneta"],
      version: { name: "VX", referencePrice: "85000.50" },
      color: { name: "Negro" },
    });
    expect(sent.files["mainImage"].name).toBe("vx.png");
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Modelo creado", tone: "success" }),
    );
    // the detail of the new model opens so images and versions can be completed
    expect(
      await screen.findByRole("dialog", { name: "Land Cruiser" }),
    ).toBeInTheDocument();
  });

  it("marks every missing field and sends nothing", async () => {
    const user = userEvent.setup();
    renderPage();
    const dialog = await openCreate(user);
    await user.click(
      within(dialog).getByRole("button", { name: "Crear modelo" }),
    );
    expect(
      await within(dialog).findByText("Revisa los campos marcados."),
    ).toBeInTheDocument();
    for (const text of [
      "El nombre del modelo es obligatorio.",
      "Elige al menos una categoría.",
      "El nombre de la versión es obligatorio.",
      "El precio de referencia es obligatorio.",
      "El nombre del color es obligatorio.",
      "La imagen de la vista principal es obligatoria.",
    ]) {
      expect(within(dialog).getByText(text)).toBeInTheDocument();
    }
    expect(callsTo("POST", /^\/vehicle-models$/)).toHaveLength(0);
  });

  it("explains a bad price before sending", async () => {
    const user = userEvent.setup();
    renderPage();
    const dialog = await openCreate(user);
    await fillCreate(dialog, user);
    const price = within(dialog).getByLabelText(/Precio de referencia/);
    await user.clear(price);
    await user.type(price, "12,5");
    await user.upload(
      within(dialog).getByLabelText("Imagen de la vista principal"),
      png(),
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Crear modelo" }),
    );
    expect(
      await within(dialog).findByText(
        /Usa solo números, con hasta dos decimales/,
      ),
    ).toBeInTheDocument();
    expect(callsTo("POST", /^\/vehicle-models$/)).toHaveLength(0);
  });

  it("rejects a wrong file type or a file over 5 MB when it is chosen", async () => {
    const user = userEvent.setup({ applyAccept: false });
    renderPage();
    const dialog = await openCreate(user);
    const picker = within(dialog).getByLabelText(
      "Imagen de la vista principal",
    );
    await user.upload(picker, png("a.gif", 1024, "image/gif"));
    expect(
      await within(dialog).findByText("La imagen debe ser JPEG, PNG o WebP."),
    ).toBeInTheDocument();
    await user.upload(picker, png("big.png", 6 * 1024 * 1024));
    expect(
      await within(dialog).findByText("La imagen supera el máximo de 5 MB."),
    ).toBeInTheDocument();
    expect(within(dialog).getByText("Sin imagen")).toBeInTheDocument();
    await user.upload(picker, png("ok.png"));
    await waitFor(() =>
      expect(
        within(dialog).queryByText(/supera el máximo/),
      ).not.toBeInTheDocument(),
    );
    expect(within(dialog).queryByText("Sin imagen")).not.toBeInTheDocument();
  });

  it("puts the server's field problems next to the fields", async () => {
    const user = userEvent.setup();
    override = (call) =>
      call.method === "POST" && call.path === "/vehicle-models"
        ? apiError(422, "VALIDATION_FAILED", "Revise los datos ingresados.", [
            {
              field: "version.referencePrice",
              code: "INVALID_PRICE",
              message: "Precio rechazado por el servidor.",
            },
          ])
        : undefined;
    renderPage();
    const dialog = await openCreate(user);
    await fillCreate(dialog, user);
    await user.upload(
      within(dialog).getByLabelText("Imagen de la vista principal"),
      png(),
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Crear modelo" }),
    );
    expect(
      await within(dialog).findByText("Precio rechazado por el servidor."),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("dialog", { name: "Nuevo modelo" }),
    ).toBeInTheDocument();
  });

  it("explains a duplicate name and keeps the form", async () => {
    const user = userEvent.setup();
    override = (call) =>
      call.method === "POST" && call.path === "/vehicle-models"
        ? apiError(
            409,
            "MODEL_NAME_ALREADY_USED",
            "Ya existe un modelo con ese nombre.",
          )
        : undefined;
    renderPage();
    const dialog = await openCreate(user);
    await fillCreate(dialog, user);
    await user.upload(
      within(dialog).getByLabelText("Imagen de la vista principal"),
      png(),
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Crear modelo" }),
    );
    expect(
      await within(dialog).findByText("Ya existe un modelo con ese nombre."),
    ).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Nombre del modelo")).toHaveValue(
      " Land Cruiser ",
    );
  });
});

// ---- the model detail ----

describe("model detail", () => {
  it("shows the data, versions with prices and colors with their images", async () => {
    const user = userEvent.setup();
    renderPage();
    const dialog = await openDetail(user);
    expect(within(dialog).getByText("Pick-up robusta")).toBeInTheDocument();
    expect(within(dialog).getByText("SR 4x2")).toBeInTheDocument();
    expect(within(dialog).getByText(/42[\s,.]?500[.,]50/)).toBeInTheDocument();
    expect(
      within(dialog).getByAltText("Blanco, vista principal"),
    ).toHaveAttribute("src", "https://img.test/c1-main.png");
    expect(within(dialog).getAllByText("1 de 6 imágenes")).toHaveLength(2);
  });

  it("is read-only for advisors", async () => {
    const user = userEvent.setup();
    renderPage("sales_advisor");
    const dialog = await openDetail(user);
    for (const name of [
      /Editar/,
      /Agregar/,
      /Archivar/,
      /Eliminar/,
      /Renombrar/,
    ]) {
      expect(
        within(dialog).queryByRole("button", { name }),
      ).not.toBeInTheDocument();
    }
    expect(
      within(dialog).getAllByRole("button", { name: /Ver imágenes de/ }),
    ).toHaveLength(2);
  });

  it("adds a version with a normalized price and shows it", async () => {
    const user = userEvent.setup();
    renderPage();
    const dialog = await openDetail(user);
    await user.click(
      within(dialog).getByRole("button", { name: /Agregar versión/ }),
    );
    const form = await screen.findByRole("dialog", { name: "Agregar versión" });
    await user.click(within(form).getByRole("button", { name: "Guardar" }));
    expect(
      await within(form).findByText("El nombre de la versión es obligatorio."),
    ).toBeInTheDocument();
    expect(callsTo("POST", /versions$/)).toHaveLength(0);

    await user.type(
      within(form).getByLabelText("Nombre de la versión"),
      "GR Sport",
    );
    await user.type(
      within(form).getByLabelText(/Precio de referencia/),
      "61000",
    );
    await user.click(within(form).getByRole("button", { name: "Guardar" }));
    await waitFor(() => expect(callsTo("POST", /versions$/)).toHaveLength(1));
    expect(callsTo("POST", /versions$/)[0].body).toEqual({
      name: "GR Sport",
      referencePrice: "61000.00",
    });
    expect(await within(dialog).findByText("GR Sport")).toBeInTheDocument();
  });

  it("edits a version sending only what changed, with its version", async () => {
    const user = userEvent.setup();
    renderPage();
    const dialog = await openDetail(user);
    await user.click(
      within(dialog).getByRole("button", { name: "Editar versión SR 4x2" }),
    );
    const form = await screen.findByRole("dialog", { name: "Editar versión" });
    expect(
      within(form).getByRole("button", { name: "Guardar" }),
    ).toBeDisabled();
    const price = within(form).getByLabelText(/Precio de referencia/);
    await user.clear(price);
    await user.type(price, "36500.5");
    await user.click(within(form).getByRole("button", { name: "Guardar" }));
    await waitFor(() =>
      expect(callsTo("PATCH", /vehicle-versions\/v1$/)).toHaveLength(1),
    );
    expect(callsTo("PATCH", /vehicle-versions\/v1$/)[0].body).toEqual({
      version: 1,
      referencePrice: "36500.50",
    });
    await waitFor(() =>
      expect(
        within(dialog).getByText(/36[\s,.]?500[.,]50/),
      ).toBeInTheDocument(),
    );
  });

  it("archives and reactivates a version, and explains why the last one cannot be archived", async () => {
    const user = userEvent.setup();
    const { toast } = renderPage();
    const dialog = await openDetail(user);
    await user.click(
      within(dialog).getByRole("button", { name: "Archivar versión SRX 4x4" }),
    );
    await waitFor(() =>
      expect(callsTo("POST", /v2\/archive$/)).toHaveLength(1),
    );
    expect(
      await within(dialog).findByRole("button", {
        name: "Reactivar versión SRX 4x4",
      }),
    ).toBeInTheDocument();
    expect(within(dialog).getByText("Archivada")).toBeInTheDocument();

    override = (call) =>
      call.path.endsWith("/v1/archive")
        ? apiError(
            409,
            "LAST_ACTIVE_VERSION",
            "Un modelo activo necesita al menos una versión activa.",
          )
        : undefined;
    await user.click(
      within(dialog).getByRole("button", { name: "Archivar versión SR 4x2" }),
    );
    await waitFor(() =>
      expect(toast).toHaveBeenCalledWith(
        expect.objectContaining({
          tone: "error",
          message: "Un modelo activo necesita al menos una versión activa.",
        }),
      ),
    );
    override = null;
    await user.click(
      within(dialog).getByRole("button", { name: "Reactivar versión SRX 4x4" }),
    );
    await waitFor(() =>
      expect(callsTo("POST", /v2\/unarchive$/)).toHaveLength(1),
    );
  });

  it("adds a color together with its Main view image", async () => {
    const user = userEvent.setup();
    renderPage();
    const dialog = await openDetail(user);
    await user.click(
      within(dialog).getByRole("button", { name: /Agregar color/ }),
    );
    const form = await screen.findByRole("dialog", { name: "Agregar color" });
    await user.click(
      within(form).getByRole("button", { name: "Agregar color" }),
    );
    expect(
      await within(form).findByText(
        "La imagen de la vista principal es obligatoria.",
      ),
    ).toBeInTheDocument();
    expect(callsTo("POST", /colors$/)).toHaveLength(0);

    await user.type(within(form).getByLabelText("Nombre del color"), "Rojo");
    await user.upload(
      within(form).getByLabelText("Imagen de la vista principal"),
      png("rojo.png"),
    );
    await user.click(
      within(form).getByRole("button", { name: "Agregar color" }),
    );
    await waitFor(() => expect(callsTo("POST", /colors$/)).toHaveLength(1));
    const sent = callsTo("POST", /colors$/)[0].form!;
    expect(sent.data).toEqual({ name: "Rojo" });
    expect(sent.files["mainImage"].name).toBe("rojo.png");
    expect(await within(dialog).findByText("Rojo")).toBeInTheDocument();
  });

  it("renames, archives and reactivates a color", async () => {
    const user = userEvent.setup();
    renderPage();
    const dialog = await openDetail(user);
    await user.click(
      within(dialog).getByRole("button", { name: "Renombrar color Blanco" }),
    );
    const form = await screen.findByRole("dialog", { name: "Renombrar color" });
    const input = within(form).getByLabelText("Nombre del color");
    await user.clear(input);
    await user.type(input, "Blanco perla");
    await user.click(within(form).getByRole("button", { name: "Guardar" }));
    await waitFor(() =>
      expect(callsTo("PATCH", /vehicle-colors\/c1$/)[0]?.body).toEqual({
        version: 1,
        name: "Blanco perla",
      }),
    );
    expect(await within(dialog).findByText("Blanco perla")).toBeInTheDocument();

    await user.click(
      within(dialog).getByRole("button", { name: "Archivar color Negro" }),
    );
    await waitFor(() =>
      expect(callsTo("POST", /c2\/archive$/)).toHaveLength(1),
    );
    expect(
      await within(dialog).findByRole("button", {
        name: "Reactivar color Negro",
      }),
    ).toBeInTheDocument();
  });
});

// ---- images ----

describe("color images", () => {
  async function openImages(
    user: ReturnType<typeof userEvent.setup>,
    _role: Role = "sales_manager",
  ) {
    const dialog = await openDetail(user);
    await user.click(
      within(dialog).getByRole("button", { name: /imágenes de Blanco/ }),
    );
    return screen.findByRole("dialog", { name: /Imágenes · Blanco/ });
  }

  it("shows the six viewpoints, uploads a new one and replaces the Main view", async () => {
    const user = userEvent.setup();
    const { toast } = renderPage();
    const images = await openImages(user);
    expect(within(images).getByText("Frente")).toBeInTheDocument();
    expect(within(images).getAllByText("Sin imagen")).toHaveLength(5);

    await user.upload(
      within(images).getByLabelText("Subir imagen: Interior"),
      png("interior.png"),
    );
    await waitFor(() =>
      expect(callsTo("PUT", /images\/interior$/)).toHaveLength(1),
    );
    expect(
      callsTo("PUT", /images\/interior$/)[0].form!.files["file"].name,
    ).toBe("interior.png");
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Imagen guardada" }),
    );
    expect(
      await within(images).findByAltText("Blanco: Interior"),
    ).toBeInTheDocument();

    await user.upload(
      within(images).getByLabelText("Reemplazar imagen: Vista principal"),
      png("nueva.png"),
    );
    await waitFor(() =>
      expect(callsTo("PUT", /images\/main_view$/)).toHaveLength(1),
    );
    await waitFor(() =>
      expect(
        within(images).getByAltText("Blanco: Vista principal"),
      ).toHaveAttribute("src", expect.stringContaining("nueva.png")),
    );
  });

  it("never offers to remove the Main view, but lets an optional image go", async () => {
    const user = userEvent.setup();
    const { toast } = renderPage();
    const images = await openImages(user);
    expect(
      within(images).queryByLabelText("Quitar imagen: Vista principal"),
    ).not.toBeInTheDocument();
    await user.upload(
      within(images).getByLabelText("Subir imagen: Frente"),
      png("f.png"),
    );
    const remove = await within(images).findByLabelText(
      "Quitar imagen: Frente",
    );
    await user.click(remove);
    await waitFor(() =>
      expect(callsTo("DELETE", /images\/exterior_front$/)).toHaveLength(1),
    );
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Imagen eliminada" }),
    );
    await waitFor(() =>
      expect(
        within(images).queryByLabelText("Quitar imagen: Frente"),
      ).not.toBeInTheDocument(),
    );
  });

  it("checks the file before uploading and shows the server's refusal", async () => {
    const user = userEvent.setup({ applyAccept: false });
    renderPage();
    const images = await openImages(user);
    await user.upload(
      within(images).getByLabelText("Subir imagen: Interior"),
      png("x.gif", 1024, "image/gif"),
    );
    expect(
      await within(images).findByText(
        /Interior: La imagen debe ser JPEG, PNG o WebP/,
      ),
    ).toBeInTheDocument();
    expect(callsTo("PUT", /images/)).toHaveLength(0);

    override = (call) =>
      call.method === "PUT"
        ? apiError(
            503,
            "STORAGE_UNAVAILABLE",
            "No se pudo guardar la imagen. Intente nuevamente en unos minutos.",
          )
        : undefined;
    await user.upload(
      within(images).getByLabelText("Subir imagen: Interior"),
      png("ok.png"),
    );
    expect(
      await within(images).findByText(/No se pudo guardar la imagen/),
    ).toBeInTheDocument();
  });

  it("lets an advisor look at the images but not change them", async () => {
    const user = userEvent.setup();
    renderPage("sales_advisor");
    const dialog = await openDetail(user);
    await user.click(
      within(dialog).getByRole("button", { name: /Ver imágenes de Blanco/ }),
    );
    const images = await screen.findByRole("dialog", {
      name: /Imágenes · Blanco/,
    });
    expect(
      within(images).queryByLabelText(
        /Subir imagen|Reemplazar imagen|Quitar imagen/,
      ),
    ).not.toBeInTheDocument();
    expect(
      within(images).getByAltText("Blanco: Vista principal"),
    ).toBeInTheDocument();
  });
});

// ---- the model's own data and administration ----

describe("model administration", () => {
  it("edits only the fields that changed and sends the version", async () => {
    const user = userEvent.setup();
    const { toast } = renderPage();
    const dialog = await openDetail(user);
    await user.click(within(dialog).getByRole("button", { name: /Editar$/ }));
    const save = within(dialog).getByRole("button", {
      name: "Guardar cambios",
    });
    expect(save).toBeDisabled();

    const name = within(dialog).getByLabelText("Nombre del modelo");
    await user.clear(name);
    await user.type(name, "Hilux Revo");
    await user.click(within(dialog).getByLabelText("Hatchback"));
    await user.click(save);
    await waitFor(() =>
      expect(callsTo("PATCH", /vehicle-models\/m1$/)).toHaveLength(1),
    );
    expect(callsTo("PATCH", /vehicle-models\/m1$/)[0].body).toEqual({
      version: 1,
      name: "Hilux Revo",
      categories: ["camioneta", "comercial", "hatchback"],
    });
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Modelo actualizado" }),
    );
  });

  it("will not leave a model without categories", async () => {
    const user = userEvent.setup();
    renderPage();
    const dialog = await openDetail(user);
    await user.click(within(dialog).getByRole("button", { name: /Editar$/ }));
    await user.click(within(dialog).getByLabelText("Camioneta"));
    await user.click(within(dialog).getByLabelText("Comercial"));
    await user.click(
      within(dialog).getByRole("button", { name: "Guardar cambios" }),
    );
    expect(
      await within(dialog).findByText("Elige al menos una categoría."),
    ).toBeInTheDocument();
    expect(callsTo("PATCH", /vehicle-models/)).toHaveLength(0);
  });

  it("explains a version conflict and loads the latest data on demand", async () => {
    const user = userEvent.setup();
    renderPage();
    const dialog = await openDetail(user);
    await user.click(within(dialog).getByRole("button", { name: /Editar$/ }));
    const name = within(dialog).getByLabelText("Nombre del modelo");
    await user.clear(name);
    await user.type(name, "Otro nombre");
    override = (call) =>
      call.method === "PATCH"
        ? apiError(
            409,
            "VERSION_CONFLICT",
            "Otro usuario modificó este registro.",
          )
        : undefined;
    await user.click(
      within(dialog).getByRole("button", { name: "Guardar cambios" }),
    );
    expect(
      await within(dialog).findByText(/Otra persona modificó este modelo/),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByRole("button", { name: "Guardar cambios" }),
    ).toBeDisabled();
    override = null;
    await user.click(
      within(dialog).getByRole("button", { name: "Cargar los cambios" }),
    );
    expect(
      await within(dialog).findByRole("button", { name: /Editar$/ }),
    ).toBeInTheDocument();
  });

  it("archives a model after confirmation and reactivates it directly", async () => {
    const user = userEvent.setup();
    const { toast } = renderPage();
    const dialog = await openDetail(user);
    await user.click(within(dialog).getByRole("button", { name: "Archivar" }));
    const confirm = await screen.findByRole("dialog", {
      name: "Archivar modelo",
    });
    await user.click(within(confirm).getByRole("button", { name: "Archivar" }));
    await waitFor(() =>
      expect(callsTo("POST", /m1\/archive$/)).toHaveLength(1),
    );
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Modelo archivado" }),
    );
    expect(
      await within(dialog).findByRole("button", { name: "Reactivar" }),
    ).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Reactivar" }));
    await waitFor(() =>
      expect(callsTo("POST", /m1\/unarchive$/)).toHaveLength(1),
    );
  });

  it("shows the server's reason when a model cannot be reactivated", async () => {
    const user = userEvent.setup();
    models = [yaris()];
    const { toast } = renderPage();
    const first = userEvent.setup();
    await screen.findByText("Sin resultados").catch(() => null);
    await first.selectOptions(screen.getByLabelText("Estado"), "archived");
    await user.click(
      await screen.findByRole("button", { name: "Ver detalle de Yaris" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Yaris" });
    override = (call) =>
      call.path.endsWith("/unarchive")
        ? apiError(
            422,
            "CATALOG_INCOMPLETE",
            "El modelo no tiene una versión activa.",
          )
        : undefined;
    await user.click(within(dialog).getByRole("button", { name: "Reactivar" }));
    await waitFor(() =>
      expect(toast).toHaveBeenCalledWith(
        expect.objectContaining({
          tone: "error",
          message: "El modelo no tiene una versión activa.",
        }),
      ),
    );
  });

  it("gives managers no delete buttons and admins reason-gated deletes", async () => {
    const user = userEvent.setup();
    const manager = renderPage("sales_manager");
    const managerDialog = await openDetail(user);
    expect(
      within(managerDialog).queryByRole("button", { name: /Eliminar/ }),
    ).not.toBeInTheDocument();
    manager.unmount();

    const { toast } = renderPage("system_admin");
    const dialog = await openDetail(user);
    await user.click(within(dialog).getByRole("button", { name: "Eliminar…" }));
    const confirm = await screen.findByRole("dialog", {
      name: "Eliminar modelo",
    });
    await user.click(
      within(confirm).getByRole("button", { name: "Eliminar definitivamente" }),
    );
    expect(
      await within(confirm).findByText("Escribe el motivo de la eliminación."),
    ).toBeInTheDocument();
    expect(callsTo("DELETE", /vehicle-models/)).toHaveLength(0);

    await user.type(
      within(confirm).getByLabelText(/Motivo/),
      "Creado por error",
    );
    await user.click(
      within(confirm).getByRole("button", { name: "Eliminar definitivamente" }),
    );
    await waitFor(() =>
      expect(callsTo("DELETE", /vehicle-models\/m1$/)).toHaveLength(1),
    );
    expect(
      callsTo("DELETE", /vehicle-models\/m1$/)[0].params.get("reason"),
    ).toBe("Creado por error");
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Modelo eliminado" }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });

  it("explains that a model in use by a sale cannot be deleted", async () => {
    const user = userEvent.setup();
    override = (call) =>
      call.method === "DELETE"
        ? apiError(
            409,
            "CATALOG_ENTRY_IN_USE",
            "No se puede eliminar: hay ventas que usan este modelo. Archívelo en su lugar.",
          )
        : undefined;
    renderPage("system_admin");
    const dialog = await openDetail(user);
    await user.click(within(dialog).getByRole("button", { name: "Eliminar…" }));
    const confirm = await screen.findByRole("dialog", {
      name: "Eliminar modelo",
    });
    await user.type(within(confirm).getByLabelText(/Motivo/), "Error");
    await user.click(
      within(confirm).getByRole("button", { name: "Eliminar definitivamente" }),
    );
    expect(
      await within(confirm).findByText(/hay ventas que usan este modelo/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("dialog", { name: "Eliminar modelo" }),
    ).toBeInTheDocument();
  });

  it("deletes a version and a color as admin, with a reason", async () => {
    const user = userEvent.setup();
    renderPage("system_admin");
    const dialog = await openDetail(user);
    await user.click(
      within(dialog).getByRole("button", { name: "Eliminar versión SRX 4x4" }),
    );
    let confirm = await screen.findByRole("dialog", {
      name: "Eliminar versión",
    });
    await user.type(within(confirm).getByLabelText(/Motivo/), "Duplicada");
    await user.click(
      within(confirm).getByRole("button", { name: "Eliminar definitivamente" }),
    );
    await waitFor(() =>
      expect(callsTo("DELETE", /vehicle-versions\/v2$/)).toHaveLength(1),
    );
    expect(
      callsTo("DELETE", /vehicle-versions\/v2$/)[0].params.get("reason"),
    ).toBe("Duplicada");
    await waitFor(() =>
      expect(within(dialog).queryByText("SRX 4x4")).not.toBeInTheDocument(),
    );

    await user.click(
      within(dialog).getByRole("button", { name: "Eliminar color Negro" }),
    );
    confirm = await screen.findByRole("dialog", { name: "Eliminar color" });
    await user.type(within(confirm).getByLabelText(/Motivo/), "Duplicado");
    await user.click(
      within(confirm).getByRole("button", { name: "Eliminar definitivamente" }),
    );
    await waitFor(() =>
      expect(callsTo("DELETE", /vehicle-colors\/c2$/)).toHaveLength(1),
    );
    await waitFor(() =>
      expect(within(dialog).queryByText("Negro")).not.toBeInTheDocument(),
    );
  });
});
