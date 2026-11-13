package usecases

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/enforcement"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
)

// OpportunityConfig controls how a source opportunity becomes organization
// offers (Sections 11-12, spec 26.3 "platform.request.offered.v1: create /
// update organization opportunity projection").
type OpportunityConfig struct {
	// Enabled turns creation on. When off, an offered event for a request no
	// organization has a projection of is acknowledged as IGNORED, exactly as
	// before this capability existed.
	Enabled bool
	// MaxOrganizations bounds the fan-out of one source request.
	MaxOrganizations int
	// MaxRecipientsPerOrganization bounds the providers a delegated offer is
	// put in front of.
	MaxRecipientsPerOrganization int
	// MinOfferTTL is the shortest response window an organization is given.
	// A source broadcast window is usually much shorter than a human
	// dispatcher needs; the source stays authoritative regardless, because
	// every acceptance is a source reservation that fails once the request
	// is gone.
	MinOfferTTL time.Duration
	// MaxOfferTTL caps the response window.
	MaxOfferTTL time.Duration
}

func (c OpportunityConfig) withDefaults() OpportunityConfig {
	if c.MaxOrganizations <= 0 {
		c.MaxOrganizations = 50
	}
	if c.MaxRecipientsPerOrganization <= 0 {
		c.MaxRecipientsPerOrganization = 50
	}
	if c.MinOfferTTL <= 0 {
		c.MinOfferTTL = 2 * time.Minute
	}
	if c.MaxOfferTTL <= 0 {
		c.MaxOfferTTL = 15 * time.Minute
	}
	if c.MaxOfferTTL < c.MinOfferTTL {
		c.MaxOfferTTL = c.MinOfferTTL
	}
	return c
}

// errNoEligibleOrganization reports that an offered request reached no
// organization. It rolls the ingestion transaction back (nothing to record)
// and is acknowledged to the producer as IGNORED.
var errNoEligibleOrganization = errors.New("marketplace: no eligible organization for opportunity")

// SourceCategory maps an authenticated producer name, or an already
// normalized category, onto the marketplace source category stored on every
// projection, offer, assignment and trip.
//
// The event endpoint authenticates callers by service name
// ("driver-dispatch-delivery-service") while every marketplace table stores
// the category ("DISPATCH"). Before this mapping existed, a real delivery from
// Dispatch could never match a projection, and a trip event could never be
// stored, because the CHECK constraint rejects a service name.
func SourceCategory(service string) string {
	switch strings.TrimSpace(service) {
	case models.SourceDispatch, "driver-dispatch-delivery-service":
		return models.SourceDispatch
	case models.SourceTaxi, "driver-taxi-service":
		return models.SourceTaxi
	case models.SourceHauling, "driver-hauling-service":
		return models.SourceHauling
	default:
		return ""
	}
}

// opportunityRequirementKeys is the allowlist of operational facts copied
// from the source payload onto the organization projection.
//
// Allowlisted, never copied wholesale: the source booking carries the customer
// fare, contact details and settlement terms, none of which an organization's
// dispatcher (and certainly none of its providers) may see.
var opportunityRequirementKeys = []string{
	"attemptNumber", "broadcastRadiusKm", "vehicleType", "vehicleClass", "truckType",
	"packageSize", "packageType", "weightKg", "weightCategory", "helpersRequired",
	"passengerCount", "serviceTier", "scheduledFor", "isFragile",
}

// opportunityLocationKeys is the allowlist for pickup/drop-off summaries:
// coarse operational location only, never a person's name or phone number.
var opportunityLocationKeys = []string{"area", "city", "region", "state", "country",
	"latitude", "longitude", "lat", "lng", "geohash"}

func allowlisted(source map[string]any, keys []string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		value, ok := source[key]
		if !ok || value == nil {
			continue
		}
		switch value.(type) {
		case string, bool, float64, float32, int, int64, int32:
			out[key] = value
		}
	}
	return out
}

func payloadMap(payload map[string]any, key string) map[string]any {
	if value, ok := payload[key].(map[string]any); ok {
		return value
	}
	return map[string]any{}
}

func payloadStrings(payload map[string]any, keys ...string) []string {
	var out []string
	for _, key := range keys {
		switch value := payload[key].(type) {
		case []any:
			for _, item := range value {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					out = append(out, strings.TrimSpace(text))
				}
			}
		case []string:
			for _, text := range value {
				if strings.TrimSpace(text) != "" {
					out = append(out, strings.TrimSpace(text))
				}
			}
		}
	}
	return out
}

