package custodychain

import "strings"

// Party is who is physically holding the goods.
//
// Three parties, in one order: SENDER -> PROVIDER -> RECEIVER. Vendor,
// Dispatch, Taxi Goods and Hauling each have their own richer status
// vocabulary and keep it; this is the common denominator every one of them
// can be projected onto so a custody question has one answer across services.
type Party string

const (
	// PartyNone is before anything has been handed over, and after a return
	// has been completed back into the sender's own hands.
	PartyNone Party = "NONE"

	// PartySender is the vendor, shop, restaurant or individual who is
	// releasing the goods.
	PartySender Party = "SENDER"

	// PartyProvider is the executing rider, driver or truck driver.
	PartyProvider Party = "PROVIDER"

	// PartyReceiver is the final recipient.
	PartyReceiver Party = "RECEIVER"
)

// Valid reports whether the party is one this platform knows.
func (p Party) Valid() bool {
	switch p {
	case PartyNone, PartySender, PartyProvider, PartyReceiver:
		return true
	default:
		return false
	}
}

// ParseParty normalizes a stored or caller-supplied custodian.
func ParseParty(value string) (Party, bool) {
	party := Party(strings.ToUpper(strings.TrimSpace(value)))
	return party, party.Valid()
}

// Operation is the named workflow a custody transition happens under.
//
// It is required rather than inferred because two transitions between the
// same pair of parties can be legal under one workflow and forbidden under
// another. Provider-to-provider is the clearest case: it is exactly what a
// supported transfer does, and exactly what a silent reassignment must never
// do. Without the operation there is no way to tell those apart at the point
// the transition is recorded.
type Operation string

const (
	// OperationPickup is the sender releasing goods to the provider after a
	// verified pickup credential.
	OperationPickup Operation = "PICKUP"

	// OperationDelivery is the provider releasing goods to the receiver after
	// a verified delivery credential.
	OperationDelivery Operation = "DELIVERY"

	// OperationProviderTransfer is an explicit, supported provider-to-provider
	// handover: breakdown, illness, safety incident, replacement.
	OperationProviderTransfer Operation = "PROVIDER_TRANSFER"

	// OperationReturn is goods travelling back to the sender.
	OperationReturn Operation = "RETURN"

	// OperationReturnCompleted is the sender taking the returned goods back.
	OperationReturnCompleted Operation = "RETURN_COMPLETED"

	// OperationPreparation covers movement that never leaves the sender:
	// created, packed, ready. It is a transition of state, not of custodian.
	OperationPreparation Operation = "PREPARATION"

	// OperationFailedAttempt is a delivery that did not complete. Whoever was
	// holding the goods is still holding them, which is the point: a failed
	// attempt must be recordable without pretending custody moved.
	OperationFailedAttempt Operation = "FAILED_ATTEMPT"
)

// Valid reports whether the operation is one this platform knows.
func (o Operation) Valid() bool {
	switch o {
	case OperationPickup, OperationDelivery, OperationProviderTransfer,
		OperationReturn, OperationReturnCompleted, OperationPreparation,
		OperationFailedAttempt:
		return true
	default:
		return false
	}
}

// Transition is one proposed custody movement.
type Transition struct {
	From      Party     `json:"from"`
	To        Party     `json:"to"`
	Operation Operation `json:"operation"`

	// SenderDelivers marks work where the sender's own staff carry the goods
	// to the receiver and no platform provider is ever involved — vendor-owned
	// delivery. Only then may custody go straight from sender to receiver.
	//
	// It is false by default on purpose. The safe rule is that goods pass
	// through whoever is carrying them, and a caller has to say deliberately
	// that nobody is.
	SenderDelivers bool `json:"senderDelivers,omitempty"`
}

// Refusal is a stable machine code for an illegal transition, so a caller can
// tell a client which rule it broke instead of "invalid".
type Refusal string

const (
	RefusalNone Refusal = ""

	// RefusalUnknownParty means a party or the operation is not a value this
	// platform defines. Recording it would make the chain unreadable later.
	RefusalUnknownParty Refusal = "CUSTODY_PARTY_UNKNOWN"

	// RefusalSkipsProvider is SENDER -> RECEIVER on work a provider is
	// carrying. The goods cannot teleport past the person holding them, and
	// accepting it would leave the provider holding goods the chain says were
	// already delivered.
	RefusalSkipsProvider Refusal = "CUSTODY_SKIPS_PROVIDER"

	// RefusalReversesTerminal is a move back out of the receiver's hands
	// without an explicit return workflow.
	RefusalReversesTerminal Refusal = "CUSTODY_REVERSES_TERMINAL"

	// RefusalUnsupportedTransfer is PROVIDER -> PROVIDER outside an explicit
	// transfer operation: a silent reassignment of goods already in custody.
	RefusalUnsupportedTransfer Refusal = "CUSTODY_TRANSFER_NOT_SUPPORTED"

	// RefusalWrongOperation is a legal pair of parties moved under the wrong
	// workflow, for example a delivery recorded as a pickup.
	RefusalWrongOperation Refusal = "CUSTODY_OPERATION_MISMATCH"

	// RefusalCustodianMismatch is a transition whose From does not match who
	// the chain already says is holding the goods. The proposal is describing
	// a different world from the recorded one, and applying it would corrupt
	// the chain rather than extend it.
	RefusalCustodianMismatch Refusal = "CUSTODY_CURRENT_HOLDER_MISMATCH"
)

