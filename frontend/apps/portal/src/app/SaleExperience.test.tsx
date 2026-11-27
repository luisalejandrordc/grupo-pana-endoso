import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  answers,
  detail,
  installApi,
  json,
  openPortal,
  stages,
  state,
  summary,
} from "./testSupport";

const path = `/sales/${summary.id}`;

beforeEach(() => {
  installApi();
  state.signedIn = true;
  answers[`GET /portal/sales/${summary.id}`] = () => json(detail);
});
afterEach(() => vi.unstubAllGlobals());

const tab = (number: number) =>
  screen.findByRole("tab", { name: new RegExp(`Etapa ${number},`) });

const section = (taskNumber: number, milestoneNumber: number) => ({
  taskNumber,
  milestoneNumber,
  title: `Tarea ${taskNumber}`,
  completedAt: "2026-03-12T15:00:00Z",
  fields: [],
  documents: [],
});

describe("the stages of a purchase", () => {
  it("opens on the stage being worked on, not on the first one", async () => {
    openPortal(path);
    expect(await tab(2)).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: /Etapa 1,/ })).toHaveAttribute(
      "aria-selected",
      "false",
    );
    const panel = screen.getByRole("tabpanel");
    expect(
      within(panel).getByRole("heading", {
        level: 3,
        name: "Asignación y Pago",
      }),
    ).toBeVisible();
    expect(within(panel).getByText("ETAPA 2 DE 6")).toBeVisible();
    // Nothing is recorded for it yet, and it says what that means.
    expect(
      within(panel).getByText(/Estamos trabajando en esta etapa/),
    ).toBeVisible();
  });

  it("shows another stage, with what was recorded in it, when it is pressed", async () => {
    openPortal(path);
    const user = userEvent.setup();
    await user.click(await tab(1));
    const panel = screen.getByRole("tabpanel");
    expect(
      within(panel).getByRole("heading", {
        level: 3,
        name: "Cotización y Documentación",
      }),
    ).toBeVisible();
    expect(within(panel).getByText("Cotización del Vehículo")).toBeVisible();
    expect(within(panel).queryByText(/Estamos trabajando/)).toBeNull();
  });

  it("says a stage that has not started has not started", async () => {
    openPortal(path);
    const user = userEvent.setup();
    await user.click(await tab(4));
    expect(
      within(screen.getByRole("tabpanel")).getByText(
        "Esta etapa aún no inicia.",
      ),
    ).toBeVisible();
  });

  it("moves with the arrow keys, Home and End, as tabs do", async () => {
    openPortal(path);
    const user = userEvent.setup();
    (await tab(2)).focus();

    await user.keyboard("{ArrowRight}");
    expect(await tab(3)).toHaveAttribute("aria-selected", "true");
    expect(await tab(3)).toHaveFocus();
    await user.keyboard("{End}");
    expect(await tab(6)).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{ArrowRight}");
    expect(await tab(6)).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{Home}");
    expect(await tab(1)).toHaveAttribute("aria-selected", "true");
  });

  it("only the open stage is in the tab order", async () => {
    openPortal(path);
    expect(await tab(2)).toHaveAttribute("tabindex", "0");
    expect(await tab(3)).toHaveAttribute("tabindex", "-1");
  });

  it("steps to the next and the previous stage with the buttons, and stops at both ends", async () => {
    openPortal(path);
    const user = userEvent.setup();
    await tab(2);

    await user.click(screen.getByRole("button", { name: /Etapa siguiente/ }));
    expect(await tab(3)).toHaveAttribute("aria-selected", "true");
    await user.click(screen.getByRole("tab", { name: /Etapa 1,/ }));
    expect(
      screen.getByRole("button", { name: /Etapa anterior/ }),
    ).toBeDisabled();
    await user.click(screen.getByRole("tab", { name: /Etapa 6,/ }));
    expect(
      screen.getByRole("button", { name: /Etapa siguiente/ }),
    ).toBeDisabled();
  });

  it("opens a finished purchase on its last stage", async () => {
    answers[`GET /portal/sales/${summary.id}`] = () =>
      json({
        ...detail,
        state: "completed",
        completedAt: "2026-06-20T15:00:00Z",
        milestones: stages(Array(6).fill("completed")),
      });
    openPortal(path);
    expect(await tab(6)).toHaveAttribute("aria-selected", "true");
  });

  it("opens on the last finished stage when none is in progress", async () => {
    answers[`GET /portal/sales/${summary.id}`] = () =>
      json({
        ...detail,
        milestones: stages([
          "completed",
          "completed",
          "pending",
          "pending",
          "pending",
          "pending",
        ]),
      });
    openPortal(path);
    expect(await tab(2)).toHaveAttribute("aria-selected", "true");
  });

  it("names every stage's state for a screen reader, not only by color", async () => {
    openPortal(path);
    expect(await tab(1)).toHaveAccessibleName(/Etapa 1, Completada/);
    expect(await tab(2)).toHaveAccessibleName(/Etapa 2, En curso/);
    expect(await tab(3)).toHaveAccessibleName(/Etapa 3, Pendiente/);
  });
});

