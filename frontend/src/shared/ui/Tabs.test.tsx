import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Tabs } from "./Tabs";

const items = [
  { id: "a", label: "Uno", count: 3 },
  { id: "b", label: "Dos" },
  { id: "c", label: "Tres" },
] as const;

describe("Tabs", () => {
  it("names each tab by its label even when it carries a count", () => {
    render(
      <Tabs label="Secciones" items={items} value="a" onChange={vi.fn()} />,
    );
    expect(screen.getByRole("tablist", { name: "Secciones" })).toBeVisible();
    expect(screen.getByRole("tab", { name: "Uno" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("tab", { name: "Dos" })).toHaveAttribute(
      "aria-selected",
      "false",
    );
  });

  it("moves the focus with the arrow keys but only picks a tab on Enter", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Tabs label="Secciones" items={items} value="a" onChange={onChange} />,
    );
    screen.getByRole("tab", { name: "Uno" }).focus();

    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "Dos" })).toHaveFocus();
    expect(onChange).not.toHaveBeenCalled();

    await user.keyboard("{End}");
    expect(screen.getByRole("tab", { name: "Tres" })).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "Uno" })).toHaveFocus();

    await user.keyboard("{ArrowLeft}{Enter}");
    expect(onChange).toHaveBeenCalledWith("c");
  });

  it("keeps only the selected tab in the tab order", () => {
    render(
      <Tabs label="Secciones" items={items} value="b" onChange={vi.fn()} />,
    );
    expect(screen.getByRole("tab", { name: "Dos" })).toHaveAttribute(
      "tabindex",
      "0",
    );
    expect(screen.getByRole("tab", { name: "Uno" })).toHaveAttribute(
      "tabindex",
      "-1",
    );
  });
}
