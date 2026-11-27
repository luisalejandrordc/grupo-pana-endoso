import { Badge, Icon } from "@endosoflow/ui";
import { useId, useRef, useState, type KeyboardEvent } from "react";
import type {
  MilestoneState,
  PortalMilestone,
  PortalSection,
  SaleState,
} from "../../shared/types";
import { milestoneStateLabels, milestoneStateTones } from "./labels";
import { SectionCard } from "./SectionCard";

/** The stage to look at first: the one being worked on, else the last one that was finished, else the first. */
function initialStage(milestones: PortalMilestone[]): number {
  const current = milestones.find((m) => m.state === "in_progress");
  if (current) return current.number;
  const done = milestones.filter((m) => m.state === "completed");
  return done.length > 0 ? done[done.length - 1].number : 1;
}

function Marker({
  state,
  number,
  halted,
}: {
  state: MilestoneState;
  number: number;
  halted: boolean;
}) {
  if (state === "completed")
    return <Icon name="check" size={16} strokeWidth={2.6} />;
  if (state === "in_progress" && !halted)
    return <span className="marker-pulse" />;
  return <span className="marker-number">{number}</span>;
}

/** What to say when a stage has nothing recorded to show yet (or at all). */
function emptyText(state: MilestoneState, halted: boolean): string {
  if (state === "completed")
    return "Esta etapa no tiene datos ni documentos para mostrar.";
  if (state === "in_progress")
    return halted
      ? "La compra se canceló mientras esta etapa estaba en curso."
      : "Estamos trabajando en esta etapa. Aquí verá la información y los documentos a medida que se completen.";
  return "Esta etapa aún no inicia.";
}

/**
 * The six stages as a row you can press, and under it one stage at a time: what it is, what was recorded in it and
 * its documents. It opens on the stage that matters now (the one in progress, or the last one finished), so the
 * page never makes a customer scroll through five others to find it. It follows the keyboard pattern of tabs.
 */
export function MilestoneStepper({
  milestones,
  sections,
  state,
}: {
  milestones: PortalMilestone[];
  sections: PortalSection[];
  state: SaleState;
}) {
  const base = useId();
  // Until the person picks one, the stage in focus follows the purchase (a refresh can move it forward).
  const [picked, setPicked] = useState<number | null>(null);
  const active = picked ?? initialStage(milestones);
  const tabs = useRef<(HTMLButtonElement | null)[]>([]);
  const halted = state === "cancelled";

  const selected = milestones.find((m) => m.number === active) ?? milestones[0];
  const own = sections.filter((s) => s.milestoneNumber === selected.number);
  const index = milestones.indexOf(selected);
  const tabId = (n: number) => `${base}-tab-${n}`;
  const panelId = `${base}-panel`;

  function go(to: number, focus = false) {
    const target = Math.max(0, Math.min(milestones.length - 1, to));
    setPicked(milestones[target].number);
    if (focus) tabs.current[target]?.focus();
  }

  function onKeyDown(event: KeyboardEvent) {
    const move =
      event.key === "ArrowRight" || event.key === "ArrowDown"
        ? index + 1
        : event.key === "ArrowLeft" || event.key === "ArrowUp"
          ? index - 1
          : event.key === "Home"
            ? 0
            : event.key === "End"
              ? milestones.length - 1
              : null;
    if (move === null) return;
    event.preventDefault();
    go(move, true);
  }

  return (
    <div className="stepper">
      <div
        className="stepper-tabs"
        role="tablist"
        aria-label="Etapas de la compra"
        onKeyDown={onKeyDown}
      >
        {milestones.map((milestone, position) => {
          const stateLabel =
            halted && milestone.state === "in_progress"
              ? "Detenida"
              : milestoneStateLabels[milestone.state];
          const before = milestones[position - 1];
          return (
            <button
              key={milestone.number}
              ref={(node) => {
                tabs.current[position] = node;
              }}
              id={tabId(milestone.number)}
              type="button"
              role="tab"
              aria-selected={milestone.number === selected.number}
              aria-controls={panelId}
              tabIndex={milestone.number === selected.number ? 0 : -1}
              className={`step-tab is-${milestone.state}${halted ? " is-halted" : ""}${
                before?.state === "completed" ? " is-joined" : ""
              }`}
              onClick={() => go(position)}
            >
              <span className="step-marker" aria-hidden="true">
                <Marker
                  state={milestone.state}
                  number={milestone.number}
                  halted={halted}
                />
              </span>
              <span className="step-label">{milestone.name}</span>
              <span className="sr-only">
                Etapa {milestone.number}, {stateLabel}
              </span>
            </button>
          );
        })}
      </div>

      <div
        key={selected.number}
        className="stepper-panel"
        role="tabpanel"
        id={panelId}
        aria-labelledby={tabId(selected.number)}
        tabIndex={0}
      >
        <div className="stage-top">
          <p className="stage-eyebrow">
            ETAPA {selected.number} DE {milestones.length}
          </p>
          <Badge
            tone={
              halted && selected.state === "in_progress"
                ? "neutral"
                : milestoneStateTones[selected.state]
            }
          >
            {halted && selected.state === "in_progress"
              ? "Detenida"
              : milestoneStateLabels[selected.state]}
          </Badge>
        </div>
        <h3 className="stage-title">{selected.name}</h3>
        <p className="stage-description">{selected.description}</p>

        {own.length > 0 ? (
          <div className="stage-sections">
            {own.map((section) => (
              <SectionCard key={section.taskNumber} section={section} />
            ))}
          </div>
        ) : (
          <p className="stage-empty">{emptyText(selected.state, halted)}</p>
        )}

        <div className="stage-nav">
          <button
            type="button"
            className="button button-ghost button-sm"
            onClick={() => go(index - 1)}
            disabled={index === 0}
          >
            <Icon name="chevron-left" size={16} /> Etapa anterior
          </button>
          <button
            type="button"
            className="button button-ghost button-sm"
            onClick={() => go(index + 1)}
            disabled={index === milestones.length - 1}
          >
            Etapa siguiente <Icon name="chevron-right" size={16} />
          </button>
        </div>
      </div>
    </div>
  );
}
