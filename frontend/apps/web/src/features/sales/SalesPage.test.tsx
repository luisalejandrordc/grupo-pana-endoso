import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Role } from "../../shared/types";
import type { SaleListItem } from "./api";
import { completedTasks } from "./progress";
import { SalesPage } from "./SalesPage";

const sale = (over: Partial<SaleListItem> = {}): SaleListItem => ({
  id: "0198aaaa-0000-7000-8000-000000000001",
  code: "EF-2026-00001",
  customerName: "Embajada de Japón",
  customerDocument: "RUC 20123456789",
  customerType: "embajada",
  vehicleModel: "Hilux",
  vehicleVersion: "SR",
  vehicleColor: "Blanco",
  imageUrl: null,
  advisorId: "adv-1",
  advisorName: "Ana Rojas",
  advisorHasPhoto: false,
  temperature: "hot",
  state: "active",
  currentTask: {
    number: 4,
    name: "Pago a Grupo Pana",
    status: "in_progress",
    milestoneNumber: 2,
    dueDate: "2026-03-09T10:00:00Z",
    isOverdue: false,
  },
  createdAt: "2026-03-02T10:00:00Z",
  updatedAt: "2026-03-02T10:00:00Z",
  ...over,
});

interface Call {
  path: string;
  params: URLSearchParams;
}
let calls: Call[];
let sales: SaleListItem[];
let failList: boolean;

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

beforeEach(() => {
  calls = [];
  failList = false;
  sales = [
    sale(),
    sale({
      id: "0198bbbb-0000-7000-8000-000000000002",
      code: "EF-2026-00002",
      customerName: "Lucía Quispe",
      state: "cancelled",
      temperature: "cold",
      currentTask: null,
    }),
    sale({
      id: "0198cccc-0000-7000-8000-000000000003",
      code: "EF-2026-00003",
      customerName: "Hiroshi Tanaka",
      currentTask: {
        number: 8,
        name: "Trámite Aduanero",
        status: "in_progress",
        milestoneNumber: 4,
        dueDate: "2026-03-01T10:00:00Z",
        isOverdue: true,
      },
    }),
  ];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input), "http://localhost");
      const path = url.pathname.replace("/api/v1", "");
      calls.push({ path, params: url.searchParams });
      if (path === "/advisors")
        return json({
          items: [
            {
              id: "adv-1",
              firstName: "Ana",
              lastName: "Rojas",
              hasPhoto: false,
            },
          ],
        });
      if (path === "/sales") {
        if (failList)
          return json(
            {
              error: { code: "INTERNAL", message: "El servidor no responde." },
            },
            500,
          );
        const state = url.searchParams.get("state");
        const items = sales.filter((s) => !state || s.state === state);
        return json({
          items,
          page: Number(url.searchParams.get("page") ?? 1),
          pageSize: 12,
          total: items.length,
        });
      }
      return json(
        { error: { code: "ROUTE_NOT_FOUND", message: "Ruta no encontrada." } },
        404,
      );
    }),
  );
});
afterEach(() => vi.unstubAllGlobals());

function Where() {
  const location = useLocation();
  return (
    <output data-testid="where">{location.pathname + location.search}</output>
  );
}

function renderPage(role: Role = "sales_manager", query = "", path = "/sales") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  const view = () => (
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[withQuery(path, query)]}>
        <Routes>
          <Route path="/sales" element={<SalesPage role={role} />} />
          <Route path="/sales/:id" element={<p>Detalle de venta</p>} />
          <Route path="/sales/new" element={<p>Registrar una venta</p>} />
        </Routes>
        <Where />
      </MemoryRouter>
    </QueryClientProvider>
  );
  render(view());
  return {
    /** Types into the search box, as a person would. */
    rerenderWith: (q: string) =>
      fireEvent.change(
        screen.getByRole("searchbox", { name: "Buscar ventas" }),
        {
          target: { value: q },
        },
      ),
  };
}

/** The search is kept in the address as ?q=. */
function withQuery(path: string, query: string) {
  if (!query) return path;
  return `${path}${path.includes("?") ? "&" : "?"}q=${encodeURIComponent(query)}`;
}

const listCalls = () => calls.filter((c) => c.path === "/sales");

describe("completedTasks", () => {
  it("is derived from the current task, because the list does not carry it", () => {
    expect(completedTasks(sale())).toBe(3);
    expect(
      completedTasks(sale({ currentTask: null, state: "completed" })),
    ).toBe(13);
    expect(
      completedTasks(sale({ currentTask: null, state: "cancelled" })),
    ).toBeNull();
    expect(completedTasks(sale({ currentTask: null }))).toBe(0);
  });
});

