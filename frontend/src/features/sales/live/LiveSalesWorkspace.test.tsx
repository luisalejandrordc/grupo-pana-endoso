import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Role } from "../../../shared/types";
import type { ApiSale } from "./api";
import { LiveSalesWorkspace } from "./LiveSalesWorkspace";

const sale = (over: Partial<ApiSale> = {}): ApiSale => ({
  id: "0198aaaa-0000-7000-8000-000000000001",
  customerId: "c1",
  customerName: "Embajada de Japón",
  customerDocumentType: "RUC",
  customerDocumentNumber: "20123456789",
  customerType: "embajada",
  modelId: "m1",
  versionId: "v1",
  colorId: "k1",
  modelName: "Hilux",
  versionName: "SR",
  colorName: "Blanco",
  referencePrice: "35000.00",
  temperature: "hot",
  state: "active",
  vehicleSeries: null,
  vin: null,
  licensePlate: null,
  advisorId: "adv-1",
  advisorName: "Ana Rojas",
  notes: null,
  currentTaskNumber: 2,
  currentTaskStatus: "in_progress",
  completedTasks: 1,
  completedAt: null,
  cancelledAt: null,
  cancelledAtTaskNumber: null,
  cancellationCategory: null,
  cancellationText: null,
  version: 1,
  createdAt: "2026-03-02T10:00:00Z",
  updatedAt: "2026-03-02T10:00:00Z",
  ...over,
});

interface Call {
  path: string;
  params: URLSearchParams;
}
let calls: Call[];
let sales: ApiSale[];
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
      customerName: "Lucía Quispe",
      state: "cancelled",
      temperature: "cold",
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
          items: [{ id: "adv-1", firstName: "Ana", lastName: "Rojas" }],
        });
      if (path === "/sales") {
        if (failList && url.searchParams.get("pageSize") === "10")
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
          pageSize: Number(url.searchParams.get("pageSize") ?? 10),
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

function renderPage(role: Role = "sales_manager", query = "") {
  const onOpen = vi.fn();
  const onCreate = vi.fn();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <LiveSalesWorkspace
        role={role}
        query={query}
        onCreate={onCreate}
        onOpen={onOpen}
      />
    </QueryClientProvider>,
  );
  return {
    onOpen,
    onCreate,
    rerenderWith: (q: string) =>
      view.rerender(
        <QueryClientProvider client={client}>
          <LiveSalesWorkspace
            role={role}
            query={q}
            onCreate={onCreate}
            onOpen={onOpen}
          />
        </QueryClientProvider>,
      ),
  };
}

const listCalls = () =>
  calls.filter((c) => c.path === "/sales" && c.params.get("pageSize") === "10");

describe("LiveSalesWorkspace", () => {
  it("lists the sales from the server", async () => {
    renderPage();
    expect(await screen.findByText("Embajada de Japón")).toBeInTheDocument();
    expect(screen.getByText("Lucía Quispe")).toBeInTheDocument();
    expect(screen.getByText("Venta 0198AAAA")).toBeInTheDocument();
    expect(screen.getByText("2 registros")).toBeInTheDocument();
    // The first request asks for the newest first, ten to a page.
    expect(listCalls()[0].params.get("sort")).toBe("-createdAt");
  });

  it("asks the server for the summary numbers", async () => {
    renderPage();
    await screen.findByText("Embajada de Japón");
    await waitFor(() => {
      const totals = calls.filter(
        (c) => c.path === "/sales" && c.params.get("pageSize") === "1",
      );
      expect(totals.some((c) => c.params.get("state") === "completed")).toBe(
        true,
      );
      expect(
        totals.some(
          (c) =>
            c.params.get("state") === "active" &&
            c.params.get("temperature") === "hot",
        ),
      ).toBe(true);
    });
  });

  it("filters on the server", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("Embajada de Japón");
    await user.selectOptions(screen.getByLabelText("Estado"), "cancelled");
    await waitFor(() =>
      expect(
        listCalls().some((c) => c.params.get("state") === "cancelled"),
      ).toBe(true),
    );
    await waitFor(() =>
      expect(screen.queryByText("Embajada de Japón")).not.toBeInTheDocument(),
    );
    expect(screen.getByText("Lucía Quispe")).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText("Tarea actual"), "4");
    await waitFor(() =>
      expect(listCalls().some((c) => c.params.get("currentTask") === "4")).toBe(
        true,
      ),
    );
  });

  it("sends the search text after the typing stops", async () => {
    const { rerenderWith } = renderPage();
    await screen.findByText("Embajada de Japón");
    rerenderWith("japón");
    await waitFor(
      () =>
        expect(listCalls().some((c) => c.params.get("q") === "japón")).toBe(
          true,
        ),
      { timeout: 2000 },
    );
  });

  it("offers the advisor filter only to management", async () => {
    renderPage("sales_manager");
    await screen.findByText("Embajada de Japón");
    expect(screen.getByLabelText("Asesor")).toBeInTheDocument();
  });

  it("does not offer the advisor filter, or ask for advisors, to an advisor", async () => {
    renderPage("sales_advisor");
    await screen.findByText("Embajada de Japón");
    expect(screen.queryByLabelText("Asesor")).not.toBeInTheDocument();
    expect(calls.some((c) => c.path === "/advisors")).toBe(false);
  });

  it("opens a sale from its row", async () => {
    const user = userEvent.setup();
    const { onOpen } = renderPage();
    const row = (await screen.findByText("Embajada de Japón")).closest(
      "tr",
    ) as HTMLElement;
    await user.click(row);
    expect(onOpen).toHaveBeenCalledWith("0198aaaa-0000-7000-8000-000000000001");
    await user.click(
      within(row).getByRole("button", { name: /abrir la venta de embajada/i }),
    );
    expect(onOpen).toHaveBeenCalledTimes(2);
  });

  it("shows the server's message when the list fails", async () => {
    failList = true;
    renderPage();
    expect(
      await screen.findByText("No se pudieron cargar las ventas"),
    ).toBeInTheDocument();
    expect(screen.getByText("El servidor no responde.")).toBeInTheDocument();
  });

  it("shows the empty state, and the way to register a sale", async () => {
    sales = [];
    const user = userEvent.setup();
    const { onCreate } = renderPage();
    expect(await screen.findByText("Aún no hay ventas")).toBeInTheDocument();
    await user.click(
      screen.getAllByRole("button", { name: /nueva venta/i })[1],
    );
    expect(onCreate).toHaveBeenCalled();
  });
});
