package basket

// Persistence.
//
// Every read and every write is customer-scoped, and the scoping is in the
// SQL rather than in a caller's discipline: a proposal id or a template id
// alone must never be enough to read or rewrite somebody else's shopping.
//
// The database carries the invariants that matter most - one generation per
// occurrence, one cart version per acceptance, totals that add up - because
// those are the ones that have to survive two workers, a replay and a
// restart. This layer hands them values and reads their verdicts.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists proposals, templates and generations.
type Store struct {
	db *pgxpool.Pool
}

// NewStore builds the store.
func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

// ---------------------------------------------------------------------------
// Proposals
// ---------------------------------------------------------------------------

// CreateProposal writes one offer.
//
// A proposal for a basket already offered against the same cart version is
// RETURNED rather than duplicated. Re-running the optimizer on an unchanged
// cart is an ordinary thing for a customer to do, and it must not accumulate
// a pile of identical offers each of which could be accepted separately.
func (s *Store) CreateProposal(
	ctx context.Context,
	proposal Proposal,
) (Proposal, error) {
	if s == nil || s.db == nil {
		return Proposal{}, ErrDependencyMissing
	}

	lines, err := json.Marshal(proposal.Lines)
	if err != nil {
		return Proposal{}, err
	}

	substitutions, err := json.Marshal(proposal.Substitutions)
	if err != nil {
		return Proposal{}, err
	}

	if len(proposal.Substitutions) == 0 {
		substitutions = []byte("[]")
	}

	proposal.ID = "bop_" + uuid.NewString()

	var (
		baseCart    *string
		baseVersion *int64
	)

	if strings.TrimSpace(proposal.BaseCartID) != "" {
		id := proposal.BaseCartID
		baseCart = &id

		version := proposal.BaseCartVersion
		baseVersion = &version
	}

	err = s.db.QueryRow(ctx, `
		INSERT INTO commerce_basket_proposals (
			id, customer_id, base_cart_id, base_cart_version, objective,
			state, currency, merchandise_total_minor, delivery_total_minor,
			final_delivered_total_minor, vendor_count, lines, substitutions,
			fingerprint, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id, created_at`,
		proposal.ID, proposal.CustomerID, baseCart, baseVersion,
		string(proposal.Objective), string(ProposalOpen), proposal.Currency,
		proposal.MerchandiseTotalMinor, proposal.DeliveryTotalMinor,
		proposal.FinalDeliveredTotalMinor, proposal.VendorCount,
		lines, substitutions, proposal.Fingerprint, proposal.ExpiresAt,
	).Scan(&proposal.ID, &proposal.CreatedAt)

	if err != nil && isUniqueViolation(err) && baseCart != nil {
		// The same basket is already on offer against this cart version.
		// Returning the existing one is the correct answer: it is the same
		// offer, and a second row would be a second thing to accept.
		return s.proposalByFingerprint(
			ctx, proposal.CustomerID, *baseCart, *baseVersion,
			proposal.Fingerprint,
		)
	}

	if err != nil {
		return Proposal{}, err
	}

	proposal.State = ProposalOpen

	return proposal, nil
}

func (s *Store) proposalByFingerprint(
	ctx context.Context,
	customerID, cartID string,
	cartVersion int64,
	fingerprint string,
) (Proposal, error) {
	row := s.db.QueryRow(ctx, proposalSelect+`
		 WHERE customer_id = $1 AND base_cart_id = $2
		   AND base_cart_version = $3 AND fingerprint = $4
		   AND state = 'OPEN'`,
		customerID, cartID, cartVersion, fingerprint,
	)

	return scanProposal(row)
}

const proposalSelect = `
	SELECT id, customer_id, COALESCE(base_cart_id, ''),
	       COALESCE(base_cart_version, 0), objective, state, currency,
	       merchandise_total_minor, delivery_total_minor,
	       final_delivered_total_minor, vendor_count, lines, substitutions,
	       fingerprint, expires_at, created_at,
	       COALESCE(accepted_cart_version, 0), accepted_at
	  FROM commerce_basket_proposals`

func scanProposal(row pgx.Row) (Proposal, error) {
	var (
		proposal      Proposal
		objective     string
		state         string
		lines         []byte
		substitutions []byte
	)

	err := row.Scan(
		&proposal.ID, &proposal.CustomerID, &proposal.BaseCartID,
		&proposal.BaseCartVersion, &objective, &state, &proposal.Currency,
		&proposal.MerchandiseTotalMinor, &proposal.DeliveryTotalMinor,
		&proposal.FinalDeliveredTotalMinor, &proposal.VendorCount,
		&lines, &substitutions, &proposal.Fingerprint, &proposal.ExpiresAt,
		&proposal.CreatedAt, &proposal.AcceptedCartVersion,
		&proposal.AcceptedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Proposal{}, ErrProposalNotFound
	}
	if err != nil {
		return Proposal{}, err
	}

	proposal.Objective = Objective(objective)
	proposal.State = ProposalState(state)

	if len(lines) > 0 {
		if err = json.Unmarshal(lines, &proposal.Lines); err != nil {
			return Proposal{}, err
		}
	}

	if len(substitutions) > 0 {
		if err = json.Unmarshal(
			substitutions, &proposal.Substitutions,
		); err != nil {
			return Proposal{}, err
		}
	}

	return proposal, nil
}

