import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { SearchField } from "./SearchField";

function Harness() {
  const [value, setValue] = useState("");
  return (
    <>
      <SearchField value={value} onChange={setValue} shortcut />
      <input aria-label="Otro campo" />
    </>
  );
}

describe("SearchField", () => {
  it("jumps to the search box when / is pressed anywhere on the page", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.keyboard("/");
    expect(screen.getByRole("searchbox", { name: "Buscar" })).toHaveFocus();
    expect(screen.getByRole("searchbox")).toHaveValue("");
  });

  it("leaves / alone while the person is typing in another field", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByLabelText("Otro campo"));
    await user.keyboard("a/b");
    expect(screen.getByLabelText("Otro campo")).toHaveValue("a/b");
  });

  it("clears with its button or with Escape", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const box = screen.getByRole("searchbox");
    await user.type(box, "tanaka");
    await user.click(screen.getByLabelText("Borrar búsqueda"));
    expect(box).toHaveValue("");
    expect(box).toHaveFocus();
    await user.type(box, "x{Escape}");
    expect(box).toHaveValue("");
  });
});
