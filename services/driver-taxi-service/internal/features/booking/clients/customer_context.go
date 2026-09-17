// Package bookingclients holds Taxi booking adapters to other services.
package bookingclients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// CustomerContextPath is customer-service's internal context route under its
// API base.
const CustomerContextPath = "/api/v1/customer/internal/customers/%s/context?scopes=mobility_preferences"

// CustomerContextClient reads the minimized mobility context for a customer.
// Only the mobility_preferences scope is requested; free-text assistance
// notes are never read into Taxi.
type CustomerContextClient struct {
	BaseURL     string
	ServiceName string
	Secret      []byte
	HTTPClient  *http.Client
	Now         func() time.Time
}

type contextEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		MobilityPreferences *struct {
			WheelchairRequired       *bool           `json:"wheelchair_required"`
			ChildSeatPreference      *string         `json:"child_seat_preference"`
			PetTravelPreference      *string         `json:"pet_travel_preference"`
			LuggageDefault           string          `json:"luggage_default"`
			CommunicationPreferences json.RawMessage `json:"communication_preferences"`
			Version                  int64           `json:"version"`
		} `json:"mobility_preferences"`
	} `json:"data"`
}

// MobilityHints never returns an error: an unavailable customer context is
// recorded on the snapshot as UNAVAILABLE and the booking proceeds on what
// the request stated explicitly.
func (c *CustomerContextClient) MobilityHints(ctx context.Context, customerID string) capability.CustomerMobilityHints {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" || len(c.Secret) == 0 {
		return capability.CustomerMobilityHints{Status: capability.CustomerContextNotRequested}
	}
	unavailable := capability.CustomerMobilityHints{Status: capability.CustomerContextUnavailable}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	target := strings.TrimRight(c.BaseURL, "/") + fmt.Sprintf(CustomerContextPath, url.PathEscape(strings.TrimSpace(customerID)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return unavailable
	}
	if serviceauth.SignRequest(req, c.ServiceName, c.Secret, nil, now()) != nil {
		return unavailable
	}
	resp, err := client.Do(req)
	if err != nil {
		return unavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return unavailable
	}
	var envelope contextEnvelope
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&envelope) != nil || !envelope.Success {
		return unavailable
	}
	prefs := envelope.Data.MobilityPreferences
	if prefs == nil {
		return capability.CustomerMobilityHints{Status: capability.CustomerContextUsed}
	}
	hints := capability.CustomerMobilityHints{Status: capability.CustomerContextUsed, Version: prefs.Version,
		WheelchairRequired: prefs.WheelchairRequired, ChildSeatPreference: prefs.ChildSeatPreference,
		PetTravelPreference: prefs.PetTravelPreference, LuggageDefault: prefs.LuggageDefault}
	var comms struct {
		QuietRide        *bool  `json:"quiet_ride"`
		PreferredChannel string `json:"preferred_channel"`
	}
	if len(prefs.CommunicationPreferences) > 0 && json.Unmarshal(prefs.CommunicationPreferences, &comms) == nil {
		hints.QuietRide, hints.PreferredChannel = comms.QuietRide, comms.PreferredChannel
	}
	return hints
}
