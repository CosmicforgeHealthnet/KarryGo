// Package repositories stores fuel reports, the index and its proposals.
package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/pricing-service/internal/database"
	"cosmicforge/logistics/services/pricing-service/internal/features/fuel/models"
)

// Store is the fuel feature's Postgres store.
type Store struct{ db database.Querier }

// NewStore returns a store over db.
func NewStore(db database.Querier) *Store { return &Store{db: db} }

// InsertReport stores r unless the driver already reported this fuel today.
// It returns the id that stands and whether r was a duplicate.
func (s *Store) InsertReport(ctx context.Context, r models.Report) (string, bool, error) {
	var marketID, zoneID any
	if r.MarketID != "" {
		marketID = r.MarketID
	}
	if r.ZoneID != "" {
		zoneID = r.ZoneID
	}
	var id string
	err := s.db.QueryRow(ctx, `
		INSERT INTO pricing_fuel_reports
		  (id, consumer_service, provider_ref, fuel_type, price_per_litre_kobo, lat_e6, lng_e6, market_id, zone_id, report_day, reported_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT ON CONSTRAINT pricing_fuel_reports_one_per_day DO NOTHING
		RETURNING id`,
		r.ID, r.ConsumerService, r.ProviderRef, r.FuelType, r.PricePerLitreKobo, r.LatE6, r.LngE6,
		marketID, zoneID, r.ReportDay, r.ReportedAt).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("fuel: insert report: %w", err)
	}
	err = s.db.QueryRow(ctx, `
		SELECT id FROM pricing_fuel_reports
		WHERE consumer_service = $1 AND provider_ref = $2 AND fuel_type = $3 AND report_day = $4`,
		r.ConsumerService, r.ProviderRef, r.FuelType, r.ReportDay).Scan(&id)
	if err != nil {
		return "", false, fmt.Errorf("fuel: read existing report: %w", err)
	}
	return id, true, nil
}

