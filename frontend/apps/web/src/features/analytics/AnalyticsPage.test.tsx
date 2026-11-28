import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Role } from "../../shared/types";
import type { Overview, ProcessData } from "./api";
import { AnalyticsPage } from "./AnalyticsPage";

const meta = {
  from: "2026-08-25",
  to: "2026-11-22",
  previousFrom: "2026-05-27",
  previousTo: "2026-08-24",
  granularity: "week" as const,
  scope: "global" as const,
  generatedAt: "2026-11-22T15:00:00Z",
};

const money = (value: string, previous = "0.00") => ({ value, previous });

function overview(internal: boolean): Overview {
  return {
    meta,
    snapshot: { activeSales: 14, overdue: 3, approaching: 2, idle: 1 },
    period: {
      created: { value: 20, previous: 16 },
      completed: { value: 9, previous: 12 },
      cancelled: { value: 2, previous: 2 },
      closeRate: {
        numerator: 9,
        denominator: 11,
        value: 9 / 11,
        previous: 0.857,
      },
      cycleTime: {
        medianHours: 300,
        p90Hours: 520,
        previousMedianHours: 340,
        sampleSize: 9,
      },
    },
    money: {
      pipelineValue: { amount: "480000.00", saleCount: 14, estimatedCount: 3 },
      pendingCollection: { amount: "210000.00", saleCount: 6 },
      completedValue: money("310000.00", "420000.00"),
      averageTicket: { value: "34444.44", previous: "35000.00" },
      collected: money("290000.00", "400000.00"),
      invoiced: money("280000.00", "390000.00"),
      lostValue: { amount: "70000.00", saleCount: 2, estimatedCount: 0 },
      internal: internal
        ? {
            commissionFinal: money("24500.00", "31000.00"),
            commissionRate: { value: 0.079, previous: 0.074 },
            commissionPipeline: { amount: "36000.00", saleCount: 12 },
            commissionDrift: "-1800.00",
            paidToToyota: money("255000.00", "350000.00"),
            amountMismatches: { count: 2, absoluteDifference: "150.00" },
          }
        : null,
    },
    pipeline: [1, 2, 3, 4, 5, 6].map((n) => ({
      milestoneNumber: n,
      name: `Hito ${n}`,
      active: n === 2 ? 5 : 1,
      onTime: n === 2 ? 3 : 1,
      approaching: 0,
      overdue: n === 2 ? 2 : 0,
      idle: 0,
      value: "35000.00",
    })),
    trend: [
      { bucket: "2026-11-09", created: 4, completed: 2, cancelled: 0 },
      { bucket: "2026-11-16", created: 3, completed: 1, cancelled: 1 },
    ],
    attention: {
      total: 6,
      items: [
        {
          saleId: "0198aaaa-0000-7000-8000-000000000001",
          code: "EF-2026-00012",
          customerName: "Embajada de Japón",
          vehicle: "Hilux SR",
          advisorName: "Ana Rojas",
          taskNumber: 4,
          taskName: "Pago a Grupo Pana",
          reason: "overdue",
          dueDate: "2026-11-19T10:00:00Z",
          hours: 77,
          value: "35000.00",
        },
      ],
    },
  };
}

function processData(): ProcessData {
  return {
    meta,
    onTimeRate: { numerator: 8, denominator: 10, value: 0.8, previous: 0.7 },
    tasks: Array.from({ length: 13 }, (_, i) => ({
      taskNumber: i + 1,
      name: `Tarea ${i + 1}`,
      milestoneNumber: 1,
      recommendedHours: null,
      completedCount: 0,
      workMedianHours: null,
      workP90Hours: null,
      waitMedianHours: null,
      deadlines: null,
      currentCount: 0,
      overdueCount: 0,
      reworkCount: 0,
    })),
    customs: [],
  };
}

let paths: string[];
let internal: boolean;

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

beforeEach(() => {
  paths = [];
  internal = true;
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
      paths.push(path + url.search);
      if (path === "/analytics/overview") return json(overview(internal));
      if (path === "/analytics/process") return json(processData());
      if (path === "/advisors") return json({ items: [] });
      if (path === "/catalog/selectable") return json({ items: [] });
      return json(
        { error: { code: "ROUTE_NOT_FOUND", message: "Ruta no encontrada." } },
        404,
      );
    }),
  );
});
afterEach(() => vi.unstubAllGlobals());

