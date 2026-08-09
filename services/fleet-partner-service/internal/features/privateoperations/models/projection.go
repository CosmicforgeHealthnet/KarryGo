package models

import (
	"strings"

	orders "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	shipments "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
)

const (
	OutcomeAssigned  = "ASSIGNED"
	OutcomeActive    = "IN_PROGRESS"
	OutcomeCompleted = "COMPLETED"
	OutcomePartial   = "PARTIALLY_COMPLETED"
	OutcomeFailed    = "FAILED"
	OutcomeCancelled = "CANCELLED"
)

func NormalizeExecutionStatus(eventType, status string) string {
	s := strings.ToUpper(strings.TrimSpace(status))
	switch s {
	case "CREATED", "ACCEPTED", "ASSIGNED", "READY_FOR_EXECUTION", "CONFIRMED":
		return OutcomeAssigned
	case "EN_ROUTE_PICKUP", "ARRIVED_PICKUP", "ARRIVED_AT_PICKUP", "LOADING", "LOADED", "IN_TRANSIT", "IN_PROGRESS", "EN_ROUTE_DELIVERY", "ARRIVED_AT_DELIVERY", "UNLOADING":
		return OutcomeActive
	case "COMPLETED", "DELIVERED":
		return OutcomeCompleted
	case "PARTIALLY_COMPLETED", "PARTIAL":
		return OutcomePartial
	case "FAILED", "UNMATCHED":
		return OutcomeFailed
	case "CANCELLED":
		return OutcomeCancelled
	}
	switch eventType {
	case "trip.created.v1":
		return OutcomeAssigned
	case "trip.completed.v1", "proof.completed.v1":
		return OutcomeCompleted
	case "trip.cancelled.v1":
		return OutcomeCancelled
	}
	return ""
}

func ShipmentProjectionPath(from, outcome string) []string {
	var target string
	switch outcome {
	case OutcomeAssigned:
		target = shipments.ShipmentAssigned
	case OutcomeActive:
		target = shipments.ShipmentInTransit
	case OutcomeCompleted:
		target = shipments.ShipmentCompleted
	case OutcomePartial:
		target = shipments.ShipmentPartiallyCompleted
	case OutcomeFailed:
		target = shipments.ShipmentFailed
	case OutcomeCancelled:
		target = shipments.ShipmentCancelled
	}
	if target == "" || from == target || shipments.TerminalShipment(from) {
		return nil
	}
	if outcome == OutcomeCancelled {
		if shipments.CanTransitionShipment(from, target) || from == shipments.ShipmentAssigned || from == shipments.ShipmentInTransit {
			return []string{target}
		}
		return nil
	}
	chain := []string{shipments.ShipmentReady, shipments.ShipmentPlanned, shipments.ShipmentAssigned, shipments.ShipmentInTransit, target}
	current := from
	var path []string
	for _, next := range chain {
		if next == current {
			continue
		}
		if shipments.CanTransitionShipment(current, next) {
			path = append(path, next)
			current = next
		}
		if current == target {
			break
		}
	}
	return path
}

func AggregateOrderOutcome(statuses []string) string {
	if len(statuses) == 0 {
		return ""
	}
	completed, partial, failed, cancelled, assigned, active := 0, 0, 0, 0, 0, 0
	for _, status := range statuses {
		switch status {
		case shipments.ShipmentCompleted:
			completed++
		case shipments.ShipmentPartiallyCompleted:
			partial++
		case shipments.ShipmentFailed:
			failed++
		case shipments.ShipmentCancelled:
			cancelled++
		case shipments.ShipmentInTransit:
			active++
		default:
			assigned++
		}
	}
	if active > 0 {
		return OutcomeActive
	}
	if assigned > 0 {
		return OutcomeAssigned
	}
	if completed == len(statuses) {
		return OutcomeCompleted
	}
	if completed+partial > 0 {
		return OutcomePartial
	}
	if failed == len(statuses) {
		return OutcomeFailed
	}
	if cancelled == len(statuses) {
		return OutcomeCancelled
	}
	if completed > 0 && completed+failed+cancelled == len(statuses) {
		return OutcomePartial
	}
	return ""
}

func OrderProjectionPath(from, outcome string) []string {
	var target string
	switch outcome {
	case OutcomeAssigned:
		target = orders.StatusAssigned
	case OutcomeActive:
		target = orders.StatusInProgress
	case OutcomeCompleted:
		target = orders.StatusCompleted
	case OutcomePartial:
		target = orders.StatusPartiallyCompleted
	case OutcomeFailed:
		target = orders.StatusFailed
	case OutcomeCancelled:
		target = orders.StatusCancelled
	}
	if target == "" || from == target || orders.Terminal(from) {
		return nil
	}
	if target == orders.StatusCancelled {
		if orders.CanTransition(from, target) || from == orders.StatusAssigned || from == orders.StatusInProgress {
			return []string{target}
		}
		return nil
	}
	chain := []string{orders.StatusPlanned, orders.StatusAssigned, orders.StatusInProgress, target}
	current := from
	var path []string
	for _, next := range chain {
		if next == current {
			continue
		}
		if orders.CanTransition(current, next) {
			path = append(path, next)
			current = next
		}
		if current == target {
			break
		}
	}
	return path
}
