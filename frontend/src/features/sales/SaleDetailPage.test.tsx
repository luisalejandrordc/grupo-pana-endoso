import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Route, Routes, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SaleDetailPage } from "./SaleDetailPage";
import {
  json,
  sale,
  SALE_ID,
  stubApi,
  task,
  withProviders,
} from "./testSupport";

afterEach(() => vi.unstubAllGlobals());

function Where() {
  const l = useLocation();
  return <output data-testid="where">{l.pathname + l.search}</output>;
}

function open(
  over: Parameters<typeof sale>[0] = {},
  routes: Parameters<typeof stubApi>[0] = {},
  path = `/sales/${SALE_ID}`,
) {
  const requests = stubApi({
    [`GET /sales/${SALE_ID}`]: () => json(sale(over)),
    [`GET /sales/${SALE_ID}/tasks/4`]: () =>
      json(task({ number: 4, name: "Pago a Grupo Pana" })),
    [`GET /sales/${SALE_ID}/tasks/1`]: () => json(task()),
    [`GET /sales/${SALE_ID}/tasks/2`]: () =>
      json(task({ number: 2, name: "Debida Diligencia" })),
    "GET /advisors": () =>
      json({
        items: [
          { id: "adv-1", firstName: "Ana", lastName: "Rojas", hasPhoto: false },
          { id: "adv-2", firstName: "Luis", lastName: "Paz", hasPhoto: true },
        ],
      }),
    ...routes,
  });
  render(
    withProviders(
      <>
        <Routes>
          <Route path="/sales" element={<p>Lista de ventas</p>} />
          <Route
            path="/sales/:saleId"
            element={<SaleDetailPage toast={vi.fn()} />}
          />
          <Route
            path="/sales/:saleId/:tab"
            element={<SaleDetailPage toast={vi.fn()} />}
          />
        </Routes>
        <Where />
      </>,
      path,
    ),
  );
  return requests;
}

async function openActions() {
  const user = userEvent.setup();
  await user.click(
    await screen.findByRole("button", { name: "Acciones de la venta" }),
  );
  return user;
}