// GetProposal reads one proposal the customer owns.
func (s *Store) GetProposal(
	ctx context.Context,
	customerID, proposalID string,
) (Proposal, error) {
	if s == nil || s.db == nil {
		return Proposal{}, ErrDependencyMissing
	}

	// Both predicates, always. The customer one is not a convenience: without
	// it a guessed or leaked id would be enough to read another customer's
	// shopping and their delivery costs.
	row := s.db.QueryRow(ctx, proposalSelect+`
		 WHERE id = $1 AND customer_id = $2`,
		strings.TrimSpace(proposalID), strings.TrimSpace(customerID),
	)

	return scanProposal(row)
}

// MarkProposalAccepted records the cart version an acceptance produced.
//
// The WHERE clause requires the proposal to still be OPEN, so two concurrent
// acceptances resolve to one update and one cart version. The loser is told
// the proposal is stale rather than being allowed to write a second
// acceptance over the first.
func (s *Store) MarkProposalAccepted(
	ctx context.Context,
	customerID, proposalID string,
	cartVersion int64,
	at time.Time,
) error {
	if s == nil || s.db == nil {
		return ErrDependencyMissing
	}

	tag, err := s.db.Exec(ctx, `
		UPDATE commerce_basket_proposals
		   SET state = 'ACCEPTED',
		       accepted_cart_version = $3,
		       accepted_at = $4,
		       updated_at = now()
		 WHERE id = $1 AND customer_id = $2 AND state = 'OPEN'`,
		proposalID, customerID, cartVersion, at,
	)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return ErrProposalStale
	}

	return nil
}

// MarkProposalState moves a proposal to a terminal state.
//
// Only an OPEN proposal moves. An accepted one stays accepted: expiring a
// proposal that already created a cart version would make the acceptance
// unexplainable afterwards.
func (s *Store) MarkProposalState(
	ctx context.Context,
	customerID, proposalID string,
	state ProposalState,
) error {
	if s == nil || s.db == nil {
		return ErrDependencyMissing
	}

	_, err := s.db.Exec(ctx, `
		UPDATE commerce_basket_proposals
		   SET state = $3, updated_at = now()
		 WHERE id = $1 AND customer_id = $2 AND state = 'OPEN'`,
		proposalID, customerID, string(state),
	)

	return err
}

// ExpireDueProposals marks lapsed offers expired.
func (s *Store) ExpireDueProposals(
	ctx context.Context,
	now time.Time,
	limit int,
) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrDependencyMissing
	}

	if limit <= 0 {
		limit = 200
	}

	tag, err := s.db.Exec(ctx, `
		UPDATE commerce_basket_proposals
		   SET state = 'EXPIRED', updated_at = now()
		 WHERE id IN (
		       SELECT id FROM commerce_basket_proposals
		        WHERE state = 'OPEN' AND expires_at <= $1
		        ORDER BY expires_at
		        LIMIT $2
		 )`,
		now, limit,
	)
	if err != nil {
		return 0, err
	}

	return tag.RowsAffected(), nil
}

// LastApprovedBasket reads the lines of the most recent accepted basket for a
// template.
//
// It is the baseline a recurring comparison is measured against. Absent - a
// first occurrence, or a template nobody has accepted yet - it returns
// nothing, and the comparison correctly reports that there is no previous
// basket rather than inventing one.
func (s *Store) LastApprovedBasket(
	ctx context.Context,
	customerID, templateID string,
) ([]ProposedLine, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}

	var lines []byte

	err := s.db.QueryRow(ctx, `
		SELECT p.lines
		  FROM commerce_recurring_basket_generations g
		  JOIN commerce_basket_proposals p
		    ON p.id = g.proposal_id AND p.customer_id = g.customer_id
		 WHERE g.template_id = $1 AND g.customer_id = $2
		   AND p.state = 'ACCEPTED'
		 ORDER BY g.occurrence_at DESC
		 LIMIT 1`,
		templateID, customerID,
	).Scan(&lines)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var previous []ProposedLine

	if err = json.Unmarshal(lines, &previous); err != nil {
		return nil, err
	}

	return previous, nil
}