// Allowed reports whether a transition may be recorded.
//
// current is who the chain already says holds the goods. It is checked
// against t.From rather than trusted from t.From, because the proposing
// service may be working from a stale read; a mismatch is refused so a late
// event cannot rewrite the holder.
func Allowed(current Party, t Transition) (bool, Refusal) {
	if !current.Valid() || !t.From.Valid() || !t.To.Valid() || !t.Operation.Valid() {
		return false, RefusalUnknownParty
	}
	if t.From != current {
		return false, RefusalCustodianMismatch
	}

	switch t.Operation {
	case OperationPreparation:
		// Preparation never moves custody. NONE -> SENDER is the one
		// exception: it is the sender taking possession of goods they just
		// created, which is how a parcel enters the chain at all.
		if t.From == t.To || (t.From == PartyNone && t.To == PartySender) {
			return true, RefusalNone
		}
		return false, RefusalWrongOperation

	case OperationPickup:
		if t.From == PartySender && t.To == PartyProvider {
			return true, RefusalNone
		}
		if t.From == PartySender && t.To == PartyReceiver {
			return false, RefusalSkipsProvider
		}
		return false, RefusalWrongOperation

	case OperationDelivery:
		if t.From == PartyProvider && t.To == PartyReceiver {
			return true, RefusalNone
		}
		if t.From == PartySender && t.To == PartyReceiver {
			// Vendor-owned delivery is a real flow: the vendor's own staff
			// carry the goods and hand them over, and no platform provider
			// ever holds them. Everything else reaching this branch is a
			// platform mission claiming delivery while the provider is still
			// holding the parcel.
			if t.SenderDelivers {
				return true, RefusalNone
			}
			return false, RefusalSkipsProvider
		}
		if t.From == PartyReceiver {
			return false, RefusalReversesTerminal
		}
		return false, RefusalWrongOperation

	case OperationFailedAttempt:
		// Nothing changed hands. The holder may be the provider on a platform
		// mission or the sender on a vendor-owned one.
		if t.From == t.To && (t.From == PartyProvider || t.From == PartySender) {
			return true, RefusalNone
		}
		return false, RefusalWrongOperation

	case OperationProviderTransfer:
		if t.From == PartyProvider && t.To == PartyProvider {
			return true, RefusalNone
		}
		return false, RefusalWrongOperation

	case OperationReturn:
		// A return can start from the provider (undelivered goods going
		// back) or, after a failed delivery attempt where the receiver never
		// took them, from the provider again. It can never start from the
		// receiver without them first handing the goods back, which is a
		// delivery-in-reverse this platform does not model.
		if t.From == PartyProvider && (t.To == PartyProvider || t.To == PartySender) {
			return true, RefusalNone
		}
		if t.From == PartyReceiver {
			return false, RefusalReversesTerminal
		}
		return false, RefusalWrongOperation

	case OperationReturnCompleted:
		if t.From == PartyProvider && t.To == PartySender {
			return true, RefusalNone
		}
		return false, RefusalWrongOperation
	}

	return false, RefusalWrongOperation
}

// RequiresCredential reports whether an operation may only be recorded after
// a handover credential has been verified.
//
// Preparation does not: nothing has changed hands. Everything else does,
// because every other operation is a party releasing goods to someone who
// must prove they are the right someone.
func (o Operation) RequiresCredential() bool {
	switch o {
	case OperationPreparation, OperationFailedAttempt:
		return false
	default:
		return true
	}
}

// PurposeFor maps an operation onto the credential purpose that authorizes
// it, given whether the sender is a Commerce vendor or an individual.
//
// The vendorSender argument is what keeps Commerce and direct work apart at
// the one point where the same operation has two different directions.
//
// Note the asymmetry it does NOT resolve: an individual sender's pickup has
// two directions of its own across this platform. Taxi's direct contract is
// customer-held and provider-verified; Dispatch and Hauling direct parcel
// work is provider-held and customer-verified. A caller that knows which of
// those it is asks for PurposeDirectPickup or PurposeSenderVerifiedPickup by
// name; this helper returns the Taxi direct contract, which is the one the
// spec's CFC-006 defines.
func PurposeFor(o Operation, vendorSender bool) (CredentialPurpose, bool) {
	switch o {
	case OperationPickup:
		if vendorSender {
			return PurposeCommercePickup, true
		}
		return PurposeDirectPickup, true
	case OperationDelivery:
		return PurposeDelivery, true
	case OperationProviderTransfer:
		return PurposeProviderTransfer, true
	case OperationReturn, OperationReturnCompleted:
		return PurposeReturn, true
	case OperationFailedAttempt:
		return "", false
	default:
		return "", false
	}
}
