import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { Modal } from "./Modal";

function Harness({ onClose = () => undefined }: { onClose?: () => void }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>Abrir</button>
      {open && (
        <Modal
          title="Motivo"
          onClose={() => {
            setOpen(false);
            onClose();
          }}
          footer={<button>Aceptar</button>}
        >
          <input aria-label="Texto" />
        </Modal>
      )}
    </>
  );
}

describe("Modal", () => {
  it("is announced as a dialog with its title", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByText("Abrir"));
    expect(screen.getByRole("dialog", { name: "Motivo" })).toHaveAttribute(
      "aria-modal",
      "true",
    );
  });

  it("puts focus on the first field and gives it back to where it came from", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const opener = screen.getByText("Abrir");
    await user.click(opener);
    expect(screen.getByLabelText("Texto")).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(opener).toHaveFocus();
  });

  it("closes on Escape and on a click outside, but not on a click inside", async () => {
    const onClose = vi.fn();
    const user = userEvent.setup();
    render(<Harness onClose={onClose} />);
    await user.click(screen.getByText("Abrir"));
    await user.click(screen.getByLabelText("Texto"));
    expect(onClose).not.toHaveBeenCalled();
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
    await user.click(screen.getByText("Abrir"));
    await user.click(document.querySelector(".modal-backdrop")!);
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("keeps Tab inside the dialog", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByText("Abrir"));
    const inside = [
      screen.getByLabelText("Cerrar ventana"),
      screen.getByLabelText("Texto"),
      screen.getByText("Aceptar"),
    ];
    for (let i = 0; i < 6; i++) {
      await user.tab();
      expect(inside).toContain(document.activeElement);
    }
    await user.tab({ shift: true });
    expect(inside).toContain(document.activeElement);
  });

  it("stops the page behind from scrolling while open", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByText("Abrir"));
    expect(document.body.style.overflow).toBe("hidden");
    await user.keyboard("{Escape}");
    expect(document.body.style.overflow).toBe("");
  });
});
