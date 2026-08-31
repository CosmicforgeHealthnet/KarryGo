package orders

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	ETAPolicyVersion       = "commerce-confidence-eta.v1"
	ETARouteFreshFor       = 15 * time.Minute
	ETADelayGrace          = 5 * time.Minute
	ETADelayNotifyMovement = 10 * time.Minute
)

type ETAConfidence string

const (
	ETAConfidenceLow    ETAConfidence = "LOW"
	ETAConfidenceMedium ETAConfidence = "MEDIUM"
	ETAConfidenceHigh   ETAConfidence = "HIGH"
)

type ETAStatus string

const (
	ETAActive      ETAStatus = "ACTIVE"
	ETAActual      ETAStatus = "ACTUAL"
	ETAInvalidated ETAStatus = "INVALIDATED"
	ETAReturning   ETAStatus = "RETURNING"
)

type ETADelayState string

const (
	ETAOnTime        ETADelayState = "ON_TIME"
	ETAAtRisk        ETADelayState = "AT_RISK"
	ETADelayed       ETADelayState = "DELAYED"
	ETARecovered     ETADelayState = "RECOVERED"
	ETANotApplicable ETADelayState = "NOT_APPLICABLE"
)

type ETATrigger string

const (
	TriggerMissionCreated       ETATrigger = "MISSION_CREATED"
	TriggerVendorDelay          ETATrigger = "VENDOR_DELAY"
	TriggerVendorEarlyReady     ETATrigger = "VENDOR_EARLY_READY"
	TriggerProviderAssigned     ETATrigger = "PROVIDER_ASSIGNED"
	TriggerProviderArrived      ETATrigger = "PROVIDER_ARRIVED"
	TriggerProviderReassigned   ETATrigger = "PROVIDER_REASSIGNED"
	TriggerFulfilmentReplan     ETATrigger = "FULFILMENT_REPLAN"
	TriggerStopAdded            ETATrigger = "STOP_ADDED"
	TriggerStopRemoved          ETATrigger = "STOP_REMOVED"
	TriggerBreakdown            ETATrigger = "BREAKDOWN"
	TriggerRouteDeviation       ETATrigger = "ROUTE_DEVIATION"
	TriggerPickupCompleted      ETATrigger = "PICKUP_COMPLETED"
	TriggerStopCompleted        ETATrigger = "STOP_COMPLETED"
	TriggerOperationalException ETATrigger = "OPERATIONAL_EXCEPTION"
	TriggerDelivered            ETATrigger = "DELIVERED"
	TriggerCancelled            ETATrigger = "CANCELLED"
	TriggerReturning            ETATrigger = "RETURNING"
	TriggerTimingUpdated        ETATrigger = "TIMING_UPDATED"
)

type ETAComponent struct {
	Kind          string    `json:"kind"`
	Source        string    `json:"source"`
	EarliestSec   int64     `json:"earliest_seconds"`
	LatestSec     int64     `json:"latest_seconds"`
	EvidenceRef   string    `json:"evidence_ref,omitempty"`
	EvidenceAt    time.Time `json:"evidence_at,omitempty"`
	SourceVersion int64     `json:"source_version,omitempty"`
}

// ETAInput contains only authoritative facts and explicit bounded policy
// uncertainty. It deliberately accepts no customer/frontend supplied ETA.
type ETAInput struct {
	ParentOrderID, FulfilmentGroupID, MissionSlotRef string
	PlanVersion, SourceVersion                       int64
	SourceEventID                                    string
	SourceAuthority                                  string
	SourceOccurredAt                                 time.Time
	Trigger                                          ETATrigger
	Mode, Domain, MissionState                       string
	PromisedLatest                                   *time.Time
	ReadyFrom, ReadyTo                               *time.Time
	ActualReady                                      bool
	ProviderAssigned, ProviderArrived, PickedUp      bool
	ProviderRef                                      string
	RouteSeconds                                     int64
	RouteProvider, RouteEvidenceVersion              string
	RouteObservedAt                                  time.Time
	PickupStopCount, CompletedPickupStops            int
	OperationalException                             bool
	PreviousDelay                                    ETADelayState
	PreviousLatest                                   *time.Time
}

type ETACalculation struct {
	Status           ETAStatus      `json:"status"`
	EarliestAt       *time.Time     `json:"earliest_at,omitempty"`
	LatestAt         *time.Time     `json:"latest_at,omitempty"`
	ActualAt         *time.Time     `json:"actual_at,omitempty"`
	Confidence       ETAConfidence  `json:"confidence"`
	DelayState       ETADelayState  `json:"delay_state"`
	PolicyVersion    string         `json:"policy_version"`
	InputFingerprint string         `json:"input_fingerprint"`
	Components       []ETAComponent `json:"components"`
	NotifyDelay      bool           `json:"-"`
}

var errInvalidETAInput = errors.New("invalid authoritative ETA input")

