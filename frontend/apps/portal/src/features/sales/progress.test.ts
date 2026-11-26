import { describe, expect, it } from "vitest";
import type { PortalMilestone } from "../../shared/types";
import { progressOf } from "./progress";

const stage = (
  number: number,
  state: PortalMilestone["state"],
): PortalMilestone => ({
  number,
  name: `Etapa ${number}`,
  description: "",
  state,
});

const mid = [
  stage(1, "completed"),
  stage(2, "completed"),
  stage(3, "in_progress"),
  stage(4, "pending"),
  stage(5, "pending"),
  stage(6, "pending"),
];

describe("progressOf", () => {
  it("names the stage being worked on and counts what is done", () => {
    const progress = progressOf(mid, "active");
    expect(progress.done).toBe(2);
    expect(progress.total).toBe(6);
    expect(progress.current?.number).toBe(3);
    expect(progress.summary).toBe("Etapa actual: Etapa 3");
  });

  it("says a finished purchase is complete, with no current stage", () => {
    const all = mid.map((m) => ({ ...m, state: "completed" as const }));
    const progress = progressOf(all, "completed");
    expect(progress.done).toBe(6);
    expect(progress.current).toBeUndefined();
    expect(progress.summary).toBe("Compra completada");
  });

  it("never claims a cancelled purchase is moving", () => {
    const progress = progressOf(mid, "cancelled");
    expect(progress.current).toBeUndefined();
    expect(progress.summary).toBe("Esta compra fue cancelada");
  });

  it("says so when nothing has started", () => {
    const none = mid.map((m) => ({ ...m, state: "pending" as const }));
    expect(progressOf(none, "active").summary).toBe(
      "Su compra está por comenzar",
    );
  });
});
