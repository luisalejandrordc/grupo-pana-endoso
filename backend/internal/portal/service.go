package portal

import (
	"context"
	"endosoflow/internal/auth"
	"endosoflow/internal/catalog"
	"endosoflow/internal/documents"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/tasks"
	"log/slog"

	"github.com/google/uuid"
)

var (
	errSaleNotFound     = apperr.NotFound("SALE_NOT_FOUND", "La compra no existe.")
	errDocumentNotFound = apperr.NotFound("DOCUMENT_NOT_FOUND", "El documento no existe.")
)

// querier is what the portal reads with. auth.Database and tasks.Querier are satisfied by the same pool.
type querier interface {
	tasks.Querier
}

// VehicleSource gives the categories and photos of a vehicle (the catalog implements it).
type VehicleSource interface {
	Presentation(ctx context.Context, modelID, colorID uuid.UUID) (*catalog.VehiclePresentation, error)
}

// DocumentSource signs the download of a file a customer may have (the documents module implements it).
type DocumentSource interface {
	CustomerDownloadURL(ctx context.Context, customerID, documentID uuid.UUID) (*documents.DownloadURL, error)
}

// Deps are the service's dependencies.
type Deps struct {
	DB        auth.Database
	Vehicles  VehicleSource
	Documents DocumentSource
	Logger    *slog.Logger
}

// Service implements the customer portal's read use cases.
type Service struct {
	db        auth.Database
	vehicles  VehicleSource
	documents DocumentSource
	logger    *slog.Logger
}

// NewService builds the service.
func NewService(d Deps) *Service {
	return &Service{db: d.DB, vehicles: d.Vehicles, documents: d.Documents, logger: d.Logger}
}

// Sales lists the purchases of the customer, newest first, each with its six milestones.
func (s *Service) Sales(ctx context.Context, customerID uuid.UUID) (*SaleList, error) {
	rows, err := customerSales(ctx, s.db, customerID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	milestones, err := milestonesOf(ctx, s.db, ids)
	if err != nil {
		return nil, err
	}
	out := &SaleList{Items: []SaleSummary{}}
	for i := range rows {
		r := &rows[i]
		view, err := s.vehicles.Presentation(ctx, r.ModelID, r.ColorID)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, SaleSummary{
			ID: r.ID, Code: SaleCode(r.CreatedAt, r.Number), State: r.State, CreatedAt: r.CreatedAt,
			Vehicle:    VehicleSummary{Model: r.Model, Version: r.Version, Color: r.Color, MainImageURL: view.MainImageURL},
			Milestones: milestones[r.ID],
		})
	}
	return out, nil
}

// Sale returns one purchase of the customer in full. Anyone else's purchase, and one that does not exist, both
// answer SALE_NOT_FOUND.
func (s *Service) Sale(ctx context.Context, customerID, saleID uuid.UUID) (*SaleDetail, error) {
	r, err := customerSale(ctx, s.db, customerID, saleID)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, errSaleNotFound
	}
	milestones, err := milestonesOf(ctx, s.db, []uuid.UUID{r.ID})
	if err != nil {
		return nil, err
	}
	view, err := s.vehicles.Presentation(ctx, r.ModelID, r.ColorID)
	if err != nil {
		return nil, err
	}
	sections, err := tasks.CustomerView(ctx, s.db, r.ID)
	if err != nil {
		return nil, err
	}
	files, err := documents.CustomerDocuments(ctx, s.db, r.ID)
	if err != nil {
		return nil, err
	}

	// A task shows up only if it has something for the customer to read or download.
	byTask := map[int][]documents.CustomerDocument{}
	for _, f := range files {
		byTask[f.TaskNumber] = append(byTask[f.TaskNumber], f)
	}
	detail := &SaleDetail{
		ID: r.ID, Code: SaleCode(r.CreatedAt, r.Number), State: r.State, CreatedAt: r.CreatedAt, CompletedAt: r.CompletedAt,
		Vehicle: Vehicle{
			Model: r.Model, Version: r.Version, Color: r.Color, ReferencePrice: r.ReferencePrice,
			Categories: view.Categories, MainImageURL: view.MainImageURL, Images: view.Images,
		},
		Advisor:    r.Advisor,
		Milestones: milestones[r.ID],
		Sections:   []Section{},
	}
	for _, sec := range sections {
		docs := byTask[sec.TaskNumber]
		if len(sec.Fields) == 0 && len(docs) == 0 {
			continue
		}
		if docs == nil {
			docs = []documents.CustomerDocument{}
		}
		detail.Sections = append(detail.Sections, Section{
			TaskNumber: sec.TaskNumber, MilestoneNumber: sec.MilestoneNumber, Title: sec.Title,
			CompletedAt: sec.CompletedAt, Fields: sec.Fields, Documents: docs,
		})
	}
	return detail, nil
}

// DownloadURL issues a short-lived address for one of the customer's files.
func (s *Service) DownloadURL(ctx context.Context, customerID, documentID uuid.UUID) (*documents.DownloadURL, error) {
	return s.documents.CustomerDownloadURL(ctx, customerID, documentID)
}
