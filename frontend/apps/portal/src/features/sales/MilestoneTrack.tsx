import type { PortalMilestone, SaleState } from "../../shared/types";
import { milestoneStateLabels } from "./labels";

/**
 * The six stages as a row of segments. Each one is named for assistive technology with its state, so the
 * progress is never conveyed by color alone.
 */
export function MilestoneTrack({
  milestones,
  state,
}: {
  milestones: PortalMilestone[];
  state: SaleState;
}) {
  return (
    <ol
      className={`track ${state === "cancelled" ? "is-cancelled" : ""}`.trim()}
      aria-label="Etapas de la compra"
    >
      {milestones.map((milestone) => (
        <li
          key={milestone.number}
          className={`track-step is-${milestone.state}`}
        >
          <span className="track-bar" />
          <span className="sr-only">
            {milestone.number}. {milestone.name}:{" "}
            {milestoneStateLabels[milestone.state]}
          </span>
        </li>
      ))}
    </ol>
  );
}
