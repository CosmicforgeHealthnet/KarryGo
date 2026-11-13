package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// VerificationEventsPath is where verification-compliance-service delivers
// organization verification decisions (ADR-006).
const VerificationEventsPath = "/organization-verification/events"

// VerificationEventHandler ingests organization verification decisions.
type VerificationEventHandler struct {
	organizations *usecases.Service
	allowed       map[string]bool
}

// NewVerificationEventHandler builds the handler for the allowlisted callers.
func NewVerificationEventHandler(organizations *usecases.Service, allowedCallers []string) *VerificationEventHandler {
	allowed := map[string]bool{}
	for _, caller := range allowedCallers {
		if name := strings.TrimSpace(caller); name != "" {
			allowed[name] = true
		}
	}
	return &VerificationEventHandler{organizations: organizations, allowed: allowed}
}

// RegisterVerificationEventRoutes mounts the ingest route on the
// HMAC-authenticated internal group.
func RegisterVerificationEventRoutes(group *gin.RouterGroup, handler *VerificationEventHandler) {
	group.POST(VerificationEventsPath, handler.ingest)
}

// verificationEvent is the Section 61 envelope with the verification payload.
type verificationEvent struct {
	EventID        string `json:"eventId"`
	EventType      string `json:"eventType"`
	OrganizationID string `json:"organizationId"`
	CorrelationID  string `json:"correlationId"`
	Payload        struct {
		SubmissionID      string `json:"submissionId"`
		Status            string `json:"status"`
		ExternalReference string `json:"externalReference"`
		SourceVersion     int64  `json:"sourceVersion"`
		Issues            []struct {
			Code              string `json:"code"`
			FieldReference    string `json:"fieldReference"`
			DocumentReference string `json:"documentReference"`
			Remediation       string `json:"remediation"`
		} `json:"issues"`
	} `json:"payload"`
}

// ingest applies one decision. The response follows the event ingestion
// contract: 200 ACCEPTED/IGNORED, 400 permanent rejection (do not retry),
// 403 caller not allowed, 500 transient (retry).
func (h *VerificationEventHandler) ingest(c *gin.Context) {
	if !h.allowed[c.GetString(middleware.ServiceCallerKey)] {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"status": "REJECTED", "code": "caller_not_allowed"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"status": "ERROR", "code": "body_read_failed"})
		return
	}
	var event verificationEvent
	if err := json.Unmarshal(body, &event); err != nil || event.EventID == "" || event.OrganizationID == "" ||
		event.Payload.Status == "" || event.Payload.SourceVersion < 1 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"status": "REJECTED", "code": "invalid_envelope"})
		return
	}
	switch event.EventType {
	case "organization.verification.status_changed.v1", "organization.verification.submitted.v1":
	default:
		c.JSON(http.StatusOK, gin.H{"status": "IGNORED", "eventId": event.EventID, "code": "unsupported_event_type"})
		return
	}
	issues := make([]platformclients.VerificationIssue, 0, len(event.Payload.Issues))
	for _, issue := range event.Payload.Issues {
		issues = append(issues, platformclients.VerificationIssue{
			Code: issue.Code, FieldReference: issue.FieldReference,
			DocumentReference: issue.DocumentReference, Remediation: issue.Remediation,
		})
	}
	correlationID := event.CorrelationID
	if correlationID == "" {
		correlationID = "verification-event:" + event.EventID
	}
	result, err := h.organizations.ApplyVerificationOutcome(c.Request.Context(), usecases.VerificationOutcomeInput{
		OrganizationID:    event.OrganizationID,
		SubmissionID:      event.Payload.SubmissionID,
		Status:            event.Payload.Status,
		ExternalReference: event.Payload.ExternalReference,
		SourceVersion:     event.Payload.SourceVersion,
		EventID:           event.EventID,
		Issues:            issues,
		CorrelationID:     correlationID,
	})
	if err != nil {
		if isNotFound(err) {
			// The organization or submission does not exist here: retrying
			// cannot help.
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"status": "REJECTED", "code": "submission_not_found"})
			return
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"status": "ERROR", "code": "ingest_failed"})
		return
	}
	status := "IGNORED"
	if result.Applied {
		status = "ACCEPTED"
	}
	c.JSON(http.StatusOK, gin.H{"status": status, "eventId": event.EventID, "activated": result.Activated})
}

func isNotFound(err error) bool {
	return errors.Is(err, orgrepos.ErrNotFound)
}
