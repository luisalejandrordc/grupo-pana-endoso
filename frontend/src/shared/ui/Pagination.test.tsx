import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Pagination } from "./Pagination";

describe("Pagination", () => {
  it("shows nothing for an empty list, nor for a single page unless asked", () => {
    const { container, rerender } = render(
      <Pagination page={1} pageSize={10} total={0} onPageChange={vi.fn()} />,
    );
    expect(container).toBeEmptyDOMElement();
    rerender(
      <Pagination page={1} pageSize={10} total={7} onPageChange={vi.fn()} />,
    );
    expect(container).toBeEmptyDOMElement();
    rerender(
      <Pagination
        page={1}
        pageSize={10}
        total={7}
        showSinglePage
        onPageChange={vi.fn()}
      />,
    );
    expect(screen.getByText(/Mostrando/)).toHaveTextContent("1–7 de 7");
  });

  it("says which rows and which page, and the first page cannot go back", () => {
    render(
      <Pagination
        page={1}
        pageSize={10}
        total={25}
        noun="ventas"
        onPageChange={vi.fn()}
      />,
    );
    expect(screen.getByText(/Mostrando/)).toHaveTextContent(
      "Mostrando 1–10 de 25 ventas",
    );
    expect(screen.getByText("Página 1 de 3")).toBeInTheDocument();
    expect(screen.getByLabelText("Página anterior")).toBeDisabled();
  });

  it("asks for the neighbouring page, and the last page cannot go on", async () => {
    const user = userEvent.setup();
    const onPageChange = vi.fn();
    const { rerender } = render(
      <Pagination
        page={2}
        pageSize={10}
        total={25}
        onPageChange={onPageChange}
      />,
    );
    await user.click(screen.getByLabelText("Página siguiente"));
    expect(onPageChange).toHaveBeenLastCalledWith(3);
    await user.click(screen.getByLabelText("Página anterior"));
    expect(onPageChange).toHaveBeenLastCalledWith(1);
    rerender(
      <Pagination
        page={3}
        pageSize={10}
        total={25}
        onPageChange={onPageChange}
      />,
    );
    expect(screen.getByText(/Mostrando/)).toHaveTextContent("21–25 de 25");
    expect(screen.getByLabelText("Página siguiente")).toBeDisabled();
  });
});
