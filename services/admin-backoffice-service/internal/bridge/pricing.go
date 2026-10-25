package bridge

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/clients"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
)

func (b *Bridge) ConfigurePricing(client *clients.Pricing, policy *adminpolicy.Engine, reads, writes bool) error {
	if client == nil {
		return errors.New("pricing client required")
	}
	if b.pricing != nil {
		return errors.New("duplicate pricing configuration")
	}
	b.pricing, b.pricingPolicy, b.pricingReads, b.pricingWrites = client, policy, reads, writes
	return nil
}

type pricingInput struct {
	IdempotencyKey    string          `json:"idempotency_key"`
	ExpectedBodyHash  string          `json:"expected_body_hash"`
	Kind              string          `json:"kind"`
	GeographyLevel    string          `json:"geography_level"`
	GeographyRef      string          `json:"geography_ref"`
	SchemaVersion     int             `json:"schema_version"`
	Body              json.RawMessage `json:"body"`
	CloneFrom         string          `json:"clone_from"`
	EffectiveFrom     string          `json:"effective_from"`
	Rollback          bool            `json:"rollback"`
	VersionID         string          `json:"version_id"`
	CommissionVersion string          `json:"commission_version_id"`
	Scope             string          `json:"scope"`
	Channel           string          `json:"channel"`
	Class             string          `json:"class"`
	DistanceMeters    int64           `json:"distance_meters"`
	DurationSeconds   int64           `json:"duration_seconds"`
	TrafficSeconds    int64           `json:"traffic_duration_seconds"`
	RouteConfidence   string          `json:"route_confidence"`
	RouteDegraded     bool            `json:"route_degraded"`
	PackageSize       string          `json:"package_size"`
	ServiceTier       string          `json:"service_tier"`
	WeightGrams       int64           `json:"weight_grams"`
	Helpers           int64           `json:"helpers"`
	ZoneID            string          `json:"zone_id"`
	MarketID          string          `json:"market_id"`
	StateID           string          `json:"state_id"`
	CountryCode       string          `json:"country_code"`
	AsOf              string          `json:"as_of"`
	Capability        string          `json:"capability"`
	MarketCode        string          `json:"market_code"`
	Mode              string          `json:"mode"`
}

func pricingTarget(path, prefix, suffix string) string {
	value := strings.TrimPrefix(path, prefix)
	return strings.TrimSuffix(value, suffix)
}

