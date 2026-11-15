import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EmployeeProfile, Role } from "../shared/types";
import App from "./App";

afterEach(() => vi.unstubAllGlobals());

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

const employee = (role: Role): EmployeeProfile => ({
  id: "e1",
  documentType: "DNI",
  documentNumber: "40123003",
  firstName: "Diego",
  lastName: "Ramírez",
  businessEmail: "diego@grupopana.pe",
  role,
  hasPhoto: false,
});

function start(path: string, role: Role | null) {
  const paths: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const p = new URL(String(input), "http://localhost").pathname.replace(
        "/api/v1",
        "",
      );
      paths.push(p);
      if (p === "/auth/refresh")
        return role
          ? json({
              accessToken: "t",
              expiresAt: "2099-01-01T00:00:00Z",
              employee: employee(role),
            })
          : json(
              { error: { code: "UNAUTHENTICATED", message: "Sin sesión." } },
              401,
            );
      if (p === "/sales")
        return json({ items: [], page: 1, pageSize: 12, total: 0 });
      if (p === "/advisors") return json({ items: [] });
      if (p === "/employees")
        return json({ items: [], page: 1, pageSize: 20, total: 0 });
      return json(
        { error: { code: "NOT_FOUND", message: "No encontrado." } },
        404,
      );
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return paths;
}

describe("App", () => {
  it("asks anyone without a session to sign in, whatever address they came to", async () => {
    start("/sales/abc", null);
    expect(
      await screen.findByRole("heading", {
        name: /Qué bueno tenerte de vuelta/,
      }),
    ).toBeInTheDocument();
  });

  it("opens the sales list at the root", async () => {
    start("/", "sales_advisor");
    expect(
      await screen.findByRole("heading", { name: "Ventas" }),
    ).toBeInTheDocument();
  });

  it("shows Colaboradores in the menu to an Admin only", async () => {
    start("/sales", "system_admin");
    const nav = await screen.findByRole("navigation", {
      name: "Navegación principal",
    });
    expect(
      within(nav).getByRole("link", { name: "Colaboradores" }),
    ).toBeInTheDocument();
  });

  it("does not show it to an Advisor, and sends them away if they type the address", async () => {
    const paths = start("/staff", "sales_advisor");
    expect(
      await screen.findByRole("heading", { name: "Ventas" }),
    ).toBeInTheDocument();
    const nav = screen.getByRole("navigation", {
      name: "Navegación principal",
    });
    expect(
      within(nav).queryByRole("link", { name: "Colaboradores" }),
    ).not.toBeInTheDocument();
    expect(paths).not.toContain("/employees");
  });

  it("says when a page does not exist", async () => {
    start("/no-existe", "sales_manager");
    expect(
      await screen.findByText("Esta página no existe"),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Ir a ventas" })).toHaveAttribute(
      "href",
      "/sales",
    );
  });

  it("offers a way to skip the menu", async () => {
    start("/sales", "sales_manager");
    expect(
      await screen.findByRole("link", { name: "Saltar al contenido" }),
    ).toHaveAttribute("href", "#content");
  });

  it("navigates from the menu without reloading", async () => {
    const user = userEvent.setup();
    start("/sales", "sales_manager");
    const nav = await screen.findByRole("navigation", {
      name: "Navegación principal",
    });
    await user.click(within(nav).getByRole("link", { name: "Catálogo" }));
    await waitFor(() =>
      expect(within(nav).getByRole("link", { name: "Catálogo" })).toHaveClass(
        "is-active",
      ),
    );
  });
});
