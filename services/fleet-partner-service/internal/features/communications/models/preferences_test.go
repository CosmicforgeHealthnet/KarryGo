package models

import (
	"reflect"
	"testing"
)

func TestNarrowChannels(t *testing.T) {
	req := []string{"EMAIL", "PUSH", "IN_APP"}
	if got := NarrowChannels(req, nil); !reflect.DeepEqual(got, req) {
		t.Fatalf("no preference = %v", got)
	}
	if got := NarrowChannels(req, &Preference{Enabled: true, Channels: []string{"push"}}); !reflect.DeepEqual(got, []string{"PUSH"}) {
		t.Fatalf("narrowed = %v", got)
	}
	if got := NarrowChannels(req, &Preference{Enabled: false, Channels: []string{"PUSH"}}); !reflect.DeepEqual(got, []string{"IN_APP"}) {
		t.Fatalf("disabled must fall back to IN_APP, got %v", got)
	}
	if got := NarrowChannels([]string{"SMS"}, &Preference{Enabled: true, Channels: []string{"EMAIL"}}); !reflect.DeepEqual(got, []string{"IN_APP"}) {
		t.Fatalf("no overlap must fall back to IN_APP, got %v", got)
	}
}
