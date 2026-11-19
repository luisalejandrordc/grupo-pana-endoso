import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { EmptyState, ErrorState } from "./EmptyState";

describe("EmptyState", () => {
  it("shows its title, its explanation and its actions", () => {
    render(
      <EmptyState
        title="Sin resultados"
        description="Prueba con otros filtros."
        actions={<button>Quitar filtros</button>}
      />,
    );
    expect(screen.getByText("Sin resultados")).toBeVisible();
    expect(screen.getByText("Prueba con otros filtros.")).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Quitar filtros" }),
    ).toBeVisible();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

describe("ErrorState", () => {
  it("is an alert, and offers to try again only when it can", async () => {
    const user = userEvent.setup();
    const retry = vi.fn();
    const { rerender } = render(
      <ErrorState
        title="No se pudo cargar"
        message="Sin conexión"
        onRetry={retry}
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Sin conexión");
    await user.click(screen.getByRole("button", { name: /Reintentar/ }));
    expect(retry).toHaveBeenCalledOnce();
    rerender(<ErrorState title="No se pudo cargar" />);
    expect(screen.queryByRole("button", { name: /Reintentar/ })).toBeNull();
  });
});
