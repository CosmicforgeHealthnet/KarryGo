package walletrepositories

// Section 17: rebinding a held payment's beneficiary after provider
// replacement.
//
// This touches two columns on the intent -- the provider and the binding
// version -- and writes one audit row. It posts no ledger entry: while a job
// is held its money sits in the job's escrow account, not with any provider,
// so there is nothing to move. The money moves once, at completion, to
// whoever is bound then.

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

var (
	// ErrRebindKeyReused: the idempotency key was already used for a
	// different request.
	ErrRebindKeyReused = errors.New("rebind idempotency key reused with a different request")
	// ErrRebindNotHeld: the payment is not held, so its beneficiary is no
	// longer (or not yet) a question a rebind can answer.
	ErrRebindNotHeld = errors.New("payment is not held")
	// ErrRebindVersionConflict: the binding changed since the caller read it.
	ErrRebindVersionConflict = errors.New("binding version conflict")
	// ErrRebindNoChange: the payment is already bound to this provider.
	ErrRebindNoChange = errors.New("payment already bound to this provider")
)

const rebindAuditColumns = `
	id::text, payment_intent_id::text, source_service, source_reference, caller_service,
	previous_provider_type, previous_provider_id, new_provider_type, new_provider_id,
	from_version, to_version, recovery_case_ref, reason_code, amount_kobo, currency,
	idempotency_key, created_at`

func scanRebind(row pgx.Row) (walletmodels.ExecutionProviderRebind, string, error) {
	var rebind walletmodels.ExecutionProviderRebind
	var fingerprint string
	err := row.Scan(&rebind.ID, &rebind.PaymentIntentID, &rebind.SourceService, &rebind.SourceReference,
		&rebind.CallerService, &rebind.PreviousProviderType, &rebind.PreviousProviderID,
		&rebind.NewProviderType, &rebind.NewProviderID, &rebind.FromVersion, &rebind.ToVersion,
		&rebind.RecoveryCaseRef, &rebind.ReasonCode, &rebind.AmountKobo, &rebind.Currency,
		&rebind.IdempotencyKey, &rebind.CreatedAt, &fingerprint)
	return rebind, fingerprint, err
}

// RebindExecutionProvider applies one rebind, or replays the original answer
// for a repeated idempotency key.
//
// The intent row is locked FIRST and the key looked up SECOND. With the order
// reversed, two concurrent sends of one request would both miss the key, the
// second would then wait on the lock and find the version already advanced --
// reporting a conflict for what is really a replay of a request that
// succeeded.
func (r *PostgresWalletRepository) RebindExecutionProvider(ctx context.Context,
	request walletmodels.ExecutionProviderRebindRequest, fingerprint string,
) (walletmodels.ExecutionProviderRebind, error) {
	var result walletmodels.ExecutionProviderRebind
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		intent, err := scanPaymentIntent(tx.QueryRow(ctx, selectPaymentIntentSQL()+
			` WHERE source_service = $1 AND source_reference = $2 FOR UPDATE`,
			request.SourceService, request.SourceReference))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound("Payment not found.", err)
		}
		if err != nil {
			return err
		}

		existing, storedFingerprint, err := scanRebind(tx.QueryRow(ctx,
			`SELECT `+rebindAuditColumns+`, request_fingerprint
			   FROM execution_provider_rebind_audit WHERE idempotency_key = $1`, request.IdempotencyKey))
		switch {
		case err == nil:
			if storedFingerprint != fingerprint || existing.PaymentIntentID != intent.ID {
				return ErrRebindKeyReused
			}
			existing.Replayed = true
			result = existing
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}

		if intent.Status != walletmodels.PaymentStatusHeld {
			return ErrRebindNotHeld
		}
		var version int64
		if err = tx.QueryRow(ctx, `SELECT execution_binding_version FROM payment_intents WHERE id = $1`,
			intent.ID).Scan(&version); err != nil {
			return err
		}
		if version != request.ExpectedVersion {
			return ErrRebindVersionConflict
		}
		if intent.ProviderID == request.NewProviderID && intent.ProviderType == request.NewProviderType {
			return ErrRebindNoChange
		}

		tag, err := tx.Exec(ctx, `
			UPDATE payment_intents
			   SET provider_id = $2, provider_type = $3,
			       execution_binding_version = execution_binding_version + 1, updated_at = now()
			 WHERE id = $1 AND execution_binding_version = $4 AND status = $5`,
			intent.ID, request.NewProviderID, request.NewProviderType, version, walletmodels.PaymentStatusHeld)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrRebindVersionConflict
		}

		result, _, err = scanRebind(tx.QueryRow(ctx, `
			INSERT INTO execution_provider_rebind_audit (
				id, payment_intent_id, source_service, source_reference, caller_service,
				previous_provider_type, previous_provider_id, new_provider_type, new_provider_id,
				from_version, to_version, recovery_case_ref, reason_code, amount_kobo, currency,
				idempotency_key, request_fingerprint
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
			RETURNING `+rebindAuditColumns+`, request_fingerprint`,
			uuid.NewString(), intent.ID, intent.SourceService, intent.SourceReference, request.CallerService,
			intent.ProviderType, intent.ProviderID, request.NewProviderType, request.NewProviderID,
			version, version+1, request.RecoveryCaseRef, request.ReasonCode, intent.AmountKobo, intent.Currency,
			request.IdempotencyKey, fingerprint))
		return err
	})
	return result, err
}

// ListExecutionProviderRebinds returns a job's rebind history, oldest first.
func (r *PostgresWalletRepository) ListExecutionProviderRebinds(ctx context.Context,
	sourceService, sourceReference string) ([]walletmodels.ExecutionProviderRebind, error) {
	rows, err := r.db.Query(ctx, `SELECT `+rebindAuditColumns+`, request_fingerprint
		FROM execution_provider_rebind_audit
		WHERE source_service = $1 AND source_reference = $2
		ORDER BY to_version`, sourceService, sourceReference)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rebinds := []walletmodels.ExecutionProviderRebind{}
	for rows.Next() {
		rebind, _, err := scanRebind(rows)
		if err != nil {
			return nil, err
		}
		rebinds = append(rebinds, rebind)
	}
	return rebinds, rows.Err()
}

// ExecutionBindingVersion reads a job's current binding version, so a caller
// that lost track of it can learn it before rebinding.
func (r *PostgresWalletRepository) ExecutionBindingVersion(ctx context.Context,
	sourceService, sourceReference string) (int64, error) {
	var version int64
	err := r.db.QueryRow(ctx, `SELECT execution_binding_version FROM payment_intents
		WHERE source_service = $1 AND source_reference = $2`, sourceService, sourceReference).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, apperrors.NotFound("Payment not found.", err)
	}
	return version, err
}
