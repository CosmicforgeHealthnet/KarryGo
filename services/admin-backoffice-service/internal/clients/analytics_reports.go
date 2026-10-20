package clients

// These typed methods cover the existing ten owner contracts. Their legacy
// responses are NOT authorized HQ cohorts: they lack complete tenant/geographic
// provenance and reviewed suppression. They remain unwired to remote routes.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type RecoveryDay struct {
	Authority                string    `json:"authority"`
	Date                     time.Time `json:"date"`
	Started                  int64     `json:"started"`
	Replaced                 int64     `json:"replaced"`
	Resolved                 int64     `json:"resolved"`
	Failed                   int64     `json:"failed"`
	AverageResolutionSeconds float64   `json:"average_resolution_seconds"`
}
type MismatchDay struct {
	Authority  string    `json:"authority"`
	Date       time.Time `json:"date"`
	ReasonCode string    `json:"reason_code"`
	Mismatches int64     `json:"mismatches"`
}
type DeclineDay struct {
	Authority  string    `json:"authority"`
	Date       time.Time `json:"date"`
	ReasonCode string    `json:"reason_code"`
	Safety     bool      `json:"safety"`
	Declines   int64     `json:"declines"`
}
type DemandDay struct {
	Authority string    `json:"authority"`
	Zone      string    `json:"zone"`
	Date      time.Time `json:"date"`
	Requests  int64     `json:"requests"`
	Unmatched int64     `json:"unmatched"`
}
type MissionOutcomeDay struct {
	Authority         string    `json:"authority"`
	ProductCode       string    `json:"product_code"`
	Date              time.Time `json:"date"`
	Committed         int64     `json:"committed"`
	Assigned          int64     `json:"assigned"`
	Completed         int64     `json:"completed"`
	Failed            int64     `json:"failed"`
	Cancelled         int64     `json:"cancelled"`
	Recovered         int64     `json:"recovered"`
	RecoveryFailed    int64     `json:"recovery_failed"`
	GuaranteeBreaches int64     `json:"guarantee_breaches"`
	SuccessRate       float64   `json:"success_rate"`
}

