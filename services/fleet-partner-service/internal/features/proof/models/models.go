// Package models holds the Section 19 proof domain: versioned proof policies,
// immutable proof records, append-only verifications and the pure rules that
// decide a record's effective status and its policy failures.
package models

import (
	"encoding/json"
	"strings"
	"time"
)

type Flags struct{ Policies, Records, Workers, KillSwitch bool }
type Policy struct {
	ID                 string          `json:"id"`
	OrganizationID     string          `json:"organizationId"`
	PolicyScope        string          `json:"policyScope"`
	ScopeReference     string          `json:"scopeReference"`
	ServiceType        string          `json:"serviceType"`
	StopType           string          `json:"stopType"`
	RequiredProofTypes json.RawMessage `json:"requiredProofTypes"`
	MinimumCount       int             `json:"minimumCount"`
	LocationRequired   bool            `json:"locationRequired"`
	TimeWindowRules    json.RawMessage `json:"timeWindowRules"`
	FailurePolicy      string          `json:"failurePolicy"`
	Status             string          `json:"status"`
	Version            int64           `json:"version"`
	CreatedBy          string          `json:"createdBy"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}
type PolicySnapshot struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organizationId"`
	PolicyID       string          `json:"policyId"`
	PolicyVersion  int64           `json:"policyVersion"`
	Snapshot       json.RawMessage `json:"snapshot"`
	PublishedBy    string          `json:"publishedBy"`
	PublishedAt    time.Time       `json:"publishedAt"`
}
type Record struct {
	ID, OrganizationID, PolicySnapshotReference, ProofType, OrderID, ShipmentID, PackageID, RouteStopID, AssignmentID, CanonicalTripReference, ActorType, ActorReference, MediaFileID, ValueHash string
	SafeValueMetadata                                                                                                                                                                            json.RawMessage
	Latitude, Longitude, Accuracy                                                                                                                                                                *float64
	SourceTimestamp, ReceivedAt                                                                                                                                                                  time.Time
	Source, VerificationStatus                                                                                                                                                                   string
	SourceVersion                                                                                                                                                                                int64
	IdempotencyKey                                                                                                                                                                               string
	Version                                                                                                                                                                                      int64
	CreatedAt                                                                                                                                                                                    time.Time
}
type RecordInput struct {
	PolicySnapshotReference string          `json:"policySnapshotReference"`
	ProofType               string          `json:"proofType"`
	OrderID                 string          `json:"orderId"`
	ShipmentID              string          `json:"shipmentId"`
	PackageID               string          `json:"packageId"`
	RouteStopID             string          `json:"routeStopId"`
	AssignmentID            string          `json:"assignmentId"`
	CanonicalTripReference  string          `json:"canonicalTripReference"`
	ActorType               string          `json:"actorType"`
	ActorReference          string          `json:"actorReference"`
	MediaFileID             string          `json:"mediaFileId"`
	Value                   string          `json:"value"`
	SafeValueMetadata       json.RawMessage `json:"safeValueMetadata"`
	Latitude                *float64        `json:"latitude"`
	Longitude               *float64        `json:"longitude"`
	Accuracy                *float64        `json:"accuracy"`
	SourceTimestamp         time.Time       `json:"sourceTimestamp"`
	Source                  string          `json:"source"`
	SourceVersion           int64           `json:"sourceVersion"`
	IdempotencyKey          string          `json:"idempotencyKey"`
}
type Verification struct {
	ID                 string          `json:"id"`
	OrganizationID     string          `json:"organizationId"`
	ProofRecordID      string          `json:"proofRecordId"`
	Status             string          `json:"status"`
	AuthorityType      string          `json:"authorityType"`
	AuthorityReference string          `json:"authorityReference"`
	SourceVersion      int64           `json:"sourceVersion"`
	ReasonCodes        json.RawMessage `json:"reasonCodes"`
	VerifiedAt         time.Time       `json:"verifiedAt"`
}

// AuthorityOrganization is the verification authority of a decision made by
// an authorized member of the organization (spec 49.3 POST /proofs/{id}/verify).
// Media-safety verifications use AuthorityMedia.
const (
	AuthorityOrganization = "ORGANIZATION"
	AuthorityMedia        = "MEDIA"
)

// RecordDetail is one proof record with its append-only verification history
// and the status that governs it.
type RecordDetail struct {
	Record             Record
	EffectiveStatus    string
	Verifications      []Verification
	PolicySnapshotHash string
}

// DecisionInput is an organization verification decision.
type DecisionInput struct {
	ReasonCodes []string
}

// PendingMediaRecord is a media-backed record without a media verification.
type PendingMediaRecord struct {
	ID, MediaFileID string
	SourceVersion   int64
}

// SupportedProofTypes is the closed set of proof types a policy may require.
var SupportedProofTypes = map[string]bool{
	"OTP": true, "SIGNATURE": true, "PHOTO": true, "PACKAGE_SCAN": true,
	"DOCUMENT_SCAN": true, "LOCATION": true, "TIMESTAMP": true,
	"RECIPIENT_NAME": true, "IDENTITY_REFERENCE": true, "SEAL": true,
	"TEMPERATURE_READING": true, "CUSTOM_FIELD": true,
}

// EffectiveStatus applies the decision order: an organization decision is
// final; otherwise a media rejection wins over the status the record was
// created with, and a media verification confirms it.
func EffectiveStatus(r Record, vs []Verification) string {
	media := ""
	for _, v := range vs {
		if v.AuthorityType == AuthorityOrganization {
			return v.Status
		}
		if v.AuthorityType == AuthorityMedia && media != "REJECTED" {
			media = v.Status
		}
	}
	if media == "REJECTED" {
		return media
	}
	if media == "VERIFIED" {
		return media
	}
	return r.VerificationStatus
}

// RequiresType reports whether the policy requires the proof type.
func RequiresType(p Policy, proofType string) bool {
	var required []string
	_ = json.Unmarshal(p.RequiredProofTypes, &required)
	for _, x := range required {
		if strings.ToUpper(x) == proofType {
			return true
		}
	}
	return false
}

// PolicyFailures re-checks a record against the policy snapshot it claims to
// satisfy. A record can only be verified if it still meets every rule of that
// snapshot and no media authority rejected its evidence.
func PolicyFailures(p Policy, r Record, vs []Verification) []string {
	var failed []string
	if !RequiresType(p, r.ProofType) {
		failed = append(failed, "PROOF_TYPE_NOT_REQUIRED")
	}
	if (p.LocationRequired || r.ProofType == "LOCATION") && (r.Latitude == nil || r.Longitude == nil) {
		failed = append(failed, "LOCATION_MISSING")
	}
	if (r.ProofType == "PHOTO" || r.ProofType == "SIGNATURE" || r.ProofType == "DOCUMENT_SCAN") && r.MediaFileID == "" {
		failed = append(failed, "MEDIA_MISSING")
	}
	if r.ProofType == "OTP" && r.ValueHash == "" {
		failed = append(failed, "OTP_VALUE_MISSING")
	}
	for _, v := range vs {
		if v.AuthorityType == AuthorityMedia && v.Status == "REJECTED" {
			failed = append(failed, "MEDIA_REJECTED")
			break
		}
	}
	return failed
}

// Raw returns v, or the default document d when v is empty.
func Raw(v json.RawMessage, d string) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage(d)
	}
	return v
}
