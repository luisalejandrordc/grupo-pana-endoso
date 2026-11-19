import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ErrorBoundary } from "./ErrorBoundary";

function Boom({ broken }: { broken: boolean }) {
  if (broken) throw new Error("boom");
  return <p>Todo bien</p>;
}

afterEach(() => vi.restoreAllMocks());

describe("ErrorBoundary", () => {
  it("replaces a screen that crashes with an explanation, and keeps the rest of the page", () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    render(
      <>
        <nav>Menú</nav>
        <ErrorBoundary>
          <Boom broken />
        </ErrorBoundary>
      </>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Algo salió mal en esta pantalla",
    );
    expect(screen.getByText("Menú")).toBeVisible();
  });

  it("gives the screen another chance once the address changes", () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const { rerender } = render(
      <ErrorBoundary resetKey="/a">
        <Boom broken />
      </ErrorBoundary>,
    );
    expect(screen.getByRole("alert")).toBeVisible();
    rerender(
      <ErrorBoundary resetKey="/b">
        <Boom broken={false} />
      </ErrorBoundary>,
    );
    expect(screen.getByText("Todo bien")).toBeVisible();
  });
});