describe("the percentage", () => {
  it("counts the tasks done: one of thirteen is 8%", async () => {
    openPortal(path);
    expect(await screen.findByText("8%")).toBeVisible();
    const bar = screen.getByRole("progressbar", {
      name: "Avance de su compra",
    });
    expect(bar).toHaveAttribute("aria-valuenow", "8");
    expect(bar).toHaveAttribute("aria-valuetext", "8% completado");
    expect(screen.getByText("Etapa 2 de 6: Asignación y Pago")).toBeVisible();
  });

  it("follows the completed tasks as they grow", async () => {
    answers[`GET /portal/sales/${summary.id}`] = () =>
      json({
        ...detail,
        sections: [1, 2, 3, 4, 5].map((n) => section(n, n <= 2 ? 1 : 2)),
      });
    openPortal(path);
    expect(await screen.findByText("38%")).toBeVisible();
  });

  it("is 100% when the purchase is complete, whatever the count", async () => {
    answers[`GET /portal/sales/${summary.id}`] = () =>
      json({
        ...detail,
        state: "completed",
        completedAt: "2026-06-20T15:00:00Z",
        milestones: stages(Array(6).fill("completed")),
      });
    openPortal(path);
    expect(await screen.findByText("100%")).toBeVisible();
    expect(screen.getByText("Compra completada")).toBeVisible();
  });

  it("keeps where a cancelled purchase stopped, and says so", async () => {
    answers[`GET /portal/sales/${summary.id}`] = () =>
      json({ ...detail, state: "cancelled" });
    openPortal(path);
    expect(await screen.findByText("8%")).toBeVisible();
    expect(screen.getByText("al cancelarse")).toBeVisible();
  });

  it("is not shown in the list, which only knows the six stages and would disagree", async () => {
    openPortal("/");
    await screen.findByRole("link", { name: /Corolla Cross/ });
    expect(screen.queryByRole("progressbar")).toBeNull();
    expect(screen.queryByText(/\d%/)).toBeNull();
  });
});

