package serviceability

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/dispatchpricing"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

var (
	ErrQuoteNotFound = errors.New("serviceability quote not found")
	ErrQuoteExpired  = errors.New("serviceability quote expired")
	ErrQuoteRejected = errors.New("serviceability quote is not approved")
	ErrQuoteMismatch = errors.New("booking does not match serviceability quote")
)

// BookingBinding is the minimal customer booking snapshot that must match the
// server-owned serviceability decision. It deliberately excludes addresses,
// receiver PII and payment fields: those are not package/route authority.
type BookingBinding struct {
	Pickup             Point
	Dropoff            Point
	PackageType        string
	PackageSize        string
	PackageWeightGrams int64
	Fragile            bool
	ServiceTier        string
}

// AuthorizeBooking performs customer ownership, expiry, decision and exact
// route/package binding before a protected booking can reserve money or begin
// matching. The quote remains the pricing authority for that booking.
func (r *Repository) AuthorizeBooking(ctx context.Context, customerID, quoteID string, binding BookingBinding, now time.Time) (Quote, error) {
	if r == nil || r.db == nil {
		return Quote{}, ErrUnavailable
	}
	quote, err := scanQuote(r.db.QueryRow(ctx, `SELECT id,customer_id::text,idempotency_key,request_fingerprint,source_type,source_ref,source_version,
		pickup_latitude,pickup_longitude,dropoff_latitude,dropoff_longitude,vehicle_class,
		decision,reason_codes,policy_version,route_snapshot,package_declaration,package_admission,
		pricing_inputs,fare_estimate_kobo,currency,expires_at,version,created_at
		FROM dispatch_serviceability_quotes WHERE id=$1 AND customer_id=$2::uuid`, strings.TrimSpace(quoteID), strings.TrimSpace(customerID)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, ErrQuoteNotFound
	}
	if err != nil {
		return Quote{}, ErrUnavailable
	}
	if err := validateBookingBinding(quote, binding, now); err != nil {
		return Quote{}, err
	}
	return quote, nil
}

func validateBookingBinding(quote Quote, binding BookingBinding, now time.Time) error {
	if quote.Expired(now.UTC()) {
		return ErrQuoteExpired
	}
	if quote.Decision != DecisionServiceable && quote.Decision != DecisionServiceableWithConditions {
		return ErrQuoteRejected
	}
	if quote.Admission.Result != AdmissionAccepted && quote.Admission.Result != AdmissionAcceptedWithConditions {
		return ErrQuoteRejected
	}
	if !samePoint(quote.Pickup, binding.Pickup) || !samePoint(quote.Dropoff, binding.Dropoff) ||
		!strings.EqualFold(strings.TrimSpace(quote.Package.PackageType), strings.TrimSpace(binding.PackageType)) ||
		dispatchpricing.NormalizeSize(quote.Package.SizeCategory) != dispatchpricing.NormalizeSize(binding.PackageSize) ||
		quote.Package.DeclaredWeightGrams != binding.PackageWeightGrams || quote.Package.Fragile != binding.Fragile ||
		dispatchpricing.NormalizeTier(pricingString(quote.PricingInputs, "service_tier")) != dispatchpricing.NormalizeTier(binding.ServiceTier) {
		return ErrQuoteMismatch
	}
	return nil
}

func samePoint(a, b Point) bool {
	return math.Abs(a.Latitude-b.Latitude) <= 0.000001 && math.Abs(a.Longitude-b.Longitude) <= 0.000001
}

func pricingString(values map[string]any, key string) string {
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}

func (r *Repository) CreateOrReplay(ctx context.Context, quote Quote) (Quote, error) {
	if r == nil || r.db == nil {
		return Quote{}, ErrUnavailable
	}
	reasons, _ := json.Marshal(quote.ReasonCodes)
	route, _ := json.Marshal(quote.Route)
	declaration, _ := json.Marshal(quote.Package)
	admission, _ := json.Marshal(quote.Admission)
	pricing, _ := json.Marshal(quote.PricingInputs)
	row := r.db.QueryRow(ctx, `INSERT INTO dispatch_serviceability_quotes(
		id,customer_id,idempotency_key,request_fingerprint,source_type,source_ref,source_version,
		pickup_latitude,pickup_longitude,dropoff_latitude,dropoff_longitude,vehicle_class,
		decision,reason_codes,policy_version,route_snapshot,package_declaration,package_admission,
		pricing_inputs,fare_estimate_kobo,currency,degraded,expires_at,version,created_at)
		VALUES($1,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
		ON CONFLICT(customer_id,idempotency_key) DO NOTHING
		RETURNING id,customer_id::text,idempotency_key,request_fingerprint,source_type,source_ref,source_version,
		pickup_latitude,pickup_longitude,dropoff_latitude,dropoff_longitude,vehicle_class,
		decision,reason_codes,policy_version,route_snapshot,package_declaration,package_admission,
		pricing_inputs,fare_estimate_kobo,currency,expires_at,version,created_at`,
		quote.ID, quote.CustomerID, quote.IdempotencyKey, quote.RequestFingerprint, quote.SourceType,
		quote.SourceRef, quote.SourceVersion, quote.Pickup.Latitude, quote.Pickup.Longitude,
		quote.Dropoff.Latitude, quote.Dropoff.Longitude, quote.VehicleClass, quote.Decision,
		reasons, quote.PolicyVersion, route, declaration, admission, pricing,
		quote.FareEstimateKobo, quote.Currency, quote.Route.Degraded, quote.ExpiresAt, quote.Version, quote.CreatedAt)
	stored, err := scanQuote(row)
	if err == nil {
		return stored, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, ErrUnavailable
	}
	existing, err := scanQuote(r.db.QueryRow(ctx, `SELECT id,customer_id::text,idempotency_key,request_fingerprint,source_type,source_ref,source_version,
		pickup_latitude,pickup_longitude,dropoff_latitude,dropoff_longitude,vehicle_class,
		decision,reason_codes,policy_version,route_snapshot,package_declaration,package_admission,
		pricing_inputs,fare_estimate_kobo,currency,expires_at,version,created_at
		FROM dispatch_serviceability_quotes WHERE customer_id=$1::uuid AND idempotency_key=$2`, quote.CustomerID, quote.IdempotencyKey))
	if err != nil {
		return Quote{}, ErrUnavailable
	}
	if existing.RequestFingerprint != quote.RequestFingerprint {
		return Quote{}, ErrFingerprintConflict
	}
	return existing, nil
}

type scanner interface{ Scan(...any) error }

func scanQuote(row scanner) (Quote, error) {
	var quote Quote
	var reasons, route, declaration, admission, pricing []byte
	err := row.Scan(&quote.ID, &quote.CustomerID, &quote.IdempotencyKey, &quote.RequestFingerprint,
		&quote.SourceType, &quote.SourceRef, &quote.SourceVersion,
		&quote.Pickup.Latitude, &quote.Pickup.Longitude, &quote.Dropoff.Latitude, &quote.Dropoff.Longitude,
		&quote.VehicleClass, &quote.Decision, &reasons, &quote.PolicyVersion, &route, &declaration,
		&admission, &pricing, &quote.FareEstimateKobo, &quote.Currency, &quote.ExpiresAt,
		&quote.Version, &quote.CreatedAt)
	if err != nil {
		return Quote{}, err
	}
	if json.Unmarshal(reasons, &quote.ReasonCodes) != nil || json.Unmarshal(route, &quote.Route) != nil ||
		json.Unmarshal(declaration, &quote.Package) != nil || json.Unmarshal(admission, &quote.Admission) != nil ||
		json.Unmarshal(pricing, &quote.PricingInputs) != nil {
		return Quote{}, ErrUnavailable
	}
	return quote, nil
}