describe("SalesPage", () => {
  it("shows the sales in progress by default, each with its code, vehicle, progress and current task", async () => {
    renderPage();
    expect(await screen.findByText("Embajada de Japón")).toBeInTheDocument();
    expect(listCalls()[0].params.get("state")).toBe("active");
    expect(screen.queryByText("Lucía Quispe")).not.toBeInTheDocument();
    const row = screen.getByText("Embajada de Japón").closest("tr")!;
    expect(within(row).getByText("EF-2026-00001")).toBeInTheDocument();
    expect(within(row).getByText("Hilux")).toBeInTheDocument();
    expect(within(row).getByText("Ana Rojas")).toBeInTheDocument();
    expect(within(row).getByText("3/13")).toBeInTheDocument();
    expect(within(row).getByText(/4\. Pago a Grupo Pana/)).toBeInTheDocument();
    expect(within(row).getByText("Prioridad alta")).toBeInTheDocument();
  });

  it("marks a task past its deadline", async () => {
    renderPage();
    const row = (await screen.findByText("Hiroshi Tanaka")).closest("tr")!;
    expect(within(row).getByText("Con retraso")).toBeInTheDocument();
    expect(within(row).getByText(/Venció/)).toBeInTheDocument();
  });

  it("switches state from the tabs and keeps it in the address", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("Embajada de Japón");
    await user.click(screen.getByRole("tab", { name: "Canceladas" }));
    expect(await screen.findByText("Lucía Quispe")).toBeInTheDocument();
    expect(screen.queryByText("Embajada de Japón")).not.toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent("state=cancelled");
    await user.click(screen.getByRole("tab", { name: "En curso" }));
    await screen.findByText("Embajada de Japón");
    expect(screen.getByTestId("where")).not.toHaveTextContent("state=");
  });

  it("restores its filters from the address, so a refresh or a shared link shows the same list", async () => {
    renderPage(
      "sales_manager",
      "",
      "/sales?state=cancelled&temperature=cold&sort=customerName",
    );
    await screen.findByText("Lucía Quispe");
    const params = listCalls()[0].params;
    expect(params.get("state")).toBe("cancelled");
    expect(params.get("temperature")).toBe("cold");
    expect(params.get("sort")).toBe("customerName");
    expect(screen.getByRole("tab", { name: "Canceladas" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("sends the search once typing settles", async () => {
    const view = renderPage();
    await screen.findByText("Embajada de Japón");
    view.rerenderWith("Tanaka");
    await waitFor(
      () =>
        expect(listCalls().some((c) => c.params.get("q") === "Tanaka")).toBe(
          true,
        ),
      { timeout: 2000 },
    );
  });

  it("filters by advisor for management only", async () => {
    renderPage("sales_manager");
    await screen.findByText("Embajada de Japón");
    expect(screen.getByLabelText("Asesor")).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "Asesor" }),
    ).toBeInTheDocument();
  });

  it("gives an advisor no advisor filter or column: every sale in the list is theirs", async () => {
    renderPage("sales_advisor");
    await screen.findByText("Embajada de Japón");
    expect(screen.queryByLabelText("Asesor")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("columnheader", { name: "Asesor" }),
    ).not.toBeInTheDocument();
    expect(calls.some((c) => c.path === "/advisors")).toBe(false);
  });

  it("opens a sale from its row, by click or keyboard", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(
      (await screen.findByText("Embajada de Japón")).closest("tr")!,
    );
    expect(await screen.findByText("Detalle de venta")).toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent(
      "/sales/0198aaaa-0000-7000-8000-000000000001",
    );
  });

  it("goes to the new-sale form", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: /Nueva venta/ }),
    );
    expect(await screen.findByText("Registrar una venta")).toBeInTheDocument();
  });

  it("says so, and offers to clear the filters, when nothing matches", async () => {
    const user = userEvent.setup();
    sales = [];
    renderPage("sales_manager", "", "/sales?state=completed");
    expect(
      await screen.findByText("Ninguna venta coincide"),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Quitar filtros" }));
    await waitFor(() =>
      expect(screen.getByTestId("where")).toHaveTextContent(/^\/sales$/),
    );
  });

  it("invites the first sale when there are none at all", async () => {
    sales = [];
    renderPage();
    expect(await screen.findByText("Aún no hay ventas")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Registrar una venta/ }),
    ).toBeInTheDocument();
  });

  it("shows the server's message, and can try again, when the list fails", async () => {
    const user = userEvent.setup();
    failList = true;
    renderPage();
    expect(
      await screen.findByText("No se pudieron cargar las ventas"),
    ).toBeInTheDocument();
    expect(screen.getByText("El servidor no responde.")).toBeInTheDocument();
    failList = false;
    await user.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByText("Embajada de Japón")).toBeInTheDocument();
  });
});
