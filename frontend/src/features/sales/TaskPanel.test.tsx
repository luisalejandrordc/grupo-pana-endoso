import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  json,
  noRights,
  sale,
  SALE_ID,
  stubApi,
  task,
  withProviders,
} from "./testSupport";
import { TaskPanel } from "./TaskPanel";

afterEach(() => vi.unstubAllGlobals());

const taskPath = `/sales/${SALE_ID}/tasks/1`;
const toast = vi.fn();

function show(
  detail = task(),
  extra: Record<string, () => Response> = {},
  saleOver: Parameters<typeof sale>[0] = {},
) {
  const requests = stubApi({
    [`GET ${taskPath}`]: () => json(detail),
    ...Object.fromEntries(Object.entries(extra).map(([k, v]) => [k, v])),
  });
  render(
    withProviders(<TaskPanel sale={sale(saleOver)} number={1} toast={toast} />),
  );
  return requests;
}

describe("what the server lets the person do", () => {
  it("offers to start a task that is ready and the person may start", async () => {
    const requests = show(
      task({
        status: "pending",
        isEligible: true,
        permissions: { ...noRights, canStart: true },
      }),
      { [`POST ${taskPath}/start`]: () => json(task()) },
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Iniciar tarea" }),
    );
    await waitFor(() =>
      expect(
        requests.some((r) => r.method === "POST" && r.path.endsWith("/start")),
      ).toBe(true),
    );
  });

  it("explains a task that is not ready instead of offering to start it", async () => {
    show(task({ status: "pending", isEligible: false, permissions: noRights }));
    expect(
      await screen.findByText(/se habilitará cuando se complete la anterior/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Iniciar tarea" }),
    ).not.toBeInTheDocument();
  });

  it("shows no actions to someone who may only look", async () => {
    show(task({ permissions: noRights }));
    await screen.findByText("Cotización del Vehículo");
    for (const name of [
      "Guardar",
      "Completar tarea",
      "Reiniciar",
      "Reabrir tarea",
    ])
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    expect(
      screen.getByLabelText(/Monto inicial a pagar a Toyota/),
    ).toBeDisabled();
  });

  it("offers Reiniciar and Reabrir only when the server allows them", async () => {
    show(
      task({
        permissions: {
          ...noRights,
          canComplete: true,
          canRestart: true,
          canEditData: true,
        },
      }),
    );
    expect(
      await screen.findByRole("button", { name: "Reiniciar" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Reabrir tarea" }),
    ).not.toBeInTheDocument();
  });

  it("shows the quantities the server works out as read-only values", async () => {
    show(task());
    expect(
      await screen.findByText("Precio final cotizado"),
    ).toBeInTheDocument();
    expect(screen.getByText(/32,500\.00/)).toBeInTheDocument();
  });
});

describe("saving and completing", () => {
  it("sends only the fields that changed, with the version the form was opened on", async () => {
    const requests = show(task(), {
      [`PATCH ${taskPath}/data`]: () => json(task({ version: 4 })),
    });
    const user = userEvent.setup();
    const field = await screen.findByLabelText(/Monto de accesorios/);
    await user.type(field, "1200.5");
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() =>
      expect(requests.some((r) => r.method === "PATCH")).toBe(true),
    );
    const patch = requests.find((r) => r.method === "PATCH")!;
    expect(patch.body).toEqual({
      version: 3,
      data: { accessoryAmount: "1200.5" },
    });
  });

  it("saves pending edits before completing, so nothing typed is lost", async () => {
    const requests = show(task(), {
      [`PATCH ${taskPath}/data`]: () => json(task({ version: 4 })),
      [`POST ${taskPath}/complete`]: () => json(task({ status: "completed" })),
    });
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText(/Monto de accesorios/), "10");
    await user.click(screen.getByRole("button", { name: "Completar tarea" }));
    await waitFor(() =>
      expect(requests.some((r) => r.path.endsWith("/complete"))).toBe(true),
    );
    const order = requests
      .filter((r) => r.method !== "GET")
      .map((r) => `${r.method} ${r.path.split("/").pop()}`);
    expect(order).toEqual(["PATCH data", "POST complete"]);
  });

  it("shows the server's refusal and marks the fields and documents it names", async () => {
    show(task(), {
      [`POST ${taskPath}/complete`]: () =>
        json(
          {
            error: {
              code: "TASK_INCOMPLETE",
              message:
                "La tarea no se puede completar: faltan datos o documentos.",
              fields: [
                {
                  field: "documents.quotation_pdf",
                  code: "DOCUMENT_REQUIRED",
                  message: "Falta el documento: Cotización oficial (PDF).",
                },
                {
                  field: "accessoryAmount",
                  code: "REQUIRED",
                  message: "Ingresa el monto de accesorios.",
                },
              ],
            },
          },
          422,
        ),
    });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Completar tarea" }),
    );
    const alert = (await screen.findAllByRole("alert"))[0];
    expect(alert).toHaveTextContent("faltan datos o documentos");
    expect(
      within(alert).getByText("Ingresa el monto de accesorios."),
    ).toBeInTheDocument();
    expect(
      screen.getAllByText("Falta el documento: Cotización oficial (PDF).")
        .length,
    ).toBeGreaterThan(0);
  });

  it("lists the warnings the server returns without blocking", async () => {
    show(
      task({
        warnings: [
          {
            code: "DATE_ORDER",
            message: "La fecha de pago es anterior a la de la cotización.",
          },
        ] as never,
      }),
    );
    expect(
      await screen.findByText(/anterior a la de la cotización/),
    ).toBeInTheDocument();
  });
});

