package recovery

import (
	"context"
	"errors"
	"testing"

	"cosmicforge/logistics/shared/go/notifications"
)

func caseFor() Case {
	return Case{ID: "case-1", BookingID: "b-1", Version: 3, OriginalProviderID: "p-old",
		ReplacementProviderID: "p-new", CorrelationID: "corr"}
}

func byRecipient(intents []notifications.NetworkIntent) map[string]string {
	out := map[string]string{}
	for _, intent := range intents {
		out[intent.RecipientType+"/"+intent.RecipientID] = intent.EventType
	}
	return out
}

func TestAReplacementTellsEveryoneItConcerns(t *testing.T) {
	got := byRecipient(recoveryIntents(EventRecoveryProviderReplaced, caseFor(),
		Audience{CustomerID: "c-1", OrganizationID: "org-1"}))
	want := map[string]string{
		"customer/c-1":             notifications.EventNetworkCustomerProviderReplaced,
		"provider/p-old":           notifications.EventNetworkProviderJobReassigned,
		"provider/p-new":           notifications.EventNetworkProviderReplacementAssigned,
		"fleet_organization/org-1": notifications.EventNetworkFleetProviderReplaced,
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for recipient, event := range want {
		if got[recipient] != event {
			t.Errorf("%s got %q, want %q", recipient, got[recipient], event)
		}
	}
}

func TestIndependentWorkNeverNotifiesAnOrganization(t *testing.T) {
	for _, name := range []string{EventRecoveryStarted, EventRecoveryProviderReplaced} {
		for _, intent := range recoveryIntents(name, caseFor(), Audience{CustomerID: "c-1"}) {
			if intent.RecipientType == notifications.RecipientFleetOrganization {
				t.Fatalf("%s notified an organization about independent work", name)
			}
		}
	}
}

// The custody path publishes a replacement twice -- when chosen and when the
// transfer completes, at different case versions. Each person is told once.
func TestARepublishedReplacementIsTheSameFact(t *testing.T) {
	first, second := caseFor(), caseFor()
	second.Version = 5
	a := recoveryIntents(EventRecoveryProviderReplaced, first, Audience{CustomerID: "c-1"})
	b := recoveryIntents(EventRecoveryProviderReplaced, second, Audience{CustomerID: "c-1"})
	if notifications.NetworkRequest(notificationSource, a[0]).IDempotencyKey !=
		notifications.NetworkRequest(notificationSource, b[0]).IDempotencyKey {
		t.Fatal("the same replacement at two case versions became two notifications")
	}
}

func TestAParkedRecoveryReachesOperations(t *testing.T) {
	got := byRecipient(recoveryIntents(EventCapabilityMismatch, caseFor(), Audience{}))
	if got["admin_operations/"+notifications.AdminOperationsQueue] != notifications.EventNetworkAdminOperatorRequired {
		t.Fatalf("got %v", got)
	}
}

func TestEveryRecoveryNotificationIsValid(t *testing.T) {
	for _, name := range []string{EventRecoveryStarted, EventGuaranteeBreached, EventRecoveryProviderReplaced,
		EventRecoveryFailed, EventCapabilityMismatch} {
		for _, intent := range recoveryIntents(name, caseFor(), Audience{CustomerID: "c", OrganizationID: "o"}) {
			if err := notifications.NetworkRequest(notificationSource, intent).Validate(); err != nil {
				t.Errorf("%s -> %s: %v", name, intent.EventType, err)
			}
		}
	}
}

type failingNotifier struct{ calls int }

func (f *failingNotifier) Send(context.Context, notifications.Request) (notifications.SendResponse, error) {
	f.calls++
	return notifications.SendResponse{}, errors.New("notification-service is down")
}

// A notification outage is logged, never returned: the recovery it announces
// already happened.
func TestANotificationOutageChangesNothing(t *testing.T) {
	notifier := &failingNotifier{}
	service := &Service{notifier: notifier, repo: nil}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("a notification failure escaped: %v", recovered)
		}
	}()
	service.notifyWithAudience(context.Background(), EventRecoveryStarted, caseFor(), Audience{CustomerID: "c"})
	if notifier.calls == 0 {
		t.Fatal("the notifier was never asked")
	}
}

// A failed recovery leaves the job unserved: the customer and operations are
// both told, and nobody else.
func TestAFailedRecoveryReachesTheCustomerAndOperations(t *testing.T) {
	got := byRecipient(recoveryIntents(EventRecoveryFailed, caseFor(), Audience{CustomerID: "c-1", OrganizationID: "org-1"}))
	want := map[string]string{
		"customer/c-1": notifications.EventNetworkCustomerRecoveryFailed,
		"admin_operations/" + notifications.AdminOperationsQueue: notifications.EventNetworkAdminRecoveryFailed,
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for recipient, event := range want {
		if got[recipient] != event {
			t.Errorf("%s got %q, want %q", recipient, got[recipient], event)
		}
	}
}
