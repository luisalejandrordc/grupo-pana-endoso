import { Icon, Modal } from "@endosoflow/ui";
import { useCallback, useEffect, useState } from "react";
import { PhotoThumbs } from "./PhotoThumbs";
import { useSwipe, type Photo } from "./photos";

const ZOOMS = [1, 1.5, 2, 3, 4];

/**
 * A photo as large as the screen allows, with the other views one tap away, arrows (and the keyboard's) to walk
 * through them and zoom in and out. Zoomed in, the photo scrolls (or is dragged, on a phone) inside its frame.
 */
export function Lightbox({
  title,
  photos,
  index,
  onIndex,
  onClose,
}: {
  title: string;
  photos: Photo[];
  index: number;
  onIndex: (index: number) => void;
  onClose: () => void;
}) {
  const [step, setStep] = useState(0);
  const zoom = ZOOMS[step];
  const photo = photos[index];
  const many = photos.length > 1;

  const show = useCallback(
    (next: number) => {
      onIndex((next + photos.length) % photos.length);
      setStep(0);
    },
    [onIndex, photos.length],
  );
  const zoomBy = (delta: number) =>
    setStep((at) => Math.max(0, Math.min(ZOOMS.length - 1, at + delta)));

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.key === "ArrowLeft" && many) show(index - 1);
      else if (event.key === "ArrowRight" && many) show(index + 1);
      else if (event.key === "+" || event.key === "=") zoomBy(1);
      else if (event.key === "-") zoomBy(-1);
      else if (event.key === "0") setStep(0);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [index, many, show]);

  // A swipe changes the photo only while it is not zoomed: zoomed in, the same gesture pans.
  const swipe = useSwipe(
    () => show(index - 1),
    () => show(index + 1),
  );

  return (
    <Modal
      immersive
      title={title}
      description={`${photo.label}${many ? ` · ${index + 1} de ${photos.length}` : ""}`}
      onClose={onClose}
    >
      <div className="lightbox">
        <div className="lightbox-frame">
          <div
            className={`lightbox-stage${zoom > 1 ? " is-zoomed" : ""}`}
            {...(zoom === 1 ? swipe : {})}
          >
            <img
              key={photo.url}
              src={photo.url}
              alt={`${title}, ${photo.label}`}
              draggable={false}
              style={
                zoom === 1
                  ? undefined
                  : {
                      width: `${zoom * 100}%`,
                      maxWidth: "none",
                      maxHeight: "none",
                    }
              }
              onDoubleClick={() => setStep((at) => (at === 0 ? 2 : 0))}
            />
          </div>
          {many && (
            <>
              <button
                type="button"
                className="photo-arrow is-prev"
                aria-label="Foto anterior"
                onClick={() => show(index - 1)}
              >
                <Icon name="chevron-left" size={22} />
              </button>
              <button
                type="button"
                className="photo-arrow is-next"
                aria-label="Foto siguiente"
                onClick={() => show(index + 1)}
              >
                <Icon name="chevron-right" size={22} />
              </button>
            </>
          )}
        </div>

        <div className="lightbox-bar">
          <div className="lightbox-zoom" role="group" aria-label="Zoom">
            <button
              type="button"
              className="icon-button"
              aria-label="Alejar"
              onClick={() => zoomBy(-1)}
              disabled={step === 0}
            >
              <Icon name="zoom-out" size={18} />
            </button>
            <button
              type="button"
              className="lightbox-level"
              aria-label="Restablecer zoom"
              onClick={() => setStep(0)}
              disabled={step === 0}
            >
              {Math.round(zoom * 100)}%
            </button>
            <button
              type="button"
              className="icon-button"
              aria-label="Acercar"
              onClick={() => zoomBy(1)}
              disabled={step === ZOOMS.length - 1}
            >
              <Icon name="zoom-in" size={18} />
            </button>
          </div>
          {many && (
            <PhotoThumbs
              photos={photos}
              current={index}
              onSelect={show}
              labelled
            />
          )}
        </div>
      </div>
    </Modal>
  );
}
