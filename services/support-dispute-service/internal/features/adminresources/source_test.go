package adminresources

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"cosmicforge/logistics/shared/go/adminowner"
)

func TestSupportOwnerProfileIsTheFixedMinimizedDictionary(t *testing.T) {
	profile := adminowner.SupportProfile()
	if profile.Owner != Owner || profile.Prefix != "SUPPORT_DISPUTE" || profile.Path != "/internal/support/v1/admin" || len(profile.Definitions) != 5 {
		t.Fatalf("unexpected profile: %+v", profile)
	}
	source := Source()
	for kind, definition := range profile.Definitions {
		if ReadPermission(kind) != definition.Capability {
			t.Fatalf("%s native grant drifted from %s", kind, definition.Capability)
		}
		spec, ok := source.Specs[kind]
		if !ok {
			t.Fatalf("no SQL spec for %s", kind)
		}
		for _, name := range definition.Dimensions {
			if spec.Dimensions[name] == "" {
				t.Fatalf("%s dimension %s unmapped", kind, name)
			}
		}
		for _, name := range definition.Counters {
			if spec.Counters[name] == "" {
				t.Fatalf("%s counter %s unmapped", kind, name)
			}
		}
		for _, name := range definition.References {
			if spec.References[name] == "" {
				t.Fatalf("%s reference %s unmapped", kind, name)
			}
		}
		table := strings.ToLower(spec.Table)
		for _, forbidden := range []string{"description", "complainant_name", "complainant_phone", "respondent_name", "respondent_phone", "incident_lat", "incident_lng", "media_url", "resolution_note", "adjudication_note", "content", "subject", "c.*", "m.*", "e.*", "d.*"} {
			if strings.Contains(table, forbidden) {
				t.Fatalf("%s projection selects %q", kind, forbidden)
			}
		}
	}
	if ReadPermission("unknown") != "" {
		t.Fatal("unknown kind has a grant")
	}
}

func TestLabelRedactsCredentialsContactsAndBoundsLength(t *testing.T) {
	cases := map[string]string{
		"Code 482193 did not work":            "Code [REDACTED] did not work",
		"Call me on +234 801 234 5678 please": "Call me on [REDACTED] please",
		"Email ada.o@example.com\nnow":        "Email [REDACTED] now",
		"Phone 08012345678\tthanks":           "Phone [REDACTED] thanks",
		"   ":                                 "",
	}
	for in, want := range cases {
		got := Label(in)
		if want == "" {
			if got != nil {
				t.Fatalf("%q produced a label", in)
			}
			continue
		}
		if got == nil || *got != want {
			t.Fatalf("Label(%q)=%v want %q", in, got, want)
		}
	}
	long := Label(strings.Repeat("ab ", 400))
	if long == nil || utf8.RuneCountInString(*long) > MaxLabelRunes {
		t.Fatal("label exceeds bound")
	}
}

func TestRuntimeDefaultsOffWithoutCredentials(t *testing.T) {
	handler, err := Runtime(context.Background(), nil)
	if err != nil || handler.Enabled || handler.PublishEnabled {
		t.Fatalf("default runtime must be disabled: %+v %v", handler, err)
	}
}
