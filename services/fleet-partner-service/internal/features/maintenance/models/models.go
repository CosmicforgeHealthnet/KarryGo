// Package models holds the Section 18 maintenance domain: preventive
// maintenance schedules, work orders and their pure rules (when a schedule is
// due, which work-order transitions are allowed).
package models

import (
	"encoding/json"
	"time"

	assetmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
)

type Schedule struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organizationId"`
	AssetID         string     `json:"assetId"`
	TriggerType     string     `json:"triggerType"`
	Status          string     `json:"status"`
	IntervalValue   *float64   `json:"intervalValue"`
	NextDueOdometer *float64   `json:"nextDueOdometer"`
	NextDueHours    *float64   `json:"nextDueHours"`
	NextDueDate     *time.Time `json:"nextDueDate"`
	Critical        bool       `json:"critical"`
	Version         int64      `json:"version"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}
type WorkOrder struct {
	ID                    string          `json:"id"`
	OrganizationID        string          `json:"organizationId"`
	AssetID               string          `json:"assetId"`
	SourceType            string          `json:"sourceType"`
	SourceReference       string          `json:"sourceReference"`
	Priority              string          `json:"priority"`
	Diagnosis             string          `json:"diagnosis"`
	RequiredWork          string          `json:"requiredWork"`
	Status                string          `json:"status"`
	ApprovedBy            string          `json:"approvedBy"`
	ReturnToServiceStatus string          `json:"returnToServiceStatus"`
	ApprovedAt            *time.Time      `json:"approvedAt"`
	StartedAt             *time.Time      `json:"startedAt"`
	CompletedAt           *time.Time      `json:"completedAt"`
	DowntimeStart         *time.Time      `json:"downtimeStart"`
	DowntimeEnd           *time.Time      `json:"downtimeEnd"`
	EvidenceMediaIDs      json.RawMessage `json:"evidenceMediaIds"`
	Version               int64           `json:"version"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
}

type WorkAction struct {
	ExpectedVersion  int64    `json:"expectedVersion"`
	Diagnosis        string   `json:"diagnosis"`
	RequiredWork     string   `json:"requiredWork"`
	Reason           string   `json:"reason"`
	Status           string   `json:"status"`
	EvidenceMediaIDs []string `json:"evidenceMediaIds"`
}

// Page selects one keyset page of schedules.
type Page struct {
	Limit   int
	After   *time.Time
	AfterID string
}

// DueSchedule is an active schedule that has come due.
type DueSchedule struct {
	ID, AssetID string
	Critical    bool
}

// ManualTransitions are the work-order status changes a plain update may make.
var ManualTransitions = map[string]map[string]bool{"DRAFT": {"PENDING_APPROVAL": true, "CANCELLED": true}, "PENDING_APPROVAL": {"CANCELLED": true}, "APPROVED": {"IN_PROGRESS": true, "CANCELLED": true}, "IN_PROGRESS": {"CANCELLED": true}}

// ScheduleDue reports whether a schedule is due for the asset at now.
func ScheduleDue(v Schedule, a assetmodels.Asset, now time.Time) bool {
	switch v.TriggerType {
	case "DATE":
		return v.NextDueDate != nil && !v.NextDueDate.After(now)
	case "MILEAGE":
		return v.NextDueOdometer != nil && a.Odometer >= *v.NextDueOdometer
	case "OPERATING_HOURS":
		return v.NextDueHours != nil && a.OperatingHours >= *v.NextDueHours
	case "COMBINED":
		return (v.NextDueDate != nil && !v.NextDueDate.After(now)) || (v.NextDueOdometer != nil && a.Odometer >= *v.NextDueOdometer) || (v.NextDueHours != nil && a.OperatingHours >= *v.NextDueHours)
	}
	return false
}
