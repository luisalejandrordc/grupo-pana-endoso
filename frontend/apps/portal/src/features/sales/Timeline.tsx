import { Badge, Icon } from "@endosoflow/ui";
import type {
  PortalMilestone,
  PortalSection,
  SaleState,
} from "../../shared/types";
import { milestoneStateLabels, milestoneStateTones } from "./labels";
import { SectionCard } from "./SectionCard";

function Marker({
  milestone,
  halted,
}: {
  milestone: PortalMilestone;
  halted: boolean;
}) {
  if (milestone.state === "completed")
    return <Icon name="check" size={16} strokeWidth={2.6} />;
  if (milestone.state === "in_progress" && !halted)
    return <span className="marker-pulse" />;
  return <span className="marker-number">{milestone.number}</span>;
}

/**
 * The six stages, top to bottom, each with what was recorded in it. Only completed tasks have anything to
 * show: the server sends nothing else, so there is nothing here to hide.
 */
export function Timeline({
  milestones,
  sections,
  state,
}: {
  milestones: PortalMilestone[];
  sections: PortalSection[];
  state: SaleState;
}) {
  // A cancelled purchase stops where it was: its "current" stage must not look like it is moving.
  const halted = state === "cancelled";
  return (
    <ol className="timeline">
      {milestones.map((milestone) => {
        const own = sections.filter(
          (section) => section.milestoneNumber === milestone.number,
        );
        const tone =
          halted && milestone.state === "in_progress"
            ? "neutral"
            : milestoneStateTones[milestone.state];
        const label =
          halted && milestone.state === "in_progress"
            ? "Detenida"
            : milestoneStateLabels[milestone.state];
        return (
          <li
            key={milestone.number}
            className={`timeline-item is-${milestone.state}${halted ? " is-halted" : ""}`}
          >
            <span className="timeline-marker" aria-hidden="true">
              <Marker milestone={milestone} halted={halted} />
            </span>
            <div className="timeline-body">
              <div className="timeline-head">
                <h3>
                  <span className="sr-only">Etapa {milestone.number}: </span>
                  {milestone.name}
                </h3>
                <Badge tone={tone}>{label}</Badge>
              </div>
              <p className="timeline-description">{milestone.description}</p>
              {own.length > 0 && (
                <div className="timeline-sections">
                  {own.map((section) => (
                    <SectionCard key={section.taskNumber} section={section} />
                  ))}
                </div>
              )}
            </div>
          </li>
        );
      })}
    </ol>
  );
}
