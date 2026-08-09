package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type walletPostingAdapter struct{ client *platformclients.Client }

func (a walletPostingAdapter) Configured() bool { return a.client != nil && a.client.Configured() }

func (a walletPostingAdapter) Submit(ctx context.Context, meta platformclients.CallMeta, request financeops.PostingRequest) (financeops.PostingOutcome, error) {
	if !a.Configured() {
		return financeops.PostingOutcome{}, platformclients.ErrNotConfigured
	}
	var fields map[string]any
	if len(request.Payload) != 0 {
		if err := json.Unmarshal(request.Payload, &fields); err != nil {
			return financeops.PostingOutcome{}, err
		}
	} else {
		fields = map[string]any{}
	}
	fields["payloadVersion"] = 1
	fields["organizationId"] = request.OrganizationID
	fields["idempotencyKey"] = request.IdempotencyKey
	fields["expectedVersion"] = request.ExpectedVersion

	path := ""
	switch request.OperationType {
	case "PROVIDER_ALLOCATION", "PAYROLL_REVERSAL":
		path = "/provider-allocations"
	case "SETTLEMENT_REQUEST":
		path = "/settlements"
	case "REVERSAL_REQUEST":
		path = "/reversals"
	default:
		return financeops.PostingOutcome{}, fmt.Errorf("wallet posting: unsupported operation %s", request.OperationType)
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Version int64  `json:"version"`
		} `json:"data"`
	}
	err := a.client.Do(ctx, meta, http.MethodPost, "/api/v1/payment-wallet/internal/finance"+path, fields, &response)
	return financeops.PostingOutcome{Reference: response.Data.ID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}

// accountingPostingAdapter is deliberately an export/operation contract, not
// a local ledger. The configured accounting authority owns posting and
// reversal execution; Fleet retains only opaque references and reconciliation
// state.
type accountingPostingAdapter struct{ client *platformclients.Client }

func (a accountingPostingAdapter) Configured() bool { return a.client != nil && a.client.Configured() }

func (a accountingPostingAdapter) Submit(ctx context.Context, meta platformclients.CallMeta, request financeops.PostingRequest) (financeops.PostingOutcome, error) {
	if !a.Configured() {
		return financeops.PostingOutcome{}, platformclients.ErrNotConfigured
	}
	body := map[string]any{
		"payloadVersion":  1,
		"organizationId":  request.OrganizationID,
		"recordType":      request.RecordType,
		"recordId":        request.RecordID,
		"operationType":   request.OperationType,
		"idempotencyKey":  request.IdempotencyKey,
		"amountMinor":     request.AmountMinor,
		"currency":        request.Currency,
		"expectedVersion": request.ExpectedVersion,
	}
	if len(request.Payload) != 0 {
		var evidence any
		if err := json.Unmarshal(request.Payload, &evidence); err != nil {
			return financeops.PostingOutcome{}, err
		}
		body["sourceEvidence"] = evidence
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Version int64  `json:"version"`
		} `json:"data"`
	}
	err := a.client.Do(ctx, meta, http.MethodPost, "/api/v1/accounting/internal/postings", body, &response)
	return financeops.PostingOutcome{Reference: response.Data.ID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}

func (a accountingPostingAdapter) ReadByKey(ctx context.Context, meta platformclients.CallMeta, request financeops.PostingRequest) (financeops.PostingOutcome, error) {
	if !a.Configured() {
		return financeops.PostingOutcome{}, platformclients.ErrNotConfigured
	}
	path := "/api/v1/accounting/internal/postings/by-idempotency?organizationId=" + url.QueryEscape(request.OrganizationID) +
		"&operationType=" + url.QueryEscape(request.OperationType) +
		"&idempotencyKey=" + url.QueryEscape(request.IdempotencyKey)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Version int64  `json:"version"`
		} `json:"data"`
	}
	err := a.client.Do(ctx, meta, http.MethodGet, path, nil, &response)
	return financeops.PostingOutcome{Reference: response.Data.ID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}

type financialPostingAdapter struct {
	wallet     walletPostingAdapter
	accounting accountingPostingAdapter
}

func (a financialPostingAdapter) Configured(operationType string) bool {
	switch operationType {
	case "PROVIDER_ALLOCATION", "PAYROLL_REVERSAL":
		return a.wallet.Configured()
	case "EXPENSE_POST", "EXPENSE_REVERSAL", "SETTLEMENT_REQUEST", "SETTLEMENT_REVERSAL":
		return a.accounting.Configured()
	default:
		return false
	}
}

func (a financialPostingAdapter) Submit(ctx context.Context, meta platformclients.CallMeta, request financeops.PostingRequest) (financeops.PostingOutcome, error) {
	if request.OperationType == "PROVIDER_ALLOCATION" || request.OperationType == "PAYROLL_REVERSAL" {
		return a.wallet.Submit(ctx, meta, request)
	}
	return a.accounting.Submit(ctx, meta, request)
}

func (a financialPostingAdapter) ReadByKey(ctx context.Context, meta platformclients.CallMeta, request financeops.PostingRequest) (financeops.PostingOutcome, error) {
	if request.OperationType == "PROVIDER_ALLOCATION" || request.OperationType == "PAYROLL_REVERSAL" {
		return a.wallet.ReadByKey(ctx, meta, request)
	}
	return a.accounting.ReadByKey(ctx, meta, request)
}