function renderPage(role: Role, url = "/analytics") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[url]}>
        <AnalyticsPage role={role} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("AnalyticsPage", () => {
  it("shows a manager the summary with the money and the commission", async () => {
    renderPage("sales_manager");
    expect(await screen.findByText("Ventas activas")).toBeInTheDocument();
    expect(screen.getByText("Comisión final")).toBeInTheDocument();
    expect(screen.getByText("Pagado a Toyota")).toBeInTheDocument();
    expect(screen.getByText("Valor en curso")).toBeInTheDocument();
    expect(screen.getByText(/No son cifras contables/i)).toBeInTheDocument();
    // The default period is the server's: no dates are sent.
    expect(paths.find((p) => p.startsWith("/analytics/overview"))).toBe(
      "/analytics/overview",
    );
  });

  it("lists what needs attention and links to the sale", async () => {
    renderPage("sales_manager");
    const link = await screen.findByRole("link", { name: "EF-2026-00012" });
    expect(link).toHaveAttribute(
      "href",
      "/sales/0198aaaa-0000-7000-8000-000000000001",
    );
    expect(screen.getByText(/Con retraso · 3 d/)).toBeInTheDocument();
    expect(screen.getByText("Ana Rojas")).toBeInTheDocument();
  });

  it("never shows an advisor the commission, the team tab or the advisor filter", async () => {
    internal = false;
    renderPage("sales_advisor");
    expect(await screen.findByText("Ventas activas")).toBeInTheDocument();
    expect(screen.queryByText("Comisión final")).not.toBeInTheDocument();
    expect(screen.queryByText("Pagado a Toyota")).not.toBeInTheDocument();
    const tabs = screen.getByRole("tablist", {
      name: "Secciones de analítica",
    });
    expect(
      within(tabs).getByRole("tab", { name: "Mi panel" }),
    ).toBeInTheDocument();
    expect(
      within(tabs).queryByRole("tab", { name: "Equipo" }),
    ).not.toBeInTheDocument();
    expect(
      within(tabs).queryByRole("tab", { name: "Sistema" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("combobox", { name: "Asesor" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("Asesor")).not.toBeInTheDocument();
  });

  it("gives only an admin the system tab", async () => {
    renderPage("system_admin");
    await screen.findByText("Ventas activas");
    const tabs = screen.getByRole("tablist", {
      name: "Secciones de analítica",
    });
    expect(
      within(tabs).getByRole("tab", { name: "Sistema" }),
    ).toBeInTheDocument();
    expect(
      within(tabs).getByRole("tab", { name: "Equipo" }),
    ).toBeInTheDocument();
  });

  it("asks for a tab's figures only when it is opened", async () => {
    const user = userEvent.setup();
    renderPage("sales_manager");
    await screen.findByText("Ventas activas");
    expect(paths.some((p) => p.startsWith("/analytics/process"))).toBe(false);
    await user.click(screen.getByRole("tab", { name: "Proceso" }));
    await waitFor(() =>
      expect(paths.some((p) => p.startsWith("/analytics/process"))).toBe(true),
    );
    expect(await screen.findByText("Plazos cumplidos")).toBeInTheDocument();
  });

  it("sends the chosen period and filters to the server", async () => {
    renderPage("sales_manager", "/analytics?period=30d&temperature=hot");
    await screen.findByText("Ventas activas");
    const call = paths.find((p) => p.startsWith("/analytics/overview")) ?? "";
    const params = new URL(call, "http://localhost").searchParams;
    expect(params.get("temperature")).toBe("hot");
    expect(params.get("from")).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(params.get("to")).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });

  it("does not call the server for a custom range that is incomplete", async () => {
    renderPage("sales_manager", "/analytics?period=custom&from=2026-03-01");
    expect(await screen.findByText(/Elige las dos fechas/)).toBeInTheDocument();
    expect(paths.some((p) => p.startsWith("/analytics/overview"))).toBe(false);
  });
});
