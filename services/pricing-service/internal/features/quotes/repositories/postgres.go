// Package repositories persists quotes, bindings and reconciliations, and reads
// the geography a quote is priced in.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/pricing-service/internal/database"
	"cosmicforge/logistics/services/pricing-service/internal/features/quotes/models"
)

// Store is the Postgres store for quotes.
type Store struct{ db database.Querier }

// NewStore returns a store over db.
func NewStore(db database.Querier) *Store { return &Store{db: db} }

const placeColumns = `
	m.id, m.code, COALESCE(m.state_id, ''), m.country_code, c.currency`

// ZoneForConsumer resolves a calling service's own zone onto a pricing zone as
// it stood at asOf. "" when the caller's zone is not mapped.
func (s *Store) ZoneForConsumer(ctx context.Context, consumerService, consumerZoneID string, asOf time.Time) (string, error) {
	var zoneID string
	err := s.db.QueryRow(ctx, `
		SELECT pricing_zone_id FROM pricing_zone_mappings
		WHERE consumer_service = $1 AND consumer_zone_id = $2 AND valid_from <= $3
		ORDER BY valid_from DESC
		LIMIT 1`, consumerService, consumerZoneID, asOf).Scan(&zoneID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("quotes: resolve consumer zone: %w", err)
	}
	return zoneID, nil
}

// ZoneCandidates lists every active zone in an active market with its current
// circle, for finding the zone that contains a pickup point.
func (s *Store) ZoneCandidates(ctx context.Context) ([]models.ZoneCandidate, error) {
	rows, err := s.db.Query(ctx, `
		SELECT z.id, zv.id, zv.shape,`+placeColumns+`
		FROM pricing_zones z
		JOIN LATERAL (
			SELECT id, shape FROM pricing_zone_versions v
			WHERE v.zone_id = z.id
			ORDER BY v.version_no DESC
			LIMIT 1
		) zv ON true
		JOIN pricing_markets m ON m.id = z.market_id
		JOIN pricing_countries c ON c.code = m.country_code
		WHERE z.status = 'ACTIVE' AND m.status = 'ACTIVE'`)
	if err != nil {
		return nil, fmt.Errorf("quotes: list zones: %w", err)
	}
	defer rows.Close()

	var out []models.ZoneCandidate
	for rows.Next() {
		var candidate models.ZoneCandidate
		var shape []byte
		if err := rows.Scan(&candidate.ZoneID, &candidate.ZoneVersionID, &shape,
			&candidate.MarketID, &candidate.MarketCode, &candidate.StateID, &candidate.CountryCode, &candidate.Currency); err != nil {
			return nil, fmt.Errorf("quotes: scan zone: %w", err)
		}
		var circle struct {
			Type        string `json:"type"`
			CenterLatE6 int64  `json:"center_lat_e6"`
			CenterLngE6 int64  `json:"center_lng_e6"`
			RadiusM     int64  `json:"radius_m"`
		}
		if err := json.Unmarshal(shape, &circle); err != nil || circle.Type != "CIRCLE" || circle.RadiusM <= 0 {
			continue // a shape this version cannot test containment for
		}
		candidate.CenterLatE6, candidate.CenterLngE6, candidate.RadiusM = circle.CenterLatE6, circle.CenterLngE6, circle.RadiusM
		out = append(out, candidate)
	}
	return out, rows.Err()
}

// PlaceForZone returns the place a zone sits in, with its current version.
func (s *Store) PlaceForZone(ctx context.Context, zoneID string) (models.Place, error) {
	var place models.Place
	err := s.db.QueryRow(ctx, `
		SELECT z.id, COALESCE((SELECT id FROM pricing_zone_versions v WHERE v.zone_id = z.id ORDER BY version_no DESC LIMIT 1), ''),`+placeColumns+`
		FROM pricing_zones z
		JOIN pricing_markets m ON m.id = z.market_id
		JOIN pricing_countries c ON c.code = m.country_code
		WHERE z.id = $1 AND z.status = 'ACTIVE' AND m.status = 'ACTIVE'`, zoneID).
		Scan(&place.ZoneID, &place.ZoneVersionID, &place.MarketID, &place.MarketCode, &place.StateID, &place.CountryCode, &place.Currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Place{}, models.ErrNoMarket
	}
	if err != nil {
		return models.Place{}, fmt.Errorf("quotes: place for zone: %w", err)
	}
	return place, nil
}

const quoteColumns = `
	id, consumer_service, consumer_ref, scope, channel, class, mode,
	market_id, market_code, COALESCE(zone_id, ''), COALESCE(zone_version_id, ''),
	tariff_version_id, COALESCE(commission_version_id, ''),
	request, result, result_hash, legacy_fare_kobo, issued_at, expires_at`

