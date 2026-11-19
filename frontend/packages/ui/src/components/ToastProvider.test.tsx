import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "./ToastProvider";
import { useToast } from "./useToast";

function Trigger() {
  const { notify } = useToast();
  return (
    <>
      <button onClick={() => notify({ title: "Guardado", tone: "success" })}>
        ok
      </button>
      <button
        onClick={() =>
          notify({ title: "Falló", message: "Sin conexión", tone: "error" })
        }
      >
        fail
      </button>
    </>
  );
}

afterEach(() => vi.useRealTimers());

describe("ToastProvider", () => {
  it("stacks notifications and lets each one be closed by hand", async () => {
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <Trigger />
      </ToastProvider>,
    );
    await user.click(screen.getByText("ok"));
    await user.click(screen.getByText("fail"));
    expect(screen.getByText("Guardado")).toBeVisible();
    expect(screen.getByText("Falló")).toBeVisible();

    await user.click(screen.getAllByLabelText("Cerrar notificación")[0]);
    expect(screen.queryByText("Guardado")).not.toBeInTheDocument();
    expect(screen.getByText("Falló")).toBeVisible();
  });

  it("announces an error as an alert and a confirmation politely", async () => {
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <Trigger />
      </ToastProvider>,
    );
    await user.click(screen.getByText("ok"));
    await user.click(screen.getByText("fail"));
    expect(screen.getByRole("alert")).toHaveTextContent("Sin conexión");
    expect(screen.getByRole("status")).toHaveTextContent("Guardado");
  });

  it("closes a confirmation by itself, and keeps an error longer", () => {
    vi.useFakeTimers();
    render(
      <ToastProvider>
        <Trigger />
      </ToastProvider>,
    );
    act(() => screen.getByText("ok").click());
    act(() => screen.getByText("fail").click());
    act(() => {
      vi.advanceTimersByTime(5000);
    });
    expect(screen.queryByText("Guardado")).not.toBeInTheDocument();
    expect(screen.getByText("Falló")).toBeInTheDocument();
    act(() => {
      vi.advanceTimersByTime(4000);
    });
    expect(screen.queryByText("Falló")).not.toBeInTheDocument();
  });
});
