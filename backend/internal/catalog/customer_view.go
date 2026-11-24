package catalog

import (
	"context"

	"github.com/google/uuid"
)

// How a customer sees the vehicle of their purchase. Catalog images live in a public bucket by design (they are
// product photos), so the portal gets their plain public addresses: no signing, no expiry.

// PresentationImage is one photo of the vehicle.
type PresentationImage struct {
	Viewpoint string `json:"viewpoint"`
	URL       string `json:"url"`
}

// VehiclePresentation is the part of the catalog a customer sees: the model's categories and the photos of the
// color they chose.
type VehiclePresentation struct {
	Categories   []string            `json:"categories"`
	MainImageURL string              `json:"mainImageUrl"`
	Images       []PresentationImage `json:"images"`
}

// Presentation reads the categories of a model and the photos of a color. Archived models and colors still have
// theirs: a sale keeps the vehicle it was made for. Missing photos are an empty list, never an error.
func (s *Service) Presentation(ctx context.Context, modelID, colorID uuid.UUID) (*VehiclePresentation, error) {
	categories, err := modelCategories(ctx, s.db, modelID)
	if err != nil {
		return nil, err
	}
	images, err := colorImages(ctx, s.db, []uuid.UUID{colorID})
	if err != nil {
		return nil, err
	}
	out := &VehiclePresentation{Categories: categories, Images: []PresentationImage{}}
	if out.Categories == nil {
		out.Categories = []string{}
	}
	for _, image := range images[colorID] {
		url := s.store.PublicURL(image.Key)
		out.Images = append(out.Images, PresentationImage{Viewpoint: image.Viewpoint, URL: url})
		if image.Viewpoint == MainView {
			out.MainImageURL = url
		}
	}
	return out, nil
}
