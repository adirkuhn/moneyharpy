package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/mbimport"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// MoneybirdImportStore implements mbimport.Store.
type MoneybirdImportStore struct {
	q *sqlc.Queries
}

// NewMoneybirdImportStore returns a MoneybirdImportStore on pool.
func NewMoneybirdImportStore(pool *pgxpool.Pool) *MoneybirdImportStore {
	return &MoneybirdImportStore{q: sqlc.New(pool)}
}

func toMoneybirdImport(r sqlc.MoneybirdImport) (mbimport.Job, error) {
	j := mbimport.Job{
		AdministrationID: resource.ID(r.AdministrationID), UserID: resource.ID(r.UserID), SourceID: r.SourceAdministrationID,
		SourceName: r.SourceName, State: mbimport.State(r.State), WaitingUntil: timeOf(r.WaitingUntil), Error: r.Error, Token: r.Token,
		Attempts: int(r.Attempts), NextAttemptAt: timeOf(r.NextAttemptAt), StartedAt: r.StartedAt.Time, FinishedAt: timeOf(r.FinishedAt),
		UpdatedAt: r.UpdatedAt.Time,
	}
	if err := json.Unmarshal(r.Steps, &j.Steps); err != nil {
		return mbimport.Job{}, fmt.Errorf("decode import steps: %w", err)
	}
	return j, nil
}

// Job implements mbimport.Store.
func (s *MoneybirdImportStore) Job(ctx context.Context, admin resource.ID) (mbimport.Job, error) {
	r, err := s.q.MoneybirdImport(ctx, int64(admin))
	if errors.Is(err, pgx.ErrNoRows) {
		return mbimport.Job{}, mbimport.ErrNotFound
	}
	if err != nil {
		return mbimport.Job{}, err
	}
	return toMoneybirdImport(r)
}

// SaveJob implements mbimport.Store.
func (s *MoneybirdImportStore) SaveJob(ctx context.Context, j mbimport.Job) error {
	steps := j.Steps
	if steps == nil {
		steps = []mbimport.Step{}
	}
	raw, err := json.Marshal(steps)
	if err != nil {
		return fmt.Errorf("encode import steps: %w", err)
	}
	return s.q.SaveMoneybirdImport(ctx, sqlc.SaveMoneybirdImportParams{
		AdministrationID: int64(j.AdministrationID), UserID: int64(j.UserID), SourceAdministrationID: j.SourceID, SourceName: j.SourceName,
		State: string(j.State), Steps: raw, WaitingUntil: optionalTime(j.WaitingUntil), Error: j.Error, Token: j.Token,
		Attempts: int32(j.Attempts), NextAttemptAt: optionalTime(j.NextAttemptAt), StartedAt: timestamp(j.StartedAt),
		FinishedAt: optionalTime(j.FinishedAt), UpdatedAt: timestamp(j.UpdatedAt),
	})
}

