import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDialog } from "./ConfirmDialog";

function setup(props: Partial<Parameters<typeof ConfirmDialog>[0]> = {}) {
  const onConfirm = vi.fn();
  const onClose = vi.fn();
  render(
    <ConfirmDialog
      title="Desactivar a Ana Rojas"
      confirmLabel="Desactivar"
      onConfirm={onConfirm}
      onClose={onClose}
      {...props}
    >
      Se cerrarán sus sesiones.
    </ConfirmDialog>,
  );
  return { onConfirm, onClose };
}

describe("ConfirmDialog", () => {
  it("says what is about to happen and waits for a decision", () => {
    const { onConfirm, onClose } = setup();
    expect(
      screen.getByRole("dialog", { name: "Desactivar a Ana Rojas" }),
    ).toBeVisible();
    expect(screen.getByText("Se cerrarán sus sesiones.")).toBeVisible();
    expect(onConfirm).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("confirms with its button and backs out with Cancelar or Escape", async () => {
    const user = userEvent.setup();
    const { onConfirm, onClose } = setup();
    await user.click(screen.getByRole("button", { name: "Desactivar" }));
    expect(onConfirm).toHaveBeenCalledOnce();
    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("marks a destructive action as one", () => {
    setup({ tone: "danger" });
    expect(screen.getByRole("button", { name: "Desactivar" })).toHaveClass(
      "button-danger",
    );
  });

  it("while it works, locks both buttons and ignores Escape so the request is not abandoned", async () => {
    const user = userEvent.setup();
    const { onClose } = setup({ busy: true });
    expect(screen.getByRole("button", { name: "Desactivar…" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeDisabled();
    await user.keyboard("{Escape}");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("shows why it failed and stays open, so the person can try again or cancel", () => {
    setup({ error: "No se puede desactivar al último administrador." });
    expect(screen.getByRole("alert")).toHaveTextContent(
      "No se puede desactivar al último administrador.",
    );
    expect(screen.getByRole("button", { name: "Desactivar" })).toBeEnabled();
  });
});