describe("a completed task", () => {
  const completed = (rights = {}) =>
    task({
      status: "completed",
      completedAt: "2026-03-04T10:00:00Z",
      completionPhase: "early",
      permissions: {
        ...noRights,
        canEditData: true,
        canUploadDocuments: true,
        canReopen: true,
        needsReason: true,
        ...rights,
      },
    });

  it("asks for a reason before a correction is saved, and sends it with the change", async () => {
    // Only this task is completed, so nothing later depends on its amounts yet.
    const onlyFirst = sale().tasks.map((t) => ({
      ...t,
      status: t.number === 1 ? "completed" : "pending",
    })) as never;
    const requests = show(
      completed(),
      { [`PATCH ${taskPath}/data`]: () => json(completed()) },
      { tasks: onlyFirst },
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: /Corregir datos/ }),
    );
    const field = screen.getByLabelText(/Comisión inicial/);
    await user.clear(field);
    await user.type(field, "2600");
    await user.click(
      screen.getByRole("button", { name: "Guardar corrección" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByRole("textbox"), "Error de digitación");
    await user.click(
      within(dialog).getByRole("button", { name: "Guardar corrección" }),
    );
    await waitFor(() =>
      expect(requests.some((r) => r.method === "PATCH")).toBe(true),
    );
    expect(requests.find((r) => r.method === "PATCH")!.body).toEqual({
      version: 3,
      data: { initialCommission: "2600" },
      reason: "Error de digitación",
    });
  });

  it("locks the amounts that later tasks depend on, and says why", async () => {
    show(
      completed(),
      {},
      {
        tasks: sale().tasks.map((t) =>
          t.number <= 2 ? { ...t, status: "completed" } : t,
        ) as never,
      },
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: /Corregir datos/ }),
    );
    expect(
      screen.getByLabelText(/Monto inicial a pagar a Toyota/),
    ).toBeDisabled();
    expect(
      screen.getAllByText(/Bloqueado: hay tareas posteriores completadas/)
        .length,
    ).toBeGreaterThan(0);
  });

  it("reopens with a reason", async () => {
    const requests = show(completed(), {
      [`POST ${taskPath}/reopen`]: () => json(task()),
    });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Reabrir tarea" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.type(
      within(dialog).getByRole("textbox"),
      "El cliente cambió el monto",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Reabrir tarea" }),
    );
    await waitFor(() =>
      expect(requests.some((r) => r.path.endsWith("/reopen"))).toBe(true),
    );
    expect(requests.find((r) => r.path.endsWith("/reopen"))!.body).toEqual({
      reason: "El cliente cambió el monto",
    });
  });
});

describe("notes", () => {
  it("saves them on their own, without a version", async () => {
    const requests = show(task(), {
      [`PATCH ${taskPath}/notes`]: () =>
        json(task({ notes: "Llamar el lunes" })),
    });
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("Notas de la tarea"),
      "Llamar el lunes",
    );
    await user.click(screen.getByRole("button", { name: "Guardar notas" }));
    await waitFor(() =>
      expect(requests.some((r) => r.path.endsWith("/notes"))).toBe(true),
    );
    expect(requests.find((r) => r.path.endsWith("/notes"))!.body).toEqual({
      notes: "Llamar el lunes",
    });
  });
});