// LeaseDueJobs implements mbimport.Store.
func (s *MoneybirdImportStore) LeaseDueJobs(ctx context.Context, now, lease time.Time, max int) ([]mbimport.Job, error) {
	rows, err := s.q.LeaseDueMoneybirdImports(ctx, sqlc.LeaseDueMoneybirdImportsParams{Lease: timestamp(lease), Now: timestamp(now), MaxCount: int32(max)})
	if err != nil {
		return nil, err
	}
	out := make([]mbimport.Job, 0, len(rows))
	for _, r := range rows {
		j, err := toMoneybirdImport(r)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

// Imported implements mbimport.Store.
func (s *MoneybirdImportStore) Imported(ctx context.Context, admin resource.ID, kind string, source resource.ID) (resource.ID, bool, error) {
	target, err := s.q.MoneybirdImportedID(ctx, sqlc.MoneybirdImportedIDParams{AdministrationID: int64(admin), Kind: kind, SourceID: int64(source)})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return resource.ID(target), true, nil
}

// RecordImported implements mbimport.Store.
func (s *MoneybirdImportStore) RecordImported(ctx context.Context, admin resource.ID, kind string, source, target resource.ID, at time.Time) error {
	return s.q.RecordMoneybirdImportedID(ctx, sqlc.RecordMoneybirdImportedIDParams{
		AdministrationID: int64(admin), Kind: kind, SourceID: int64(source), TargetID: int64(target), ImportedAt: timestamp(at),
	})
}

// ImportedAt implements mbimport.Store.
func (s *MoneybirdImportStore) ImportedAt(ctx context.Context, admin resource.ID, kind string, source resource.ID) (time.Time, bool, error) {
	at, err := s.q.MoneybirdImportedAt(ctx, sqlc.MoneybirdImportedAtParams{AdministrationID: int64(admin), Kind: kind, SourceID: int64(source)})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return at.Time, true, nil
}

// Empty implements mbimport.Store.
func (s *MoneybirdImportStore) Empty(ctx context.Context, admin resource.ID) (bool, error) {
	has, err := s.q.AdministrationHasData(ctx, int64(admin))
	if err != nil {
		return false, err
	}
	return !has.Bool, nil
}

// RecordProblem implements mbimport.Store.
func (s *MoneybirdImportStore) RecordProblem(ctx context.Context, admin resource.ID, p mbimport.Problem) error {
	return s.q.RecordMoneybirdImportProblem(ctx, sqlc.RecordMoneybirdImportProblemParams{
		AdministrationID: int64(admin), Step: p.Step, Kind: p.Kind, SourceID: int64(p.SourceID), Message: p.Message, Partial: p.Partial,
		CreatedAt: timestamp(p.CreatedAt),
	})
}

// ResolveProblems implements mbimport.Store.
func (s *MoneybirdImportStore) ResolveProblems(ctx context.Context, admin resource.ID, kind string, source resource.ID) error {
	return s.q.ResolveMoneybirdImportProblems(ctx, sqlc.ResolveMoneybirdImportProblemsParams{AdministrationID: int64(admin), Kind: kind, SourceID: int64(source)})
}

// Partials implements mbimport.Store.
func (s *MoneybirdImportStore) Partials(ctx context.Context, admin resource.ID, kind string) (map[resource.ID][]string, error) {
	rows, err := s.q.MoneybirdImportPartials(ctx, sqlc.MoneybirdImportPartialsParams{AdministrationID: int64(admin), Kind: kind})
	if err != nil {
		return nil, err
	}
	out := map[resource.ID][]string{}
	for _, r := range rows {
		out[resource.ID(r.SourceID)] = append(out[resource.ID(r.SourceID)], r.Message)
	}
	return out, nil
}

// ForgetProblem implements mbimport.Store.
func (s *MoneybirdImportStore) ForgetProblem(ctx context.Context, admin resource.ID, kind string, source resource.ID, message string) error {
	return s.q.ForgetMoneybirdImportProblem(ctx, sqlc.ForgetMoneybirdImportProblemParams{AdministrationID: int64(admin), Kind: kind, SourceID: int64(source), Message: message})
}

// Problems implements mbimport.Store.
func (s *MoneybirdImportStore) Problems(ctx context.Context, admin resource.ID, page, perPage int) ([]mbimport.Problem, int, error) {
	total, err := s.q.CountMoneybirdImportProblems(ctx, int64(admin))
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.MoneybirdImportProblems(ctx, sqlc.MoneybirdImportProblemsParams{AdministrationID: int64(admin), Limit: int32(perPage), Offset: int32((page - 1) * perPage)})
	if err != nil {
		return nil, 0, err
	}
	out := make([]mbimport.Problem, len(rows))
	for i, r := range rows {
		out[i] = mbimport.Problem{Step: r.Step, Kind: r.Kind, SourceID: resource.ID(r.SourceID), Message: r.Message, Partial: r.Partial, CreatedAt: r.CreatedAt.Time}
	}
	return out, int(total), nil
}
