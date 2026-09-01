package deliveryrecovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

func sameCodes(left, right []string) bool {
	canonical := func(values []string) []string {
		seen := map[string]struct{}{}
		result := make([]string, 0, len(values))
		for _, value := range values {
			value = strings.ToUpper(strings.TrimSpace(value))
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
		sort.Strings(result)
		return result
	}
	left, right = canonical(left), canonical(right)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

var receiverReasons = map[string][]RecoveryAction{
	"RECEIVER_NO_ANSWER":     {RetrySameDay, Reschedule, AlternativeReceiver, ReturnToSender, OperationsReview},
	"RECEIVER_UNREACHABLE":   {RetrySameDay, Reschedule, AlternativeReceiver, ReturnToSender, OperationsReview},
	"RECEIVER_REFUSED":       {ReturnToSender, OperationsReview},
	"INVALID_CONTACT":        {AlternativeReceiver, ReturnToSender, OperationsReview},
	"ACCESS_RESTRICTED":      {RetrySameDay, Reschedule, ReturnToSender, OperationsReview},
	"LOCATION_UNSAFE":        {ReturnToSender, OperationsReview},
	"DELIVERY_WINDOW_MISSED": {Reschedule, ReturnToSender, OperationsReview},
	"OTHER_REVIEW_REQUIRED":  {OperationsReview},
}

var contactMethods = map[string]bool{"PHONE": true, "EMAIL": true, "OTHER_SERVER_DEFINED_METHOD": true}
var contactOutcomes = map[string]bool{"UNREACHABLE": true, "DECLINED": true, "NUMBER_INVALID": true, "CONTACTED_WAITING": true, "CONTACTED_CANNOT_RECEIVE": true}
var incidentReasons = map[string]bool{"BREAKDOWN": true, "VEHICLE_FAILURE": true, "ILLNESS": true, "SAFETY_CONCERN": true, "ACCIDENT": true, "OPERATIONS_DIRECTED_REPLACEMENT": true, "OTHER_REVIEW_REQUIRED": true}

func (p Policy) normalized() Policy {
	if p.GracePeriod <= 0 {
		p.GracePeriod = 10 * time.Minute
	}
	if p.MaximumSameDayWindow <= 0 {
		p.MaximumSameDayWindow = 8 * time.Hour
	}
	return p
}

func allowedActions(reason string, p Policy) []RecoveryAction {
	base := append([]RecoveryAction(nil), receiverReasons[strings.ToUpper(strings.TrimSpace(reason))]...)
	result := make([]RecoveryAction, 0, len(base)+1)
	for _, action := range base {
		if action == ReturnToSender && !p.ReturnEnabled {
			continue
		}
		result = append(result, action)
	}
	if p.HubHoldEnabled && (reason == "RECEIVER_NO_ANSWER" || reason == "RECEIVER_UNREACHABLE" || reason == "ACCESS_RESTRICTED") {
		result = append(result, HubHold)
	}
	return result
}

func containsAction(actions []RecoveryAction, wanted RecoveryAction) bool {
	for _, action := range actions {
		if action == wanted {
			return true
		}
	}
	return false
}

func fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func opaqueReceiverRef(contact string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(contact))))
	return "receiver_" + hex.EncodeToString(sum[:16])
}