// InsertQuote stores q, or returns the quote already issued for the same
// caller, reference and class. created reports which.
func (s *Store) InsertQuote(ctx context.Context, q models.Quote) (stored models.Quote, created bool, err error) {
	request, err := json.Marshal(q.Request)
	if err != nil {
		return models.Quote{}, false, fmt.Errorf("quotes: encode request: %w", err)
	}
	result, err := json.Marshal(q.Result)
	if err != nil {
		return models.Quote{}, false, fmt.Errorf("quotes: encode result: %w", err)
	}

	row := s.db.QueryRow(ctx, `
		INSERT INTO pricing_quotes (
			id, consumer_service, consumer_ref, scope, channel, class, mode,
			market_id, market_code, zone_id, zone_version_id,
			tariff_version_id, commission_version_id,
			request, result, result_hash, gross_kobo, provider_kobo, platform_fee_kobo, tax_kobo, currency,
			legacy_fare_kobo, issued_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''), NULLIF($11, ''), $12, NULLIF($13, ''),
			$14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
		ON CONFLICT (consumer_service, consumer_ref, class) DO NOTHING
		RETURNING`+quoteColumns,
		q.ID, q.ConsumerService, q.ConsumerRef, q.Scope, q.Channel, q.Class, q.Mode,
		q.Place.MarketID, q.Place.MarketCode, q.Place.ZoneID, q.Place.ZoneVersionID,
		q.TariffVersionID, q.CommissionVersionID,
		request, result, q.ResultHash, q.Result.GrossKobo, q.Result.Allocation.ProviderKobo,
		q.Result.Allocation.PlatformFeeKobo, q.Result.Allocation.TaxKobo, q.Result.Currency,
		q.LegacyFareKobo, q.IssuedAt, q.ExpiresAt)
	stored, err = scanQuote(row)
	if err == nil {
		return stored, true, nil
	}
	if !errors.Is(err, models.ErrNotFound) {
		return models.Quote{}, false, err
	}
	// Conflict: the same caller already priced this reference and class.
	existing, err := s.findByRef(ctx, q.ConsumerService, q.ConsumerRef, q.Class)
	return existing, false, err
}

// FindByRef returns the quote a caller was issued for its reference and class.
func (s *Store) FindByRef(ctx context.Context, consumerService, consumerRef, class string) (models.Quote, error) {
	return s.findByRef(ctx, consumerService, consumerRef, class)
}

func (s *Store) findByRef(ctx context.Context, consumerService, consumerRef, class string) (models.Quote, error) {
	return scanQuote(s.db.QueryRow(ctx, `SELECT`+quoteColumns+` FROM pricing_quotes
		WHERE consumer_service = $1 AND consumer_ref = $2 AND class = $3`, consumerService, consumerRef, class))
}

// GetQuote returns the quote with id, if it belongs to consumerService.
func (s *Store) GetQuote(ctx context.Context, id, consumerService string) (models.Quote, error) {
	return scanQuote(s.db.QueryRow(ctx, `SELECT`+quoteColumns+` FROM pricing_quotes
		WHERE id = $1 AND consumer_service = $2`, id, consumerService))
}

type scannable interface{ Scan(dest ...any) error }

func scanQuote(row scannable) (models.Quote, error) {
	var q models.Quote
	var request, result []byte
	err := row.Scan(&q.ID, &q.ConsumerService, &q.ConsumerRef, &q.Scope, &q.Channel, &q.Class, &q.Mode,
		&q.Place.MarketID, &q.Place.MarketCode, &q.Place.ZoneID, &q.Place.ZoneVersionID,
		&q.TariffVersionID, &q.CommissionVersionID,
		&request, &result, &q.ResultHash, &q.LegacyFareKobo, &q.IssuedAt, &q.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Quote{}, models.ErrNotFound
	}
	if err != nil {
		return models.Quote{}, fmt.Errorf("quotes: scan quote: %w", err)
	}
	if err := json.Unmarshal(request, &q.Request); err != nil {
		return models.Quote{}, fmt.Errorf("quotes: decode request: %w", err)
	}
	if err := json.Unmarshal(result, &q.Result); err != nil {
		return models.Quote{}, fmt.Errorf("quotes: decode result: %w", err)
	}
	q.Place.Currency = q.Result.Currency
	return q, nil
}

// Bind records the binding, or returns the existing one for the quote.
func (s *Store) Bind(ctx context.Context, binding models.Binding) (models.Binding, error) {
	_, err := s.db.Exec(ctx, `
		INSERT INTO pricing_quote_bindings (id, quote_id, consumer_service, booking_ref, bound_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (quote_id) DO NOTHING`,
		binding.ID, binding.QuoteID, binding.ConsumerService, binding.BookingRef, binding.BoundAt)
	if err != nil {
		return models.Binding{}, fmt.Errorf("quotes: bind: %w", err)
	}
	return s.BindingFor(ctx, binding.QuoteID)
}