func CalculateConfidenceETA(in ETAInput) (ETACalculation, error) {
	in.SourceOccurredAt = in.SourceOccurredAt.UTC()
	if strings.TrimSpace(in.ParentOrderID) == "" || strings.TrimSpace(in.MissionSlotRef) == "" ||
		strings.TrimSpace(in.SourceEventID) == "" || strings.TrimSpace(in.SourceAuthority) == "" || in.SourceVersion < 1 || in.SourceOccurredAt.IsZero() ||
		in.PlanVersion < 0 || in.PickupStopCount < 0 || in.PickupStopCount > 32 ||
		in.CompletedPickupStops < 0 || in.CompletedPickupStops > in.PickupStopCount {
		return ETACalculation{}, errInvalidETAInput
	}
	state := strings.ToUpper(strings.TrimSpace(in.MissionState))
	if state == "COMPLETED" || state == "DELIVERED" {
		actual := in.SourceOccurredAt
		out := ETACalculation{Status: ETAActual, EarliestAt: &actual, LatestAt: &actual,
			ActualAt: &actual, Confidence: ETAConfidenceHigh, DelayState: ETANotApplicable,
			PolicyVersion: ETAPolicyVersion, Components: []ETAComponent{}}
		out.InputFingerprint = etaFingerprint(in, out.Components)
		return out, nil
	}
	if state == "CANCELLED" || state == "FAILED" || state == "EXHAUSTED" {
		out := ETACalculation{Status: ETAInvalidated, Confidence: ETAConfidenceLow,
			DelayState: ETANotApplicable, PolicyVersion: ETAPolicyVersion, Components: []ETAComponent{}}
		out.InputFingerprint = etaFingerprint(in, out.Components)
		return out, nil
	}
	if state == "RETURNING" || state == "RETURN_REQUIRED" || state == "RETURNED" {
		out := ETACalculation{Status: ETAReturning, Confidence: ETAConfidenceLow,
			DelayState: ETANotApplicable, PolicyVersion: ETAPolicyVersion, Components: []ETAComponent{}}
		out.InputFingerprint = etaFingerprint(in, out.Components)
		return out, nil
	}
	if in.RouteSeconds <= 0 || in.RouteSeconds > int64((7*24*time.Hour)/time.Second) ||
		strings.TrimSpace(in.RouteProvider) == "" || strings.TrimSpace(in.RouteEvidenceVersion) == "" || in.RouteObservedAt.IsZero() {
		return ETACalculation{}, errInvalidETAInput
	}

	now := in.SourceOccurredAt
	readyEarly, readyLate := now, now.Add(20*time.Minute)
	readinessSource := "BOUNDED_POLICY"
	if in.ReadyFrom != nil && in.ReadyTo != nil && !in.ReadyTo.Before(*in.ReadyFrom) {
		readyEarly, readyLate = in.ReadyFrom.UTC(), in.ReadyTo.UTC()
		if readyEarly.Before(now) {
			readyEarly = now
		}
		if readyLate.Before(readyEarly) {
			readyLate = readyEarly
		}
		readinessSource = "VENDOR_READINESS"
	}
	if in.ActualReady {
		readyEarly, readyLate, readinessSource = now, now, "VENDOR_ACTUAL_READY"
	}
	readinessKind := "VENDOR_PREPARATION"
	switch strings.ToUpper(strings.TrimSpace(in.Domain)) {
	case "FOOD", "RESTAURANT", "FAST_FOOD", "PASTRY":
		readinessKind = "FOOD_PREPARATION"
	case "SHOP", "RETAIL":
		readinessKind = "SHOP_PICKING"
	}
	components := []ETAComponent{{Kind: readinessKind, Source: readinessSource,
		EarliestSec: secondsBetween(now, readyEarly), LatestSec: secondsBetween(now, readyLate),
		SourceVersion: in.SourceVersion}}

	providerEarly, providerLate := 15*time.Minute, 30*time.Minute
	providerSource := "UNASSIGNED_POLICY_BOUND"
	if in.ProviderAssigned {
		providerEarly, providerLate, providerSource = 5*time.Minute, 12*time.Minute, "ASSIGNMENT_POLICY_BOUND"
	}
	if in.ProviderArrived {
		providerEarly, providerLate, providerSource = 0, 2*time.Minute, "PROVIDER_ARRIVAL_EVENT"
	}
	if in.PickedUp {
		providerEarly, providerLate, providerSource = 0, 0, "CUSTODY_CONFIRMED"
	}
	components = append(components, ETAComponent{Kind: "PROVIDER_TO_PICKUP", Source: providerSource,
		EarliestSec: int64(providerEarly / time.Second), LatestSec: int64(providerLate / time.Second),
		EvidenceRef: in.ProviderRef, EvidenceAt: now, SourceVersion: in.SourceVersion})

	remainingStops := in.PickupStopCount - in.CompletedPickupStops
	if remainingStops < 0 {
		remainingStops = 0
	}
	handlingEarly := time.Duration(remainingStops) * 2 * time.Minute
	handlingLate := time.Duration(remainingStops) * 5 * time.Minute
	components = append(components, ETAComponent{Kind: "PICKUP_MANIFEST_HANDLING", Source: "COMMERCE_STOP_PROGRESS",
		EarliestSec: int64(handlingEarly / time.Second), LatestSec: int64(handlingLate / time.Second), SourceVersion: in.SourceVersion})
	if remainingStops > 1 {
		components = append(components, ETAComponent{Kind: "ADDITIONAL_VENDOR_STOPS", Source: "DISPATCH_ROUTE_SEQUENCE",
			EarliestSec: int64(time.Duration(remainingStops-1) * time.Minute / time.Second),
			LatestSec:   int64(time.Duration(remainingStops-1) * 3 * time.Minute / time.Second),
			EvidenceRef: in.RouteEvidenceVersion, EvidenceAt: in.RouteObservedAt.UTC(), SourceVersion: in.SourceVersion})
	}

	confidence := ETAConfidenceLow
	if in.ProviderAssigned {
		confidence = ETAConfidenceMedium
	}
	freshRoute := now.Sub(in.RouteObservedAt.UTC()) <= ETARouteFreshFor && !in.RouteObservedAt.After(now.Add(time.Minute))
	if in.PickedUp && freshRoute {
		confidence = ETAConfidenceHigh
	}
	if !freshRoute || in.OperationalException || in.Trigger == TriggerBreakdown || in.Trigger == TriggerProviderReassigned {
		confidence = ETAConfidenceLow
	}
	varianceBps := int64(2000)
	if confidence == ETAConfidenceMedium {
		varianceBps = 1200
	} else if confidence == ETAConfidenceHigh {
		varianceBps = 700
	}
	routeEarly := in.RouteSeconds * (10000 - varianceBps) / 10000
	routeLate := in.RouteSeconds * (10000 + varianceBps) / 10000
	components = append(components, ETAComponent{Kind: "RECEIVER_ROAD_ROUTE", Source: in.RouteProvider,
		EarliestSec: routeEarly, LatestSec: routeLate, EvidenceRef: in.RouteEvidenceVersion,
		EvidenceAt: in.RouteObservedAt.UTC(), SourceVersion: in.SourceVersion})

	pickupEarly := now.Add(providerEarly)
	pickupLate := now.Add(providerLate)
	if readyEarly.After(pickupEarly) {
		pickupEarly = readyEarly
	}
	if readyLate.After(pickupLate) {
		pickupLate = readyLate
	}
	earliest := pickupEarly.Add(handlingEarly + time.Duration(routeEarly)*time.Second)
	latest := pickupLate.Add(handlingLate + time.Duration(routeLate)*time.Second)
	delay := classifyDelay(in.PromisedLatest, latest, in.PreviousDelay)
	out := ETACalculation{Status: ETAActive, EarliestAt: &earliest, LatestAt: &latest,
		Confidence: confidence, DelayState: delay, PolicyVersion: ETAPolicyVersion, Components: components}
	out.NotifyDelay = delay == ETADelayed && (in.PreviousDelay != ETADelayed || in.PreviousLatest == nil || latest.Sub(*in.PreviousLatest) >= ETADelayNotifyMovement)
	out.InputFingerprint = etaFingerprint(in, components)
	return out, nil
}

