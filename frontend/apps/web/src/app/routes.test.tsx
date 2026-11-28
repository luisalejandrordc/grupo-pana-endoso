import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
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

function Where() {
  const { pathname, search } = useLocation();
  return <output data-testid="where">{pathname + search}</output>;
}

function start(path: string, role: Role) {
  const urls: URL[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input), "http://localhost");
      urls.push(url);
      const p = url.pathname.replace("/api/v1", "");
      if (p === "/auth/refresh")
        return json({
          accessToken: "t",
          expiresAt: "2099-01-01T00:00:00Z",
          employee: employee(role),
        });
      if (p === "/sales")
        return json({ items: [], page: 1, pageSize: 12, total: 0 });
      if (p === "/advisors") return json({ items: [] });
      if (p === "/employees")
        return json({ items: [], page: 1, pageSize: 20, total: 0 });
      return json({ error: { code: "NOT_FOUND", message: "No." } }, 404);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <App />
        <Where />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return urls;
}

describe("routes", () => {
  it("sends a role without access back to the sales list instead of an empty screen", async () => {
    start("/staff", "sales_advisor");
    expect(
      await screen.findByRole("heading", { name: "Ventas" }),
    ).toBeVisible();
    expect(screen.getByTestId("where")).toHaveTextContent("/sales");
    expect(
      screen.queryByRole("heading", { name: "Equipo de trabajo" }),
    ).not.toBeInTheDocument();
  });

  it("lets the administrator into the staff list", async () => {
    start("/staff", "system_admin");
    expect(
      await screen.findByRole("heading", { name: "Equipo de trabajo" }),
    ).toBeVisible();
  });

  it("explains an address that leads nowhere and offers the way back", async () => {
    start("/no-existe", "sales_manager");
    expect(await screen.findByText("Esta página no existe")).toBeVisible();
    expect(screen.getByRole("link", { name: "Ir a ventas" })).toHaveAttribute(
      "href",
      "/sales",
    );
  });

  it("puts the search in the list itself, beside its filters, and not in a bar of its own", async () => {
    start("/sales", "sales_manager");
    await screen.findByRole("heading", { name: "Ventas" });
    const search = screen.getByRole("searchbox", { name: "Buscar ventas" });
    const toolbar = search.closest(".toolbar") as HTMLElement;
    // The same row as the filters it works with.
    expect(
      within(toolbar).getByRole("combobox", { name: "Prioridad" }),
    ).toBeVisible();
    // On a wide screen no top bar is drawn at all: only the narrow layout has one, for the menu and the logo.
    const bar = document.querySelector("header.topbar") as HTMLElement;
    expect(within(bar).queryByRole("searchbox")).toBeNull();
  });

  it("keeps the search in the address, and asks the server for it", async () => {
    const user = userEvent.setup();
    const urls = start("/sales", "sales_manager");
    await screen.findByRole("heading", { name: "Ventas" });
    await user.type(
      screen.getByRole("searchbox", { name: "Buscar ventas" }),
      "Tanaka",
    );
    expect(screen.getByTestId("where")).toHaveTextContent("/sales?q=Tanaka");
    await waitFor(() =>
      expect(
        urls.some(
          (u) =>
            u.pathname.endsWith("/sales") &&
            u.searchParams.get("q") === "Tanaka",
        ),
      ).toBe(true),
    );
  });

  it("starts a search from the address, as when a link is shared", async () => {
    start("/sales?q=Embajada", "sales_manager");
    expect(
      await screen.findByRole("searchbox", { name: "Buscar ventas" }),
    ).toHaveValue("Embajada");
  });

  it("has the same search in the staff list, kept in the address too", async () => {
    const user = userEvent.setup();
    start("/staff", "system_admin");
    await screen.findByRole("heading", { name: "Equipo de trabajo" });
    await user.type(
      screen.getByRole("searchbox", { name: "Buscar colaboradores" }),
      "Lucía",
    );
    expect(screen.getByTestId("where")).toHaveTextContent("/staff?q=");
  });

  it("gives each list its own search, and a search does not follow the person to another list", async () => {
    const user = userEvent.setup();
    start("/sales?q=Tanaka", "sales_manager");
    await screen.findByRole("searchbox", { name: "Buscar ventas" });
    await user.click(
      within(
        screen.getByRole("navigation", { name: "Navegación principal" }),
      ).getByRole("link", { name: "Clientes" }),
    );
    const clients = await screen.findByRole("searchbox", {
      name: "Buscar clientes",
    });
    expect(clients).toHaveValue("");
    expect(screen.getByTestId("where")).toHaveTextContent(/^\/customers$/);
  });

  it("has no search where there is no list to narrow", async () => {
    start("/profile", "sales_manager");
    await screen.findByRole("heading", { name: "Mi perfil" });
    expect(screen.queryByRole("searchbox")).toBeNull();
  });

  it("names the browser tab after the screen", async () => {
    start("/sales", "sales_manager");
    await screen.findByRole("heading", { name: "Ventas" });
    expect(document.title).toBe("Ventas · EndosoFlow");
  });

  it("links the signed-in person's card to their profile", async () => {
    start("/sales", "sales_manager");
    await screen.findByRole("heading", { name: "Ventas" });
    const chip = screen.getByRole("link", { name: /Diego Ramírez/ });
    expect(chip).toHaveAttribute("href", "/profile");
    expect(
      within(screen.getByRole("navigation", { name: "Navegación principal" }))
        .queryAllByRole("link")
        .map((link) => link.textContent),
    ).toEqual(["Ventas", "Clientes", "Catálogo"]);
  });
});
