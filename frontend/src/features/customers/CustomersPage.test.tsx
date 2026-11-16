import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Role } from "../../shared/types";
import type { ApiCustomer } from "./api";
import { CustomersPage } from "./CustomersPage";

// ---- a small fake of the customers API, so these tests cover the screen without a backend ----

const person = (over: Partial<ApiCustomer> = {}): ApiCustomer => ({
  id: "11111111-aaaa-bbbb-cccc-000000000001",
  customerType: "diplomatico",
  documentType: "DNI",
  documentNumber: "00123456",
  firstName: "Lucía",
  lastName: "Quispe",
  gender: "F",
  legalName: null,
  displayName: "Lucía Quispe",
  nameSource: "manual",
  nationality: "Peruana",
  phone: "999111222",
  email: "lucia@test.pe",
  contactFirstName: null,
  contactLastName: null,
  contactJobTitle: null,
  portalStatus: "not_invited",
  isActive: true,
  version: 1,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  ...over,
});

const embassy = (): ApiCustomer =>
  person({
    id: "22222222-aaaa-bbbb-cccc-000000000002",
    customerType: "embajada",
    documentType: "RUC",
    documentNumber: "20123456789",
    firstName: null,
    lastName: null,
    gender: null,
    legalName: "Embajada de Japón",
    displayName: "Embajada de Japón",
    contactFirstName: "Hiro",
    contactLastName: "Tanaka",
    contactJobTitle: "Agregado",
    email: "japon@test.pe",
    isActive: false,
    portalStatus: "invited",
  });

interface Call {
  method: string;
  path: string;
  params: URLSearchParams;
  body: Record<string, unknown> | null;
}

type Handler = (call: Call) => Response | undefined;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
const apiError = (
  status: number,
  code: string,
  message: string,
  fields?: unknown,
) => json({ error: { code, message, fields } }, status);

let customers: ApiCustomer[];
let calls: Call[];
let override: Handler | null;

function installFetch() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), "http://localhost");
      const call: Call = {
        method: (init?.method ?? "GET").toUpperCase(),
        path: url.pathname.replace("/api/v1", ""),
        params: url.searchParams,
        body: init?.body ? JSON.parse(String(init.body)) : null,
      };
      calls.push(call);
      const custom = override?.(call);
      if (custom) return custom;

      if (call.method === "GET" && call.path === "/customers") {
        const q = (call.params.get("q") ?? "").toLowerCase();
        const type = call.params.get("customerType");
        const status = call.params.get("status") ?? "all";
        const filtered = customers.filter(
          (c) =>
            c.displayName.toLowerCase().includes(q) &&
            (!type || c.customerType === type) &&
            (status === "all" || (status === "active") === c.isActive),
        );
        const page = Number(call.params.get("page") ?? 1);
        const size = Number(call.params.get("pageSize") ?? 20);
        return json({
          items: filtered.slice((page - 1) * size, page * size),
          page,
          pageSize: size,
          total: filtered.length,
        });
      }
      if (call.method === "GET" && call.path === "/customers/lookup")
        return json({ exists: false });
      if (call.method === "POST" && call.path === "/customers") {
        const created = person({
          id: "99999999-aaaa-bbbb-cccc-000000000009",
          ...(call.body as Partial<ApiCustomer>),
          displayName:
            `${call.body?.firstName ?? call.body?.legalName} ${call.body?.lastName ?? ""}`.trim(),
        });
        customers = [created, ...customers];
        return json(created, 201);
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
  const view = render(
    <QueryClientProvider client={client}>
      <CustomersPage query={query} role={role} toast={toast} />
    </QueryClientProvider>,
  );
  return {
    toast,
    client,
    ...view,
    rerenderWith: (q: string) =>
      view.rerender(
        <QueryClientProvider client={client}>
          <CustomersPage query={q} role={role} toast={toast} />
        </QueryClientProvider>,
      ),
  };
}

const last = <T,>(items: T[]) => items[items.length - 1];
const listCalls = () =>
  calls.filter(
    (c) =>
      c.method === "GET" &&
      c.path === "/customers" &&
      c.params.get("pageSize") === "20",
  );