// offerDeadline chooses the organization's response deadline.
func (s *Service) offerDeadline(sourceExpiry *time.Time) time.Time {
	cfg := s.opts.Opportunities.withDefaults()
	now := s.opts.Now().UTC()
	deadline := now.Add(cfg.MinOfferTTL)
	if sourceExpiry != nil && sourceExpiry.After(deadline) {
		deadline = sourceExpiry.UTC()
	}
	if limit := now.Add(cfg.MaxOfferTTL); deadline.After(limit) {
		deadline = limit
	}
	return deadline
}

// createOpportunities opens one projection and one offer per eligible
// organization for a request no organization holds yet.
//
// Eligibility is decided by the existing eligibility layers, never by new
// rules invented here:
//   - the durable company gate (enforcement.Company, operation ASSIGNMENT)
//     records an auditable decision that the organization may take work in
//     this category; an organization it refuses — or cannot vouch for — is
//     skipped, never offered;
//   - the marketplace EligibilityChecker decides, per managed provider,
//     whether the membership can carry work in this category right now.
//
// An organization with no eligible provider gets no offer: an opportunity no
// one in the organization could execute is noise for its dispatchers.
func (s *Service) createOpportunities(ctx context.Context, tx pgx.Tx, category string,
	event InboundEvent) (repositories.SourceEventOutcome, error) {
	cfg := s.opts.Opportunities.withDefaults()
	requestID := sourceRequestID(event)
	if strings.TrimSpace(requestID) == "" {
		return "", codedBadRequest("invalid_envelope", "Offered events must identify the request.")
	}

	candidates, err := s.opts.Projections.ListOpportunityCandidates(ctx, tx, category,
		payloadStrings(event.Payload, "eligibleOrganizationIds", "organizationIds"),
		payloadStrings(event.Payload, "eligibleProviderIds", "providerIds"),
		cfg.MaxOrganizations*cfg.MaxRecipientsPerOrganization)
	if err != nil {
		return "", err
	}

	byOrganization := map[string][]repositories.OpportunityCandidate{}
	var organizations []string
	for _, candidate := range candidates {
		if _, seen := byOrganization[candidate.OrganizationID]; !seen {
			organizations = append(organizations, candidate.OrganizationID)
		}
		byOrganization[candidate.OrganizationID] = append(byOrganization[candidate.OrganizationID], candidate)
	}
	sort.Strings(organizations)
	if len(organizations) > cfg.MaxOrganizations {
		organizations = organizations[:cfg.MaxOrganizations]
	}

	sourceExpiry := payloadTime(event.Payload, "expiresAt")
	deadline := s.offerDeadline(sourceExpiry)
	created := 0
	for _, organizationID := range organizations {
		ok, err := s.openOpportunity(ctx, tx, category, requestID, event, organizationID,
			byOrganization[organizationID], deadline, sourceExpiry, cfg)
		if err != nil {
			return "", err
		}
		if ok {
			created++
		}
	}
	s.opts.Metrics.IncCounter("fleet_partner_marketplace_opportunities_total",
		map[string]string{"source": category}, float64(created))
	if created == 0 {
		return "", errNoEligibleOrganization
	}
	return repositories.SourceEventApplied, nil
}