func validReport(authority string, days int, correlation string) bool {
	return networkanalytics.ValidAuthority(authority) && days >= 1 && days <= 90 && security.Opaque(correlation)
}
func segment(value string) bool { return security.Opaque(value) && !strings.ContainsAny(value, "/%\\") }
func reportWindow(authority string, days int) url.Values {
	return url.Values{"authority": {authority}, "days": {strconv.Itoa(days)}}
}
func readAnalytics[T any](ctx context.Context, a *Analytics, path string, query url.Values, correlation string) (T, error) {
	var out T
	if !security.Opaque(correlation) {
		return out, ErrInvalid
	}
	u := *a.origin
	u.Path = "/api/v1/analytics/network/" + path
	u.RawPath = ""
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return out, ErrInvalid
	}
	req.Header.Set("X-Correlation-ID", correlation)
	if serviceauth.SignRequest(req, config.Caller, a.secret, nil, time.Now()) != nil {
		return out, ErrUnavailable
	}
	res, err := a.client.Do(req)
	if err != nil {
		return out, ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 || res.StatusCode == 409 {
		return out, ErrRefused
	}
	if res.StatusCode != 200 {
		return out, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1048577))
	if err != nil || len(raw) > 1048576 {
		return out, ErrInvalid
	}
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || !envelope.Success || len(envelope.Data) == 0 || string(envelope.Data) == "null" || json.Unmarshal(envelope.Data, &out) != nil {
		return out, ErrInvalid
	}
	return out, nil
}
func (a *Analytics) MissionSuccess(ctx context.Context, authority string, days int, correlation string) ([]MissionSuccessDay, error) {
	if !validReport(authority, days, correlation) {
		return nil, ErrInvalid
	}
	return readAnalytics[[]MissionSuccessDay](ctx, a, "mission-success", reportWindow(authority, days), correlation)
}
func (a *Analytics) Recovery(ctx context.Context, authority string, days int, correlation string) ([]RecoveryDay, error) {
	if !validReport(authority, days, correlation) {
		return nil, ErrInvalid
	}
	return readAnalytics[[]RecoveryDay](ctx, a, "recovery", reportWindow(authority, days), correlation)
}
func (a *Analytics) CapabilityMismatches(ctx context.Context, authority string, days int, correlation string) ([]MismatchDay, error) {
	if !validReport(authority, days, correlation) {
		return nil, ErrInvalid
	}
	return readAnalytics[[]MismatchDay](ctx, a, "capability-mismatches", reportWindow(authority, days), correlation)
}
func (a *Analytics) Declines(ctx context.Context, authority string, days int, correlation string) ([]DeclineDay, error) {
	if !validReport(authority, days, correlation) {
		return nil, ErrInvalid
	}
	return readAnalytics[[]DeclineDay](ctx, a, "declines", reportWindow(authority, days), correlation)
}
func (a *Analytics) Demand(ctx context.Context, authority string, days int, correlation string) ([]DemandDay, error) {
	if !validReport(authority, days, correlation) {
		return nil, ErrInvalid
	}
	return readAnalytics[[]DemandDay](ctx, a, "demand", reportWindow(authority, days), correlation)
}
func (a *Analytics) ProviderQuality(ctx context.Context, authority, provider string, days int, correlation string) (networkanalytics.ProviderQuality, error) {
	var out networkanalytics.ProviderQuality
	if !validReport(authority, days, correlation) || !segment(provider) {
		return out, ErrInvalid
	}
	out, err := readAnalytics[networkanalytics.ProviderQuality](ctx, a, "provider-quality/"+authority+"/"+provider, url.Values{"days": {strconv.Itoa(days)}}, correlation)
	if err == nil && (out.Authority != authority || out.ProviderID != provider) {
		return out, ErrInvalid
	}
	return out, err
}
func (a *Analytics) BranchReadiness(ctx context.Context, branch string, days int, correlation string) (networkanalytics.BranchReadiness, error) {
	var out networkanalytics.BranchReadiness
	if !segment(branch) || days < 1 || days > 90 {
		return out, ErrInvalid
	}
	out, err := readAnalytics[networkanalytics.BranchReadiness](ctx, a, "readiness/branches/"+branch, url.Values{"days": {strconv.Itoa(days)}}, correlation)
	if err == nil && out.BranchID != branch {
		return out, ErrInvalid
	}
	return out, err
}
func (a *Analytics) Supply(ctx context.Context, authority, zone, product, correlation string) (networkanalytics.SupplySnapshot, error) {
	var out networkanalytics.SupplySnapshot
	if !networkanalytics.ValidAuthority(authority) || !security.Opaque(zone) || product != "" && !security.Opaque(product) {
		return out, ErrInvalid
	}
	out, err := readAnalytics[networkanalytics.SupplySnapshot](ctx, a, "supply", url.Values{"authority": {authority}, "zone": {zone}, "product": {product}}, correlation)
	if err == nil && (out.Authority != authority || out.Zone != zone) {
		return out, ErrInvalid
	}
	return out, err
}
func (a *Analytics) MissionOutcomes(ctx context.Context, authority, product string, days int, correlation string) ([]MissionOutcomeDay, error) {
	if !validReport(authority, days, correlation) || !security.Opaque(product) {
		return nil, ErrInvalid
	}
	query := reportWindow(authority, days)
	query.Set("product", product)
	return readAnalytics[[]MissionOutcomeDay](ctx, a, "mission-outcomes", query, correlation)
}
func (a *Analytics) DemandForecast(ctx context.Context, authority, zone, product string, at time.Time, correlation string) (networkanalytics.DemandForecast, error) {
	var out networkanalytics.DemandForecast
	if !networkanalytics.ValidAuthority(authority) || !security.Opaque(zone) || !security.Opaque(product) || at.IsZero() {
		return out, ErrInvalid
	}
	out, err := readAnalytics[networkanalytics.DemandForecast](ctx, a, "demand-forecast", url.Values{"authority": {authority}, "zone": {zone}, "product": {product}, "at": {at.UTC().Format(time.RFC3339)}}, correlation)
	if err == nil && (out.Authority != authority || out.Zone != zone || out.ProductCode != product) {
		return out, ErrInvalid
	}
	return out, err
}
