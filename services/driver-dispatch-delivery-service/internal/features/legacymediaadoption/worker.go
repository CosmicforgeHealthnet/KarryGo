// Package legacymediaadoption is Dispatch's owner-side caller of the Media
// legacy adoption contract (shared/go/adminmedia LegacyAdoption).
//
// Where Dispatch stores legacy media asset IDs that belong to a case:
// dispatch_hub_hold_checkpoints.media_ref holds the legacy media_assets ID of
// the hub ARRIVAL/RELEASE photo (uploaded under owner_service
// "dispatch-delivery-service", purpose "proof_image", owner_id = the mission
// reference). Each checkpoint belongs to a hub hold operation, which belongs
// to exactly one delivery recovery action (the `delivery-recoveries` admin
// case) on one trip.
//
// Other legacy references are deliberately not attested: secure-handover
// evidence (dispatch_evidence_refs), custody/commerce evidence and trip proof
// photos are not linked to an incident, delivery-recovery or return case, and
// trip proofs persist only public URLs, not asset IDs. Incidents and returns
// store no media reference at all.
//
// The worker is off unless DISPATCH_LEGACY_MEDIA_ADOPTION_ENABLED=true. It
// never invents a case or subject: the case, trip, organization and legacy
// owner reference all come from Dispatch's own committed rows, and trips
// without an organization (independent bookings) are skipped because the
// modern case evidence model is organization-scoped.
package legacymediaadoption

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"cosmicforge/logistics/shared/go/adminmedia"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "driver-dispatch-delivery-service"
const CaseKind = "delivery-recoveries"

// Hub custody photos record the parcel condition at the hub.
const Purpose = "CARGO_CONDITION"

type Adopter interface {
	Adopt(context.Context, adminmedia.LegacyAdoption) (adminmedia.LegacyAdoptionReceipt, error)
}

// Candidate is one hub checkpoint photo with its authoritative case facts.
type Candidate struct {
	LegacyAssetID, MissionRef, OrganizationID, RecoveryID, TripID string
}

func (c Candidate) Attestation() adminmedia.LegacyAdoption {
	return adminmedia.LegacyAdoption{ContractVersion: adminmedia.LegacyAdoptionContract, LegacyAssetID: c.LegacyAssetID, LegacyOwnerReference: c.MissionRef, OrganizationID: c.OrganizationID, CaseKind: CaseKind, CaseReference: c.RecoveryID, SubjectType: "TRIP", SubjectReference: c.TripID, Purpose: Purpose}
}

type Worker struct {
	DB       *pgxpool.Pool
	Media    Adopter
	Interval time.Duration
	Batch    int

	mu   sync.Mutex
	done map[string]bool
}

// candidateSQL reads only committed Dispatch rows. The mission reference is
// derived exactly as the hub upload path derives it, so Media's comparison
// with the legacy owner_id fails closed if anything changed.
// A trip with several missions has no unambiguous upload owner reference and
// is skipped rather than guessed.
const candidateSQL = `SELECT c.media_ref, CASE (SELECT count(*) FROM dispatch_delivery_missions dm WHERE dm.trip_id=t.id)
	WHEN 0 THEN 'trip:'||t.id::text
	WHEN 1 THEN (SELECT dm.id::text FROM dispatch_delivery_missions dm WHERE dm.trip_id=t.id)
	ELSE '' END, t.organization_id, r.id::text, t.id::text
FROM dispatch_hub_hold_checkpoints c
JOIN dispatch_hub_hold_operations h ON h.id=c.hub_hold_operation_id AND h.trip_id=c.trip_id
JOIN dispatch_delivery_recovery_actions r ON r.id=h.recovery_action_id AND r.trip_id=h.trip_id
JOIN trips t ON t.id=r.trip_id
WHERE COALESCE(t.organization_id,'')<>''
ORDER BY c.recorded_at, c.id
LIMIT $1`

func (w *Worker) Candidates(ctx context.Context) ([]Candidate, error) {
	batch := w.Batch
	if batch <= 0 || batch > 500 {
		batch = 100
	}
	rows, err := w.DB.Query(ctx, candidateSQL, batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.LegacyAssetID, &c.MissionRef, &c.OrganizationID, &c.RecoveryID, &c.TripID); err != nil {
			return nil, err
		}
		if id, err := uuid.Parse(c.LegacyAssetID); err != nil || id.String() != c.LegacyAssetID || c.MissionRef == "" {
			continue // not a legacy media_assets identifier
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RunOnce attests each not-yet-settled candidate. Adoption is idempotent in
// Media, so a restart simply re-attests and receives the original mapping.
// Denied/absent/conflicting outcomes are terminal for this process and
// logged; an unavailable Media is retried on the next pass.
func (w *Worker) RunOnce(ctx context.Context) (adopted int, err error) {
	if w == nil || w.DB == nil || w.Media == nil {
		return 0, errors.New("legacy media adoption not configured")
	}
	candidates, err := w.Candidates(ctx)
	if err != nil {
		return 0, err
	}
	w.mu.Lock()
	if w.done == nil {
		w.done = map[string]bool{}
	}
	w.mu.Unlock()
	for _, c := range candidates {
		w.mu.Lock()
		skip := w.done[c.LegacyAssetID]
		w.mu.Unlock()
		if skip {
			continue
		}
		_, err := w.Media.Adopt(ctx, c.Attestation())
		switch {
		case err == nil:
			adopted++
		case errors.Is(err, adminmedia.ErrDenied), errors.Is(err, adminmedia.ErrNotFound), errors.Is(err, adminmedia.ErrConflict), errors.Is(err, adminmedia.ErrInvalid):
			log.Printf("legacy media adoption refused for delivery recovery %s: %v", c.RecoveryID, err)
		default:
			continue // unavailable: retry next pass
		}
		w.mu.Lock()
		w.done[c.LegacyAssetID] = true
		w.mu.Unlock()
	}
	return adopted, nil
}

func (w *Worker) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("legacy media adoption pass failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// FromEnvironment returns nil (worker off) unless
// DISPATCH_LEGACY_MEDIA_ADOPTION_ENABLED=true. Enabled operation requires
// DISPATCH_LEGACY_MEDIA_ADOPTION_MEDIA_ORIGIN (bare Media origin) and
// DISPATCH_LEGACY_MEDIA_ADOPTION_SECRET_FILE (dedicated base64url key, at
// least 32 bytes, enrolled for this owner in Media's adoption sources).
func FromEnvironment(db *pgxpool.Pool) (*Worker, error) {
	if os.Getenv("DISPATCH_LEGACY_MEDIA_ADOPTION_ENABLED") != "true" {
		return nil, nil
	}
	raw, err := os.ReadFile(os.Getenv("DISPATCH_LEGACY_MEDIA_ADOPTION_SECRET_FILE"))
	if err != nil || len(raw) > 4096 || db == nil {
		return nil, errors.New("legacy media adoption credential unavailable")
	}
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(bytes.TrimSpace(raw))))
	if err != nil || len(key) < 32 {
		return nil, errors.New("invalid legacy media adoption credential")
	}
	client, err := adminmedia.NewLegacyAdoptionClient(Owner, os.Getenv("DISPATCH_LEGACY_MEDIA_ADOPTION_MEDIA_ORIGIN"), key)
	if err != nil {
		return nil, err
	}
	return &Worker{DB: db, Media: client}, nil
}
