package walletrepositories

// Section 12: posting one Fleet-managed work settlement.
//
// The job escrow, the platform revenue account and the provider payable all
// live in this ledger, so a managed settlement is one balanced journal here
// rather than a posting in one ledger and a transfer in another. The
// organization payable joins them for the same reason.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/managedsettlement"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
)

// ErrManagedSettlementConflict means the same identity was sent with different
// content.
//
// It is deliberately distinct from "already posted": a repeat is success, a
// contradiction is not, and collapsing the two is how a changed instruction
// gets silently ignored.
var ErrManagedSettlementConflict = errors.New("managed settlement identity reused with different content")

const managedSettlementColumns = `
	id, instruction_id, instruction_version,
	organization_id, membership_id, provider_id, provider_type,
	source_service, source_job_id, source_assignment_id,
	COALESCE(payment_intent_id::text, ''), payment_source_service, payment_source_reference,
	currency, gross_basis_minor, provider_share_minor, organization_share_minor, platform_share_minor,
	compensation_plan_id, compensation_plan_version, compensation_resolution_version,
	status, failure_code,
	ledger_transaction_id, reversal_transaction_id,
	idempotency_key, request_fingerprint, correlation_id,
	created_at, updated_at, posted_at, reversed_at`

func selectManagedSettlementSQL() string {
	return `SELECT ` + managedSettlementColumns + ` FROM managed_settlements`
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanManagedSettlement(row rowScanner) (walletmodels.ManagedSettlement, error) {
	var m walletmodels.ManagedSettlement
	var ledgerTx, reversalTx *string
	var postedAt, reversedAt *time.Time
	err := row.Scan(
		&m.ID, &m.InstructionID, &m.InstructionVersion,
		&m.OrganizationID, &m.MembershipID, &m.ProviderID, &m.ProviderType,
		&m.SourceService, &m.SourceJobID, &m.SourceAssignmentID,
		&m.PaymentIntentID, &m.PaymentSourceService, &m.PaymentSourceReference,
		&m.Currency, &m.GrossBasisMinor, &m.ProviderShareMinor, &m.OrganizationShareMinor, &m.PlatformShareMinor,
		&m.CompensationPlanID, &m.CompensationPlanVersion, &m.CompensationResolutionVersion,
		&m.Status, &m.FailureCode,
		&ledgerTx, &reversalTx,
		&m.IdempotencyKey, &m.RequestFingerprint, &m.CorrelationID,
		&m.CreatedAt, &m.UpdatedAt, &postedAt, &reversedAt,
	)
	if err != nil {
		return walletmodels.ManagedSettlement{}, err
	}
	if ledgerTx != nil {
		m.LedgerTransactionID = *ledgerTx
	}
	if reversalTx != nil {
		m.ReversalTransactionID = *reversalTx
	}
	m.PostedAt, m.ReversedAt = postedAt, reversedAt
	return m, nil
}

// ManagedSettlementFingerprint is the content identity of an instruction.
//
// Two sends that agree on every posted figure are the same settlement; a send
// that changes any of them is a different one wearing the same name.
func ManagedSettlementFingerprint(in walletmodels.ManagedSettlementInstruction, platformShareMinor int64) string {
	parts := []string{
		in.InstructionID, strconv.FormatInt(in.InstructionVersion, 10),
		in.OrganizationID, in.MembershipID, in.ProviderID, in.ProviderType,
		in.SourceService, in.SourceJobID, in.SourceAssignmentID, in.Currency,
		strconv.FormatInt(in.GrossBasisMinor, 10),
		strconv.FormatInt(in.ProviderShareMinor, 10),
		strconv.FormatInt(in.OrganizationShareMinor, 10),
		strconv.FormatInt(platformShareMinor, 10),
	}
	// The payment reference joins the fingerprint only when it is present, so
	// an instruction that carries none fingerprints exactly as it did before
	// the field existed: a retry of a settlement posted then must still be
	// recognised as the same settlement, not refused as a contradiction.
	if in.PaymentSourceReference != "" {
		parts = append(parts, "payment_source_reference="+in.PaymentSourceReference)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

// GetManagedSettlementByInstruction reads one settlement by instruction identity.
func (r *PostgresWalletRepository) GetManagedSettlementByInstruction(ctx context.Context, instructionID string, version int64) (walletmodels.ManagedSettlement, error) {
	row := r.db.QueryRow(ctx, selectManagedSettlementSQL()+
		` WHERE instruction_id = $1 AND instruction_version = $2`, instructionID, version)
	settlement, err := scanManagedSettlement(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return walletmodels.ManagedSettlement{}, apperrors.NotFound("Managed settlement was not found.", nil)
	}
	return settlement, err
}

// GetManagedSettlementByIdempotencyKey reads one settlement by its key.
func (r *PostgresWalletRepository) GetManagedSettlementByIdempotencyKey(ctx context.Context, key string) (walletmodels.ManagedSettlement, error) {
	row := r.db.QueryRow(ctx, selectManagedSettlementSQL()+` WHERE idempotency_key = $1`, key)
	settlement, err := scanManagedSettlement(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return walletmodels.ManagedSettlement{}, apperrors.NotFound("Managed settlement was not found.", nil)
	}
	return settlement, err
}

// ListManagedSettlementsForSource reads every settlement posted for one
// execution job, newest first.
//
// An execution service reconciling a completion asks this: it is the answer to
// "did my money actually move?", which a timeout cannot tell it.
//
// The service name is matched case-insensitively, which is not a nicety. Fleet
// instructs with the source service spelled as the instruction carried it --
// "taxi" in practice -- while the shared read client normalises to "TAXI" so
// that two code paths asking about one job agree. A case-sensitive match here
// meant an execution service could never find its own settlement: the read
// returned nothing, which reads as "not settled", which is the one answer that
// leads to paying a job twice.
//
// An execution service asking under its payment spelling ("taxi-service")
// is answered as though it had asked under its canonical name: settlements
// are only ever stored canonically, so that is the one name they can be
// found under.
func (r *PostgresWalletRepository) ListManagedSettlementsForSource(ctx context.Context, sourceService, sourceJobID string) ([]walletmodels.ManagedSettlement, error) {
	if canonical, ok := managedsettlement.CanonicalSource(sourceService); ok {
		sourceService = canonical
	}
	rows, err := r.db.Query(ctx, selectManagedSettlementSQL()+
		` WHERE lower(source_service) = lower($1) AND source_job_id = $2 ORDER BY created_at DESC`, sourceService, sourceJobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []walletmodels.ManagedSettlement
	for rows.Next() {
		settlement, scanErr := scanManagedSettlement(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, settlement)
	}
	return out, rows.Err()
}

// PostManagedSettlement posts one instruction, exactly once.
//
// Everything that decides an amount happens before the journal is written, and
// every one of those decisions is a check rather than a calculation: Wallet is
// verifying Fleet's arithmetic against the money it actually holds, not
// redoing it.
func (r *PostgresWalletRepository) PostManagedSettlement(
	ctx context.Context, in walletmodels.ManagedSettlementInstruction,
) (walletmodels.ManagedSettlement, error) {
	in = in.Normalized()
	var settlement walletmodels.ManagedSettlement

	err := r.withTx(ctx, func(tx pgx.Tx) error {
		// Lock the job's money first. Everything after this reads a stable
		// intent, so two concurrent sends of the same instruction serialize
		// here rather than racing to post two journals.
		intent, err := r.resolveManagedPaymentForUpdate(ctx, tx, in.SourceService, in.PaymentReference())
		if err != nil {
			return err
		}
		if !strings.EqualFold(intent.Currency, in.Currency) {
			return apperrors.Conflict("Instruction currency does not match the payment currency.", nil)
		}

		// The platform's share was fixed when the customer paid. Fleet may
		// state it, and is checked against it, but never changes it -- two
		// authorities computing one number is how they drift apart.
		platformShare := intent.PlatformFeeKobo
		if in.PlatformShareMinor != nil && *in.PlatformShareMinor != platformShare {
			return apperrors.Conflict("Instruction platform share disagrees with the authorized platform fee.", nil)
		}
		distributable := intent.AmountKobo - platformShare
		if distributable < 0 {
			return apperrors.Internal("Platform fee cannot exceed the payment amount.", nil)
		}

		fingerprint := ManagedSettlementFingerprint(in, platformShare)

		existing, findErr := r.findManagedSettlementForUpdate(ctx, tx, in.InstructionID, in.InstructionVersion)
		if findErr == nil {
			if existing.RequestFingerprint != fingerprint {
				return ErrManagedSettlementConflict
			}
			// A retry of a settlement that already posted returns the original
			// outcome. This is the whole reason a lost response is safe. No
			// event is appended: the journal was posted once, so it is
			// announced once, and a consumer counting postings must not see
			// the caller's retries.
			settlement = existing
			return nil
		} else if !errors.Is(findErr, pgx.ErrNoRows) {
			return findErr
		}

		if in.GrossBasisMinor != distributable {
			return apperrors.Conflict(fmt.Sprintf(
				"Instruction was resolved against %d minor units but %d is distributable.",
				in.GrossBasisMinor, distributable), nil)
		}
		// Restating the invariant here, against the intent rather than against
		// the instruction alone: platform + organization + provider is exactly
		// what was held. No residual is dropped and none is invented.
		if platformShare+in.OrganizationShareMinor+in.ProviderShareMinor != intent.AmountKobo {
			return apperrors.Conflict("Allocation does not account for the full settleable amount.", nil)
		}

		switch intent.Status {
		case walletmodels.PaymentStatusHeld:
		case walletmodels.PaymentStatusCompleted:
			// The job was already settled, but not by this instruction --
			// otherwise the row above would have been found. Posting now would
			// pay the same work twice.
			return apperrors.Conflict("This job was already settled by another path.", nil)
		default:
			return apperrors.Conflict("Payment is not held for settlement.", nil)
		}

		escrow, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeJob,
			jobOwnerID(intent.SourceService, intent.SourceReference),
			walletmodels.AccountJobEscrow, intent.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}

		entries := []walletmodels.LedgerEntry{{
			AccountID: escrow.ID, Side: walletmodels.SideDebit,
			AmountKobo: intent.AmountKobo, Currency: intent.Currency,
			Memo: "Release managed job escrow",
		}}

		if platformShare > 0 {
			platform, accErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypePlatform, "karry-go",
				walletmodels.AccountPlatformRevenue, intent.Currency, walletmodels.NormalCredit)
			if accErr != nil {
				return accErr
			}
			entries = append(entries, walletmodels.LedgerEntry{
				AccountID: platform.ID, Side: walletmodels.SideCredit,
				AmountKobo: platformShare, Currency: intent.Currency, Memo: "Platform revenue",
			})
		}
		if in.OrganizationShareMinor > 0 {
			organization, accErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeOrganization,
				in.OrganizationID, walletmodels.AccountOrganizationPayable,
				intent.Currency, walletmodels.NormalCredit)
			if accErr != nil {
				return accErr
			}
			entries = append(entries, walletmodels.LedgerEntry{
				AccountID: organization.ID, Side: walletmodels.SideCredit,
				AmountKobo: in.OrganizationShareMinor, Currency: intent.Currency,
				Memo: "Organization entitlement",
			})
		}
		// A zero provider share is the salaried case and is entirely normal.
		// No line is written for it: a zero-amount entry would be rejected by
		// the ledger, and inventing a token credit would make a salaried
		// provider look like they earned per job.
		if in.ProviderShareMinor > 0 {
			provider, accErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider,
				providerOwnerID(in.ProviderType, in.ProviderID),
				walletmodels.AccountProviderPayable, intent.Currency, walletmodels.NormalCredit)
			if accErr != nil {
				return accErr
			}
			entries = append(entries, walletmodels.LedgerEntry{
				AccountID: provider.ID, Side: walletmodels.SideCredit,
				AmountKobo: in.ProviderShareMinor, Currency: intent.Currency,
				Memo: "Provider entitlement",
			})
		}

		transactionID := uuid.NewString()
		if err = r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              transactionID,
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: walletmodels.JournalManagedSettlement,
			Status:          postedStatus,
			SourceService:   intent.SourceService,
			SourceReference: intent.SourceReference,
			IdempotencyKey:  in.IdempotencyKey,
			Metadata: map[string]interface{}{
				"instruction_id":      in.InstructionID,
				"instruction_version": in.InstructionVersion,
				"organization_id":     in.OrganizationID,
				"membership_id":       in.MembershipID,
				"correlation_id":      in.CorrelationID,
			},
		}, entries); err != nil {
			return err
		}

		if _, err = tx.Exec(ctx, `
			UPDATE payment_intents
			SET status = $2, settlement_mode = $3, updated_at = now()
			WHERE id = $1`,
			intent.ID, walletmodels.PaymentStatusCompleted, walletmodels.SettlementModeManaged); err != nil {
			return err
		}

		settlement, err = r.insertManagedSettlement(ctx, tx, walletmodels.ManagedSettlement{
			ID: uuid.NewString(), InstructionID: in.InstructionID, InstructionVersion: in.InstructionVersion,
			OrganizationID: in.OrganizationID, MembershipID: in.MembershipID,
			ProviderID: in.ProviderID, ProviderType: in.ProviderType,
			SourceService: in.SourceService, SourceJobID: in.SourceJobID,
			SourceAssignmentID: in.SourceAssignmentID,
			PaymentIntentID:    intent.ID, PaymentSourceService: intent.SourceService,
			PaymentSourceReference: intent.SourceReference,
			Currency:               intent.Currency,
			GrossBasisMinor:        in.GrossBasisMinor, ProviderShareMinor: in.ProviderShareMinor,
			OrganizationShareMinor: in.OrganizationShareMinor, PlatformShareMinor: platformShare,
			CompensationPlanID: in.CompensationPlanID, CompensationPlanVersion: in.CompensationPlanVersion,
			CompensationResolutionVersion: in.CompensationResolutionVersion,
			Status:                        walletmodels.ManagedSettlementPosted,
			LedgerTransactionID:           transactionID,
			IdempotencyKey:                in.IdempotencyKey, RequestFingerprint: fingerprint,
			CorrelationID: in.CorrelationID,
		})
		if err != nil {
			return err
		}
		// WAL-019. Written inside the posting transaction, so the event and
		// the money it announces commit together. An event appended afterwards
		// can be lost while the journal stands, and a consumer reconciling
		// from events would then be permanently wrong about a settlement that
		// really happened.
		return appendManagedSettlementEvent(ctx, tx, EventManagedSettlementPosted, settlement, "")
	})
	if err != nil {
		// WAL-019. A terminal refusal is a durable fact Fleet has to learn
		// even if this response never arrives: the instruction is wrong and
		// someone must correct it. Recording it in a second transaction is
		// deliberate -- the first one rolled back, which is exactly why the
		// refusal needs writing separately.
		//
		// A transient or ambiguous error is NOT recorded. Resent unchanged it
		// may well post, and calling it failed would tell Fleet a job is
		// unsettleable when it is merely unsettled.
		if failureCode, terminal := terminalRefusal(err); terminal {
			if recordErr := r.recordManagedSettlementRefusal(ctx, in, failureCode); recordErr != nil {
				// The refusal itself is still returned. Failing to record it
				// must not turn a clear refusal into an ambiguous error.
				log.Printf("managed settlement refusal not recorded instruction_id=%s: %v", in.InstructionID, recordErr)
			}
		}
		return walletmodels.ManagedSettlement{}, err
	}
	return settlement, nil
}

