package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// InvoicingStore implements invoicing.Store.
type InvoicingStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewInvoicingStore returns an InvoicingStore on pool.
func NewInvoicingStore(pool *pgxpool.Pool) *InvoicingStore {
	return &InvoicingStore{pool: pool, q: sqlc.New(pool)}
}

// Settings implements invoicing.Store.
func (s *InvoicingStore) Settings(ctx context.Context, admin resource.ID) (invoicing.Settings, error) {
	r, err := s.q.InvoiceSettings(ctx, int64(admin))
	if err != nil {
		return invoicing.Settings{}, err
	}
	return invoicing.Settings{
		InvoiceIDTemplate: r.InvoiceIDTemplate, InvoiceSequenceID: r.InvoiceSequenceID,
		EstimateIDTemplate: r.EstimateIDTemplate, EstimateSequenceID: r.EstimateSequenceID,
		InvoiceBCC: r.InvoiceBccEmailAddress, InvoiceRemindersBCC: r.InvoiceRemindersBccEnabled,
		EstimateBCC: r.EstimateBccEmailAddress, OrderBCC: r.OrderBccEmailAddress,
		DefaultEmailUBL: r.DefaultEmailUbl, PaymentsWithoutProof: r.PaymentsWithoutProof,
	}, nil
}

// SaveSettings implements invoicing.Store.
func (s *InvoicingStore) SaveSettings(ctx context.Context, admin resource.ID, st invoicing.Settings) error {
	return s.q.UpdateInvoiceSettings(ctx, sqlc.UpdateInvoiceSettingsParams{
		ID: int64(admin), InvoiceIDTemplate: st.InvoiceIDTemplate, InvoiceSequenceID: st.InvoiceSequenceID,
		EstimateIDTemplate: st.EstimateIDTemplate, EstimateSequenceID: st.EstimateSequenceID,
		InvoiceBccEmailAddress: st.InvoiceBCC, InvoiceRemindersBccEnabled: st.InvoiceRemindersBCC,
		EstimateBccEmailAddress: st.EstimateBCC, OrderBccEmailAddress: st.OrderBCC,
		DefaultEmailUbl: st.DefaultEmailUBL, PaymentsWithoutProof: st.PaymentsWithoutProof,
	})
}

// Workflows implements invoicing.Store.
func (s *InvoicingStore) Workflows(ctx context.Context, admin resource.ID) ([]invoicing.Workflow, error) {
	rows, err := s.q.ListWorkflows(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	steps, err := s.steps(ctx, admin)
	if err != nil {
		return nil, err
	}
	out := make([]invoicing.Workflow, len(rows))
	for i, r := range rows {
		out[i] = toWorkflow(r, steps[r.ID])
	}
	return out, nil
}

// Workflow implements invoicing.Store.
func (s *InvoicingStore) Workflow(ctx context.Context, admin, id resource.ID) (invoicing.Workflow, error) {
	r, err := s.q.WorkflowByID(ctx, sqlc.WorkflowByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return invoicing.Workflow{}, invoicing.ErrWorkflowNotFound
	}
	if err != nil {
		return invoicing.Workflow{}, err
	}
	steps, err := s.steps(ctx, admin)
	if err != nil {
		return invoicing.Workflow{}, err
	}
	return toWorkflow(r, steps[r.ID]), nil
}

func (s *InvoicingStore) steps(ctx context.Context, admin resource.ID) (map[int64][]invoicing.WorkflowStep, error) {
	rows, err := s.q.ListWorkflowSteps(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := map[int64][]invoicing.WorkflowStep{}
	for _, r := range rows {
		out[r.WorkflowID] = append(out[r.WorkflowID], invoicing.WorkflowStep{
			ID: resource.ID(r.ID), StepOrder: int(r.StepOrder), DueInterval: int(r.DueInterval),
			ReminderDelay: int(r.ReminderDelay), ReminderText: r.ReminderText, ReminderAutoSend: r.ReminderAutoSend,
			CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		})
	}
	return out, nil
}

func toWorkflow(r sqlc.Workflow, steps []invoicing.WorkflowStep) invoicing.Workflow {
	return invoicing.Workflow{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Type: invoicing.WorkflowType(r.Type),
		Name: r.Name, Default: r.IsDefault, Currency: r.Currency, Language: r.Language, Active: r.Active,
		PricesAreInclTax: r.PricesAreInclTax, ShowQRCode: r.ShowQrCode, Steps: steps,
		InvoiceEmailSubject: r.InvoiceEmailSubject, InvoiceEmailBody: r.InvoiceEmailBody,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// CreateWorkflow implements invoicing.Store.
func (s *InvoicingStore) CreateWorkflow(ctx context.Context, w invoicing.Workflow) (bool, error) {
	created := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		_, err := q.InsertWorkflow(ctx, sqlc.InsertWorkflowParams{
			ID: int64(w.ID), AdministrationID: int64(w.AdministrationID), Type: string(w.Type), Name: w.Name,
			IsDefault: w.Default, Currency: w.Currency, Language: w.Language, Active: w.Active,
			PricesAreInclTax: w.PricesAreInclTax, ShowQrCode: w.ShowQRCode,
			InvoiceEmailSubject: w.InvoiceEmailSubject, InvoiceEmailBody: w.InvoiceEmailBody,
			CreatedAt: timestamp(w.CreatedAt), UpdatedAt: timestamp(w.UpdatedAt),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // A default of this type exists already.
		}
		if err != nil {
			return err
		}
		created = true
		if err := insertSteps(ctx, q, w); err != nil {
			return err
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{
			ID: int64(w.ID), Type: string(resource.TypeWorkflow), AdministrationID: int64(w.AdministrationID),
		})
	})
	return created, err
}

// UpdateWorkflow implements invoicing.Store.
func (s *InvoicingStore) UpdateWorkflow(ctx context.Context, w invoicing.Workflow) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if w.Default {
			if err := q.ClearDefaultWorkflow(ctx, sqlc.ClearDefaultWorkflowParams{
				AdministrationID: int64(w.AdministrationID), Type: string(w.Type), ID: int64(w.ID),
			}); err != nil {
				return err
			}
		}
		if err := q.UpdateWorkflow(ctx, sqlc.UpdateWorkflowParams{
			AdministrationID: int64(w.AdministrationID), ID: int64(w.ID), Name: w.Name, IsDefault: w.Default,
			Currency: w.Currency, Language: w.Language, Active: w.Active, PricesAreInclTax: w.PricesAreInclTax,
			ShowQrCode: w.ShowQRCode, UpdatedAt: timestamp(w.UpdatedAt),
			InvoiceEmailSubject: w.InvoiceEmailSubject, InvoiceEmailBody: w.InvoiceEmailBody,
		}); err != nil {
			return err
		}
		if err := q.DeleteWorkflowSteps(ctx, int64(w.ID)); err != nil {
			return err
		}
		return insertSteps(ctx, q, w)
	})
}

