import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { PasswordField } from "./PasswordField";

function Harness(props: { error?: string; hint?: string }) {
  const [value, setValue] = useState("");
  return (
    <PasswordField
      label="Nueva contraseña"
      value={value}
      onChange={setValue}
      autoComplete="new-password"
      {...props}
    />
  );
}

describe("PasswordField", () => {
  it("is a password input named by its label, hidden until asked", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const input = screen.getByLabelText("Nueva contraseña");
    expect(input).toHaveAttribute("type", "password");
    expect(input).toHaveAttribute("autocomplete", "new-password");

    await user.type(input, "secreto");
    const toggle = screen.getByRole("button", {
      name: "Mostrar nueva contraseña",
    });
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    await user.click(toggle);
    expect(input).toHaveAttribute("type", "text");
    expect(input).toHaveValue("secreto");
    expect(
      screen.getByRole("button", { name: "Ocultar nueva contraseña" }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("names its toggle after the field, so two password fields do not share a button name", () => {
    render(
      <>
        <PasswordField
          label="Contraseña actual"
          value=""
          onChange={() => {}}
          autoComplete="current-password"
        />
        <PasswordField
          label="Nueva contraseña"
          value=""
          onChange={() => {}}
          autoComplete="new-password"
        />
      </>,
    );
    expect(
      screen.getByRole("button", { name: "Mostrar contraseña actual" }),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Mostrar nueva contraseña" }),
    ).toBeVisible();
  });

  it("describes the input with its hint, or with its error when there is one", () => {
    const { rerender } = render(<Harness hint="Mínimo 10 caracteres." />);
    const input = screen.getByLabelText("Nueva contraseña");
    expect(input).toHaveAccessibleDescription("Mínimo 10 caracteres.");
    expect(input).not.toHaveAttribute("aria-invalid");
    rerender(
      <Harness hint="Mínimo 10 caracteres." error="Es demasiado común." />,
    );
    expect(input).toHaveAccessibleDescription("Es demasiado común.");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("alert")).toHaveTextContent("Es demasiado común.");
  });
});