// ErrManagedPaymentAmbiguous means a settlement identity matched more than one
// payment. It is refused, never resolved by picking one.
var ErrManagedPaymentAmbiguous = errors.New("managed settlement identity matches more than one payment")

// resolveManagedPaymentForUpdate finds and locks the one payment intent a
// managed job's money is held in.
//
// The settlement is named canonically (TAXI, DISPATCH, HAULING) but the
// payment was created by the execution service under its own provisioned name
// -- "taxi-service", "dispatch-delivery-service", "driver-hauling-service" --
// so the canonical name is widened to exactly the spellings
// shared/go/managedsettlement lists for it, and nothing else. The reference is
// the one the execution service supplied: the job id itself, or, for Dispatch,
// the booking the payment was taken for.
//
// Exactly one intent may match. None is "no payment for this job"; more than
// one means two spellings of one service both hold a payment under the same
// reference, and choosing between them would be a guess about which customer's
// money to release -- so it is refused as a conflict for a person to resolve.
func (r *PostgresWalletRepository) resolveManagedPaymentForUpdate(
	ctx context.Context, tx pgx.Tx, sourceService, paymentReference string,
) (walletmodels.PaymentIntent, error) {
	canonical, ok := managedsettlement.CanonicalSource(sourceService)
	if !ok {
		return walletmodels.PaymentIntent{}, apperrors.Validation("Check the settlement instruction.",
			[]apperrors.FieldViolation{{Field: "source_service", Message: "Source service is not an execution service."}})
	}
	intents, err := r.paymentIntentsForSpellings(ctx, tx,
		managedsettlement.PaymentSourceServices(canonical), paymentReference, true)
	if err != nil {
		return walletmodels.PaymentIntent{}, err
	}
	switch len(intents) {
	case 0:
		return walletmodels.PaymentIntent{}, apperrors.NotFound("No payment exists for this job.", nil)
	case 1:
		return intents[0], nil
	default:
		return walletmodels.PaymentIntent{}, apperrors.Conflict(
			"More than one payment matches this job; it cannot be settled until that is resolved.",
			ErrManagedPaymentAmbiguous)
	}
}