func classifyDelay(promised *time.Time, latest time.Time, previous ETADelayState) ETADelayState {
	if promised == nil || promised.IsZero() {
		return ETANotApplicable
	}
	if latest.After(promised.Add(ETADelayGrace)) {
		return ETADelayed
	}
	if latest.After(*promised) {
		return ETAAtRisk
	}
	if previous == ETADelayed || previous == ETAAtRisk {
		return ETARecovered
	}
	return ETAOnTime
}

func etaFingerprint(in ETAInput, components []ETAComponent) string {
	semantic := struct {
		PlanVersion                            int64
		Trigger                                ETATrigger
		Mode, Domain, State, Provider          string
		Promise, ReadyFrom, ReadyTo            *time.Time
		ActualReady, Assigned, Arrived, Picked bool
		RouteSeconds                           int64
		RouteProvider, RouteVersion            string
		RouteAt                                time.Time
		Stops, Completed                       int
		Exception                              bool
		Components                             []ETAComponent
	}{in.PlanVersion, in.Trigger, strings.ToUpper(in.Mode), strings.ToUpper(in.Domain), strings.ToUpper(in.MissionState), in.ProviderRef,
		in.PromisedLatest, in.ReadyFrom, in.ReadyTo, in.ActualReady, in.ProviderAssigned, in.ProviderArrived, in.PickedUp,
		in.RouteSeconds, in.RouteProvider, in.RouteEvidenceVersion, in.RouteObservedAt.UTC(), in.PickupStopCount,
		in.CompletedPickupStops, in.OperationalException, components}
	raw, _ := json.Marshal(semantic)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func secondsBetween(from, to time.Time) int64 {
	if to.Before(from) {
		return 0
	}
	return int64(to.Sub(from) / time.Second)
}
