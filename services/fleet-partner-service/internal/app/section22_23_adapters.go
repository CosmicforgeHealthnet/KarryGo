package app

import (
	"context"
	communicationsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	engagementmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	financeopsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type walletPostingAdapter struct{ client *platformclients.Client }

func (a walletPostingAdapter) Configured() bool { return a.client != nil && a.client.Configured() }

func (a walletPostingAdapter) Submit(ctx context.Context, meta platformclients.CallMeta, request financeopsmodels.PostingRequest) (financeopsmodels.PostingOutcome, error) {
	if !a.Configured() {
		return financeopsmodels.PostingOutcome{}, platformclients.ErrNotConfigured
	}
	var fields map[string]any
	if len(request.Payload) != 0 {
		if err := json.Unmarshal(request.Payload, &fields); err != nil {
			return financeopsmodels.PostingOutcome{}, err
		}
	} else {
		fields = map[string]any{}
	}
	fields["payloadVersion"] = 1
	fields["organizationId"] = request.OrganizationID
	fields["idempotencyKey"] = request.IdempotencyKey
	fields["expectedVersion"] = request.ExpectedVersion

	// Section 12. A managed work settlement is not a provider allocation: it
	// releases one job's escrow across platform, organization and provider in
	// a single balanced journal, so it has its own contract and its own
	// read-back. It carries Fleet's resolved figures verbatim -- Wallet
	// verifies them against the held payment and posts, or refuses.
	if request.OperationType == financeopsmodels.OperationManagedWorkSettlement {
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				ID                 string `json:"id"`
				InstructionVersion int64  `json:"instruction_version"`
				Status             string `json:"status"`
			} `json:"data"`
		}
		err := a.client.Do(ctx, meta, http.MethodPost,
			"/api/v1/payment-wallet/internal/finance/v1/managed-settlements", fields, &response)
		return financeopsmodels.PostingOutcome{
			Reference: response.Data.ID, Status: response.Data.Status,
			SourceVersion: response.Data.InstructionVersion,
		}, err
	}

	path := ""
	switch request.OperationType {
	case "PROVIDER_ALLOCATION", "PAYROLL_REVERSAL":
		path = "/provider-allocations"
	case "SETTLEMENT_REQUEST":
		path = "/settlements"
	case "REVERSAL_REQUEST":
		path = "/reversals"
	default:
		return financeopsmodels.PostingOutcome{}, fmt.Errorf("wallet posting: unsupported operation %s", request.OperationType)
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
	return financeopsmodels.PostingOutcome{Reference: response.Data.ID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}

// accountingPostingAdapter is deliberately an export/operation contract, not
// a local ledger. The configured accounting authority owns posting and
// reversal execution; Fleet retains only opaque references and reconciliation
// state.
type accountingPostingAdapter struct{ client *platformclients.Client }

func (a accountingPostingAdapter) Configured() bool { return a.client != nil && a.client.Configured() }

func (a accountingPostingAdapter) Submit(ctx context.Context, meta platformclients.CallMeta, request financeopsmodels.PostingRequest) (financeopsmodels.PostingOutcome, error) {
	if !a.Configured() {
		return financeopsmodels.PostingOutcome{}, platformclients.ErrNotConfigured
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
			return financeopsmodels.PostingOutcome{}, err
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
	return financeopsmodels.PostingOutcome{Reference: response.Data.ID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}

func (a accountingPostingAdapter) ReadByKey(ctx context.Context, meta platformclients.CallMeta, request financeopsmodels.PostingRequest) (financeopsmodels.PostingOutcome, error) {
	if !a.Configured() {
		return financeopsmodels.PostingOutcome{}, platformclients.ErrNotConfigured
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
	return financeopsmodels.PostingOutcome{Reference: response.Data.ID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}

type financialPostingAdapter struct {
	wallet     walletPostingAdapter
	accounting accountingPostingAdapter
}

func (a financialPostingAdapter) Configured(operationType string) bool {
	switch operationType {
	case "PROVIDER_ALLOCATION", "PAYROLL_REVERSAL", financeopsmodels.OperationManagedWorkSettlement:
		return a.wallet.Configured()
	case "EXPENSE_POST", "EXPENSE_REVERSAL", "SETTLEMENT_REQUEST", "SETTLEMENT_REVERSAL":
		return a.accounting.Configured()
	default:
		return false
	}
}

// walletOperation reports whether an operation belongs to payment-wallet.
// A managed work settlement does: it releases job escrow in Wallet's ledger.
// Routing it to the accounting adapter (the old default) would have sent the
// money instruction to the wrong authority.
func walletOperation(operationType string) bool {
	switch operationType {
	case "PROVIDER_ALLOCATION", "PAYROLL_REVERSAL", financeopsmodels.OperationManagedWorkSettlement:
		return true
	default:
		return false
	}
}

func (a financialPostingAdapter) Submit(ctx context.Context, meta platformclients.CallMeta, request financeopsmodels.PostingRequest) (financeopsmodels.PostingOutcome, error) {
	if walletOperation(request.OperationType) {
		return a.wallet.Submit(ctx, meta, request)
	}
	return a.accounting.Submit(ctx, meta, request)
}

func (a financialPostingAdapter) ReadByKey(ctx context.Context, meta platformclients.CallMeta, request financeopsmodels.PostingRequest) (financeopsmodels.PostingOutcome, error) {
	if walletOperation(request.OperationType) {
		return a.wallet.ReadByKey(ctx, meta, request)
	}
	return a.accounting.ReadByKey(ctx, meta, request)
}

func (a walletPostingAdapter) ReadByKey(ctx context.Context, meta platformclients.CallMeta, request financeopsmodels.PostingRequest) (financeopsmodels.PostingOutcome, error) {
	if !a.Configured() {
		return financeopsmodels.PostingOutcome{}, platformclients.ErrNotConfigured
	}
	// A managed settlement is read back by the key it was posted under. This
	// is what turns an ambiguous timeout into a definite answer instead of a
	// guess (WAL-017, Invariant 8).
	if request.OperationType == financeopsmodels.OperationManagedWorkSettlement {
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				ID                 string `json:"id"`
				InstructionVersion int64  `json:"instruction_version"`
				Status             string `json:"status"`
			} `json:"data"`
		}
		err := a.client.Do(ctx, meta, http.MethodGet,
			"/api/v1/payment-wallet/internal/finance/v1/managed-settlements/by-key/"+
				url.PathEscape(request.IdempotencyKey), nil, &response)
		return financeopsmodels.PostingOutcome{
			Reference: response.Data.ID, Status: response.Data.Status,
			SourceVersion: response.Data.InstructionVersion,
		}, err
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
	return financeopsmodels.PostingOutcome{Reference: response.Data.ID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}

type notificationDeliveryAdapter struct{ client *platformclients.Client }

func (a notificationDeliveryAdapter) Configured() bool {
	return a.client != nil && a.client.Configured()
}
func (a notificationDeliveryAdapter) Submit(ctx context.Context, organizationID, key string, payload json.RawMessage) (communicationsmodels.DeliveryOutcome, error) {
	if !a.Configured() {
		return communicationsmodels.DeliveryOutcome{}, platformclients.ErrNotConfigured
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return communicationsmodels.DeliveryOutcome{}, err
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
	return communicationsmodels.DeliveryOutcome{Reference: response.Data.MessageID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}
func (a notificationDeliveryAdapter) Read(ctx context.Context, organizationID, key string) (communicationsmodels.DeliveryOutcome, error) {
	if !a.Configured() {
		return communicationsmodels.DeliveryOutcome{}, platformclients.ErrNotConfigured
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
	return communicationsmodels.DeliveryOutcome{Reference: response.Data.MessageID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}

type supportCaseAdapter struct{ client *platformclients.Client }

func (a supportCaseAdapter) Configured() bool { return a.client != nil && a.client.Configured() }
func (a supportCaseAdapter) Create(ctx context.Context, organizationID, key string, input engagementmodels.SupportCreateInput) (engagementmodels.SupportProjection, error) {
	if !a.Configured() {
		return engagementmodels.SupportProjection{}, platformclients.ErrNotConfigured
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
	return engagementmodels.SupportProjection{CaseID: response.Data.ID, Status: response.Data.Status, Priority: response.Data.Priority, OwnerReference: response.Data.OwnerReference, SourceVersion: response.Data.Version}, err
}

// supportMessageResponse is support-dispute-service's organization message.
type supportMessageResponse struct {
	Success bool `json:"success"`
	Data    struct {
		ID              string    `json:"id"`
		ClientMessageID string    `json:"client_message_id"`
		Content         string    `json:"content"`
		Visibility      string    `json:"visibility"`
		FileIDs         []string  `json:"file_ids"`
		CreatedAt       time.Time `json:"created_at"`
	} `json:"data"`
}

func (r supportMessageResponse) message() engagementmodels.SupportMessage {
	return engagementmodels.SupportMessage{ID: r.Data.ID, ClientMessageID: r.Data.ClientMessageID, Text: r.Data.Content,
		Visibility: r.Data.Visibility, FileIDs: r.Data.FileIDs, CreatedAt: r.Data.CreatedAt}
}

// CreateMessage posts an organization message on the organization's own
// platform case; support-dispute-service stores it once per client message id.
func (a supportCaseAdapter) CreateMessage(ctx context.Context, request engagementmodels.SupportMessageRequest) (engagementmodels.SupportMessage, error) {
	if !a.Configured() {
		return engagementmodels.SupportMessage{}, platformclients.ErrNotConfigured
	}
	body := map[string]any{"organization_reference": request.OrganizationID, "client_message_id": request.ClientMessageID,
		"sender_reference": request.SenderReference, "text": request.Text, "file_ids": request.FileIDs, "visibility": request.Visibility}
	var response supportMessageResponse
	path := "/api/v1/support-disputes/admin/organization-cases/" + url.PathEscape(request.SupportCaseID) + "/messages"
	err := a.client.Do(ctx, platformclients.CallMeta{OrganizationID: request.OrganizationID, IdempotencyKey: request.ClientMessageID}, http.MethodPost, path, body, &response)
	return response.message(), err
}

// ReadMessage reads an organization message back by its client message id.
func (a supportCaseAdapter) ReadMessage(ctx context.Context, organizationID, supportCaseID, clientMessageID string) (engagementmodels.SupportMessage, error) {
	if !a.Configured() {
		return engagementmodels.SupportMessage{}, platformclients.ErrNotConfigured
	}
	var response supportMessageResponse
	path := "/api/v1/support-disputes/admin/organization-cases/" + url.PathEscape(supportCaseID) + "/messages/by-client-id/" +
		url.PathEscape(clientMessageID) + "?organization_reference=" + url.QueryEscape(organizationID)
	err := a.client.Do(ctx, platformclients.CallMeta{OrganizationID: organizationID}, http.MethodGet, path, nil, &response)
	return response.message(), err
}

func (a supportCaseAdapter) Read(ctx context.Context, organizationID, caseID string) (engagementmodels.SupportProjection, error) {
	if !a.Configured() {
		return engagementmodels.SupportProjection{}, platformclients.ErrNotConfigured
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
	return engagementmodels.SupportProjection{CaseID: response.Data.ID, Status: response.Data.Status, Priority: response.Data.Priority, OwnerReference: response.Data.OwnerReference, SourceVersion: response.Data.Version}, err
}