describe("the photos", () => {
  it("opens on the main view even when the catalog lists other views first", async () => {
    openPortal(path);
    const image = await screen.findByRole("img", { name: /Vista principal/ });
    expect(image).toHaveAttribute("src", "https://img.example/main.jpg");
    expect(screen.getByText("1 / 3")).toBeVisible();
  });

  it("walks through the other views with the arrows, around in both directions", async () => {
    openPortal(path);
    const user = userEvent.setup();
    await screen.findByRole("img", { name: /Vista principal/ });

    await user.click(screen.getByRole("button", { name: "Foto siguiente" }));
    expect(screen.getByRole("img", { name: /Frente/ })).toHaveAttribute(
      "src",
      "https://img.example/front.jpg",
    );
    expect(screen.getByText("2 / 3")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "Foto anterior" }));
    await user.click(screen.getByRole("button", { name: "Foto anterior" }));
    expect(screen.getByRole("img", { name: /Interior/ })).toBeVisible();
  });

  it("goes to a view when its thumbnail is pressed", async () => {
    openPortal(path);
    const user = userEvent.setup();
    const thumbs = await screen.findByRole("list", {
      name: "Vistas del vehículo",
    });
    await user.click(within(thumbs).getByRole("button", { name: "Interior" }));
    expect(screen.getByRole("img", { name: /Interior/ })).toBeVisible();
    expect(
      within(thumbs).getByRole("button", { name: "Interior" }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      within(thumbs).getByRole("button", { name: "Vista principal" }),
    ).toHaveAttribute("aria-pressed", "false");
  });

  it("offers no arrows or thumbnails when there is only one photo", async () => {
    answers[`GET /portal/sales/${summary.id}`] = () =>
      json({
        ...detail,
        vehicle: {
          ...detail.vehicle,
          images: [
            { viewpoint: "main_view", url: "https://img.example/main.jpg" },
          ],
        },
      });
    openPortal(path);
    await screen.findByRole("img", { name: /Vista principal/ });
    expect(screen.queryByRole("button", { name: "Foto siguiente" })).toBeNull();
    expect(
      screen.queryByRole("list", { name: "Vistas del vehículo" }),
    ).toBeNull();
  });

  it("opens the photo large and closes it with Escape, returning to where it was", async () => {
    openPortal(path);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", {
        name: "Ampliar foto: Vista principal",
      }),
    );

    const dialog = screen.getByRole("dialog", {
      name: "Corolla Cross XEI Hybrid",
    });
    // Outside the gallery's own box, so a key pressed in the viewer is never also heard by the gallery behind it.
    expect(dialog.closest(".gallery")).toBeNull();
    expect(within(dialog).getByText("Vista principal · 1 de 3")).toBeVisible();

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Ampliar foto: Vista principal" }),
    ).toHaveFocus();
  });

  it("changes the view in the large viewer with the buttons and the keyboard, and keeps the gallery in step", async () => {
    openPortal(path);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", {
        name: "Ampliar foto: Vista principal",
      }),
    );
    const dialog = screen.getByRole("dialog");

    await user.keyboard("{ArrowRight}");
    expect(within(dialog).getByText("Frente · 2 de 3")).toBeVisible();
    await user.click(
      within(dialog).getByRole("button", { name: "Foto siguiente" }),
    );
    expect(within(dialog).getByText("Interior · 3 de 3")).toBeVisible();
    await user.keyboard("{ArrowRight}");
    expect(within(dialog).getByText("Vista principal · 1 de 3")).toBeVisible();
    await user.keyboard("{ArrowLeft}");
    expect(within(dialog).getByText("Interior · 3 de 3")).toBeVisible();

    await user.keyboard("{Escape}");
    // What was last seen in the viewer is what the page shows now.
    expect(screen.getByRole("img", { name: /Interior/ })).toBeVisible();
  });

  it("zooms in and out, steps back to 100%, and starts again on each photo", async () => {
    openPortal(path);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", {
        name: "Ampliar foto: Vista principal",
      }),
    );
    const dialog = screen.getByRole("dialog");

    expect(
      within(dialog).getByRole("button", { name: "Alejar" }),
    ).toBeDisabled();
    expect(
      within(dialog).getByRole("button", { name: "Restablecer zoom" }),
    ).toHaveTextContent("100%");

    await user.click(within(dialog).getByRole("button", { name: "Acercar" }));
    expect(
      within(dialog).getByRole("button", { name: "Restablecer zoom" }),
    ).toHaveTextContent("150%");
    await user.keyboard("+");
    expect(
      within(dialog).getByRole("button", { name: "Restablecer zoom" }),
    ).toHaveTextContent("200%");
    await user.keyboard("-");
    expect(
      within(dialog).getByRole("button", { name: "Restablecer zoom" }),
    ).toHaveTextContent("150%");

    await user.click(
      within(dialog).getByRole("button", { name: "Restablecer zoom" }),
    );
    expect(
      within(dialog).getByRole("button", { name: "Restablecer zoom" }),
    ).toHaveTextContent("100%");

    await user.click(within(dialog).getByRole("button", { name: "Acercar" }));
    await user.keyboard("{ArrowRight}");
    expect(
      within(dialog).getByRole("button", { name: "Restablecer zoom" }),
    ).toHaveTextContent("100%");
  });

  it("zooms with a double click and goes back with another", async () => {
    openPortal(path);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", {
        name: "Ampliar foto: Vista principal",
      }),
    );
    const dialog = screen.getByRole("dialog");

    await user.dblClick(
      within(dialog).getByRole("img", { name: /Vista principal/ }),
    );
    expect(
      within(dialog).getByRole("button", { name: "Restablecer zoom" }),
    ).toHaveTextContent("200%");
    await user.dblClick(
      within(dialog).getByRole("img", { name: /Vista principal/ }),
    );
    expect(
      within(dialog).getByRole("button", { name: "Restablecer zoom" }),
    ).toHaveTextContent("100%");
  });
});

describe("the account menu", () => {
  it("has no list of pages: the logo leads to the purchases and the person has one dropdown", async () => {
    openPortal("/");
    await screen.findByRole("heading", { name: "Mis compras" });
    expect(screen.queryByRole("navigation")).toBeNull();
    const trigger = screen.getByRole("button", {
      name: "Cuenta de María Fernanda Quispe",
    });
    expect(trigger).toHaveTextContent("María Fernanda Quispe");
    expect(trigger).toHaveAttribute("aria-haspopup", "menu");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByRole("link", { name: /mis compras/i })).toHaveAttribute(
      "href",
      "/",
    );
  });

  it("offers the account and signing out, and takes the person to their account", async () => {
    openPortal("/");
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: /Cuenta de María/ }),
    );

    const items = screen
      .getAllByRole("menuitem")
      .map((item) => item.textContent);
    expect(items).toEqual(["Mi cuenta", "Cerrar sesión"]);

    await user.click(screen.getByRole("menuitem", { name: "Mi cuenta" }));
    expect(
      await screen.findByRole("heading", { name: "Mi cuenta" }),
    ).toBeVisible();
    expect(screen.getByRole("link", { name: "Mis compras" })).toHaveAttribute(
      "href",
      "/",
    );
  });

  it("closes with Escape", async () => {
    openPortal("/");
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: /Cuenta de María/ }),
    );
    expect(screen.getByRole("menu")).toBeVisible();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).toBeNull();
  });
});