describe("the sale page", () => {
  it("shows who, what and where the sale stands", async () => {
    open();
    expect(
      await screen.findByRole("heading", { name: "Embajada de Japón" }),
    ).toBeInTheDocument();
    expect(screen.getByText("EF-2026-00001")).toBeInTheDocument();
    expect(screen.getByText("Hilux SR · Blanco")).toBeInTheDocument();
    expect(screen.getByText("3", { selector: "strong" })).toBeInTheDocument(); // 3 of 13 done
    expect(screen.getByText("Ana Rojas")).toBeInTheDocument();
  });

  it("opens on the task in hand, and on the one named in the address", async () => {
    open();
    expect(
      await screen.findByRole("heading", { name: "Pago a Grupo Pana" }),
    ).toBeInTheDocument();
  });

  it("opens the task named in the address", async () => {
    open({}, {}, `/sales/${SALE_ID}?task=2`);
    expect(
      await screen.findByRole("heading", { name: "Debida Diligencia" }),
    ).toBeInTheDocument();
  });

  it("moves between tasks from the route, keeping the choice in the address", async () => {
    const user = userEvent.setup();
    open();
    await screen.findByRole("heading", { name: "Pago a Grupo Pana" });
    await user.click(screen.getByRole("button", { name: /^Tarea 2:/ }));
    expect(
      await screen.findByRole("heading", { name: "Debida Diligencia" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent("task=2");
  });

  it("says when the sale cannot be found, with a way back", async () => {
    open(
      {},
      {
        [`GET /sales/${SALE_ID}`]: () =>
          json(
            {
              error: {
                code: "SALE_NOT_FOUND",
                message: "No se encontró la venta.",
              },
            },
            404,
          ),
      },
    );
    expect(
      await screen.findByText("No se encontró la venta"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Volver a ventas" }),
    ).toHaveAttribute("href", "/sales");
  });
});

describe("actions follow the server's permissions", () => {
  it("offers exactly the actions the person may take", async () => {
    open({
      permissions: {
        canReassign: true,
        canCancel: false,
        canReopen: false,
        canDelete: true,
        canEditNotes: true,
      },
    });
    await openActions();
    const items = screen.getAllByRole("menuitem").map((i) => i.textContent);
    expect(items).toEqual(["Reasignar asesor", "Eliminar definitivamente"]);
  });

  it("shows no menu at all when there is nothing to do", async () => {
    open({
      permissions: {
        canReassign: false,
        canCancel: false,
        canReopen: false,
        canDelete: false,
        canEditNotes: false,
      },
    });
    await screen.findByRole("heading", { name: "Embajada de Japón" });
    expect(
      screen.queryByRole("button", { name: "Acciones de la venta" }),
    ).not.toBeInTheDocument();
  });

  it("explains a cancelled sale and offers to reopen it", async () => {
    open({
      state: "cancelled",
      currentTask: null,
      cancellation: {
        category: "customer_withdrew",
        text: "Cambió de destino",
        cancelledAt: "2026-03-05T10:00:00Z",
        cancelledByName: "Jorge Salas",
        atTaskNumber: 4,
      },
      permissions: {
        canReassign: false,
        canCancel: false,
        canReopen: true,
        canDelete: false,
        canEditNotes: false,
      },
    });
    expect(
      await screen.findByText(/Cancelada el .* por Jorge Salas/),
    ).toBeInTheDocument();
    expect(screen.getByText(/Cambió de destino/)).toBeInTheDocument();
    await openActions();
    expect(
      screen.getByRole("menuitem", { name: "Reabrir venta" }),
    ).toBeInTheDocument();
  });
});

describe("the dialogs send what the backend expects", () => {
  it("cancels with a category and a written reason, both required", async () => {
    const requests = open(
      {},
      {
        [`POST /sales/${SALE_ID}/cancel`]: () =>
          json(sale({ state: "cancelled" })),
      },
    );
    const user = await openActions();
    await user.click(screen.getByRole("menuitem", { name: "Cancelar venta" }));
    const dialog = await screen.findByRole("dialog");
    const confirm = within(dialog).getByRole("button", {
      name: /Cancelar venta/,
    });
    expect(confirm).toBeDisabled();
    await user.type(within(dialog).getByRole("textbox"), "El cliente desistió");
    await user.click(confirm);
    await waitFor(() =>
      expect(requests.some((r) => r.path.endsWith("/cancel"))).toBe(true),
    );
    expect(
      requests.find((r) => r.path.endsWith("/cancel"))!.body,
    ).toMatchObject({ reason: "El cliente desistió" });
    expect(
      requests.find((r) => r.path.endsWith("/cancel"))!.body,
    ).toHaveProperty("category");
  });

  it("reassigns to another advisor, never the current one", async () => {
    const requests = open(
      {},
      { [`POST /sales/${SALE_ID}/reassign`]: () => json(sale()) },
    );
    const user = await openActions();
    await user.click(
      screen.getByRole("menuitem", { name: "Reasignar asesor" }),
    );
    const dialog = await screen.findByRole("dialog");
    const select = await within(dialog).findByRole("combobox");
    expect(
      within(select).queryByRole("option", { name: /Ana Rojas/ }),
    ).not.toBeInTheDocument();
    await user.selectOptions(select, "adv-2");
    await user.click(within(dialog).getByRole("button", { name: /Reasignar/ }));
    await waitFor(() =>
      expect(requests.some((r) => r.path.endsWith("/reassign"))).toBe(true),
    );
    expect(
      requests.find((r) => r.path.endsWith("/reassign"))!.body,
    ).toMatchObject({ advisorId: "adv-2" });
  });

  it("deletes only when the sale code is typed, sending the reason and the code in the body", async () => {
    const requests = open(
      {
        permissions: {
          canReassign: false,
          canCancel: false,
          canReopen: false,
          canDelete: true,
          canEditNotes: false,
        },
      },
      {
        [`DELETE /sales/${SALE_ID}`]: () => new Response(null, { status: 204 }),
      },
    );
    const user = await openActions();
    await user.click(
      screen.getByRole("menuitem", { name: "Eliminar definitivamente" }),
    );
    const dialog = await screen.findByRole("dialog");
    const confirm = within(dialog).getByRole("button", { name: /Eliminar/ });
    const [reason, code] = within(dialog).getAllByRole("textbox");
    await user.type(reason, "Venta de prueba");
    await user.type(code, "EF-2026-0000");
    expect(confirm).toBeDisabled();
    await user.type(code, "1");
    await user.click(confirm);
    await waitFor(() =>
      expect(requests.some((r) => r.method === "DELETE")).toBe(true),
    );
    const sent = requests.find((r) => r.method === "DELETE")!;
    expect(sent.path).toBe(`/sales/${SALE_ID}`);
    expect(sent.body).toEqual({
      reason: "Venta de prueba",
      confirm: "EF-2026-00001",
    });
    expect(await screen.findByText("Lista de ventas")).toBeInTheDocument();
  });

  it("closes a dialog with Escape", async () => {
    open();
    const user = await openActions();
    await user.click(screen.getByRole("menuitem", { name: "Cancelar venta" }));
    await screen.findByRole("dialog");
    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });
});

describe("unsaved changes", () => {
  it("asks before leaving a task with edits, and keeps them if the person stays", async () => {
    const user = userEvent.setup();
    open({}, {}, `/sales/${SALE_ID}?task=1`);
    const field = await screen.findByLabelText(/Monto de accesorios/);
    await user.type(field, "99");
    await user.click(screen.getByRole("button", { name: /^Tarea 3:/ }));
    expect(
      await screen.findByText("Hay cambios sin guardar"),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Volver" }));
    expect(screen.getByLabelText(/Monto de accesorios/)).toHaveValue("99");
    expect(screen.getByTestId("where")).not.toHaveTextContent("task=3");
  });

  it("leaves without asking when nothing was changed", async () => {
    const user = userEvent.setup();
    open();
    await screen.findByRole("heading", { name: "Pago a Grupo Pana" });
    await user.click(screen.getByRole("button", { name: /^Tarea 2:/ }));
    expect(
      screen.queryByText("Hay cambios sin guardar"),
    ).not.toBeInTheDocument();
    expect(
      await screen.findByRole("heading", { name: "Debida Diligencia" }),
    ).toBeInTheDocument();
  });
}
