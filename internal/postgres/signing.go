package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// EstimateSigning implements sales.SigningStore.
func (s *SalesStore) EstimateSigning(ctx context.Context, admin, estimate resource.ID) (sales.Signing, bool, error) {
	r, err := s.q.GetEstimateSigning(ctx, sqlc.GetEstimateSigningParams{EstimateID: int64(estimate), AdministrationID: int64(admin)})
	if errors.Is(err, pgx.ErrNoRows) {
		return sales.Signing{}, false, nil
	}
	if err != nil {
		return sales.Signing{}, false, err
	}
	return sales.Signing{
		AdministrationID: resource.ID(r.AdministrationID), EstimateID: resource.ID(r.EstimateID), ViewCode: r.ViewCode,
		ViewCodeExpiresAt: timeOf(r.ViewCodeExpiresAt), SentByUserID: idOf(r.SentByUserID), SenderSignature: r.SenderSignature,
		SenderSignedAt: timeOf(r.SenderSignedAt), SenderUserID: idOf(r.SenderUserID), SignerName: r.SignerName, SignerEmail: r.SignerEmail,
		SignerIP: r.SignerIp, SignerUserAgent: r.SignerUserAgent, Signature: r.Signature, AuditTrail: r.AuditTrail, SignedAt: timeOf(r.SignedAt),
		UpdatedAt: r.UpdatedAt.Time,
	}, true, nil
}

// SaveSigning implements sales.SigningStore.
func (s *SalesStore) SaveSigning(ctx context.Context, g sales.Signing) error {
	return s.q.UpsertEstimateSigning(ctx, sqlc.UpsertEstimateSigningParams{
		EstimateID: int64(g.EstimateID), AdministrationID: int64(g.AdministrationID), ViewCode: g.ViewCode,
		ViewCodeExpiresAt: optionalTime(g.ViewCodeExpiresAt), SentByUserID: optionalID(g.SentByUserID), SenderSignature: g.SenderSignature,
		SenderSignedAt: optionalTime(g.SenderSignedAt), SenderUserID: optionalID(g.SenderUserID), SignerName: g.SignerName, SignerEmail: g.SignerEmail,
		SignerIp: g.SignerIP, SignerUserAgent: g.SignerUserAgent, Signature: g.Signature, AuditTrail: g.AuditTrail, SignedAt: optionalTime(g.SignedAt),
		UpdatedAt: timestamp(g.UpdatedAt),
	})
}
