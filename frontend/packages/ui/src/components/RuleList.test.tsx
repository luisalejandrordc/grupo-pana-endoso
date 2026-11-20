import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RuleList } from "./RuleList";

describe("RuleList", () => {
  it("says in words which rules are met and which are pending", () => {
    render(
      <RuleList
        label="Requisitos de la contraseña"
        rules={[
          { label: "Al menos 10 caracteres", met: true },
          { label: "Las contraseñas coinciden", met: false },
        ]}
      />,
    );
    const items = within(
      screen.getByRole("list", { name: "Requisitos de la contraseña" }),
    ).getAllByRole("listitem");
    expect(items[0]).toHaveTextContent("Al menos 10 caracteres (cumplido)");
    expect(items[0]).toHaveClass("is-met");
    expect(items[1]).toHaveTextContent("Las contraseñas coinciden (pendiente)");
    expect(items[1]).not.toHaveClass("is-met");
  });
});