func insertSteps(ctx context.Context, q *sqlc.Queries, w invoicing.Workflow) error {
	for _, st := range w.Steps {
		if err := q.InsertWorkflowStep(ctx, sqlc.InsertWorkflowStepParams{
			ID: int64(st.ID), WorkflowID: int64(w.ID), StepOrder: int32(st.StepOrder), DueInterval: int32(st.DueInterval),
			ReminderDelay: int32(st.ReminderDelay), ReminderText: st.ReminderText, ReminderAutoSend: st.ReminderAutoSend,
			CreatedAt: timestamp(st.CreatedAt), UpdatedAt: timestamp(st.UpdatedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteWorkflow implements invoicing.Store.
func (s *InvoicingStore) DeleteWorkflow(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.DeleteWorkflow(ctx, sqlc.DeleteWorkflowParams{AdministrationID: int64(admin), ID: int64(id)})
		if err != nil {
			return err
		}
		if n == 0 {
			return invoicing.ErrWorkflowNotFound
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

func fileColumns(f *invoicing.StoredFile) (hash, contentType pgtype.Text) {
	if f == nil {
		return pgtype.Text{}, pgtype.Text{}
	}
	return pgtype.Text{String: f.Hash, Valid: true}, pgtype.Text{String: f.ContentType, Valid: true}
}

func storedFile(hash, contentType pgtype.Text) *invoicing.StoredFile {
	if !hash.Valid {
		return nil
	}
	return &invoicing.StoredFile{Hash: hash.String, ContentType: contentType.String}
}

func toDocumentStyle(r sqlc.DocumentStyle) (invoicing.DocumentStyle, error) {
	l, err := decodeLayout(r.Layout)
	if err != nil {
		return invoicing.DocumentStyle{}, err
	}
	d := invoicing.DocumentStyle{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Name: r.Name,
		IdentityID: resource.ID(r.IdentityID), Default: r.IsDefault, Logo: storedFile(r.LogoHash, r.LogoContentType),
		LogoContainerFullWidth: r.LogoContainerFullWidth, LogoDisplayWidth: int(r.LogoDisplayWidth),
		LogoPosition: r.LogoPosition, Background: storedFile(r.BackgroundHash, r.BackgroundContentType),
		PrintOnStationery: r.PrintOnStationery, PaperSize: r.PaperSize, AddressPosition: r.AddressPosition,
		FontSize: int(r.FontSize), FontFamily: r.FontFamily, CustomCSS: r.CustomCss, Layout: l,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if err := decodeChannels(r.Channels, &d); err != nil {
		return invoicing.DocumentStyle{}, err
	}
	if err := decodeFiles(r.Files, &d); err != nil {
		return invoicing.DocumentStyle{}, err
	}
	return d, nil
}

// styleColumns encodes the JSON columns of a document style.
func styleColumns(d invoicing.DocumentStyle) (layout, channels, files []byte, err error) {
	if layout, err = encodeLayout(d.Layout); err != nil {
		return nil, nil, nil, err
	}
	if channels, err = encodeChannels(d); err != nil {
		return nil, nil, nil, err
	}
	files, err = encodeFiles(d)
	return layout, channels, files, err
}

// DocumentStyles implements invoicing.Store.
func (s *InvoicingStore) DocumentStyles(ctx context.Context, admin resource.ID) ([]invoicing.DocumentStyle, error) {
	rows, err := s.q.ListDocumentStyles(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]invoicing.DocumentStyle, len(rows))
	for i, r := range rows {
		if out[i], err = toDocumentStyle(r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// DocumentStyle implements invoicing.Store.
func (s *InvoicingStore) DocumentStyle(ctx context.Context, admin, id resource.ID) (invoicing.DocumentStyle, error) {
	r, err := s.q.DocumentStyleByID(ctx, sqlc.DocumentStyleByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return invoicing.DocumentStyle{}, invoicing.ErrDocumentStyleNotFound
	}
	if err != nil {
		return invoicing.DocumentStyle{}, err
	}
	return toDocumentStyle(r)
}

// CreateDocumentStyle implements invoicing.Store.
func (s *InvoicingStore) CreateDocumentStyle(ctx context.Context, d invoicing.DocumentStyle) (bool, error) {
	raw, channels, files, err := styleColumns(d)
	if err != nil {
		return false, err
	}
	logoHash, logoType := fileColumns(d.Logo)
	bgHash, bgType := fileColumns(d.Background)
	created := false
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		_, err := q.InsertDocumentStyle(ctx, sqlc.InsertDocumentStyleParams{
			ID: int64(d.ID), AdministrationID: int64(d.AdministrationID), Name: d.Name, IdentityID: int64(d.IdentityID),
			IsDefault: d.Default, LogoHash: logoHash, LogoContentType: logoType,
			LogoContainerFullWidth: d.LogoContainerFullWidth, LogoDisplayWidth: int32(d.LogoDisplayWidth),
			LogoPosition: d.LogoPosition, BackgroundHash: bgHash, BackgroundContentType: bgType,
			PrintOnStationery: d.PrintOnStationery, PaperSize: d.PaperSize, AddressPosition: d.AddressPosition,
			FontSize: int32(d.FontSize), FontFamily: d.FontFamily, CustomCss: d.CustomCSS, Layout: raw,
			CreatedAt: timestamp(d.CreatedAt), UpdatedAt: timestamp(d.UpdatedAt), Channels: channels, Files: files,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // The administration has a default already.
		}
		if err != nil {
			return err
		}
		created = true
		return q.InsertResource(ctx, sqlc.InsertResourceParams{
			ID: int64(d.ID), Type: string(resource.TypeDocumentStyle), AdministrationID: int64(d.AdministrationID),
		})
	})
	return created, err
}

// UpdateDocumentStyle implements invoicing.Store.
func (s *InvoicingStore) UpdateDocumentStyle(ctx context.Context, d invoicing.DocumentStyle) error {
	raw, channels, files, err := styleColumns(d)
	if err != nil {
		return err
	}
	logoHash, logoType := fileColumns(d.Logo)
	bgHash, bgType := fileColumns(d.Background)
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if d.Default {
			if err := q.ClearDefaultDocumentStyle(ctx, sqlc.ClearDefaultDocumentStyleParams{
				AdministrationID: int64(d.AdministrationID), ID: int64(d.ID),
			}); err != nil {
				return err
			}
		}
		return q.UpdateDocumentStyle(ctx, sqlc.UpdateDocumentStyleParams{
			AdministrationID: int64(d.AdministrationID), ID: int64(d.ID), Name: d.Name, IdentityID: int64(d.IdentityID),
			IsDefault: d.Default, LogoHash: logoHash, LogoContentType: logoType,
			LogoContainerFullWidth: d.LogoContainerFullWidth, LogoDisplayWidth: int32(d.LogoDisplayWidth),
			LogoPosition: d.LogoPosition, BackgroundHash: bgHash, BackgroundContentType: bgType,
			PrintOnStationery: d.PrintOnStationery, PaperSize: d.PaperSize, AddressPosition: d.AddressPosition,
			FontSize: int32(d.FontSize), FontFamily: d.FontFamily, CustomCss: d.CustomCSS, Layout: raw,
			UpdatedAt: timestamp(d.UpdatedAt), Channels: channels, Files: files,
		})
	})
}

// DeleteDocumentStyle implements invoicing.Store.
func (s *InvoicingStore) DeleteDocumentStyle(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.DeleteDocumentStyle(ctx, sqlc.DeleteDocumentStyleParams{AdministrationID: int64(admin), ID: int64(id)})
		if err != nil {
			return err
		}
		if n == 0 {
			return invoicing.ErrDocumentStyleNotFound
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// FileInUse implements invoicing.Store.
func (s *InvoicingStore) FileInUse(ctx context.Context, admin resource.ID, hash string) (bool, error) {
	return s.q.DocumentFileInUse(ctx, sqlc.DocumentFileInUseParams{AdministrationID: int64(admin), Hash: hash})
}
