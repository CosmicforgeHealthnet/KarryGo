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

// GuestTravellerPath requests only the guest_traveller scope, bound to one
// traveller the customer owns.
const GuestTravellerPath = "/api/v1/customer/internal/customers/%s/context?scopes=guest_traveller&guest_traveller_id=%s"

type guestEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		GuestTraveller *struct {
			ID                   string  `json:"id"`
			Name                 string  `json:"name"`
			Phone                *string `json:"phone"`
			Version              int64   `json:"version"`
			MobilityRequirements *struct {
				WheelchairRequired  *bool   `json:"wheelchair_required"`
				ChildSeatPreference *string `json:"child_seat_preference"`
				PetTravelPreference *string `json:"pet_travel_preference"`
				LuggageDefault      string  `json:"luggage_default"`
			} `json:"mobility_requirements"`
		} `json:"guest_traveller"`
	} `json:"data"`
}

// GuestTraveller reads one traveller for a booking snapshot. A 404 is
// capability.ErrGuestTravellerNotFound; anything else that is not a clean
// answer for exactly that traveller is capability.ErrGuestTravellerUnavailable.
func (c *CustomerContextClient) GuestTraveller(ctx context.Context, customerID, guestTravellerID string) (capability.GuestTraveller, error) {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" || len(c.Secret) == 0 {
		return capability.GuestTraveller{}, capability.ErrGuestTravellerUnavailable
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	id := strings.TrimSpace(guestTravellerID)
	target := strings.TrimRight(c.BaseURL, "/") + fmt.Sprintf(GuestTravellerPath,
		url.PathEscape(strings.TrimSpace(customerID)), url.QueryEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return capability.GuestTraveller{}, capability.ErrGuestTravellerUnavailable
	}
	if serviceauth.SignRequest(req, c.ServiceName, c.Secret, nil, now()) != nil {
		return capability.GuestTraveller{}, capability.ErrGuestTravellerUnavailable
	}
	resp, err := client.Do(req)
	if err != nil {
		return capability.GuestTraveller{}, capability.ErrGuestTravellerUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return capability.GuestTraveller{}, capability.ErrGuestTravellerNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return capability.GuestTraveller{}, capability.ErrGuestTravellerUnavailable
	}
	var envelope guestEnvelope
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&envelope) != nil || !envelope.Success {
		return capability.GuestTraveller{}, capability.ErrGuestTravellerUnavailable
	}
	guest := envelope.Data.GuestTraveller
	// The authority must answer for exactly the traveller asked for.
	if guest == nil || guest.ID != id || strings.TrimSpace(guest.Name) == "" {
		return capability.GuestTraveller{}, capability.ErrGuestTravellerUnavailable
	}
	out := capability.GuestTraveller{
		Snapshot: capability.TravellerSnapshot{GuestTravellerRef: guest.ID, Name: guest.Name, Phone: guest.Phone, SourceVersion: guest.Version},
		Hints:    capability.CustomerMobilityHints{Status: capability.CustomerContextUsed, Version: guest.Version},
	}
	if needs := guest.MobilityRequirements; needs != nil {
		out.Hints.WheelchairRequired, out.Hints.ChildSeatPreference = needs.WheelchairRequired, needs.ChildSeatPreference
		out.Hints.PetTravelPreference, out.Hints.LuggageDefault = needs.PetTravelPreference, needs.LuggageDefault
	}
	return out, nil
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
