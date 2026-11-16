import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Menu } from "./Menu";

function setup() {
  const first = vi.fn();
  const second = vi.fn();
  render(
    <Menu
      label="Acciones"
      items={[
        { label: "Editar", onSelect: first },
        { label: "Archivar", onSelect: second, danger: true },
      ]}
    >
      Acciones
    </Menu>,
  );
  return { first, second };
}

describe("Menu", () => {
  it("opens with the focus on the first action and walks the list with the arrows", async () => {
    const user = userEvent.setup();
    setup();
    await user.click(screen.getByRole("button", { name: "Acciones" }));
    expect(screen.getByRole("menuitem", { name: "Editar" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: "Archivar" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: "Editar" })).toHaveFocus();
  });

  it("closes on Escape and gives the focus back to its button", async () => {
    const user = userEvent.setup();
    setup();
    const button = screen.getByRole("button", { name: "Acciones" });
    await user.click(button);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(button).toHaveFocus();
    expect(button).toHaveAttribute("aria-expanded", "false");
  });

  it("runs the chosen action and closes", async () => {
    const user = userEvent.setup();
    const { first, second } = setup();
    await user.click(screen.getByRole("button", { name: "Acciones" }));
    await user.click(screen.getByRole("menuitem", { name: "Archivar" }));
    expect(second).toHaveBeenCalledOnce();
    expect(first).not.toHaveBeenCalled();
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });
});
