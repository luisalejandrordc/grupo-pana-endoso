import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { Field } from "./Field";

function Notes({ limit = 10, error }: { limit?: number; error?: string }) {
  const [value, setValue] = useState("");
  return (
    <Field
      label="Notas"
      error={error}
      hint={error ? undefined : "Solo el equipo."}
    >
      <textarea
        value={value}
        maxLength={limit}
        onChange={(event) => setValue(event.target.value)}
      />
    </Field>
  );
}

describe("Field", () => {
  it("has no counter when the box has no limit", () => {
    render(
      <Field label="Nombre">
        <input defaultValue="Ana" />
      </Field>,
    );
    expect(screen.queryByText(/\d+ \/ \d+/)).toBeNull();
  });

  it("shows how much of the limit is used, and follows the typing", async () => {
    render(<Notes limit={10} />);
    const user = userEvent.setup();
    expect(screen.getByText("0 / 10")).toBeVisible();
    await user.type(screen.getByLabelText("Notas"), "hola");
    expect(screen.getByText("4 / 10")).toBeVisible();
  });

  it("warns near the limit and says so when it is reached, and the box stops there", async () => {
    render(<Notes limit={10} />);
    const user = userEvent.setup();
    const box = screen.getByLabelText("Notas");

    await user.type(box, "123456789");
    expect(screen.getByText("9 / 10")).toHaveClass("is-near");
    expect(screen.getByRole("status")).toHaveTextContent("acercando al límite");

    await user.type(box, "0123");
    expect(box).toHaveValue("1234567890");
    expect(screen.getByText("10 / 10")).toHaveClass("is-full");
    expect(screen.getByRole("status")).toHaveTextContent(
      "Llegaste al límite de 10 caracteres.",
    );
  });

  it("stays quiet for a screen reader until the limit is close", async () => {
    render(<Notes limit={100} />);
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Notas"), "unas pocas palabras");
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
  });

  it("tells a screen reader the limit when it enters the box", () => {
    render(<Notes limit={2000} />);
    expect(screen.getByLabelText("Notas")).toHaveAccessibleDescription(
      /Solo el equipo\..*Máximo 2000 caracteres\./,
    );
  });

  it("keeps the error and the counter together", () => {
    render(<Notes error="Escribe el motivo." />);
    expect(screen.getByRole("alert")).toHaveTextContent("Escribe el motivo.");
    expect(screen.getByText("0 / 10")).toBeVisible();
    expect(screen.getByLabelText("Notas")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
  });

  it("counts a single-line input too", () => {
    render(
      <Field label="Motivo">
        <input value="abc" maxLength={500} onChange={() => undefined} />
      </Field>,
    );
    expect(screen.getByText("3 / 500")).toBeVisible();
  });
});
