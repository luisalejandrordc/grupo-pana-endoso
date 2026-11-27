import type { Photo } from "./photos";

/** One small button per photo; the one on screen is marked. */
export function PhotoThumbs({
  photos,
  current,
  onSelect,
  labelled = false,
}: {
  photos: Photo[];
  current: number;
  onSelect: (index: number) => void;
  /** Shows the name of each view under its thumbnail (the large viewer does). */
  labelled?: boolean;
}) {
  return (
    <ul className="gallery-thumbs" aria-label="Vistas del vehículo">
      {photos.map((photo, index) => (
        <li key={photo.viewpoint}>
          <button
            type="button"
            className={`gallery-thumb ${index === current ? "is-active" : ""}`.trim()}
            aria-pressed={index === current}
            aria-label={photo.label}
            title={photo.label}
            onClick={() => onSelect(index)}
          >
            <img src={photo.url} alt="" loading="lazy" />
            {labelled && (
              <span className="gallery-thumb-label">{photo.label}</span>
            )}
          </button>
        </li>
      ))}
    </ul>
  );
}