// MarketFuelPairs lists the markets and fuels with reports in [from, to].
func (s *Store) MarketFuelPairs(ctx context.Context, from, to time.Time) ([][2]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT market_id, fuel_type FROM pricing_fuel_reports
		WHERE market_id IS NOT NULL AND report_day BETWEEN $1 AND $2
		ORDER BY market_id, fuel_type`, from, to)
	if err != nil {
		return nil, fmt.Errorf("fuel: list markets: %w", err)
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var pair [2]string
		if err := rows.Scan(&pair[0], &pair[1]); err != nil {
			return nil, err
		}
		out = append(out, pair)
	}
	return out, rows.Err()
}

// Prices returns one price per driver for a market and fuel in [from, to]: the
// latest day each driver reported, so a driver who reports daily counts once.
func (s *Store) Prices(ctx context.Context, marketID, fuelType string, from, to time.Time) ([]int64, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ON (consumer_service, provider_ref) price_per_litre_kobo
		FROM pricing_fuel_reports
		WHERE market_id = $1 AND fuel_type = $2 AND report_day BETWEEN $3 AND $4
		ORDER BY consumer_service, provider_ref, report_day DESC`, marketID, fuelType, from, to)
	if err != nil {
		return nil, fmt.Errorf("fuel: read prices: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var p int64
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpsertIndex records (or recomputes) an index row.
func (s *Store) UpsertIndex(ctx context.Context, i models.Index) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO pricing_fuel_index (market_id, fuel_type, index_day, median_kobo, report_count, dropped_count, window_days, computed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (market_id, fuel_type, index_day) DO UPDATE
		SET median_kobo = EXCLUDED.median_kobo, report_count = EXCLUDED.report_count,
		    dropped_count = EXCLUDED.dropped_count, window_days = EXCLUDED.window_days, computed_at = now()`,
		i.MarketID, i.FuelType, i.Day, i.MedianKobo, i.ReportCount, i.DroppedCount, i.WindowDays)
	if err != nil {
		return fmt.Errorf("fuel: upsert index: %w", err)
	}
	return nil
}

// IndexesForDay returns every index computed for day.
func (s *Store) IndexesForDay(ctx context.Context, day time.Time) ([]models.Index, error) {
	rows, err := s.db.Query(ctx, `
		SELECT market_id, fuel_type, index_day, median_kobo, report_count, dropped_count, window_days
		FROM pricing_fuel_index WHERE index_day = $1 ORDER BY market_id, fuel_type`, day)
	if err != nil {
		return nil, fmt.Errorf("fuel: read indexes: %w", err)
	}
	defer rows.Close()
	var out []models.Index
	for rows.Next() {
		var i models.Index
		if err := rows.Scan(&i.MarketID, &i.FuelType, &i.Day, &i.MedianKobo, &i.ReportCount, &i.DroppedCount, &i.WindowDays); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// ProposalThatProduced returns the proposal whose drafted version is versionID,
// if any: an active tariff the index produced was set at that fuel price.
func (s *Store) ProposalThatProduced(ctx context.Context, versionID string) (models.Proposal, bool, error) {
	var p models.Proposal
	err := s.db.QueryRow(ctx, `
		SELECT id, fuel_price_kobo FROM pricing_fuel_proposals WHERE proposed_version_id = $1`, versionID).
		Scan(&p.ID, &p.FuelPriceKobo)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Proposal{}, false, nil
	}
	if err != nil {
		return models.Proposal{}, false, fmt.Errorf("fuel: read proposal: %w", err)
	}
	return p, true, nil
}

// BlockingProposal reports whether a new proposal for this market and tariff
// must wait: one is still awaiting a decision or activation, or one built on the same live
// tariff was made since cooldownSince (so a rejection is not re-proposed daily).
func (s *Store) BlockingProposal(ctx context.Context, marketID, tariffKind, baseVersionID string, cooldownSince time.Time) (bool, error) {
	var blocked bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM pricing_fuel_proposals p
		  JOIN pricing_policy_versions v ON v.id = p.proposed_version_id
		  WHERE p.market_id = $1 AND p.tariff_kind = $2
		    AND (v.status IN ('DRAFT', 'SUBMITTED')
		         OR (v.status = 'APPROVED' AND NOT EXISTS (
		               SELECT 1 FROM pricing_policy_activations a WHERE a.version_id = v.id))
		         OR (p.base_version_id = $3 AND p.created_at >= $4))
		)`, marketID, tariffKind, baseVersionID, cooldownSince).Scan(&blocked)
	if err != nil {
		return false, fmt.Errorf("fuel: check open proposals: %w", err)
	}
	return blocked, nil
}

// InsertProposal records a proposal.
func (s *Store) InsertProposal(ctx context.Context, p models.Proposal) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO pricing_fuel_proposals
		  (id, market_id, tariff_kind, fuel_type, index_day, fuel_price_kobo, baseline_kobo, change_bps, share_bps,
		   report_count, rules_version_id, base_version_id, proposed_version_id, approval_request_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		p.ID, p.MarketID, p.TariffKind, p.FuelType, p.IndexDay, p.FuelPriceKobo, p.BaselineKobo, p.ChangeBPS, p.ShareBPS,
		p.ReportCount, p.RulesVersionID, p.BaseVersionID, p.ProposedVersionID, p.ApprovalRequestID)
	if err != nil {
		return fmt.Errorf("fuel: insert proposal: %w", err)
	}
	return nil
}

// MarketChain returns the geography above a market, for policy resolution.
func (s *Store) MarketChain(ctx context.Context, marketID string) (stateID, countryCode string, err error) {
	err = s.db.QueryRow(ctx, `SELECT COALESCE(state_id, ''), country_code FROM pricing_markets WHERE id = $1`, marketID).Scan(&stateID, &countryCode)
	if err != nil {
		return "", "", fmt.Errorf("fuel: read market: %w", err)
	}
	return stateID, countryCode, nil
}
