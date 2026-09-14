package fulfilmentgroup

import (
	"fmt"

	"cosmicforge/logistics/shared/go/custodychain"
)

// This file projects Vendor's own custody vocabulary onto the platform-wide
// one in shared/go/custodychain, so the rule about which custody moves are
// possible is stated once for every service rather than re-derived here.
//
// Why it is needed: the custody chain was append-only and monotonic, but it
// accepted any pair of custodians. A logistics authority reporting DELIVERED
// for a parcel still sitting with the vendor would record VENDOR -> CUSTOMER
// and advance the parcel to DELIVERED, even though no provider ever collected
// it. The chain would then say the goods reached the customer while the
// parcel was still on the vendor's counter, and nothing downstream could tell
// the difference.

// partyFor maps a Vendor custodian onto the platform custody vocabulary.
func partyFor(custodian CustodianType) (custodychain.Party, bool) {
	switch custodian {
	case CustodianNone, "":
		return custodychain.PartyNone, true
	case CustodianVendor:
		return custodychain.PartySender, true
	case CustodianPlatform:
		return custodychain.PartyProvider, true
	case CustodianCustomer:
		return custodychain.PartyReceiver, true
	default:
		return "", false
	}
}

// operationFor maps a Vendor custody event onto the workflow it belongs to.
//
// senderDelivers is vendor-owned delivery, where the vendor's own staff carry
// the goods. There the "handover" is to the vendor's own rider, so custody
// never leaves the sender until the receiver takes it — which is why a
// same-party move is preparation rather than a pickup that never happened.
func operationFor(event CustodyEventType, senderDelivers, sameParty bool) (custodychain.Operation, bool) {
	if senderDelivers && sameParty && event != CustodyFailed {
		return custodychain.OperationPreparation, true
	}
	switch event {
	case CustodyCreated, CustodyPacked, CustodyReady:
		return custodychain.OperationPreparation, true
	case CustodyHandedOver:
		return custodychain.OperationPickup, true
	case CustodyTransferred:
		return custodychain.OperationProviderTransfer, true
	case CustodyDelivered:
		return custodychain.OperationDelivery, true
	case CustodyFailed:
		return custodychain.OperationFailedAttempt, true
	case CustodyReturnInitiated:
		return custodychain.OperationReturn, true
	case CustodyReturned:
		return custodychain.OperationReturnCompleted, true
	default:
		return "", false
	}
}

// CustodyRefusedError is an illegal custody transition. It carries the shared
// refusal code so a caller can tell a client which rule was broken rather
// than returning a bare "invalid".
type CustodyRefusedError struct {
	Refusal custodychain.Refusal
	From    CustodianType
	To      CustodianType
	Event   CustodyEventType
}

func (e *CustodyRefusedError) Error() string {
	return fmt.Sprintf("custody transition %s -> %s under %s refused: %s",
		e.From, e.To, e.Event, e.Refusal)
}

// Unwrap lets callers keep treating this as an invalid request.
func (e *CustodyRefusedError) Unwrap() error { return ErrInvalid }

// checkCustodyTransition refuses a custody event that could not physically
// have happened.
//
// A vendor-owned mission is identified by the event carrying a vendor mission
// rather than a platform one, so the flag is derived from the event's own
// data instead of an argument a caller could get wrong.
func checkCustodyTransition(event CustodyEvent, from CustodianType) error {
	fromParty, ok := partyFor(from)
	if !ok {
		return &CustodyRefusedError{Refusal: custodychain.RefusalUnknownParty,
			From: from, To: event.ToCustodianType, Event: event.EventType}
	}
	toParty, ok := partyFor(event.ToCustodianType)
	if !ok {
		return &CustodyRefusedError{Refusal: custodychain.RefusalUnknownParty,
			From: from, To: event.ToCustodianType, Event: event.EventType}
	}

	senderDelivers := event.VendorMissionID != ""
	operation, ok := operationFor(event.EventType, senderDelivers, fromParty == toParty)
	if !ok {
		return &CustodyRefusedError{Refusal: custodychain.RefusalUnknownParty,
			From: from, To: event.ToCustodianType, Event: event.EventType}
	}

	allowed, refusal := custodychain.Allowed(fromParty, custodychain.Transition{
		From: fromParty, To: toParty, Operation: operation,
		SenderDelivers: senderDelivers,
	})
	if allowed {
		return nil
	}
	return &CustodyRefusedError{Refusal: refusal,
		From: from, To: event.ToCustodianType, Event: event.EventType}
}
