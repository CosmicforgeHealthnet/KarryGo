// Package repositories persists pricing geography.
package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/pricing-service/internal/database"
	"cosmicforge/logistics/services/pricing-service/internal/features/geography/models"
)

// Store reads and writes geography.
type Store struct{ db database.Querier }

// NewStore returns a store over db.
func NewStore(db database.Querier) *Store { return &Store{db: db} }

// UpsertCountry records a country.
func (s *Store) UpsertCountry(ctx context.Context, country models.Country) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO pricing_countries (code, name, currency) VALUES ($1, $2, $3)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, currency = EXCLUDED.currency`,
		country.Code, country.Name, country.Currency)
	if err != nil {
		return fmt.Errorf("geography: upsert country: %w", err)
	}
	return nil
}

// UpsertMarket records a market, keyed by its code so the seed is re-runnable.
func (s *Store) UpsertMarket(ctx context.Context, market models.Market) (models.Market, error) {
	if err := models.ValidateMarket(market); err != nil {
		return models.Market{}, err
	}
	var stateID any
	if market.StateID != "" {
		stateID = market.StateID
	}
	timezone := market.Timezone
	if timezone == "" {
		timezone = "Africa/Lagos"
	}

	var stored models.Market
	var storedState *string
	err := s.db.QueryRow(ctx, `
		INSERT INTO pricing_markets (id, country_code, state_id, code, name, timezone)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, timezone = EXCLUDED.timezone
		RETURNING id, country_code, state_id, code, name, timezone, status, created_at`,
		market.ID, market.CountryCode, stateID, market.Code, market.Name, timezone,
	).Scan(&stored.ID, &stored.CountryCode, &storedState, &stored.Code, &stored.Name,
		&stored.Timezone, &stored.Status, &stored.CreatedAt)
	if err != nil {
		return models.Market{}, fmt.Errorf("geography: upsert market: %w", err)
	}
	if storedState != nil {
		stored.StateID = *storedState
	}
	return stored, nil
}

// MarketByCode finds a market by the code the booking services already use.
func (s *Store) MarketByCode(ctx context.Context, code string) (models.Market, error) {
	var market models.Market
	var stateID *string
	err := s.db.QueryRow(ctx, `
		SELECT id, country_code, state_id, code, name, timezone, status, created_at
		FROM pricing_markets WHERE code = $1`, code,
	).Scan(&market.ID, &market.CountryCode, &stateID, &market.Code, &market.Name,
		&market.Timezone, &market.Status, &market.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Market{}, fmt.Errorf("%w: market %s", models.ErrNotFound, code)
	}
	if err != nil {
		return models.Market{}, fmt.Errorf("geography: market by code: %w", err)
	}
	if stateID != nil {
		market.StateID = *stateID
	}
	return market, nil
}

// UpsertZone records a zone within a market.
func (s *Store) UpsertZone(ctx context.Context, zone models.Zone) (models.Zone, error) {
	var stored models.Zone
	err := s.db.QueryRow(ctx, `
		INSERT INTO pricing_zones (id, market_id, code, name) VALUES ($1, $2, $3, $4)
		ON CONFLICT (market_id, code) DO UPDATE SET name = EXCLUDED.name
		RETURNING id, market_id, code, name, status`,
		zone.ID, zone.MarketID, zone.Code, zone.Name,
	).Scan(&stored.ID, &stored.MarketID, &stored.Code, &stored.Name, &stored.Status)
	if err != nil {
		return models.Zone{}, fmt.Errorf("geography: upsert zone: %w", err)
	}
	return stored, nil
}

// AddZoneVersion records a new immutable shape for a zone.
func (s *Store) AddZoneVersion(ctx context.Context, version models.ZoneVersion) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO pricing_zone_versions (id, zone_id, version_no, shape, shape_hash, created_by)
		VALUES ($1, $2, COALESCE((SELECT MAX(version_no) FROM pricing_zone_versions WHERE zone_id = $2), 0) + 1, $3, $4, $5)`,
		version.ID, version.ZoneID, version.Shape, version.ShapeHash, version.CreatedBy)
	if err != nil {
		return fmt.Errorf("geography: add zone version: %w", err)
	}
	return nil
}

// MapConsumerZone records that a consuming service's zone is priced as a pricing
// zone from a moment. A remap is a new row, never an edit, so a past quote still
// resolves the mapping that applied then.
func (s *Store) MapConsumerZone(ctx context.Context, id, consumerService, consumerZoneID, pricingZoneID, createdBy string, validFrom time.Time) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO pricing_zone_mappings (id, consumer_service, consumer_zone_id, pricing_zone_id, valid_from, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (consumer_service, consumer_zone_id, valid_from) DO NOTHING`,
		id, consumerService, consumerZoneID, pricingZoneID, validFrom, createdBy)
	if err != nil {
		return fmt.Errorf("geography: map consumer zone: %w", err)
	}
	return nil
}

// PricingZoneFor resolves a consuming service's zone to a pricing zone as it
// stood at asOf.
func (s *Store) PricingZoneFor(ctx context.Context, consumerService, consumerZoneID string, asOf time.Time) (string, error) {
	var zoneID string
	err := s.db.QueryRow(ctx, `
		SELECT pricing_zone_id FROM pricing_zone_mappings
		WHERE consumer_service = $1 AND consumer_zone_id = $2 AND valid_from <= $3
		ORDER BY valid_from DESC
		LIMIT 1`, consumerService, consumerZoneID, asOf).Scan(&zoneID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Not an error: many callers have no zone mapping, and pricing then
		// resolves at market level. Refusing here would make a zone mapping
		// mandatory for every consumer.
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("geography: resolve pricing zone: %w", err)
	}
	return zoneID, nil
}
