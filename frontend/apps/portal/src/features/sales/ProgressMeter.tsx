import type { PortalMilestone, SaleState } from "../../shared/types";
import { percentOf, progressOf } from "./progress";

/**
 * The big number of the purchase: how much of it is done, with a bar and one line about where it stands.
 * A cancelled purchase keeps the point where it stopped, in a muted tone, so it never looks like it is moving.
 */
export function ProgressMeter({
  milestones,
  state,
  completedTasks,
}: {
  milestones: PortalMilestone[];
  state: SaleState;
  completedTasks: number;
}) {
  const percent = percentOf(state, completedTasks);
  const progress = progressOf(milestones, state);
  const caption =
    state === "active" && progress.current
      ? `Etapa ${progress.current.number} de ${progress.total}: ${progress.current.name}`
      : progress.summary;

  return (
    <div className={`meter is-${state}`}>
      <div className="meter-head">
        <strong className="meter-percent">{percent}%</strong>
        <span className="meter-word">
          {state === "cancelled" ? "al cancelarse" : "completado"}
        </span>
      </div>
      <div
        className="meter-bar"
        role="progressbar"
        aria-label="Avance de su compra"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={percent}
        aria-valuetext={`${percent}% completado`}
      >
        <span style={{ width: `${percent}%` }} />
      </div>
      <p className="meter-caption">{caption}</p>
    </div>
  );
}
