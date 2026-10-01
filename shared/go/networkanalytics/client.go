package networkanalytics

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// Read paths.
const (
	BranchReadinessPath = "/internal/analytics/v1/readiness/branches/"
	ProviderQualityPath = "/internal/analytics/v1/provider-quality/"
	SupplySnapshotPath  = "/internal/analytics/v1/supply-snapshot"
	DemandForecastPath  = "/internal/analytics/v1/demand-forecast"
)

// NeutralScore is what every advisory score reads when there is nothing to
// say: neither a reason to prefer nor a reason to avoid.
const NeutralScore = 0.5

// Signal is the common shape of an advisory answer.
//
// Available is false when analytics could not be asked, did not answer in
// time, or does not have enough data. In every one of those cases Score is
// NeutralScore, so a caller that ignores Available still behaves as if
// analytics did not exist -- which is the only safe default for something
// advisory.
type Signal struct {
	Available  bool      `json:"available"`
	Score      float64   `json:"score"`
	SampleSize int64     `json:"sample_size"`
	Status     string    `json:"status"`
	DataAsOf   time.Time `json:"data_as_of,omitempty"`
}

// Signal statuses.
const (
	StatusOK               = "OK"
	StatusInsufficientData = "INSUFFICIENT_DATA"
	StatusUnavailable      = "ANALYTICS_UNAVAILABLE"
)

// Neutral is the answer when analytics has nothing to contribute.
func Neutral(status string) Signal {
	return Signal{Available: false, Score: NeutralScore, Status: status}
}

// BranchReadiness is how reliably a branch is ready when it said it would be.
type BranchReadiness struct {
	Signal
	BranchID string `json:"branch_id"`
	// ProfileVersion advances every time a readiness fact is counted, so a
	// reader can tell a changed profile from an unchanged one.
	ProfileVersion      int64   `json:"profile_version"`
	OnTimeRate          float64 `json:"on_time_rate"`
	AverageLatenessSecs float64 `json:"average_lateness_seconds"`
	AveragePrepSecs     float64 `json:"average_prep_seconds"`
}

// ProviderQuality is an operational reliability read for one provider.
type ProviderQuality struct {
	Signal
	Authority           string  `json:"authority"`
	ProviderID          string  `json:"provider_id"`
	CompletionRate      float64 `json:"completion_rate"`
	RecoveryInvolvement float64 `json:"recovery_involvement_rate"`
	// Declines and SafetyDeclines are reported, never scored: a safety
	// decline is the provider refusing unsafe work.
	Declines       int64 `json:"declines"`
	SafetyDeclines int64 `json:"safety_declines"`
}

// SupplySnapshot is the latest reported supply in one zone.
type SupplySnapshot struct {
	Signal
	Authority   string `json:"authority"`
	Zone        string `json:"zone"`
	ProductCode string `json:"product_code,omitempty"`
	// OnlineCount is always known. EligibleCount and OccupiedCount are nil
	// when the authority could not determine them: UNKNOWN, not zero.
	OnlineCount        int64     `json:"online_count"`
	EligibleCount      *int64    `json:"eligible_count"`
	OccupiedCount      *int64    `json:"occupied_count"`
	SupplyState        string    `json:"supply_state,omitempty"`
	AvailableProviders int64     `json:"available_providers"`
	BusyProviders      int64     `json:"busy_providers"`
	ObservedAt         time.Time `json:"observed_at,omitempty"`
}

// Client reads advisory signals. Its zero value -- or one with no base URL --
// returns neutral answers without making a call.
type Client struct {
	BaseURL     string
	ServiceName string
	Secret      []byte
	// Timeout bounds every read. It defaults to 800ms: an advisory read that
	// makes the caller wait longer than that has already cost more than it
	// could be worth.
	Timeout    time.Duration
	HTTPClient *http.Client
}

func (c Client) configured() bool {
	return strings.TrimSpace(c.BaseURL) != "" && strings.TrimSpace(c.ServiceName) != "" && len(c.Secret) > 0
}

