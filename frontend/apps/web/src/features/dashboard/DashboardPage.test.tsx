import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Role } from "../../shared/types";
import { DashboardPage } from "./DashboardPage";

const ADVISOR = "0198aaaa-0000-7000-8000-000000000001";

const summary = {
  period: { from: "2026-08-25", to: "2026-11-22" },
  generatedAt: "2026-11-22T15:00:00Z",
  kpis: {
    activeSales: 7,
    createdInPeriod: 12,
    completedInPeriod: 5,
    cancelledInPeriod: 1,
    closingRate: 83.3,
    overdueTasks: 2,
    approachingTasks: 1,
    stalledTasks: 3,
  },
  trend: [
    { month: "2026-10", created: 5, completed: 2, cancelled: 0 },
    { month: "2026-11", created: 7, completed: 3, cancelled: 1 },
  ],
  milestones: [1, 2, 3, 4, 5, 6].map((number) => ({
    number,
    name: `Hito ${number}`,
    activeSales: number === 2 ? 4 : 0,
  })),
  attention: {
    total: 1,
    items: [
      {
        saleId: "0198bbbb-0000-7000-8000-000000000002",
        saleCode: "EF-2026-00042",
        customerName: "Embajada de Japón",
        vehicle: "Hilux SR",
        advisorId: ADVISOR,
        advisorName: "Ana Rojas",
        taskNumber: 8,
        taskName: "Trámite Aduanero",
        kind: "overdue",
        dueDate: "2026-11-20T10:00:00Z",
        waitingSince: null,
      },
    ],
  },
};

const finance = {
  period: summary.period,
  generatedAt: summary.generatedAt,
  pipeline: {
    sales: 2,
    quotedValue: "69001.00",
    finalCommission: "1500.00",
    provisionalCommission: "2500.00",
  },
  completed: {
    sales: 1,
    quotedValue: "34500.50",
    commission: "1500.00",
    averageCommission: "1500.00",
    accessories: "1200.50",
    releaseProcessing: "800.00",
    toyotaAmount: "31000.00",
  },
  cash: {
    moneyIn: "34500.50",
    moneyOut: "31000.00",
    net: "3500.50",
    paymentMismatches: 0,
    heldInActiveSales: "0.00",
    inCancelledSales: "0.00",
  },
  invoices: ["toyota", "commission", "accessories", "release_processing"].map(
    (kind) => ({
      kind,
      count: 1,
      amount: "100.00",
      mismatches: 0,
    }),
  ),
  monthly: [
    {
      month: "2026-11",
      commission: "1500.00",
      moneyIn: "34500.50",
      moneyOut: "31000.00",
    },
  ],
};

interface Call {
  path: string;
  params: URLSearchParams;
}
let calls: Call[];
let failSummary: boolean;

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

beforeEach(() => {
  calls = [];
  failSummary = false;
  // jsdom has no ResizeObserver, which the charts use to measure themselves.
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
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
              id: ADVISOR,
              firstName: "Ana",
              lastName: "Rojas",
              hasPhoto: false,
            },
          ],
        });
      if (path === "/catalog/selectable") return json({ items: [] });
      if (path === "/dashboard/summary")
        return failSummary
          ? json(
              {
                error: {
                  code: "INTERNAL_ERROR",
                  message: "El servidor no responde.",
                },
              },
              500,
            )
          : json(summary);
      if (path === "/dashboard/finance") return json(finance);
      return json(
        { error: { code: "ROUTE_NOT_FOUND", message: "Ruta no encontrada." } },
        404,
      );
    }),
  );
});
afterEach(() => vi.unstubAllGlobals());

function renderPage(role: Role, path = "/dashboard") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/dashboard" element={<DashboardPage role={role} />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const summaryCalls = () => calls.filter((c) => c.path === "/dashboard/summary");

describe("DashboardPage", () => {
  it("opens on the summary with the headline numbers and what needs attention", async () => {
    renderPage("sales_manager");
    expect(await screen.findByText("Ventas en curso")).toBeInTheDocument();
    const card = screen.getByText("Ventas en curso").closest(".kpi")!;
    expect(within(card as HTMLElement).getByText("7")).toBeInTheDocument();
    expect(within(card as HTMLElement).getByText("Hoy")).toBeInTheDocument();
    expect(screen.getByText("83,3 %")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "EF-2026-00042" });
    expect(link).toHaveAttribute(
      "href",
      "/sales/0198bbbb-0000-7000-8000-000000000002",
    );
    expect(
      screen.getByText("Con retraso", { selector: ".badge" }),
    ).toBeInTheDocument();
    // The default period is the last 90 days.
    const first = summaryCalls()[0].params;
    expect(first.get("to")! > first.get("from")!).toBe(true);
  });

  it("gives an Advisor neither the team nor the system tab, and never sends an advisor filter", async () => {
    renderPage("sales_advisor", `/dashboard?advisor=${ADVISOR}`);
    await screen.findByText("Ventas en curso");
    const tabs = within(
      screen.getByRole("tablist", { name: "Secciones del panel" }),
    );
    expect(tabs.getByRole("tab", { name: "Finanzas" })).toBeInTheDocument();
    expect(
      tabs.queryByRole("tab", { name: "Asesores" }),
    ).not.toBeInTheDocument();
    expect(
      tabs.queryByRole("tab", { name: "Sistema" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Asesor")).not.toBeInTheDocument();
    expect(summaryCalls().every((c) => !c.params.has("advisorId"))).toBe(true);
  });

  it("lets a Manager filter by advisor and open the team tab; only an Admin gets the system tab", async () => {
    const user = userEvent.setup();
    renderPage("sales_manager");
    await screen.findByText("Ventas en curso");
    expect(screen.getByRole("tab", { name: "Asesores" })).toBeInTheDocument();
    expect(
      screen.queryByRole("tab", { name: "Sistema" }),
    ).not.toBeInTheDocument();
    await screen.findByRole("option", { name: "Ana Rojas" });
    await user.selectOptions(screen.getByLabelText("Asesor"), ADVISOR);
    await waitFor(() =>
      expect(
        summaryCalls().some((c) => c.params.get("advisorId") === ADVISOR),
      ).toBe(true),
    );
  });

  it("shows the system tab to an Admin", async () => {
    renderPage("system_admin");
    await screen.findByText("Ventas en curso");
    expect(screen.getByRole("tab", { name: "Sistema" })).toBeInTheDocument();
  });

  it("loads a tab only when it is opened, and labels recorded money as not accounting", async () => {
    const user = userEvent.setup();
    renderPage("sales_manager");
    await screen.findByText("Ventas en curso");
    expect(calls.some((c) => c.path === "/dashboard/finance")).toBe(false);
    await user.click(screen.getByRole("tab", { name: "Finanzas" }));
    expect(await screen.findByText("Comisión provisional")).toBeInTheDocument();
    expect(screen.getByText(/no ingresos contables/)).toBeInTheDocument();
    expect(screen.getAllByText(/34[,\s]?500[.,]50/).length).toBeGreaterThan(0);
  });

  it("explains a failure and lets the person retry", async () => {
    const user = userEvent.setup();
    failSummary = true;
    renderPage("sales_manager");
    expect(
      await screen.findByText("No pudimos cargar este panel"),
    ).toBeInTheDocument();
    failSummary = false;
    await user.click(screen.getByRole("button", { name: /Reintentar/ }));
    expect(await screen.findByText("Ventas en curso")).toBeInTheDocument();
  });
});
