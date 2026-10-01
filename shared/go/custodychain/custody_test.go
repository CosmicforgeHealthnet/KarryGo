package custodychain

import "testing"

func TestHappyPathIsSenderThenProviderThenReceiver(t *testing.T) {
	steps := []struct {
		current Party
		t       Transition
	}{
		{PartyNone, Transition{From: PartyNone, To: PartySender, Operation: OperationPreparation}},
		{PartySender, Transition{From: PartySender, To: PartySender, Operation: OperationPreparation}},
		{PartySender, Transition{From: PartySender, To: PartyProvider, Operation: OperationPickup}},
		{PartyProvider, Transition{From: PartyProvider, To: PartyReceiver, Operation: OperationDelivery}},
	}
	for _, step := range steps {
		if ok, refusal := Allowed(step.current, step.t); !ok {
			t.Fatalf("%s -> %s under %s was refused: %s",
				step.t.From, step.t.To, step.t.Operation, refusal)
		}
	}
}

// CFC-017: goods cannot move from the sender straight to the receiver on work
// a provider is carrying. Accepting it would mark the order delivered while
// the provider is still holding the parcel.
func TestCustodyCannotSkipTheProvider(t *testing.T) {
	for _, operation := range []Operation{OperationPickup, OperationDelivery} {
		ok, refusal := Allowed(PartySender, Transition{
			From: PartySender, To: PartyReceiver, Operation: operation})
		if ok {
			t.Fatalf("SENDER -> RECEIVER was allowed under %s", operation)
		}
		if refusal != RefusalSkipsProvider {
			t.Fatalf("expected %s under %s, got %s",
				RefusalSkipsProvider, operation, refusal)
		}
	}
}

// CFC-017: once the receiver has the goods, nothing moves them back without an
// explicit return flow, and this platform's return flow never starts at the
// receiver.
func TestCustodyCannotReverseOutOfTheReceiver(t *testing.T) {
	for _, operation := range []Operation{OperationDelivery, OperationReturn} {
		ok, refusal := Allowed(PartyReceiver, Transition{
			From: PartyReceiver, To: PartyProvider, Operation: operation})
		if ok {
			t.Fatalf("RECEIVER -> PROVIDER was allowed under %s", operation)
		}
		if refusal != RefusalReversesTerminal {
			t.Fatalf("expected %s under %s, got %s",
				RefusalReversesTerminal, operation, refusal)
		}
	}
}

// CFC-019: a provider holding goods is never silently swapped for another.
// The same pair of parties is legal under PROVIDER_TRANSFER and illegal under
// every other operation, which is exactly why the operation is part of the
// transition rather than inferred from it.
func TestProviderToProviderNeedsAnExplicitTransfer(t *testing.T) {
	ok, refusal := Allowed(PartyProvider, Transition{
		From: PartyProvider, To: PartyProvider, Operation: OperationPickup})
	if ok {
		t.Fatal("a provider swap was allowed under PICKUP")
	}
	if refusal != RefusalWrongOperation {
		t.Fatalf("expected %s, got %s", RefusalWrongOperation, refusal)
	}

	if ok, refusal := Allowed(PartyProvider, Transition{
		From: PartyProvider, To: PartyProvider, Operation: OperationProviderTransfer,
	}); !ok {
		t.Fatalf("an explicit provider transfer was refused: %s", refusal)
	}
}

// A proposal that disagrees with the recorded holder is refused rather than
// applied. A late event carrying a stale "from" would otherwise rewrite who
// is holding the goods.
func TestTransitionMustAgreeWithTheRecordedHolder(t *testing.T) {
	ok, refusal := Allowed(PartyProvider, Transition{
		From: PartySender, To: PartyProvider, Operation: OperationPickup})
	if ok {
		t.Fatal("a pickup was applied to goods the provider already holds")
	}
	if refusal != RefusalCustodianMismatch {
		t.Fatalf("expected %s, got %s", RefusalCustodianMismatch, refusal)
	}
}