// BranchReadiness reads a branch's readiness reliability.
func (c Client) BranchReadiness(ctx context.Context, branchID string) BranchReadiness {
	answer := BranchReadiness{Signal: Neutral(StatusUnavailable), BranchID: branchID}
	if strings.TrimSpace(branchID) == "" || !c.read(ctx, BranchReadinessPath+url.PathEscape(branchID), &answer) {
		answer.Signal, answer.BranchID = Neutral(StatusUnavailable), branchID
	}
	return answer
}

// ProviderQuality reads a provider's operational reliability.
func (c Client) ProviderQuality(ctx context.Context, authority, providerID string) ProviderQuality {
	answer := ProviderQuality{Signal: Neutral(StatusUnavailable), Authority: authority, ProviderID: providerID}
	path := ProviderQualityPath + url.PathEscape(authority) + "/" + url.PathEscape(providerID)
	if !ValidAuthority(authority) || strings.TrimSpace(providerID) == "" || !c.read(ctx, path, &answer) {
		answer.Signal, answer.Authority, answer.ProviderID = Neutral(StatusUnavailable), authority, providerID
	}
	return answer
}

// SupplySnapshot reads the latest supply in a zone.
func (c Client) SupplySnapshot(ctx context.Context, authority, zone string) SupplySnapshot {
	answer := SupplySnapshot{Signal: Neutral(StatusUnavailable), Authority: authority, Zone: zone}
	path := SupplySnapshotPath + "?authority=" + url.QueryEscape(authority) + "&zone=" + url.QueryEscape(zone)
	if !ValidAuthority(authority) || strings.TrimSpace(zone) == "" || !c.read(ctx, path, &answer) {
		answer.Signal, answer.Authority, answer.Zone = Neutral(StatusUnavailable), authority, zone
	}
	return answer
}

// read fetches one signal. It reports false on ANY problem -- unconfigured,
// timeout, transport, non-2xx, unreadable body -- and the caller substitutes
// the neutral answer.
func (c Client) read(ctx context.Context, path string, target any) bool {
	if !c.configured() {
		return false
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 800 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+path, nil)
	if err != nil {
		return false
	}
	if err = serviceauth.SignRequest(request, c.ServiceName, c.Secret, []byte{}, time.Now().UTC()); err != nil {
		return false
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err = json.NewDecoder(response.Body).Decode(&envelope); err != nil || len(bytes.TrimSpace(envelope.Data)) == 0 {
		return false
	}
	return json.Unmarshal(envelope.Data, target) == nil
}

// DemandForecast is the deterministic expected demand for one zone, product
// and upcoming bucket.
type DemandForecast struct {
	Signal
	Authority   string    `json:"authority"`
	Zone        string    `json:"zone"`
	ProductCode string    `json:"product_code"`
	BucketStart time.Time `json:"bucket_start"`
	// Expected is the mean of the same slot in previous weeks.
	Expected float64 `json:"expected"`
	// CurrentCount is the bucket's count so far, when it has one.
	CurrentCount *int64 `json:"current_count"`
	Method       string `json:"method"`
	ModelVersion string `json:"model_version"`
}

// DemandForecastMethod names the forecast: the mean of the same weekday and
// time slot over the previous DemandForecastWeeks weeks. No learning, no
// weights -- it can be recomputed by hand from the buckets.
const (
	DemandForecastMethod = "same_slot_mean_v1"
	DemandForecastWeeks  = 4
	// DemandForecastMinimumSlots is the least history a forecast is trusted
	// on. Below it the answer is neutral.
	DemandForecastMinimumSlots = 3
)

// DemandForecast reads expected demand for a zone and product.
func (c Client) DemandForecast(ctx context.Context, authority, zone, product string) DemandForecast {
	answer := DemandForecast{Signal: Neutral(StatusUnavailable), Authority: authority, Zone: zone, ProductCode: product}
	path := DemandForecastPath + "?authority=" + url.QueryEscape(authority) + "&zone=" + url.QueryEscape(zone) +
		"&product=" + url.QueryEscape(product)
	if !ValidAuthority(authority) || strings.TrimSpace(zone) == "" || !c.read(ctx, path, &answer) {
		answer = DemandForecast{Signal: Neutral(StatusUnavailable), Authority: authority, Zone: zone, ProductCode: product}
	}
	return answer
}
