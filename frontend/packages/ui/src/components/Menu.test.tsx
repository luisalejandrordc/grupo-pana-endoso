import { fireEvent, render, screen, within } from "@testing-library/react";
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

  it("draws the list on the page, so a scrolling container around the button cannot cut it", async () => {
    const user = userEvent.setup();
    render(
      <div data-testid="clip" style={{ overflow: "hidden", height: 20 }}>
        <Menu label="Acciones" items={[{ label: "Editar", onSelect: vi.fn() }]}>
          Acciones
        </Menu>
      </div>,
    );
    await user.click(screen.getByRole("button", { name: "Acciones" }));
    expect(screen.getByRole("menu")).toBeVisible();
    expect(within(screen.getByTestId("clip")).queryByRole("menu")).toBeNull();
  });

  describe("placement", () => {
    function place(rect: Partial<DOMRect>, height = 500) {
      Object.defineProperty(window, "innerHeight", {
        value: height,
        configurable: true,
      });
      Object.defineProperty(window, "innerWidth", {
        value: 1000,
        configurable: true,
      });
      vi.spyOn(
        HTMLButtonElement.prototype,
        "getBoundingClientRect",
      ).mockReturnValue({
        top: 0,
        bottom: 0,
        left: 0,
        right: 0,
        ...rect,
      } as DOMRect);
    }

    it("opens below its button when there is room, aligned to its right edge", async () => {
      const user = userEvent.setup();
      place({ top: 100, bottom: 136, right: 900 });
      setup();
      await user.click(screen.getByRole("button", { name: "Acciones" }));
      const list = screen.getByRole("menu");
      expect(list.style.top).toBe("142px");
      expect(list.style.right).toBe("100px");
      expect(list.style.bottom).toBe("");
    });

    it("opens upward near the bottom of the screen", async () => {
      const user = userEvent.setup();
      place({ top: 450, bottom: 486, right: 900 }, 500);
      setup();
      await user.click(screen.getByRole("button", { name: "Acciones" }));
      const list = screen.getByRole("menu");
      expect(list.style.bottom).toBe("56px");
      expect(list.style.top).toBe("");
    });
  });

  it("closes when the page scrolls, since the list would no longer sit under its button", async () => {
    const user = userEvent.setup();
    setup();
    await user.click(screen.getByRole("button", { name: "Acciones" }));
    fireEvent.scroll(window);
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("skips actions that are disabled and cannot run them", async () => {
    const user = userEvent.setup();
    const never = vi.fn();
    render(
      <Menu
        label="Acciones"
        items={[
          { label: "Editar", onSelect: vi.fn() },
          { label: "Desactivar", onSelect: never, disabled: true },
          { label: "Reenviar", onSelect: vi.fn() },
        ]}
      >
        Acciones
      </Menu>,
    );
    await user.click(screen.getByRole("button", { name: "Acciones" }));
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: "Reenviar" })).toHaveFocus();
    await user.click(screen.getByRole("menuitem", { name: "Desactivar" }));
    expect(never).not.toHaveBeenCalled();
  });
});
