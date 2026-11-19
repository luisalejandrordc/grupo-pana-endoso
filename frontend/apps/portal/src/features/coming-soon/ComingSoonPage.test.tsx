import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import App from "../../app/App";

function open(path: string) {
  render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  );
}

describe("customer portal placeholder", () => {
  it("tells visitors the portal is coming, and who to ask in the meantime", () => {
    open("/");
    expect(
      screen.getByRole("heading", { name: "Estamos preparando tu portal" }),
    ).toBeVisible();
    expect(screen.getByText("Próximamente")).toBeVisible();
    expect(screen.getByText(/Contacta a tu asesor/)).toBeVisible();
    expect(screen.getByRole("img", { name: "EndosoFlow" })).toBeVisible();
  });

  it("shows the same page at any address, so a bookmark or a shared link never breaks", () => {
    open("/sales/123/documentos");
    expect(
      screen.getByRole("heading", { name: "Estamos preparando tu portal" }),
    ).toBeVisible();
  });

  it("names the browser tab", () => {
    open("/");
    expect(document.title).toBe("Portal de clientes · EndosoFlow");
  });

  it("asks for nothing yet: no form, no login", () => {
    open("/");
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
