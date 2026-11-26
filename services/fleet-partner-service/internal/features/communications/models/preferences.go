package models

import (
	"encoding/json"
	"strings"
)

// PreferenceInput sets the delivery channels for one event type, for the
// caller (USER) or as the organization default (ORGANIZATION).
type PreferenceInput struct {
	EventType       string          `json:"eventType"`
	Channels        []string        `json:"channels"`
	QuietHours      json.RawMessage `json:"quietHours"`
	Enabled         *bool           `json:"enabled"`
	ExpectedVersion int64           `json:"expectedVersion"`
}

// Preference is the stored preference used to narrow delivery.
type Preference struct {
	Channels []string
	Enabled  bool
}

// NarrowChannels applies a recipient's preference to the channels a rule or
// caller requested. A disabled preference, or one with no overlap, falls back
// to IN_APP so an operational notification is never silently dropped
// (spec 53.2: delivery failure must not hide a business event).
func NarrowChannels(requested []string, pref *Preference) []string {
	if pref == nil {
		return requested
	}
	allowed := map[string]bool{}
	for _, c := range pref.Channels {
		allowed[strings.ToUpper(c)] = true
	}
	var out []string
	if pref.Enabled {
		for _, c := range requested {
			if allowed[strings.ToUpper(c)] {
				out = append(out, c)
			}
		}
	}
	if len(out) == 0 {
		return []string{"IN_APP"}
	}
	return out
}

// ValidChannel reports whether c is a supported delivery channel.
func ValidChannel(c string) bool { return validChannels[strings.ToUpper(c)] }