// openOpportunity evaluates and, when eligible, opens one organization's
// opportunity. It returns false without error for an ineligible organization.
func (s *Service) openOpportunity(ctx context.Context, tx pgx.Tx, category string,
	requestID string, event InboundEvent, organizationID string,
	candidates []repositories.OpportunityCandidate, deadline time.Time,
	sourceExpiry *time.Time, cfg OpportunityConfig) (bool, error) {
	// Organization-level durable decision. A refusal or an unreachable
	// authority means "not offered"; it never fails the whole event, because
	// one organization's eligibility must not decide whether another is
	// offered the work.
	subject := enforcement.Company(organizationID, eligibilitymodels.OperationAssignment)
	subject.ServiceCategory = category
	subject.ActorType = eligibilitymodels.ActorSystem
	subject.ActorID = "marketplace-opportunity"
	subject.CorrelationID = event.CorrelationID
	subject.CausationID = event.EventID
	if _, err := enforcement.Require(ctx, s.opts.DurableEligibility, subject); err != nil {
		s.opts.Metrics.IncCounter("fleet_partner_marketplace_opportunity_skipped_total",
			map[string]string{"source": category, "reason": "organization_not_eligible"}, 1)
		return false, nil
	}

	var eligible []string
	for _, candidate := range candidates {
		if s.opts.Eligibility != nil {
			_, decision, err := s.opts.Eligibility.CheckMembership(ctx, MembershipInput{
				OrganizationID:  organizationID,
				MembershipID:    candidate.MembershipID,
				SourceService:   category,
				ServiceCategory: category,
				SourceRequestID: requestID,
				RequiredUntil:   deadline,
				SkipSourceCheck: true,
			})
			if err != nil {
				return false, err
			}
			if !decision.Eligible {
				continue
			}
		}
		eligible = append(eligible, candidate.MembershipID)
		if len(eligible) >= cfg.MaxRecipientsPerOrganization {
			break
		}
	}
	if len(eligible) == 0 {
		s.opts.Metrics.IncCounter("fleet_partner_marketplace_opportunity_skipped_total",
			map[string]string{"source": category, "reason": "no_eligible_provider"}, 1)
		return false, nil
	}

	mode := models.ModeCentralized
	if s.opts.OperatingModes != nil {
		resolved, err := s.opts.OperatingModes.OperatingMode(ctx, organizationID)
		if err != nil {
			// Without a known operating mode nobody can say whether a
			// dispatcher or a provider may accept, so the organization is not
			// offered the work. One organization's missing settings must not
			// fail the delivery for every other organization.
			s.opts.Metrics.IncCounter("fleet_partner_marketplace_opportunity_skipped_total",
				map[string]string{"source": category, "reason": "operating_mode_unavailable"}, 1)
			return false, nil
		}
		if resolved != "" {
			mode = resolved
		}
	}

	now := s.opts.Now().UTC()
	eventID := event.EventID
	occurred := event.OccurredAt
	projection := &models.RequestProjection{
		OrganizationID:    organizationID,
		SourceService:     category,
		PlatformRequestID: requestID,
		ServiceType:       firstNonEmpty(payloadString(event.Payload, "serviceType"), strings.ToLower(category)),
		SourceStatus:      payloadString(event.Payload, "status"),
		PickupSummary:     allowlisted(payloadMap(event.Payload, "pickupSummary"), opportunityLocationKeys),
		DropoffSummary:    allowlisted(payloadMap(event.Payload, "dropoffSummary"), opportunityLocationKeys),
		OperationalRequirements: mergeMaps(
			allowlisted(event.Payload, opportunityRequirementKeys),
			allowlisted(payloadMap(event.Payload, "operationalRequirements"), opportunityRequirementKeys)),
		OfferedAt:         now,
		ExpiresAt:         sourceExpiry,
		SourceVersion:     event.AggregateVersion,
		LastEventID:       &eventID,
		LastSourceEventAt: &occurred,
	}
	if booking := payloadString(event.Payload, "bookingId"); booking != "" {
		projection.PlatformBookingID = &booking
	}
	created, err := s.opts.Projections.InsertOpportunity(ctx, tx, projection)
	if err != nil {
		return false, err
	}
	if !created {
		// Another delivery already opened this organization's opportunity.
		return false, nil
	}

	offer := &models.Offer{
		RequestProjectionID: projection.ID,
		OperatingMode:       mode,
		OfferedAt:           now,
		ExpiresAt:           deadline,
	}
	if err := s.opts.Offers.Create(ctx, tx, organizationID, offer); err != nil {
		return false, err
	}
	recipients := 0
	if models.AllowsProviderDirectAcceptance(mode) {
		for _, membershipID := range eligible {
			if err := s.opts.Offers.CreateRecipient(ctx, tx, organizationID, &models.OfferRecipient{
				OfferID: offer.ID, MembershipID: membershipID, ExpiresAt: deadline,
			}); err != nil {
				return false, err
			}
			recipients++
		}
	}

	entry := audit.Entry{
		OrganizationID: organizationID,
		ActorType:      "system",
		ActorID:        "marketplace-opportunity",
		Action:         "marketplace.opportunity_received",
		ResourceType:   "marketplace_request",
		ResourceID:     projection.ID,
		NewValues: map[string]any{
			"offerId":           offer.ID,
			"sourceService":     category,
			"platformRequestId": requestID,
			"operatingMode":     mode,
			"recipientCount":    recipients,
			"expiresAt":         deadline,
		},
		CorrelationID: event.CorrelationID,
		CausationID:   event.EventID,
	}
	if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
		return false, err
	}
	if s.opts.Events != nil {
		if err := s.opts.Events.Publish(ctx, tx, "organization.offer.received.v1", "offer", offer.ID,
			offer.Version, organizationID, event.CorrelationID, map[string]any{
				"offerId":             offer.ID,
				"requestProjectionId": projection.ID,
				"sourceService":       category,
				"platformRequestId":   requestID,
				"operatingMode":       mode,
				"recipientCount":      recipients,
				"expiresAt":           deadline,
			}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mergeMaps(maps ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range maps {
		for key, value := range m {
			out[key] = value
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Offer expiry (Section 27 required worker "offer-expiry", spec 62)
// ---------------------------------------------------------------------------

// ExpireOffers expires OFFERED organization offers whose deadline passed.
//
// Each claimed offer is re-checked under its row lock (an acceptance holds the
// same lock and changes the status first), closes its still-pending provider
// recipients, moves its projection from OFFERED to EXPIRED, and records an
// audit entry and an organization.offer.expired.v1 event in the same
// transaction. The source request is untouched: the source owns it, and an
// organization letting its window lapse does not end the request for anyone
// else.
func (s *Service) ExpireOffers(ctx context.Context, limit int) (int, error) {
	expired := 0
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		now := s.opts.Now().UTC()
		due, err := s.opts.Offers.ClaimDueOffers(ctx, tx, now, limit)
		if err != nil {
			return err
		}
		for index := range due {
			offer := due[index]
			if offer.Status != models.OfferOffered || now.Before(offer.ExpiresAt) {
				continue
			}
			if err := s.opts.Offers.SetStatus(ctx, tx, offer.OrganizationID, offer.ID,
				models.OfferExpired, offer.Version, "SYSTEM", "offer-expiry",
				"offer_deadline_elapsed", ""); err != nil {
				return err
			}
			closed, err := s.opts.Offers.CloseOpenRecipients(ctx, tx, offer.OrganizationID,
				offer.ID, models.RecipientExpired)
			if err != nil {
				return err
			}
			// Tenant revalidation: the projection is loaded inside the offer's
			// own organization. An offer whose projection is not in that
			// organization is corrupt data and is left for an operator rather
			// than "repaired" across a tenant boundary.
			projection, err := s.opts.Projections.LockByID(ctx, tx, offer.OrganizationID,
				offer.RequestProjectionID)
			if err != nil && !errors.Is(err, repositories.ErrNotFound) {
				return err
			}
			if projection != nil && projection.ProjectionStatus == models.ProjectionOffered {
				if err := s.opts.Projections.SetStatus(ctx, tx, offer.OrganizationID, projection.ID,
					models.ProjectionExpired, projection.Version); err != nil {
					return err
				}
			}
			if err := s.opts.Audit.Record(ctx, tx, audit.Entry{
				OrganizationID: offer.OrganizationID,
				ActorType:      "system",
				ActorID:        "offer-expiry",
				Action:         "marketplace.offer_expired",
				ResourceType:   "marketplace_offer",
				ResourceID:     offer.ID,
				PreviousValues: map[string]any{"status": models.OfferOffered},
				NewValues: map[string]any{"status": models.OfferExpired,
					"recipientsExpired": closed, "expiresAt": offer.ExpiresAt},
			}); err != nil {
				return err
			}
			if s.opts.Events != nil {
				if err := s.opts.Events.Publish(ctx, tx, "organization.offer.expired.v1", "offer",
					offer.ID, offer.Version+1, offer.OrganizationID, "", map[string]any{
						"offerId":             offer.ID,
						"requestProjectionId": offer.RequestProjectionID,
						"expiredAt":           now,
					}); err != nil {
					return err
				}
			}
			expired++
		}
		return nil
	})
	if err != nil {
		return expired, err
	}
	if expired > 0 {
		s.opts.Metrics.IncCounter("fleet_partner_marketplace_offer_expiries_total", nil, float64(expired))
	}
	return expired, nil
}