// BindingFor returns a quote's binding.
func (s *Store) BindingFor(ctx context.Context, quoteID string) (models.Binding, error) {
	var b models.Binding
	err := s.db.QueryRow(ctx, `
		SELECT id, quote_id, consumer_service, booking_ref, bound_at
		FROM pricing_quote_bindings WHERE quote_id = $1`, quoteID).
		Scan(&b.ID, &b.QuoteID, &b.ConsumerService, &b.BookingRef, &b.BoundAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Binding{}, models.ErrNotFound
	}
	if err != nil {
		return models.Binding{}, fmt.Errorf("quotes: read binding: %w", err)
	}
	return b, nil
}

// Reconcile records the reconciliation, or returns the existing one.
func (s *Store) Reconcile(ctx context.Context, r models.Reconciliation) (models.Reconciliation, error) {
	request, err := json.Marshal(r.Request)
	if err != nil {
		return models.Reconciliation{}, fmt.Errorf("quotes: encode request: %w", err)
	}
	result, err := json.Marshal(r.Result)
	if err != nil {
		return models.Reconciliation{}, fmt.Errorf("quotes: encode result: %w", err)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO pricing_quote_reconciliations (
			id, quote_id, request, result, result_hash, gross_kobo, provider_kobo, platform_fee_kobo,
			legacy_final_kobo, reconciled_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (quote_id) DO NOTHING`,
		r.ID, r.QuoteID, request, result, r.ResultHash, r.Result.GrossKobo, r.Result.Allocation.ProviderKobo,
		r.Result.Allocation.PlatformFeeKobo, r.LegacyFinalKobo, r.ReconciledAt)
	if err != nil {
		return models.Reconciliation{}, fmt.Errorf("quotes: reconcile: %w", err)
	}
	return s.reconciliationFor(ctx, r.QuoteID)
}

func (s *Store) reconciliationFor(ctx context.Context, quoteID string) (models.Reconciliation, error) {
	var r models.Reconciliation
	var request, result []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, quote_id, request, result, result_hash, legacy_final_kobo, reconciled_at
		FROM pricing_quote_reconciliations WHERE quote_id = $1`, quoteID).
		Scan(&r.ID, &r.QuoteID, &request, &result, &r.ResultHash, &r.LegacyFinalKobo, &r.ReconciledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Reconciliation{}, models.ErrNotFound
	}
	if err != nil {
		return models.Reconciliation{}, fmt.Errorf("quotes: read reconciliation: %w", err)
	}
	if err := json.Unmarshal(request, &r.Request); err != nil {
		return models.Reconciliation{}, fmt.Errorf("quotes: decode reconciliation request: %w", err)
	}
	if err := json.Unmarshal(result, &r.Result); err != nil {
		return models.Reconciliation{}, fmt.Errorf("quotes: decode reconciliation result: %w", err)
	}
	return r, nil
}

// ShadowSummary compares pricing's answer with what callers charged, per
// scope and class, for quotes issued since `since`.
func (s *Store) ShadowSummary(ctx context.Context, since time.Time) ([]models.ShadowSummaryRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT q.scope, q.class,
		       count(*),
		       count(q.legacy_fare_kobo),
		       count(*) FILTER (WHERE q.legacy_fare_kobo = q.gross_kobo),
		       COALESCE(sum(q.gross_kobo - q.legacy_fare_kobo), 0)::bigint,
		       COALESCE(max(abs(q.gross_kobo - q.legacy_fare_kobo)), 0)::bigint,
		       count(r.id) FILTER (WHERE r.legacy_final_kobo IS NOT NULL),
		       count(r.id) FILTER (WHERE r.legacy_final_kobo = r.gross_kobo)
		FROM pricing_quotes q
		LEFT JOIN pricing_quote_reconciliations r ON r.quote_id = q.id
		WHERE q.mode = 'SHADOW' AND q.issued_at >= $1
		GROUP BY q.scope, q.class
		ORDER BY q.scope, q.class`, since)
	if err != nil {
		return nil, fmt.Errorf("quotes: shadow summary: %w", err)
	}
	defer rows.Close()
	var out []models.ShadowSummaryRow
	for rows.Next() {
		var row models.ShadowSummaryRow
		if err := rows.Scan(&row.Scope, &row.Class, &row.Quotes, &row.Compared, &row.Matching,
			&row.TotalDifference, &row.MaxAbsDifference, &row.Reconciled, &row.ReconciledMatching); err != nil {
			return nil, fmt.Errorf("quotes: scan summary: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