// ---------------------------------------------------------------------------
// Recurrence generations
// ---------------------------------------------------------------------------

// ClaimOccurrence reserves one scheduled occurrence, or reads back whoever
// reserved it first.
//
// The second return value distinguishes the two, and the caller needs it: a
// created claim means "do the work", and an existing one means "somebody
// already did, return theirs". ON CONFLICT DO NOTHING is what makes the race
// resolve in the database rather than between two workers.
func (s *Store) ClaimOccurrence(
	ctx context.Context,
	generation Generation,
) (Generation, bool, error) {
	if s == nil || s.db == nil {
		return Generation{}, false, ErrDependencyMissing
	}

	generation.ID = "brg_" + uuid.NewString()

	var id string

	err := s.db.QueryRow(ctx, `
		INSERT INTO commerce_recurring_basket_generations (
			id, template_id, customer_id, occurrence_key, occurrence_at,
			timezone, state
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (template_id, occurrence_key) DO NOTHING
		RETURNING id`,
		generation.ID, generation.TemplateID, generation.CustomerID,
		generation.OccurrenceKey, generation.OccurrenceAt,
		generation.Timezone, string(GenerationProposed),
	).Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		// Somebody else holds this occurrence, or this process did before it
		// crashed. Reading their row back is what makes a retry resume rather
		// than duplicate.
		existing, readErr := s.GetOccurrence(
			ctx, generation.CustomerID,
			generation.TemplateID, generation.OccurrenceKey,
		)

		return existing, false, readErr
	}

	if err != nil {
		return Generation{}, false, err
	}

	generation.ID = id
	generation.State = GenerationProposed

	return generation, true, nil
}

// CompleteOccurrence writes the outcome of a claimed occurrence.
func (s *Store) CompleteOccurrence(
	ctx context.Context,
	generation Generation,
) error {
	if s == nil || s.db == nil {
		return ErrDependencyMissing
	}

	comparison, err := json.Marshal(generation.Comparison)
	if err != nil {
		return err
	}

	var proposalID *string

	if strings.TrimSpace(generation.ProposalID) != "" {
		id := generation.ProposalID
		proposalID = &id
	}

	_, err = s.db.Exec(ctx, `
		UPDATE commerce_recurring_basket_generations
		   SET state = $3, proposal_id = $4, comparison = $5, reason = $6,
		       updated_at = now()
		 WHERE id = $1 AND customer_id = $2`,
		generation.ID, generation.CustomerID, string(generation.State),
		proposalID, comparison, generation.Reason,
	)

	return err
}

const generationSelect = `
	SELECT id, template_id, customer_id, occurrence_key, occurrence_at,
	       timezone, state, COALESCE(proposal_id, ''), comparison, reason,
	       created_at
	  FROM commerce_recurring_basket_generations`

// GetOccurrence reads one generation by its occurrence identity.
func (s *Store) GetOccurrence(
	ctx context.Context,
	customerID, templateID, occurrenceKey string,
) (Generation, error) {
	if s == nil || s.db == nil {
		return Generation{}, ErrDependencyMissing
	}

	row := s.db.QueryRow(ctx, generationSelect+`
		 WHERE template_id = $1 AND customer_id = $2 AND occurrence_key = $3`,
		templateID, customerID, occurrenceKey,
	)

	return scanGeneration(row)
}

func scanGeneration(row pgx.Row) (Generation, error) {
	var (
		generation Generation
		state      string
		comparison []byte
	)

	err := row.Scan(
		&generation.ID, &generation.TemplateID, &generation.CustomerID,
		&generation.OccurrenceKey, &generation.OccurrenceAt,
		&generation.Timezone, &state, &generation.ProposalID, &comparison,
		&generation.Reason, &generation.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Generation{}, ErrTemplateNotFound
	}
	if err != nil {
		return Generation{}, err
	}

	generation.State = GenerationState(state)

	if len(comparison) > 0 {
		if err = json.Unmarshal(
			comparison, &generation.Comparison,
		); err != nil {
			return Generation{}, err
		}
	}

	return generation, nil
}

// ListOccurrences reads a template's generations, newest first.
func (s *Store) ListOccurrences(
	ctx context.Context,
	customerID, templateID string,
	limit int,
) ([]Generation, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.db.Query(ctx, generationSelect+`
		 WHERE template_id = $1 AND customer_id = $2
		 ORDER BY occurrence_at DESC, id
		 LIMIT $3`,
		templateID, customerID, limit,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var out []Generation

	for rows.Next() {
		generation, scanErr := scanGeneration(rows)

		if scanErr != nil {
			return nil, scanErr
		}

		out = append(out, generation)
	}

	return out, rows.Err()
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}