func TestUnknownPartyOrOperationIsRefused(t *testing.T) {
	cases := []Transition{
		{From: PartySender, To: "COURIER", Operation: OperationPickup},
		{From: "WAREHOUSE", To: PartyProvider, Operation: OperationPickup},
		{From: PartySender, To: PartyProvider, Operation: "HANDOVER"},
	}
	for _, transition := range cases {
		ok, refusal := Allowed(PartySender, transition)
		if ok {
			t.Fatalf("%+v was allowed", transition)
		}
		if refusal != RefusalUnknownParty {
			t.Fatalf("%+v: expected %s, got %s", transition, RefusalUnknownParty, refusal)
		}
	}
}

func TestReturnEndsBackWithTheSender(t *testing.T) {
	if ok, refusal := Allowed(PartyProvider, Transition{
		From: PartyProvider, To: PartySender, Operation: OperationReturnCompleted,
	}); !ok {
		t.Fatalf("a completed return was refused: %s", refusal)
	}
	if ok, _ := Allowed(PartySender, Transition{
		From: PartySender, To: PartySender, Operation: OperationReturnCompleted,
	}); ok {
		t.Fatal("a return completed without the provider ever handing anything back")
	}
}

func TestOnlyPreparationSkipsTheCredentialRequirement(t *testing.T) {
	for _, operation := range []Operation{
		OperationPickup, OperationDelivery, OperationProviderTransfer,
		OperationReturn, OperationReturnCompleted,
	} {
		if !operation.RequiresCredential() {
			t.Fatalf("%s moves goods without requiring a verified credential", operation)
		}
	}
	if OperationPreparation.RequiresCredential() {
		t.Fatal("preparation demands a credential although nothing changes hands")
	}
}

func TestParseParty(t *testing.T) {
	if party, ok := ParseParty(" provider "); !ok || party != PartyProvider {
		t.Fatalf("parsed %q ok=%v", party, ok)
	}
	if _, ok := ParseParty("platform_provider"); ok {
		t.Fatal("a party this package does not define parsed")
	}
}

// Vendor-owned delivery is the one legitimate SENDER -> RECEIVER move: the
// vendor's own staff carried the goods and no platform provider ever held
// them. It has to be asked for explicitly, so a platform mission cannot reach
// the same branch by accident.
func TestVendorOwnedDeliveryMayGoStraightToTheReceiver(t *testing.T) {
	if ok, refusal := Allowed(PartySender, Transition{
		From: PartySender, To: PartyReceiver,
		Operation: OperationDelivery, SenderDelivers: true,
	}); !ok {
		t.Fatalf("vendor-owned delivery was refused: %s", refusal)
	}
	if ok, refusal := Allowed(PartySender, Transition{
		From: PartySender, To: PartyReceiver, Operation: OperationDelivery,
	}); ok || refusal != RefusalSkipsProvider {
		t.Fatalf("a platform delivery skipped the provider: ok=%v refusal=%s", ok, refusal)
	}
}

// A failed delivery attempt must be recordable without pretending custody
// moved: the provider is still holding the parcel, and that is the fact the
// recovery workflow depends on.
func TestFailedAttemptLeavesCustodyWhereItIs(t *testing.T) {
	if ok, refusal := Allowed(PartyProvider, Transition{
		From: PartyProvider, To: PartyProvider, Operation: OperationFailedAttempt,
	}); !ok {
		t.Fatalf("a failed attempt was refused: %s", refusal)
	}
	if ok, _ := Allowed(PartyProvider, Transition{
		From: PartyProvider, To: PartyReceiver, Operation: OperationFailedAttempt,
	}); ok {
		t.Fatal("a failed attempt handed the goods to the receiver")
	}
	if OperationFailedAttempt.RequiresCredential() {
		t.Fatal("a failed attempt demands a credential although nothing changed hands")
	}
	if _, ok := PurposeFor(OperationFailedAttempt, false); ok {
		t.Fatal("a failed attempt resolved to a credential purpose")
	}
}
