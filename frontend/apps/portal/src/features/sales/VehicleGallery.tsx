import { Icon } from "@endosoflow/ui";
import { useState, type KeyboardEvent } from "react";
import type { PortalVehicle } from "../../shared/types";
import { Lightbox } from "./Lightbox";
import { PhotoThumbs } from "./PhotoThumbs";
import { photosOf, useSwipe } from "./photos";
import { VehicleImage } from "./VehicleImage";

/**
 * The vehicle's photos. It opens on the main view; arrows, swipes, the thumbnails and the keyboard walk through
 * the other views, and pressing the photo opens it large (with zoom) in a viewer.
 */
export function VehicleGallery({ vehicle }: { vehicle: PortalVehicle }) {
  const name = `${vehicle.model} ${vehicle.version}, color ${vehicle.color}`;
  const photos = photosOf(vehicle);
  const [index, setIndex] = useState(0);
  const [open, setOpen] = useState(false);
  const many = photos.length > 1;

  const show = (next: number) =>
    setIndex((next + photos.length) % photos.length);
  const swipe = useSwipe(
    () => show(index - 1),
    () => show(index + 1),
  );

  if (photos.length === 0)
    return (
      <div className="gallery">
        <VehicleImage className="gallery-main" src="" alt={name} />
      </div>
    );

  const photo = photos[index];

  function onKeyDown(event: KeyboardEvent) {
    // While the viewer is open it answers the keyboard itself.
    if (!many || open) return;
    if (event.key === "ArrowLeft") show(index - 1);
    else if (event.key === "ArrowRight") show(index + 1);
  }

  return (
    <>
      <div className="gallery" onKeyDown={onKeyDown}>
        <div className="gallery-stage" {...swipe}>
          <button
            type="button"
            className="gallery-open"
            aria-label={`Ampliar foto: ${photo.label}`}
            onClick={() => setOpen(true)}
          >
            <VehicleImage
              key={photo.url}
              className="gallery-main"
              src={photo.url}
              alt={`${name}, ${photo.label}`}
            />
            <span className="gallery-expand" aria-hidden="true">
              <Icon name="maximize" size={16} />
            </span>
          </button>
          {many && (
            <>
              <button
                type="button"
                className="photo-arrow is-prev"
                aria-label="Foto anterior"
                onClick={() => show(index - 1)}
              >
                <Icon name="chevron-left" size={20} />
              </button>
              <button
                type="button"
                className="photo-arrow is-next"
                aria-label="Foto siguiente"
                onClick={() => show(index + 1)}
              >
                <Icon name="chevron-right" size={20} />
              </button>
            </>
          )}
          <p className="gallery-caption" aria-live="polite">
            {photo.label}
            {many && (
              <span>
                {index + 1} / {photos.length}
              </span>
            )}
          </p>
        </div>
        {many && (
          <PhotoThumbs photos={photos} current={index} onSelect={setIndex} />
        )}
      </div>
      {open && (
        <Lightbox
          title={`${vehicle.model} ${vehicle.version}`}
          photos={photos}
          index={index}
          onIndex={setIndex}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}
