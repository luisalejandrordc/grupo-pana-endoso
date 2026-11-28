import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  CancelSaleDialog,
  DeleteSaleDialog,
  ReasonDialog,
  ReassignDialog,
} from "./dialogs";
import { NotesTab } from "./SaleTabs";
import {
  json,
  sale,
  SALE_ID,
  stubApi,
  task,
  withProviders,
} from "./testSupport";
import { TaskPanel } from "./TaskPanel";

afterEach(() => vi.unstubAllGlobals());

const noop = () => undefined;

/** Free text shows how much of its limit is used. The limits are the server's: a text the box takes is accepted. */
describe("the character counters", () => {
  it("count the notes of a task against the server's 2000", async () => {
    stubApi({ [`GET /sales/${SALE_ID}/tasks/1`]: () => json(task()) });
    render(
      withProviders(<TaskPanel sale={sale()} number={1} toast={vi.fn()} />),
    );
    const user = userEvent.setup();
    expect(await screen.findByText("0 / 2000")).toBeVisible();
    await user.type(screen.getByLabelText("Notas de la tarea"), "Llamar");
    expect(screen.getByText("6 / 2000")).toBeVisible();
  });

  it("count the notes of a sale against the same 2000", () => {
    render(
      withProviders(
        <NotesTab
          sale={sale({ notes: "Cliente prefiere blanco" })}
          toast={vi.fn()}
        />,
      ),
    );
    expect(screen.getByText("23 / 2000")).toBeVisible();
  });

  it("count the reason for a difference in a payment (500) with the box that stops there", async () => {
    stubApi({
      [`GET /sales/${SALE_ID}/tasks/4`]: () =>
        json(
          task({
            number: 4,
            name: "Pago del Cliente a Pana",
            data: {
              bank: null,
              paymentDate: null,
              transactionNumber: null,
              depositedAmount: null,
              mismatchReason: null,
            },
            calculated: { expectedAmount: "32500.00" },
            documents: [],
          }),
        ),
    });
    render(
      withProviders(<TaskPanel sale={sale()} number={4} toast={vi.fn()} />),
    );
    expect(await screen.findByText("0 / 500")).toBeVisible();
    expect(screen.getByLabelText("Motivo de la diferencia")).toHaveAttribute(
      "maxlength",
      "500",
    );
  });

  it("count the reason of a restart, reopening or correction against 500", async () => {
    render(
      withProviders(
        <ReasonDialog
          title="Reiniciar tarea"
          label="Motivo del reinicio"
          confirmLabel="Reiniciar"
          onClose={noop}
          onConfirm={async () => undefined}
        />,
      ),
    );
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Motivo del reinicio"), "Error");
    expect(screen.getByText("5 / 500")).toBeVisible();
  });

  it("count the detail of a cancellation against the server's 1000, not 500", () => {
    render(
      withProviders(
        <CancelSaleDialog onClose={noop} onConfirm={async () => undefined} />,
      ),
    );
    expect(screen.getByText("0 / 1000")).toBeVisible();
    expect(screen.getByLabelText("Detalle del motivo")).toHaveAttribute(
      "maxlength",
      "1000",
    );
  });

  it("count the reason of a reassignment and of a deletion against 500", () => {
    const { unmount } = render(
      withProviders(
        <ReassignDialog
          sale={sale()}
          advisors={[]}
          loading={false}
          onClose={noop}
          onConfirm={async () => undefined}
        />,
      ),
    );
    expect(screen.getByText("0 / 500")).toBeVisible();
    unmount();
    render(
      withProviders(
        <DeleteSaleDialog
          sale={sale()}
          onClose={noop}
          onConfirm={async () => undefined}
        />,
      ),
    );
    expect(screen.getByText("0 / 500")).toBeVisible();
  });

  it("warn near the limit and stop at it", async () => {
    render(
      withProviders(
        <ReasonDialog
          title="Reabrir tarea"
          label="Motivo de la reapertura"
          confirmLabel="Reabrir"
          onClose={noop}
          onConfirm={async () => undefined}
        />,
      ),
    );
    const box = screen.getByLabelText("Motivo de la reapertura");
    await userEvent.setup({ delay: null }).click(box);
    await userEvent.setup({ delay: null }).paste("x".repeat(460));
    expect(screen.getByText("460 / 500")).toHaveClass("is-near");
    await userEvent.setup({ delay: null }).paste("y".repeat(100));
    expect((box as HTMLTextAreaElement).value).toHaveLength(500);
    expect(screen.getByText("500 / 500")).toHaveClass("is-full");
  });
});