// paymentIntentsForSpellings reads every intent held under any of the given
// source spellings with the given reference, locking them when asked.
func (r *PostgresWalletRepository) paymentIntentsForSpellings(
	ctx context.Context, q interface {
		Query(context.Context, string, ...any) (pgx.Rows, error)
	}, spellings []string, reference string, forUpdate bool,
) ([]walletmodels.PaymentIntent, error) {
	query := selectPaymentIntentSQL() + ` WHERE source_service = ANY($1::text[]) AND source_reference = $2 ORDER BY id`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	rows, err := q.Query(ctx, query, spellings, reference)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var intents []walletmodels.PaymentIntent
	for rows.Next() {
		intent, scanErr := scanPaymentIntent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		intents = append(intents, intent)
	}
	return intents, rows.Err()
}

func (r *PostgresWalletRepository) findManagedSettlementForUpdate(
	ctx context.Context, tx pgx.Tx, instructionID string, version int64,
) (walletmodels.ManagedSettlement, error) {
	row := tx.QueryRow(ctx, selectManagedSettlementSQL()+
		` WHERE instruction_id = $1 AND instruction_version = $2 FOR UPDATE`, instructionID, version)
	return scanManagedSettlement(row)
}

func (r *PostgresWalletRepository) insertManagedSettlement(
	ctx context.Context, tx pgx.Tx, m walletmodels.ManagedSettlement,
) (walletmodels.ManagedSettlement, error) {
	var ledgerTx *string
	if m.LedgerTransactionID != "" {
		ledgerTx = &m.LedgerTransactionID
	}
	postedAt := time.Now().UTC()
	row := tx.QueryRow(ctx, `
		INSERT INTO managed_settlements (
			id, instruction_id, instruction_version,
			organization_id, membership_id, provider_id, provider_type,
			source_service, source_job_id, source_assignment_id,
			currency, gross_basis_minor, provider_share_minor, organization_share_minor, platform_share_minor,
			compensation_plan_id, compensation_plan_version, compensation_resolution_version,
			status, ledger_transaction_id, idempotency_key, request_fingerprint, correlation_id, posted_at,
			payment_intent_id, payment_source_service, payment_source_reference
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,
			NULLIF($25, '')::uuid, $26, $27)
		RETURNING `+managedSettlementColumns,
		m.ID, m.InstructionID, m.InstructionVersion,
		m.OrganizationID, m.MembershipID, m.ProviderID, m.ProviderType,
		m.SourceService, m.SourceJobID, m.SourceAssignmentID,
		m.Currency, m.GrossBasisMinor, m.ProviderShareMinor, m.OrganizationShareMinor, m.PlatformShareMinor,
		m.CompensationPlanID, m.CompensationPlanVersion, m.CompensationResolutionVersion,
		m.Status, ledgerTx, m.IdempotencyKey, m.RequestFingerprint, m.CorrelationID, postedAt,
		m.PaymentIntentID, m.PaymentSourceService, m.PaymentSourceReference,
	)
	return scanManagedSettlement(row)
}