func (a walletPostingAdapter) ReadByKey(ctx context.Context, meta platformclients.CallMeta, request financeops.PostingRequest) (financeops.PostingOutcome, error) {
	if !a.Configured() {
		return financeops.PostingOutcome{}, platformclients.ErrNotConfigured
	}
	operationType := request.OperationType
	if operationType == "PAYROLL_REVERSAL" {
		operationType = "PROVIDER_ALLOCATION"
	}
	path := "/api/v1/payment-wallet/internal/finance/operations/" + url.PathEscape(operationType) +
		"/by-idempotency?organizationId=" + url.QueryEscape(request.OrganizationID) +
		"&idempotencyKey=" + url.QueryEscape(request.IdempotencyKey)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Version int64  `json:"version"`
		} `json:"data"`
	}
	err := a.client.Do(ctx, meta, http.MethodGet, path, nil, &response)
	return financeops.PostingOutcome{Reference: response.Data.ID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}

type notificationDeliveryAdapter struct{ client *platformclients.Client }

func (a notificationDeliveryAdapter) Configured() bool {
	return a.client != nil && a.client.Configured()
}
func (a notificationDeliveryAdapter) Submit(ctx context.Context, organizationID, key string, payload json.RawMessage) (engagement.DeliveryOutcome, error) {
	if !a.Configured() {
		return engagement.DeliveryOutcome{}, platformclients.ErrNotConfigured
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return engagement.DeliveryOutcome{}, err
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			MessageID string `json:"message_id"`
			Status    string `json:"status"`
			Version   int64  `json:"version"`
		} `json:"data"`
	}
	meta := platformclients.CallMeta{OrganizationID: organizationID, IdempotencyKey: key}
	err := a.client.Do(ctx, meta, http.MethodPost, "/api/v1/notifications/send", body, &response)
	return engagement.DeliveryOutcome{Reference: response.Data.MessageID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}
func (a notificationDeliveryAdapter) Read(ctx context.Context, organizationID, key string) (engagement.DeliveryOutcome, error) {
	if !a.Configured() {
		return engagement.DeliveryOutcome{}, platformclients.ErrNotConfigured
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			MessageID string `json:"id"`
			Status    string `json:"status"`
			Version   int64  `json:"version"`
		} `json:"data"`
	}
	path := "/api/v1/notifications/messages/by-idempotency/" + url.PathEscape(key)
	err := a.client.Do(ctx, platformclients.CallMeta{OrganizationID: organizationID}, http.MethodGet, path, nil, &response)
	return engagement.DeliveryOutcome{Reference: response.Data.MessageID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}

type supportCaseAdapter struct{ client *platformclients.Client }

func (a supportCaseAdapter) Configured() bool { return a.client != nil && a.client.Configured() }
func (a supportCaseAdapter) Create(ctx context.Context, organizationID, key string, input engagement.SupportCreateInput) (engagement.SupportProjection, error) {
	if !a.Configured() {
		return engagement.SupportProjection{}, platformclients.ErrNotConfigured
	}
	key = strings.TrimSpace(key)
	body := map[string]any{"organization_reference": organizationID, "idempotency_key": key, "case_type": input.CaseType, "priority": input.Priority, "subject": input.Subject, "description": input.Description, "customer_reference": input.CustomerReference, "provider_reference": input.ProviderReference, "trip_reference": input.TripReference, "order_reference": input.OrderReference, "assignment_reference": input.AssignmentReference}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ID             string `json:"id"`
			Status         string `json:"status"`
			Priority       string `json:"priority"`
			OwnerReference string `json:"owner_reference"`
			Version        int64  `json:"version"`
		} `json:"data"`
	}
	err := a.client.Do(ctx, platformclients.CallMeta{OrganizationID: organizationID, IdempotencyKey: key}, http.MethodPost, "/api/v1/support-disputes/admin/organization-cases", body, &response)
	return engagement.SupportProjection{CaseID: response.Data.ID, Status: response.Data.Status, Priority: response.Data.Priority, OwnerReference: response.Data.OwnerReference, SourceVersion: response.Data.Version}, err
}
func (a supportCaseAdapter) Read(ctx context.Context, organizationID, caseID string) (engagement.SupportProjection, error) {
	if !a.Configured() {
		return engagement.SupportProjection{}, platformclients.ErrNotConfigured
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ID             string `json:"id"`
			Status         string `json:"status"`
			Priority       string `json:"priority"`
			OwnerReference string `json:"owner_reference"`
			Version        int64  `json:"version"`
		} `json:"data"`
	}
	path := "/api/v1/support-disputes/admin/organization-cases/" + url.PathEscape(caseID) + "?organization_reference=" + url.QueryEscape(organizationID)
	err := a.client.Do(ctx, platformclients.CallMeta{OrganizationID: organizationID}, http.MethodGet, path, nil, &response)
	return engagement.SupportProjection{CaseID: response.Data.ID, Status: response.Data.Status, Priority: response.Data.Priority, OwnerReference: response.Data.OwnerReference, SourceVersion: response.Data.Version}, err
}
