import { Icon, type IconName } from "../../../shared/ui/Icon";

export function TimelineItem({
  icon,
  title,
  text,
  date,
  actor,
  tone,
}: {
  icon: IconName;
  title: string;
  text: string;
  date: string;
  actor: string;
  tone: string;
}) {
  return (
    <div className="timeline-item">
      <div className={`timeline-node ${tone}`}>
        <Icon name={icon} size={15} />
      </div>
      <div className="timeline-content">
        <div className="timeline-title-row">
          <strong>{title}</strong>
          <span>{date}</span>
        </div>
        <p>{text}</p>
        <small>Por {actor}</small>
      </div>
    </div>
  );
}
