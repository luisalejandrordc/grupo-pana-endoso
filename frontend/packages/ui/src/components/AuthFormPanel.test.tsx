import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AuthFormPanel } from "./AuthFormPanel";

describe("AuthFormPanel", () => {
  it("draws the same page for every site: logo, heading, content, security and help", () => {
    render(
      <AuthFormPanel
        kicker="ACCESO DE PRUEBA"
        title="Bienvenido"
        lede="Ingrese para continuar."
        security="Acceso protegido"
        helpLead="¿Necesita ayuda?"
        help="Escriba a su asesor."
      >
        <button>Entrar</button>
      </AuthFormPanel>,
    );
    expect(screen.getByRole("img", { name: "EndosoFlow" })).toBeVisible();
    expect(screen.getByText("ACCESO DE PRUEBA")).toBeVisible();
    expect(
      screen.getByRole("heading", { level: 1, name: "Bienvenido" }),
    ).toBeVisible();
    expect(screen.getByText("Ingrese para continuar.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Entrar" })).toBeVisible();
    expect(screen.getByText("Acceso protegido")).toBeVisible();
    expect(screen.getByText("¿Necesita ayuda?")).toBeVisible();
    expect(screen.getByText(/Escriba a su asesor/)).toBeVisible();
  });

  it("leaves the lede out when there is none", () => {
    const { container } = render(
      <AuthFormPanel kicker="K" title="T" security="s" helpLead="h" help="x">
        <span />
      </AuthFormPanel>,
    );
    expect(container.querySelector(".login-form-lede")).toBeNull();
  });
});