func (b *Bridge) servePricing(w http.ResponseWriter, r *http.Request, body []byte, actor security.Claims, audit persistence.Audit, query adminread.Query, capabilityID, path, correlation string) {
	if b.pricing == nil || !b.pricingReads {
		b.pricingFailure(w, r, audit, correlation, 503, "capability_blocked")
		return
	}
	operation := clients.PricingOperation{Method: r.Method, Staff: actor.Subject, Reason: audit.ReasonCode, IdempotencyKey: correlation, Scope: query.Scope}
	input := pricingInput{}
	if r.Method != http.MethodGet {
		if (!b.pricingWrites || !b.features.RemoteWrites || !b.features.PricingWrites) && capabilityID != "pricing.simulate" && capabilityID != "pricing.resolve" {
			b.pricingFailure(w, r, audit, correlation, 503, "capability_blocked")
			return
		}
		if err := security.Strict(body, &input); err != nil {
			b.pricingFailure(w, r, audit, correlation, 400, "invalid_command")
			return
		}
		write := capabilityID != "pricing.simulate" && capabilityID != "pricing.resolve"
		if write && !opaquePricingInput(input.IdempotencyKey) {
			b.pricingFailure(w, r, audit, correlation, 400, "invalid_idempotency_key")
			return
		}
		if write {
			operation.IdempotencyKey = input.IdempotencyKey
		}
		if strings.HasPrefix(capabilityID, "pricing.policy.") && capabilityID != "pricing.policy.draft.create" && capabilityID != "pricing.policy.versions.read" && capabilityID != "pricing.policy.version.read" {
			decoded, err := hex.DecodeString(input.ExpectedBodyHash)
			if err != nil || len(decoded) != 32 || input.ExpectedBodyHash != strings.ToLower(input.ExpectedBodyHash) {
				b.pricingFailure(w, r, audit, correlation, 400, "invalid_source_version")
				return
			}
		}
	}
	payload := map[string]any{}
	switch capabilityID {
	case "pricing.policy.versions.read":
		operation.Path, operation.Target = "policy.versions", pricingTarget(path, "/pricing/policies/", "/versions")
	case "pricing.policy.version.read":
		operation.Path, operation.Target = "policy.version", strings.TrimPrefix(path, "/pricing/policy-versions/")
	case "pricing.policy.draft.create":
		operation.Path = "policies/drafts"
		payload = map[string]any{"performed_by": actor.Subject, "reason": audit.ReasonCode, "kind": input.Kind, "geography_level": input.GeographyLevel, "geography_ref": input.GeographyRef, "schema_version": input.SchemaVersion, "body": input.Body, "clone_from": input.CloneFrom, "idempotency_key": operation.IdempotencyKey}
	case "pricing.policy.draft.update":
		operation.Path, operation.Target = "policy.version", strings.TrimPrefix(path, "/pricing/policy-versions/")
		payload = map[string]any{"performed_by": actor.Subject, "reason": audit.ReasonCode, "body": input.Body, "expected_body_hash": input.ExpectedBodyHash, "idempotency_key": operation.IdempotencyKey}
	case "pricing.policy.submit", "pricing.policy.approve", "pricing.policy.reject", "pricing.policy.activate":
		action := strings.TrimPrefix(capabilityID, "pricing.policy.")
		operation.Path = "policy." + action
		operation.Target = pricingTarget(path, "/pricing/policy-versions/", "/"+action)
		payload = map[string]any{"performed_by": actor.Subject, "reason": audit.ReasonCode, "expected_body_hash": input.ExpectedBodyHash, "idempotency_key": operation.IdempotencyKey}
		if action == "activate" {
			payload["effective_from"], payload["rollback"] = input.EffectiveFrom, input.Rollback
		}
	case "pricing.simulate":
		operation.Path = "simulate"
		payload = map[string]any{"version_id": input.VersionID, "commission_version_id": input.CommissionVersion, "scope": input.Scope, "channel": input.Channel, "class": input.Class, "distance_meters": input.DistanceMeters, "duration_seconds": input.DurationSeconds, "traffic_duration_seconds": input.TrafficSeconds, "route_confidence": input.RouteConfidence, "route_degraded": input.RouteDegraded, "package_size": input.PackageSize, "service_tier": input.ServiceTier, "weight_grams": input.WeightGrams, "helpers": input.Helpers}
	case "pricing.resolve":
		operation.Path = "resolve"
		if input.ZoneID != "" && input.ZoneID != query.Scope.Zone || input.MarketID != "" && input.MarketID != query.Scope.Market {
			b.pricingFailure(w, r, audit, correlation, 403, "scope_mismatch")
			return
		}
		payload = map[string]any{"kind": input.Kind, "zone_id": query.Scope.Zone, "market_id": query.Scope.Market, "state_id": input.StateID, "country_code": input.CountryCode, "as_of": input.AsOf}
	case "pricing.approvals.read":
		operation.Path = "approvals"
	case "pricing.rollouts.read":
		operation.Path = "rollouts"
	case "pricing.rollouts.manage":
		operation.Path = "rollouts"
		payload = map[string]any{"performed_by": actor.Subject, "reason": audit.ReasonCode, "scope": input.Scope, "capability": input.Capability, "market_code": input.MarketCode, "mode": input.Mode, "idempotency_key": operation.IdempotencyKey}
	case "pricing.market.freeze.read":
		operation.Path, operation.Target = "market.freeze", pricingTarget(path, "/pricing/markets/", "/freeze")
	case "pricing.market.freeze", "pricing.market.unfreeze":
		action := strings.TrimPrefix(capabilityID, "pricing.market.")
		operation.Path = "market." + action
		operation.Target = pricingTarget(path, "/pricing/markets/", "/"+action)
		payload = map[string]any{"performed_by": actor.Subject, "reason": audit.ReasonCode, "idempotency_key": operation.IdempotencyKey}
	default:
		b.pricingFailure(w, r, audit, correlation, 404, "not_found")
		return
	}
	if r.Method != http.MethodGet {
		operation.Payload, _ = json.Marshal(payload)
	}
	audit.TargetRef = operation.Target
	result, err := b.pricing.Execute(r.Context(), operation)
	if err != nil {
		status, code := 503, "owner_unavailable"
		switch {
		case errors.Is(err, clients.ErrInvalid):
			status, code = 400, "owner_contract_invalid"
		case errors.Is(err, clients.ErrNotFound):
			status, code = 404, "not_found"
		case errors.Is(err, clients.ErrScopeDenied):
			status, code = 403, "forbidden"
		case errors.Is(err, clients.ErrRefused):
			status, code = 409, "owner_refused"
		}
		b.pricingFailure(w, r, audit, correlation, status, code)
		return
	}
	audit.Outcome = "AUTHORIZED"
	if !b.audit(r, audit) {
		b.reply(w, correlation, 503, nil, &apiError{Code: "audit_unavailable", Message: "Request could not be completed.", Retryable: true})
		return
	}
	b.reply(w, correlation, 200, result, nil)
}

func opaquePricingInput(value string) bool {
	if len(value) < 1 || len(value) > 128 || value != strings.TrimSpace(value) {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("._:@-", char)) {
			return false
		}
	}
	return true
}

func (b *Bridge) pricingFailure(w http.ResponseWriter, r *http.Request, audit persistence.Audit, correlation string, status int, code string) {
	audit.Outcome = code
	if !b.audit(r, audit) {
		status, code = 503, "audit_unavailable"
	}
	b.reply(w, correlation, status, nil, &apiError{Code: code, Message: "Request could not be completed.", Retryable: status == 503 && code != "capability_blocked"})
}

func (b *Bridge) pricingAvailable(permission string, actor security.Claims) bool {
	if b.pricing == nil || !b.pricingReads || b.pricingPolicy == nil {
		return false
	}
	rule, err := b.pricingPolicy.Rule(permission, b.now())
	readOnly := permission == "pricing.policy.read" || permission == "pricing.approval.read" || permission == "pricing.rollout.read" || permission == "pricing.market_freeze.read" || permission == "pricing.simulate" || permission == "pricing.resolve"
	return err == nil && rule.Owner == clients.PricingOwner && rule.SourceContract == "pricing.admin.v1" && hasRole(actor.Roles, rule.Roles) && len(actor.Zones) > 0 && len(actor.Tenants) > 0 && (readOnly || b.pricingWrites && b.features.RemoteWrites && b.features.PricingWrites)
}