beforeEach(() => {
  customers = [person(), embassy()];
  calls = [];
  override = null;
  installFetch();
});
afterEach(() => vi.unstubAllGlobals());

// ---- listing ----

describe("listing", () => {
  it("shows the customers and the totals from the API", async () => {
    renderPage();
    expect(await screen.findByText("Lucía Quispe")).toBeInTheDocument();
    expect(screen.getByText("Embajada de Japón")).toBeInTheDocument();
    expect(
      within(screen.getByRole("table")).getByText("Embajada"),
    ).toBeInTheDocument(); // type label, not the raw key
    expect(screen.getByText("Invitado")).toBeInTheDocument();
    expect(screen.getByText("2 registros")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByText(/Mostrando/)).toHaveTextContent("1–2 de 2"),
    );
  });

  it("asks the server for the search, filters and page instead of filtering in the browser", async () => {
    const user = userEvent.setup();
    const { rerenderWith } = renderPage();
    await screen.findByText("Lucía Quispe");

    rerenderWith("lucía");
    await waitFor(
      () => expect(last(listCalls())?.params.get("q")).toBe("lucía"),
      { timeout: 2000 },
    );
    expect(await screen.findByText("1 registros")).toBeInTheDocument();
    expect(screen.queryByText("Embajada de Japón")).not.toBeInTheDocument();

    rerenderWith("");
    await user.selectOptions(
      screen.getByLabelText("Tipo de cliente"),
      "embajada",
    );
    await waitFor(() =>
      expect(last(listCalls())?.params.get("customerType")).toBe("embajada"),
    );
    await user.selectOptions(screen.getByLabelText("Estado"), "inactive");
    await waitFor(() =>
      expect(last(listCalls())?.params.get("status")).toBe("inactive"),
    );
    expect(await screen.findByText("Embajada de Japón")).toBeInTheDocument();
  });

  it("pages through results and returns to page 1 when a filter changes", async () => {
    customers = Array.from({ length: 45 }, (_, i) =>
      person({
        id: `id-${i}`,
        displayName: `Cliente ${String(i).padStart(2, "0")}`,
      }),
    );
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("Cliente 00");
    expect(screen.getByText(/Página 1 de 3/)).toBeInTheDocument();

    await user.click(screen.getByLabelText("Página siguiente"));
    await screen.findByText("Cliente 20");
    expect(last(listCalls())?.params.get("page")).toBe("2");
    expect(screen.getByLabelText("Página anterior")).toBeEnabled();

    await user.click(screen.getByLabelText("Página siguiente"));
    await screen.findByText("Cliente 40");
    expect(screen.getByLabelText("Página siguiente")).toBeDisabled();

    await user.selectOptions(screen.getByLabelText("Estado"), "active");
    await screen.findByText(/Página 1 de 3/);
    expect(last(listCalls())?.params.get("page")).toBe("1");
  });

  it("explains an error and retries on demand", async () => {
    const user = userEvent.setup();
    let failing = true;
    override = (call) =>
      failing &&
      call.path === "/customers" &&
      call.params.get("pageSize") === "20"
        ? apiError(500, "INTERNAL_ERROR", "Ocurrió un error inesperado.")
        : undefined;
    renderPage();
    expect(
      await screen.findByText("No se pudo cargar la lista"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Ocurrió un error inesperado."),
    ).toBeInTheDocument();
    failing = false;
    await user.click(screen.getByRole("button", { name: /Reintentar/ }));
    expect(await screen.findByText("Lucía Quispe")).toBeInTheDocument();
  });

  it("tells an empty directory from an empty search", async () => {
    customers = [];
    const view = renderPage("sales_advisor");
    expect(await screen.findByText("Aún no hay clientes")).toBeInTheDocument();
    expect(
      screen.getByText(/cuando los creas o cuando se te asigna/),
    ).toBeInTheDocument();
    view.unmount();

    customers = [person()];
    const second = renderPage("sales_manager");
    await screen.findByText("Lucía Quispe");
    second.rerenderWith("zzz");
    expect(
      await screen.findByText("Sin resultados", {}, { timeout: 2000 }),
    ).toBeInTheDocument();
  });
});