// ReverseManagedSettlement reverses a posted settlement with a compensating
// journal (WAL-018).
//
// The original journal is never deleted or edited. A ledger whose history can
// be rewritten cannot be audited, and "the balance is right now" is not the
// same claim as "every movement is explained". So the correction is itself a
// posting, with the sides of the original swapped, carrying the original
// instruction's identity.
func (r *PostgresWalletRepository) ReverseManagedSettlement(
	ctx context.Context, instructionID string, version int64, reasonCode string,
) (walletmodels.ManagedSettlement, error) {
	var settlement walletmodels.ManagedSettlement

	err := r.withTx(ctx, func(tx pgx.Tx) error {
		existing, err := r.findManagedSettlementForUpdate(ctx, tx, instructionID, version)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.NotFound("Managed settlement was not found.", nil)
			}
			return err
		}
		if existing.Status == walletmodels.ManagedSettlementReversed {
			// Already reversed. Reversing again would move the money a second
			// time, so the original outcome is returned instead -- and no
			// second event is emitted, because nothing happened.
			settlement = existing
			return nil
		}
		if existing.Status != walletmodels.ManagedSettlementPosted {
			return apperrors.Conflict("Only a posted settlement can be reversed.", nil)
		}

		// The money goes back into the escrow it was released from, which is
		// the payment's escrow -- named by the payment's own source, not by
		// the settlement's canonical identity. The two differ for every real
		// execution service ("taxi-service" pays, "TAXI" settles), and
		// reversing into an escrow named after the settlement would credit an
		// account that never held this money.
		paymentService, paymentReference := existing.PaymentSourceService, existing.PaymentSourceReference
		if paymentService == "" || paymentReference == "" {
			// A settlement recorded before the payment binding existed was
			// resolved by an exact match, so its identity is its payment's.
			paymentService, paymentReference = existing.SourceService, existing.SourceJobID
		}
		escrow, err := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeJob,
			jobOwnerID(paymentService, paymentReference),
			walletmodels.AccountJobEscrow, existing.Currency, walletmodels.NormalCredit)
		if err != nil {
			return err
		}
		total := existing.TotalAllocatedMinor()
		entries := []walletmodels.LedgerEntry{{
			AccountID: escrow.ID, Side: walletmodels.SideCredit,
			AmountKobo: total, Currency: existing.Currency,
			Memo: "Reverse managed job settlement",
		}}
		if existing.PlatformShareMinor > 0 {
			platform, accErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypePlatform, "karry-go",
				walletmodels.AccountPlatformRevenue, existing.Currency, walletmodels.NormalCredit)
			if accErr != nil {
				return accErr
			}
			entries = append(entries, walletmodels.LedgerEntry{
				AccountID: platform.ID, Side: walletmodels.SideDebit,
				AmountKobo: existing.PlatformShareMinor, Currency: existing.Currency,
				Memo: "Reverse platform revenue",
			})
		}
		if existing.OrganizationShareMinor > 0 {
			organization, accErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeOrganization,
				existing.OrganizationID, walletmodels.AccountOrganizationPayable,
				existing.Currency, walletmodels.NormalCredit)
			if accErr != nil {
				return accErr
			}
			entries = append(entries, walletmodels.LedgerEntry{
				AccountID: organization.ID, Side: walletmodels.SideDebit,
				AmountKobo: existing.OrganizationShareMinor, Currency: existing.Currency,
				Memo: "Reverse organization entitlement",
			})
		}
		if existing.ProviderShareMinor > 0 {
			provider, accErr := r.ensureAccount(ctx, tx, walletmodels.OwnerTypeProvider,
				providerOwnerID(existing.ProviderType, existing.ProviderID),
				walletmodels.AccountProviderPayable, existing.Currency, walletmodels.NormalCredit)
			if accErr != nil {
				return accErr
			}
			entries = append(entries, walletmodels.LedgerEntry{
				AccountID: provider.ID, Side: walletmodels.SideDebit,
				AmountKobo: existing.ProviderShareMinor, Currency: existing.Currency,
				Memo: "Reverse provider entitlement",
			})
		}

		reversalID := uuid.NewString()
		if err = r.postLedgerTransaction(ctx, tx, walletmodels.LedgerTransaction{
			ID:              reversalID,
			Reference:       "ldt_" + uuid.NewString(),
			TransactionType: walletmodels.JournalManagedSettlementReversal,
			Status:          postedStatus,
			SourceService:   paymentService,
			SourceReference: paymentReference,
			IdempotencyKey:  "reverse:" + existing.IdempotencyKey,
			Metadata: map[string]interface{}{
				"instruction_id":       existing.InstructionID,
				"instruction_version":  existing.InstructionVersion,
				"reversed_transaction": existing.LedgerTransactionID,
				"reason_code":          reasonCode,
			},
		}, entries); err != nil {
			return err
		}

		row := tx.QueryRow(ctx, `
			UPDATE managed_settlements
			SET status = $2, reversal_transaction_id = $3, failure_code = $4,
			    reversed_at = now(), updated_at = now()
			WHERE id = $1
			RETURNING `+managedSettlementColumns,
			existing.ID, walletmodels.ManagedSettlementReversed, reversalID, reasonCode)
		settlement, err = scanManagedSettlement(row)
		if err != nil {
			return err
		}
		// WAL-019. The posted event is not retracted -- it remains true, the
		// money did move -- so the reversal is announced as its own fact,
		// carrying the reason it happened.
		return appendManagedSettlementEvent(ctx, tx, EventManagedSettlementReversed, settlement, reasonCode)
	})
	if err != nil {
		return walletmodels.ManagedSettlement{}, err
	}
	return settlement, nil
}
