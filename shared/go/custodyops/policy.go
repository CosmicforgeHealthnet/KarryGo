package custodyops

import (
	"sort"
	"strings"
)

var evidenceTypes = map[EvidenceType]struct{}{
	EvidencePickup: {}, EvidenceDelivery: {}, EvidenceDamage: {}, EvidenceDiscrepancy: {},
	EvidenceFailedAttempt: {}, EvidenceReturn: {}, EvidenceProviderTransfer: {},
}

var exceptionActions = map[ExceptionReason]map[RecoveryAction]struct{}{
	ExceptionVendorNotReady:      set(ActionWait, ActionReschedule),
	ExceptionPackageMismatch:     set(ActionWait, ActionReturnToVendor),
	ExceptionProhibitedItem:      set(ActionReturnToVendor),
	ExceptionDamagedPackage:      set(ActionReturnToSender, ActionReturnToVendor, ActionProviderReplacement),
	ExceptionReceiverUnavailable: set(ActionWait, ActionReschedule, ActionAlternativeReceiver, ActionReturnToSender, ActionReturnToVendor),
	ExceptionReceiverRefused:     set(ActionReturnToSender, ActionReturnToVendor),
	ExceptionWrongAddress:        set(ActionReschedule, ActionAlternativeReceiver, ActionReturnToSender, ActionReturnToVendor),
	ExceptionAddressInaccessible: set(ActionWait, ActionReschedule, ActionReturnToSender, ActionReturnToVendor),
	ExceptionProviderBreakdown:   set(ActionProviderReplacement),
	ExceptionWeatherOrRoad:       set(ActionWait, ActionReschedule, ActionReturnToSender, ActionReturnToVendor),
	ExceptionSecurityRisk:        set(ActionWait, ActionReturnToSender, ActionReturnToVendor, ActionProviderReplacement),
}

func set(values ...RecoveryAction) map[RecoveryAction]struct{} {
	result := make(map[RecoveryAction]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func validEvidenceType(value EvidenceType) bool { _, ok := evidenceTypes[value]; return ok }

func (p Policy) ValidateException(command ExceptionCommand) error {
	actions, ok := exceptionActions[command.Reason]
	if !ok {
		return ErrInvalid
	}
	if _, ok = actions[command.RecoveryAction]; !ok {
		return ErrPolicy
	}
	if command.RecoveryAction == ActionControlledHolding {
		if _, ok := p.ControlledHoldingRefs[trim(command.HoldingRef)]; !ok {
			return ErrPolicy
		}
	}
	if command.RecoveryAction == ActionReschedule && command.ScheduledFor == nil {
		return ErrInvalid
	}
	if command.RecoveryAction == ActionAlternativeReceiver && trim(command.ReceiverRef) == "" {
		return ErrInvalid
	}
	if command.RecoveryAction == ActionAlternativeReceiver && (trim(command.PreviousReceiverChallengeRef) == "" || trim(command.ReplacementReceiverChallengeRef) == "" || command.PreviousReceiverChallengeRef == command.ReplacementReceiverChallengeRef) {
		return ErrInvalid
	}
	if command.RecoveryAction == ActionReschedule && trim(command.PreviousReceiverChallengeRef) != "" && trim(command.ReplacementReceiverChallengeRef) == "" {
		return ErrInvalid
	}
	if (trim(command.FeeQuoteRef) == "") != (trim(command.FeeApprovalRef) == "") {
		return ErrInvalid
	}
	return nil
}

func CanonicalRefs(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, ErrInvalid
		}
		if _, ok := seen[value]; ok {
			return nil, ErrInvalid
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func sameRefs(left, right []string) bool {
	left, errLeft := CanonicalRefs(left)
	right, errRight := CanonicalRefs(right)
	if errLeft != nil || errRight != nil || len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func expectedEvidenceForState(state string) EvidenceType {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "PICKUP", "PICKUP_REQUIRED":
		return EvidencePickup
	case "DELIVERY", "DELIVERY_REQUIRED":
		return EvidenceDelivery
	case "RETURN", "RETURN_REQUIRED":
		return EvidenceReturn
	case "PROVIDER_TRANSFER", "TRANSFER_REQUIRED":
		return EvidenceProviderTransfer
	default:
		return ""
	}
}
