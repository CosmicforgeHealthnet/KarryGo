//go:build ignore

// Command generate writes the Section 14 CRM OpenAPI contract from the same
// route catalogue used by the production Gin registrars.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	crmhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/http"
)

type M = map[string]any

func main() {
	_, here, _, _ := runtime.Caller(0)
	out := filepath.Join(filepath.Dir(here), "section14-crm.openapi.json")
	spec := build()
	b, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		panic(err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(out, b, 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s (%d paths, %d operations, %d schemas)\n", out,
		len(spec["paths"].(M)), operationCount(spec["paths"].(M)),
		len(spec["components"].(M)["schemas"].(M)))
}

func build() M {
	h := crmhttp.NewHandler(nil, nil)
	p := crmhttp.NewPublicHandler(nil, nil)
	i := crmhttp.NewInternalHandler(nil)
	routes := crmhttp.AllRoutes(h, p, i)
	paths := M{}
	for _, r := range routes {
		path := openAPIPath(r.FullPath())
		item, _ := paths[path].(M)
		if item == nil {
			item = M{}
			paths[path] = item
		}
		item[strings.ToLower(r.Method)] = operation(r, path)
	}
	return M{
		"openapi": "3.1.0",
		"info": M{
			"title":       "KarryGo Fleet Partner — Section 14 CRM API",
			"version":     "14.0.0",
			"description": "This specification covers Section 14 only. Section 13 is documented separately and Section 15 is deferred. Quotation acceptance creates a conversion intent, not an order. CRM customers are separate from marketplace customer identities. Fleet does not own accounts receivable and does not calculate marketplace fares, commissions, debt, or available credit.",
		},
		"servers": []any{M{"url": "https://api.example.invalid", "description": "Deployment-specific Fleet Partner API origin"}},
		"tags":    tags(), "paths": paths,
		"security": []any{M{"CorporateBearer": []any{}}},
		"components": M{
			"securitySchemes": securitySchemes(),
			"parameters":      parameters(),
			"responses":       responses(),
			"schemas":         schemas(),
		},
	}
}

func operation(r crmhttp.Route, path string) M {
	tag := tagFor(path)
	op := M{
		"operationId":                 r.OperationID,
		"tags":                        []any{tag},
		"summary":                     humanize(r.OperationID),
		"description":                 description(r),
		"x-handler":                   r.Handler,
		"x-feature-flag":              r.FeatureFlag,
		"x-idempotency-required":      r.IdempotencyRequired,
		"x-expected-version-required": r.ExpectedVersionRequired,
		"x-tenant-scope":              r.Security == crmhttp.SecurityCorporate,
		"x-branch-scope":              r.Security == crmhttp.SecurityCorporate,
	}
	if r.Permission != "" {
		op["x-permission"] = r.Permission
	}
	switch r.Security {
	case crmhttp.SecurityCustomerToken:
		op["security"] = []any{M{"QuotationToken": []any{}}}
	case crmhttp.SecurityInternalHMAC:
		op["security"] = []any{M{"ServiceHMACv2": []any{}}}
	default:
		op["security"] = []any{M{"CorporateBearer": []any{}}}
	}
	params := []any{}
	for _, name := range pathParameters(path) {
		params = append(params, M{"name": name, "in": "path", "required": true,
			"schema":      M{"type": "string", "minLength": 1, "maxLength": 128},
			"description": "Opaque resource identifier."})
	}
	params = append(params, M{"$ref": "#/components/parameters/CorrelationID"}, M{"$ref": "#/components/parameters/RequestID"})
	if r.Security == crmhttp.SecurityCorporate {
		params = append(params, M{"$ref": "#/components/parameters/BranchID"})
	}
	if r.Security == crmhttp.SecurityInternalHMAC {
		for _, n := range []string{"ServiceName", "ServiceTimestamp", "ServiceNonce", "ServiceSignatureVersion", "ServiceSignature"} {
			params = append(params, M{"$ref": "#/components/parameters/" + n})
		}
	}
	if r.Security == crmhttp.SecurityCustomerToken {
		params = append(params, M{"$ref": "#/components/parameters/QuotationTokenQuery"})
	}
	if r.IdempotencyRequired {
		params = append(params, M{"$ref": "#/components/parameters/IdempotencyKey"})
	}
	if r.ExpectedVersionRequired {
		params = append(params, M{"$ref": "#/components/parameters/IfMatch"})
	}
	if strings.HasPrefix(r.OperationID, "list") || strings.Contains(r.OperationID, "History") || strings.Contains(r.OperationID, "Observations") {
		params = append(params, M{"$ref": "#/components/parameters/Cursor"}, M{"$ref": "#/components/parameters/Limit"}, M{"$ref": "#/components/parameters/Sort"})
		for _, name := range queryFilters(r.OperationID) {
			params = append(params, M{"name": name, "in": "query", "required": false, "schema": M{"type": "string", "maxLength": 256}})
		}
	}
	op["parameters"] = params
	if request := requestSchema(r.OperationID); request != "" {
		op["requestBody"] = M{"required": requestRequired(r.OperationID), "content": M{"application/json": M{"schema": ref(request), "examples": safeExample(r.OperationID)}}}
	}
	status, response := success(r.OperationID)
	op["responses"] = operationResponses(r, status, response)
	return op
}

func operationResponses(r crmhttp.Route, status, schema string) M {
	result := M{status: M{"description": "Successful Section 14 operation.", "content": M{"application/json": M{"schema": envelope(schema, strings.HasPrefix(r.OperationID, "list") || strings.Contains(r.OperationID, "History") || strings.Contains(r.OperationID, "Versions") || strings.Contains(r.OperationID, "Documents") || strings.Contains(r.OperationID, "Observations"))}}}}
	if r.OperationID == "ingestServiceLevelObservation" {
		result["201"] = M{"description": "New SLA observation recorded.", "content": M{"application/json": M{"schema": envelope(schema, false)}}}
	}
	for _, code := range []string{"400", "401", "403", "404", "422", "429", "500", "503"} {
		result[code] = M{"$ref": "#/components/responses/Error" + code}
	}
	if r.IdempotencyRequired || r.ExpectedVersionRequired {
		result["409"] = M{"$ref": "#/components/responses/Error409"}
	}
	if r.Security == crmhttp.SecurityCustomerToken {
		result["410"] = M{"$ref": "#/components/responses/Error410"}
	}
	return result
}

func envelope(name string, list bool) M {
	data := any(ref(name))
	if list {
		data = M{"type": "array", "items": ref(name)}
	}
	return M{"type": "object", "required": []any{"data", "meta"}, "properties": M{"data": data, "meta": ref("ResponseMeta")}}
}

func requestSchema(id string) string {
	m := map[string]string{
		"createCustomer": "CustomerCreateRequest", "updateCustomer": "CustomerUpdateRequest", "archiveCustomer": "ReasonedVersionCommand", "changeCustomerStatus": "CustomerStatusCommand", "changeCustomerRiskStatus": "CustomerRiskCommand",
		"createContact": "ContactCreateRequest", "updateContact": "ContactUpdateRequest", "archiveContact": "ReasonedVersionCommand", "makeContactPrimary": "ExpectedVersionCommand",
		"createLocation": "LocationCreateRequest", "updateLocation": "LocationUpdateRequest", "archiveLocation": "ReasonedVersionCommand",
		"setCreditProfile": "CreditProfileRequest", "applyCreditHold": "CreditHoldCommand", "releaseCreditHold": "CreditReleaseCommand",
		"createOpportunity": "OpportunityCreateRequest", "updateOpportunity": "OpportunityUpdateRequest", "markOpportunityWon": "OpportunityWonCommand", "markOpportunityLost": "OpportunityLostCommand", "holdOpportunity": "OpportunityHoldCommand", "resumeOpportunity": "OpportunityResumeCommand",
		"createQuotation": "QuotationCreateRequest", "updateQuotation": "QuotationUpdateRequest", "submitQuotation": "QuotationSubmitRequest", "issueQuotation": "QuotationIssueRequest", "acceptQuotationAsStaff": "QuotationAcceptanceRequest", "rejectQuotationAsStaff": "QuotationRejectionRequest", "cancelQuotation": "QuotationCancellationRequest", "supersedeQuotation": "QuotationSupersessionRequest", "attachQuotationDocument": "DocumentAttachRequest",
		"acceptQuotationAsCustomer": "CustomerQuotationAcceptanceRequest", "rejectQuotationAsCustomer": "QuotationRejectionRequest",
		"createContract": "ContractCreateRequest", "updateContract": "ContractUpdateRequest", "submitContract": "ContractSubmissionRequest", "activateContract": "ContractActivationRequest", "suspendContract": "ContractLifecycleRequest", "resumeContract": "ContractLifecycleRequest", "terminateContract": "ContractLifecycleRequest", "renewContract": "ContractRenewalRequest", "attachContractDocument": "DocumentAttachRequest", "replaceContractDocument": "DocumentReplaceRequest", "removeContractDocument": "DocumentRemoveRequest",
		"createServiceLevelAgreement": "SLACreateRequest", "updateServiceLevelAgreement": "SLAUpdateRequest", "activateServiceLevelAgreement": "SLALifecycleRequest", "suspendServiceLevelAgreement": "SLALifecycleRequest", "terminateServiceLevelAgreement": "SLALifecycleRequest", "ingestServiceLevelObservation": "SLAObservationIngestionRequest",
		"replayNotificationOperation": "EmptyRequest",
	}
	return m[id]
}

func requestRequired(id string) bool { return requestSchema(id) != "EmptyRequest" }

func success(id string) (string, string) {
	if id == "replayNotificationOperation" {
		return "202", "NotificationOperationReference"
	}
	created := strings.HasPrefix(id, "create") || strings.HasPrefix(id, "attach") || id == "renewContract"
	status := "200"
	if created {
		status = "201"
	}
	if id == "listCustomerHistory" {
		return status, "CustomerHistoryItem"
	}
	if strings.Contains(id, "Customer") && !strings.Contains(id, "Quotation") && !strings.Contains(id, "Credit") {
		return status, "CustomerResponse"
	}
	if strings.Contains(id, "Contact") {
		return status, "ContactResponse"
	}
	if strings.Contains(id, "Location") {
		return status, "LocationResponse"
	}
	if strings.Contains(id, "Opportunity") {
		if strings.Contains(id, "History") {
			return status, "OpportunityHistoryItem"
		}
		return status, "OpportunityResponse"
	}
	if strings.Contains(id, "Credit") {
		if strings.Contains(id, "History") {
			return status, "CreditHistoryItem"
		}
		return status, "CreditProfileResponse"
	}
	if strings.Contains(id, "Quotation") {
		switch {
		case id == "viewQuotationAsCustomer":
			return status, "SecureCustomerQuotationResponse"
		case id == "issueQuotation":
			return status, "QuotationIssueResponse"
		case strings.Contains(id, "acceptQuotationAsCustomer"):
			return status, "CustomerQuotationAcceptanceResponse"
		case strings.Contains(id, "rejectQuotationAsCustomer"):
			return status, "CustomerQuotationResolutionResponse"
		case strings.Contains(id, "acceptQuotationAsStaff"):
			return status, "QuotationAcceptanceResponse"
		case strings.Contains(id, "Version"):
			return status, "QuotationVersionResponse"
		case strings.Contains(id, "History"):
			return status, "QuotationHistoryItem"
		case strings.Contains(id, "Document"):
			return status, "MediaObjectReference"
		default:
			return status, "QuotationResponse"
		}
	}
	if strings.Contains(id, "Contract") {
		switch {
		case id == "activateContract":
			return status, "ContractActivationResponse"
		case id == "removeContractDocument":
			return status, "DocumentRemovalResponse"
		case strings.Contains(id, "Version"):
			return status, "ContractVersionResponse"
		case strings.Contains(id, "History"):
			return status, "ContractHistoryItem"
		case strings.Contains(id, "Document"):
			return status, "ContractDocumentReference"
		default:
			return status, "ContractResponse"
		}
	}
	if strings.Contains(id, "ServiceLevel") {
		if id == "ingestServiceLevelObservation" {
			return status, "SLAObservationIngestionResponse"
		}
		if strings.Contains(id, "Observation") {
			return status, "SLAObservationResponse"
		}
		if strings.Contains(id, "History") {
			return status, "SLAHistoryItem"
		}
		return status, "SLAResponse"
	}
	if strings.Contains(id, "Notification") {
		return status, "NotificationOperationReference"
	}
	return status, "GenericResource"
}

func schemas() M {
	s := M{}
	str := func(max int) M { return M{"type": "string", "maxLength": max} }
	id := M{"type": "string", "minLength": 1, "maxLength": 128, "description": "Opaque identifier."}
	dt := M{"type": "string", "format": "date-time"}
	date := M{"type": "string", "format": "date"}
	integer := M{"type": "integer", "format": "int64"}
	s["ErrorDetail"] = obj(M{"field": str(128), "reason": str(512)}, nil)
	s["StableErrorCode"] = enum("bad_request", "validation_failed", "validation_error", "unauthorized", "forbidden", "not_found", "conflict", "idempotency_conflict", "expected_version_mismatch", "rate_limited", "internal_error", "service_unavailable", "customer_not_found", "customer_already_exists", "customer_on_hold", "customer_risk_restricted", "customer_archived", "customer_invalid_state", "customer_contact_not_found", "customer_contact_conflict", "customer_location_not_found", "customer_location_invalid_timezone", "customer_location_invalid_coordinates", "opportunity_not_found", "opportunity_invalid_transition", "quotation_not_found", "quotation_invalid_state", "quotation_expired", "quotation_already_resolved", "quotation_approval_required", "quotation_version_conflict", "quotation_document_not_ready", "quotation_acceptance_conflict", "quotation_token_invalid", "quotation_token_expired", "contract_not_found", "contract_invalid_state", "contract_not_active", "contract_activation_blocked", "contract_approval_required", "contract_document_not_ready", "contract_version_conflict", "sla_not_found", "sla_invalid_state", "sla_observation_conflict", "sla_source_unknown", "credit_profile_not_found", "customer_credit_hold", "credit_change_approval_required")
	s["Error"] = obj(M{"code": ref("StableErrorCode"), "message": str(1024), "details": M{"type": "array", "items": ref("ErrorDetail")}, "requestId": str(128)}, []string{"code", "message", "details", "requestId"})
	s["ErrorEnvelope"] = obj(M{"error": ref("Error")}, []string{"error"})
	s["ResponseMeta"] = obj(M{"requestId": str(128), "nextCursor": str(2048)}, []string{"requestId"})
	s["Money"] = obj(M{"amountMinor": integer, "currency": M{"type": "string", "pattern": "^[A-Z]{3}$"}}, []string{"amountMinor", "currency"})
	s["VersionMetadata"] = obj(M{"version": M{"type": "integer", "format": "int64", "minimum": 1}, "updatedAt": dt}, []string{"version", "updatedAt"})
	s["FreshnessMetadata"] = obj(M{"sourceVersion": integer, "syncedAt": dt}, []string{"sourceVersion"})
	s["AuditSafeReference"] = obj(M{"id": id, "type": str(64)}, []string{"id", "type"})
	s["Address"] = obj(M{"line1": str(256), "line2": str(256), "city": str(128), "stateOrRegion": str(128), "postalCode": str(32), "countryCode": M{"type": "string", "pattern": "^[A-Z]{2}$"}}, []string{"line1", "city", "countryCode"})
	s["ExpectedVersionCommand"] = obj(M{"expectedVersion": M{"type": "integer", "format": "int64", "minimum": 1}}, []string{"expectedVersion"})
	s["ReasonedVersionCommand"] = obj(M{"expectedVersion": M{"type": "integer", "format": "int64", "minimum": 1}, "reasonCode": str(128)}, []string{"expectedVersion"})
	s["EmptyRequest"] = obj(M{}, nil)

	// Customer, contact, location and sales DTOs mirror the JSON projections in http/dto.go.
	s["CustomerType"] = enum("INDIVIDUAL", "BUSINESS", "GOVERNMENT", "NONPROFIT", "INTERNAL_BUSINESS_UNIT", "OTHER")
	s["CustomerStatus"] = enum("PROSPECT", "ACTIVE", "ON_HOLD", "SUSPENDED", "INACTIVE", "ARCHIVED")
	s["CustomerRiskStatus"] = enum("NOT_ASSESSED", "LOW", "MEDIUM", "HIGH", "RESTRICTED")
	s["CustomerCreateRequest"] = obj(M{"branchId": id, "customerType": ref("CustomerType"), "legalName": str(256), "displayName": str(256), "registrationNumber": str(128), "taxReference": str(128), "marketplaceCustomerId": id, "accountOwnerUserId": id, "defaultCurrency": M{"type": "string", "pattern": "^[A-Z]{3}$"}, "paymentTermsReference": str(128)}, []string{"customerType", "legalName", "defaultCurrency"})
	s["CustomerUpdateRequest"] = obj(M{"branchId": id, "legalName": str(256), "displayName": str(256), "registrationNumber": str(128), "taxReference": str(128), "marketplaceCustomerId": id, "accountOwnerUserId": id, "defaultCurrency": M{"type": "string", "pattern": "^[A-Z]{3}$"}, "paymentTermsReference": str(128), "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"expectedVersion"})
	s["CustomerStatusCommand"] = obj(M{"status": ref("CustomerStatus"), "reasonCode": str(128), "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"status", "expectedVersion"})
	s["CustomerRiskCommand"] = obj(M{"riskStatus": ref("CustomerRiskStatus"), "reasonCode": str(128), "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"riskStatus", "expectedVersion"})
	customer := M{"id": id, "branchId": id, "customerType": ref("CustomerType"), "legalName": str(256), "displayName": str(256), "registrationNumberMasked": str(128), "taxReferenceMasked": str(128), "marketplaceCustomerId": id, "status": ref("CustomerStatus"), "riskStatus": ref("CustomerRiskStatus"), "accountOwnerUserId": id, "defaultCurrency": str(3), "paymentTermsReference": str(128), "creditProfileId": id, "version": integer, "createdAt": dt, "updatedAt": dt, "archivedAt": dt}
	s["CustomerResponse"] = obj(customer, []string{"id", "customerType", "legalName", "displayName", "status", "riskStatus", "defaultCurrency", "version", "createdAt", "updatedAt"})
	s["SensitiveCustomerResponse"] = allOf("CustomerResponse", M{"registrationNumber": str(128), "taxReference": str(128)})
	s["CustomerHistoryItem"] = obj(M{"id": id, "changeType": str(64), "previousStatus": ref("CustomerStatus"), "newStatus": ref("CustomerStatus"), "previousRiskStatus": ref("CustomerRiskStatus"), "newRiskStatus": ref("CustomerRiskStatus"), "changedFields": M{"type": "array", "items": str(128)}, "reasonCode": str(128), "approvalRequestId": id, "actorType": str(64), "actorId": id, "occurredAt": dt}, []string{"id", "changeType", "changedFields", "actorType", "actorId", "occurredAt"})
	s["PreferredChannel"] = enum("EMAIL", "PHONE", "SMS", "WHATSAPP", "IN_APP", "NONE")
	s["ContactPurpose"] = enum("GENERAL", "OPERATIONS", "BILLING", "LEGAL", "SALES", "EMERGENCY", "OTHER")
	s["ConsentStatus"] = enum("UNKNOWN", "GRANTED", "WITHDRAWN", "NOT_REQUIRED")
	s["ContactCreateRequest"] = obj(M{"fullName": str(256), "roleTitle": str(128), "email": M{"type": "string", "format": "email", "maxLength": 320}, "phone": str(64), "preferredChannel": ref("PreferredChannel"), "contactPurpose": ref("ContactPurpose"), "isPrimary": M{"type": "boolean"}, "notificationPreferences": M{"type": "object"}, "consentStatus": ref("ConsentStatus"), "consentSource": str(128)}, []string{"fullName", "contactPurpose"})
	s["ContactUpdateRequest"] = obj(M{"fullName": str(256), "roleTitle": str(128), "email": M{"type": "string", "format": "email"}, "phone": str(64), "preferredChannel": ref("PreferredChannel"), "contactPurpose": ref("ContactPurpose"), "notificationPreferences": M{"type": "object"}, "consentStatus": ref("ConsentStatus"), "consentSource": str(128), "status": str(64), "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"expectedVersion"})
	s["ContactResponse"] = obj(M{"id": id, "customerId": id, "fullName": str(256), "roleTitle": str(128), "emailMasked": str(320), "phoneMasked": str(64), "preferredChannel": ref("PreferredChannel"), "channelDeliverable": M{"type": "boolean"}, "contactPurpose": ref("ContactPurpose"), "isPrimary": M{"type": "boolean"}, "notificationPreferences": M{"type": "object"}, "consentStatus": ref("ConsentStatus"), "consentRecordedAt": dt, "status": ref("LocationStatus"), "version": integer, "createdAt": dt, "updatedAt": dt, "archivedAt": dt}, []string{"id", "customerId", "fullName", "preferredChannel", "channelDeliverable", "contactPurpose", "isPrimary", "consentStatus", "status", "version", "createdAt", "updatedAt"})
	s["SensitiveContactResponse"] = allOf("ContactResponse", M{"email": M{"type": "string", "format": "email"}, "phone": str(64)})
	s["LocationType"] = enum("PICKUP", "DELIVERY", "BILLING", "WAREHOUSE", "OFFICE", "DEPOT", "SITE", "PORT", "TERMINAL", "OTHER")
	s["LocationStatus"] = enum("ACTIVE", "INACTIVE", "ARCHIVED")
	s["LocationCreateRequest"] = obj(M{"locationType": ref("LocationType"), "label": str(256), "address": ref("Address"), "timezone": M{"type": "string", "description": "IANA time-zone identifier."}, "latitude": M{"type": "number", "minimum": -90, "maximum": 90}, "longitude": M{"type": "number", "minimum": -180, "maximum": 180}, "geofenceReference": id, "operatingHours": M{"type": "object"}, "accessInstructions": str(2048), "safetyInstructions": str(2048), "contactReference": id}, []string{"locationType", "label", "address", "timezone"})
	s["LocationUpdateRequest"] = obj(M{"locationType": ref("LocationType"), "label": str(256), "status": ref("LocationStatus"), "address": ref("Address"), "timezone": M{"type": "string"}, "latitude": M{"type": "number", "minimum": -90, "maximum": 90}, "longitude": M{"type": "number", "minimum": -180, "maximum": 180}, "clearCoordinates": M{"type": "boolean"}, "geofenceReference": id, "operatingHours": M{"type": "object"}, "accessInstructions": str(2048), "safetyInstructions": str(2048), "contactReference": id, "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"expectedVersion"})
	s["LocationResponse"] = obj(M{"id": id, "customerId": id, "locationType": ref("LocationType"), "label": str(256), "city": str(128), "stateOrRegion": str(128), "countryCode": str(2), "timezone": str(128), "operatingHours": M{"type": "object"}, "contactReference": id, "geofenceReference": id, "hasCoordinates": M{"type": "boolean"}, "hasAccessInstructions": M{"type": "boolean"}, "hasSafetyInstructions": M{"type": "boolean"}, "status": ref("LocationStatus"), "version": integer, "createdAt": dt, "updatedAt": dt, "archivedAt": dt}, []string{"id", "customerId", "locationType", "label", "city", "stateOrRegion", "countryCode", "timezone", "hasCoordinates", "hasAccessInstructions", "hasSafetyInstructions", "status", "version", "createdAt", "updatedAt"})
	s["SensitiveLocationResponse"] = allOf("LocationResponse", M{"addressLine1": str(256), "addressLine2": str(256), "postalCode": str(32), "latitude": M{"type": "number", "minimum": -90, "maximum": 90}, "longitude": M{"type": "number", "minimum": -180, "maximum": 180}, "accessInstructions": str(2048), "safetyInstructions": str(2048)})
	s["OpportunityStage"] = enum("LEAD", "QUALIFIED", "NEEDS_ANALYSIS", "PROPOSAL", "NEGOTIATION", "WON", "LOST", "ON_HOLD")
	s["OpportunityCreateRequest"] = obj(M{"branchId": id, "customerId": id, "title": str(256), "description": str(4000), "expectedValue": ref("Money"), "probabilityBasisPoints": M{"type": "integer", "minimum": 0, "maximum": 10000}, "expectedCloseDate": date, "serviceCapabilities": M{"type": "array", "items": str(128)}, "accountOwnerUserId": id, "nextAction": str(512), "nextActionAt": dt}, []string{"customerId", "title"})
	s["OpportunityUpdateRequest"] = obj(M{"title": str(256), "description": str(4000), "expectedValue": ref("Money"), "clearExpectedValue": M{"type": "boolean"}, "probabilityBasisPoints": M{"type": "integer", "minimum": 0, "maximum": 10000}, "serviceCapabilities": M{"type": "array", "items": str(128)}, "accountOwnerUserId": id, "nextAction": str(512), "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"expectedVersion"})
	s["OpportunityWonCommand"] = obj(M{"expectedVersion": M{"type": "integer", "minimum": 1}, "reasonCode": str(128)}, []string{"expectedVersion"})
	s["OpportunityLostCommand"] = s["OpportunityWonCommand"]
	s["OpportunityHoldCommand"] = s["OpportunityWonCommand"]
	s["OpportunityResumeCommand"] = s["OpportunityWonCommand"]
	s["OpportunityResponse"] = obj(M{"id": id, "branchId": id, "customerId": id, "title": str(256), "description": str(4000), "stage": ref("OpportunityStage"), "expectedValue": ref("Money"), "probabilityBasisPoints": M{"type": "integer"}, "expectedCloseDate": date, "serviceCapabilities": M{"type": "array", "items": str(128)}, "accountOwnerUserId": id, "nextAction": str(512), "nextActionAt": dt, "lossReason": str(512), "holdReason": str(512), "version": integer, "createdAt": dt, "updatedAt": dt, "closedAt": dt}, []string{"id", "customerId", "title", "stage", "probabilityBasisPoints", "serviceCapabilities", "version", "createdAt", "updatedAt"})
	s["OpportunityHistoryItem"] = obj(M{"id": id, "previousStage": ref("OpportunityStage"), "newStage": ref("OpportunityStage"), "reasonCode": str(128), "actorType": str(64), "actorId": id, "occurredAt": dt}, []string{"id", "previousStage", "newStage", "actorType", "actorId", "occurredAt"})

	// Commercial schemas use integer minor units and decimal-safe quantity strings.
	s["QuotationStatus"] = enum("DRAFT", "AWAITING_APPROVAL", "APPROVED", "ISSUED", "VIEWED", "ACCEPTED", "REJECTED", "EXPIRED", "CANCELLED", "SUPERSEDED")
	s["ManualQuotationLineItem"] = obj(M{"description": str(512), "quantity": M{"type": "string", "pattern": "^[0-9]+(\\.[0-9]{1,6})?$"}, "unitType": str(64), "unitAmountMinor": integer, "lineAmountMinor": integer, "externalPricingReference": str(256), "manualReason": str(512)}, []string{"description", "quantity", "unitAmountMinor"})
	s["CustomerQuotationLineItem"] = obj(M{"description": str(512), "quantity": M{"type": "string", "pattern": "^[0-9]+(\\.[0-9]{1,6})?$"}, "unitType": str(64), "unitAmountMinor": integer, "lineAmountMinor": integer, "currency": M{"type": "string", "pattern": "^[A-Z]{3}$"}}, []string{"description", "quantity", "unitAmountMinor", "lineAmountMinor", "currency"})
	s["QuotationCreateRequest"] = obj(M{"branchId": id, "customerId": id, "opportunityId": id, "currency": M{"type": "string", "pattern": "^[A-Z]{3}$"}, "validFrom": dt, "validUntil": dt, "lineItems": M{"type": "array", "minItems": 1, "items": ref("ManualQuotationLineItem")}, "serviceScope": M{"type": "object"}, "terms": M{"type": "object"}, "internalNotes": str(4000)}, []string{"customerId", "currency", "validFrom", "validUntil", "lineItems"})
	s["QuotationUpdateRequest"] = obj(M{"validFrom": dt, "validUntil": dt, "lineItems": M{"type": "array", "items": ref("ManualQuotationLineItem")}, "replaceLineItems": M{"type": "boolean"}, "serviceScope": M{"type": "object"}, "terms": M{"type": "object"}, "internalNotes": str(4000), "opportunityId": id, "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"expectedVersion"})
	s["QuotationSubmitRequest"] = s["ReasonedVersionCommand"]
	s["QuotationIssueRequest"] = obj(M{"mintCustomerToken": M{"type": "boolean"}, "singleUseToken": M{"type": "boolean"}, "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"expectedVersion"})
	s["QuotationAcceptanceRequest"] = obj(M{"acceptedVersionId": id, "acceptanceMethod": str(64), "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"acceptedVersionId"})
	s["CustomerQuotationAcceptanceRequest"] = obj(M{"acceptedVersionId": id}, []string{"acceptedVersionId"})
	s["QuotationRejectionRequest"] = obj(M{"reasonCode": str(128), "note": str(2048), "actorReference": id, "evidenceDocumentReference": id, "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"reasonCode"})
	s["QuotationCancellationRequest"] = s["ReasonedVersionCommand"]
	s["QuotationSupersessionRequest"] = obj(M{"expectedVersion": M{"type": "integer", "minimum": 1}, "replacementQuotationId": id, "reasonCode": str(128)}, []string{"expectedVersion"})
	s["QuotationResponse"] = obj(M{"id": id, "branchId": id, "customerId": id, "opportunityId": id, "quotationNumber": str(64), "status": ref("QuotationStatus"), "total": ref("Money"), "validFrom": dt, "validUntil": dt, "serviceScope": M{"type": "object"}, "terms": M{"type": "object"}, "currentVersion": M{"type": "integer"}, "issuedVersionId": id, "acceptedVersionId": id, "approvalRequestId": id, "supersededById": id, "lineItems": M{"type": "array", "items": ref("ManualQuotationLineItem")}, "version": integer, "createdAt": dt, "updatedAt": dt, "issuedAt": dt, "resolvedAt": dt}, []string{"id", "customerId", "quotationNumber", "status", "total", "validFrom", "validUntil", "currentVersion", "version", "createdAt", "updatedAt"})
	s["QuotationVersionResponse"] = obj(M{"id": id, "quotationId": id, "versionNumber": M{"type": "integer", "minimum": 1}, "customerSnapshot": M{"type": "object"}, "contactSnapshot": M{"type": "object"}, "total": ref("Money"), "lineItems": M{"type": "array", "items": ref("ManualQuotationLineItem")}, "serviceScope": M{"type": "object"}, "terms": M{"type": "object"}, "validFrom": dt, "validUntil": dt, "documentReferences": M{"type": "array", "items": id}, "approvalRequestId": id, "issuedBy": id, "issuedAt": dt, "contentFingerprint": str(128)}, []string{"id", "quotationId", "versionNumber", "customerSnapshot", "total", "lineItems", "validFrom", "validUntil", "documentReferences", "issuedBy", "issuedAt", "contentFingerprint"})
	s["QuotationHistoryItem"] = obj(M{"id": id, "previousStatus": ref("QuotationStatus"), "newStatus": ref("QuotationStatus"), "quotationVersion": M{"type": "integer", "minimum": 0}, "reasonCode": str(128), "approvalRequestId": id, "actorType": str(64), "actorId": id, "occurredAt": dt}, []string{"id", "newStatus", "quotationVersion", "actorType", "occurredAt"})
	s["AcceptanceRecord"] = obj(M{"id": id, "quotationId": id, "acceptedVersionId": id, "actorType": str(64), "actorReference": id, "acceptanceMethod": str(64), "evidenceDocumentReference": id, "acceptedAt": dt}, []string{"id", "quotationId", "acceptedVersionId", "actorType", "acceptanceMethod", "acceptedAt"})
	s["ConversionIntent"] = obj(M{"id": id, "quotationId": id, "acceptedVersionId": id, "acceptanceId": id, "conversionType": M{"const": "FUTURE_PRIVATE_ORDER"}, "status": M{"const": "PENDING_SECTION_15"}, "createdAt": dt}, []string{"id", "quotationId", "acceptedVersionId", "acceptanceId", "conversionType", "status", "createdAt"})
	s["ConversionIntent"].(M)["example"] = M{"id": "conversion_example_01", "quotationId": "quo_example_01", "acceptedVersionId": "qv_example_01", "acceptanceId": "acceptance_example_01", "conversionType": "FUTURE_PRIVATE_ORDER", "status": "PENDING_SECTION_15", "createdAt": "2026-08-03T10:00:00Z"}
	s["QuotationAcceptanceResponse"] = obj(M{"quotation": ref("QuotationResponse"), "acceptance": ref("AcceptanceRecord"), "conversionIntent": ref("ConversionIntent"), "replayed": M{"type": "boolean"}}, []string{"quotation", "acceptance", "conversionIntent", "replayed"})
	s["CustomerQuotationAcceptanceResponse"] = obj(M{"quotationNumber": str(64), "status": ref("QuotationStatus"), "acceptedAt": dt, "conversionIntent": ref("ConversionIntent")}, []string{"quotationNumber", "status", "acceptedAt", "conversionIntent"})
	s["CustomerQuotationResolutionResponse"] = obj(M{"quotationNumber": str(64), "status": ref("QuotationStatus")}, []string{"quotationNumber", "status"})
	s["CustomerAccessTokenMetadataResponse"] = obj(M{"customerAccessToken": M{"type": "string", "writeOnly": true, "description": "Returned exactly once; never persisted raw."}, "customerAccessTokenExpiresAt": dt}, nil)
	s["QuotationIssueResponse"] = obj(M{"quotation": ref("QuotationResponse"), "issuedVersion": ref("QuotationVersionResponse"), "customerAccessToken": M{"type": "string", "writeOnly": true, "description": "Optional raw secure token returned exactly once when mintCustomerToken is true."}, "customerAccessTokenExpiresAt": dt}, []string{"quotation", "issuedVersion"})
	s["SecureCustomerQuotationResponse"] = obj(M{"quotationNumber": str(64), "status": ref("QuotationStatus"), "currency": str(3), "totalAmountMinor": integer, "validFrom": dt, "validUntil": dt, "issuedAt": dt, "customerName": str(256), "lineItems": M{"type": "array", "items": ref("CustomerQuotationLineItem")}, "serviceScope": M{"type": "object"}, "terms": M{"type": "object"}, "canRespond": M{"type": "boolean"}, "expired": M{"type": "boolean"}}, []string{"quotationNumber", "status", "currency", "totalAmountMinor", "validFrom", "validUntil", "issuedAt", "customerName", "lineItems", "canRespond", "expired"})
	s["SecureCustomerQuotationResponse"].(M)["example"] = M{"quotationNumber": "Q-EXAMPLE-001", "status": "ISSUED", "currency": "EUR", "totalAmountMinor": 25000, "validFrom": "2026-08-01T00:00:00Z", "validUntil": "2026-08-15T00:00:00Z", "issuedAt": "2026-08-01T09:00:00Z", "customerName": "Example Logistics", "lineItems": []any{M{"description": "Example service", "quantity": "2.000", "unitAmountMinor": 12500}}, "canRespond": true, "expired": false}

	s["ContractStatus"] = enum("DRAFT", "AWAITING_APPROVAL", "APPROVED", "ACTIVE", "SUSPENDED", "EXPIRED", "TERMINATED", "RENEWAL_PENDING", "SUPERSEDED")
	s["ContractCreateRequest"] = obj(M{"customerId": id, "quotationId": id, "effectiveFrom": dt, "effectiveTo": dt, "serviceScope": M{"type": "object"}, "commercialSnapshot": M{"type": "object"}, "renewalTerms": M{"type": "object"}, "terminationTerms": M{"type": "object"}}, []string{"customerId", "effectiveFrom"})
	s["ContractUpdateRequest"] = obj(M{"effectiveFrom": dt, "effectiveTo": dt, "serviceScope": M{"type": "object"}, "commercialSnapshot": M{"type": "object"}, "renewalTerms": M{"type": "object"}, "terminationTerms": M{"type": "object"}, "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"expectedVersion"})
	s["ContractSubmissionRequest"] = s["ReasonedVersionCommand"]
	s["ContractActivationRequest"] = obj(M{"expectedVersion": M{"type": "integer", "minimum": 1}, "signatoryReferences": M{"type": "array", "items": id}}, []string{"expectedVersion"})
	s["ContractLifecycleRequest"] = obj(M{"reasonCode": str(128), "effectiveAt": dt, "evidenceDocumentReference": id, "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"expectedVersion"})
	s["ContractRenewalRequest"] = obj(M{"expectedVersion": M{"type": "integer", "minimum": 1}, "effectiveFrom": dt, "effectiveTo": dt}, []string{"expectedVersion", "effectiveFrom"})
	s["ContractResponse"] = obj(M{"id": id, "customerId": id, "quotationId": id, "contractNumber": str(64), "status": ref("ContractStatus"), "effectiveFrom": dt, "effectiveTo": dt, "serviceScope": M{"type": "object"}, "currentVersion": M{"type": "integer"}, "version": integer, "createdAt": dt, "updatedAt": dt}, []string{"id", "customerId", "contractNumber", "status", "effectiveFrom", "currentVersion", "version", "createdAt", "updatedAt"})
	s["ContractVersionResponse"] = obj(M{"id": id, "contractId": id, "versionNumber": M{"type": "integer", "minimum": 1}, "customerSnapshot": M{"type": "object"}, "serviceScope": M{"type": "object"}, "commercialSnapshot": M{"type": "object"}, "effectiveFrom": dt, "effectiveTo": dt, "renewalTerms": M{"type": "object"}, "terminationTerms": M{"type": "object"}, "documentReferences": M{"type": "array", "items": id}, "signatoryReferences": M{"type": "array", "items": id}, "approvalRequestId": id, "contentFingerprint": str(128), "activatedBy": id, "activatedAt": dt}, []string{"id", "contractId", "versionNumber", "customerSnapshot", "effectiveFrom", "documentReferences", "signatoryReferences", "contentFingerprint", "activatedBy", "activatedAt"})
	s["ContractActivationResponse"] = obj(M{"contract": ref("ContractResponse"), "activeVersion": ref("ContractVersionResponse")}, []string{"contract", "activeVersion"})
	s["ContractHistoryItem"] = historySchema()
	s["DocumentAttachRequest"] = obj(M{"mediaObjectId": id, "documentPurpose": str(128), "isMandatory": M{"type": "boolean"}, "versionId": id}, []string{"mediaObjectId", "documentPurpose"})
	s["DocumentReplaceRequest"] = obj(M{"mediaObjectId": id, "documentPurpose": str(128), "isMandatory": M{"type": "boolean"}}, []string{"mediaObjectId", "documentPurpose"})
	s["DocumentRemoveRequest"] = obj(M{"reasonCode": str(128)}, nil)
	s["MediaObjectReference"] = obj(M{"id": id, "mediaObjectId": id, "documentPurpose": str(128), "isMandatory": M{"type": "boolean"}, "statusProjection": str(64), "readinessState": str(64), "usable": M{"type": "boolean"}, "versionId": id, "sourceService": str(128), "sourceVersion": integer, "syncedAt": dt, "createdAt": dt}, []string{"id", "mediaObjectId", "documentPurpose", "statusProjection", "readinessState", "usable", "sourceService", "sourceVersion", "createdAt"})
	s["ContractDocumentReference"] = s["MediaObjectReference"]
	s["DocumentRemovalResponse"] = obj(M{"removed": M{"type": "boolean", "const": true}, "mediaObjectRetained": M{"type": "boolean", "const": true}}, []string{"removed", "mediaObjectRetained"})

	s["SLAStatus"] = enum("DRAFT", "ACTIVE", "SUSPENDED", "EXPIRED", "TERMINATED", "SUPERSEDED")
	s["SLAMetricCode"] = enum("PICKUP_ON_TIME", "DELIVERY_ON_TIME", "ORDER_COMPLETION_TIME", "PACKAGE_DAMAGE_RATE", "PROOF_COMPLETION_TIME", "FAILED_ATTEMPT_RATE")
	s["SLAOutcome"] = enum("MET", "MISSED", "NOT_EVALUATED", "INSUFFICIENT_DATA", "UNKNOWN")
	s["SLACreateRequest"] = obj(M{"customerId": id, "contractId": id, "futureServiceCatalogItemId": id, "metricCode": ref("SLAMetricCode"), "targetValue": M{"type": "string"}, "measurementUnit": str(64), "measurementWindow": str(128), "escalationPolicy": M{"type": "object"}, "effectiveFrom": dt, "effectiveTo": dt}, []string{"customerId", "metricCode", "targetValue", "measurementUnit", "measurementWindow", "effectiveFrom"})
	s["SLAUpdateRequest"] = obj(M{"targetValue": M{"type": "string"}, "measurementUnit": str(64), "measurementWindow": str(128), "escalationPolicy": M{"type": "object"}, "effectiveFrom": dt, "effectiveTo": dt, "expectedVersion": M{"type": "integer", "minimum": 1}}, []string{"expectedVersion"})
	s["SLALifecycleRequest"] = s["ReasonedVersionCommand"]
	s["SLAResponse"] = obj(M{"id": id, "customerId": id, "contractId": id, "futureServiceCatalogItemId": id, "metricCode": ref("SLAMetricCode"), "targetValue": M{"type": "string"}, "measurementUnit": str(64), "measurementWindow": str(128), "status": ref("SLAStatus"), "version": integer, "createdAt": dt, "updatedAt": dt}, []string{"id", "customerId", "metricCode", "targetValue", "measurementUnit", "measurementWindow", "status", "version", "createdAt", "updatedAt"})
	s["SLAObservationIngestionRequest"] = obj(M{"organizationId": id, "slaId": id, "sourceType": str(128), "sourceReference": str(256), "measuredValue": M{"type": [2]any{"string", "null"}}, "measurementUnit": str(64), "measuredAt": dt, "windowStart": dt, "windowEnd": dt, "sourceVersion": integer, "eventId": id, "payloadHash": str(128), "causationId": id}, []string{"organizationId", "slaId", "sourceType", "measuredAt", "windowStart", "windowEnd", "sourceVersion", "eventId", "payloadHash"})
	s["SLAObservationResponse"] = obj(M{"id": id, "slaId": id, "sourceType": str(128), "sourceReference": str(256), "measuredValue": M{"type": [2]any{"string", "null"}}, "measurementUnit": str(64), "measuredAt": dt, "windowStart": dt, "windowEnd": dt, "outcome": ref("SLAOutcome"), "sourceVersion": integer, "createdAt": dt}, []string{"id", "slaId", "sourceType", "measuredAt", "windowStart", "windowEnd", "outcome", "sourceVersion", "createdAt"})
	s["SLAObservationIngestionResponse"] = obj(M{"observation": ref("SLAObservationResponse"), "duplicate": M{"type": "boolean"}}, []string{"observation", "duplicate"})
	s["SLAHistoryItem"] = historySchema()

	s["HoldStatus"] = enum("NO_HOLD", "REVIEW_REQUIRED", "ON_HOLD", "RESTRICTED")
	s["CreditProfileRequest"] = obj(M{"currency": M{"type": "string", "pattern": "^[A-Z]{3}$"}, "creditLimit": ref("Money"), "paymentTermsDays": M{"type": "integer", "minimum": 0, "maximum": 365}, "expectedVersion": M{"type": "integer", "minimum": 0}}, []string{"currency", "paymentTermsDays"})
	s["CreditProfileResponse"] = obj(M{"id": id, "customerId": id, "currency": str(3), "paymentTermsDays": M{"type": "integer"}, "holdStatus": ref("HoldStatus"), "blocking": M{"type": "boolean"}, "hasCreditLimit": M{"type": "boolean"}, "riskStatusSnapshot": ref("CustomerRiskStatus"), "version": integer, "effectiveFrom": dt, "createdAt": dt, "updatedAt": dt}, []string{"id", "customerId", "currency", "paymentTermsDays", "holdStatus", "blocking", "hasCreditLimit", "riskStatusSnapshot", "version", "effectiveFrom", "createdAt", "updatedAt"})
	s["CreditHoldCommand"] = obj(M{"expectedVersion": M{"type": "integer", "minimum": 1}, "holdStatus": ref("HoldStatus"), "reasonCode": str(128)}, []string{"expectedVersion", "holdStatus"})
	s["CreditReleaseCommand"] = obj(M{"expectedVersion": M{"type": "integer", "minimum": 1}, "reasonCode": str(128)}, []string{"expectedVersion"})
	s["CreditHistoryItem"] = obj(M{"id": id, "previousHoldStatus": ref("HoldStatus"), "newHoldStatus": ref("HoldStatus"), "previousLimit": ref("Money"), "newLimit": ref("Money"), "reasonCode": str(128), "approvalRequestId": id, "actorType": str(64), "actorId": id, "occurredAt": dt}, []string{"id", "newHoldStatus", "actorType", "actorId", "occurredAt"})
	s["NotificationOperationReference"] = obj(M{"id": id, "subjectType": str(64), "subjectId": id, "intentType": str(128), "recipientReference": id, "channel": str(64), "status": str(64), "attempts": M{"type": "integer", "minimum": 0}, "nextAttemptAt": dt, "safeErrorCode": str(128), "deadLetterReason": str(512), "createdAt": dt, "updatedAt": dt}, []string{"id", "subjectType", "subjectId", "intentType", "channel", "status", "attempts", "nextAttemptAt", "createdAt", "updatedAt"})
	s["GenericResource"] = obj(M{"id": id}, nil)
	return s
}

func parameters() M {
	return M{
		"CorrelationID":           header("X-Correlation-ID", false, M{"type": "string", "maxLength": 128}, "Optional trace correlation identifier; echoed in responses."),
		"RequestID":               header("X-Request-ID", false, M{"type": "string", "maxLength": 128}, "Optional request identifier accepted by shared HTTP middleware."),
		"BranchID":                header("X-Branch-ID", false, M{"type": "string", "maxLength": 128}, "Optional branch scope; authorization still verifies organization membership and branch access."),
		"IdempotencyKey":          header("Idempotency-Key", true, M{"type": "string", "minLength": 1, "maxLength": 128}, "Required replay key. Reuse with the same fingerprint returns the stable result; reuse with a different fingerprint returns 409."),
		"IfMatch":                 header("If-Match", false, M{"type": "string", "pattern": "^\\\"?[1-9][0-9]*\\\"?$"}, "Expected aggregate version. The equivalent expectedVersion JSON field may be used; one of them is required."),
		"Cursor":                  M{"name": "cursor", "in": "query", "required": false, "schema": M{"type": "string", "maxLength": 2048}},
		"Limit":                   M{"name": "limit", "in": "query", "required": false, "schema": M{"type": "integer", "minimum": 1, "maximum": 200, "default": 50}},
		"Sort":                    M{"name": "sort", "in": "query", "required": false, "schema": M{"type": "string", "maxLength": 64}, "description": "Supported deterministic sort expression; unsupported values are rejected."},
		"ServiceName":             header("X-Service-Name", true, M{"type": "string", "maxLength": 128}, "Verified service identity, also checked against the SLA caller allowlist."),
		"ServiceTimestamp":        header("X-Service-Timestamp", true, M{"type": "string", "pattern": "^[0-9]+$"}, "Unix timestamp bound into the HMAC and checked for freshness."),
		"ServiceNonce":            header("X-Service-Nonce", true, M{"type": "string", "minLength": 16, "maxLength": 128}, "Single-use nonce bound into the HMAC."),
		"ServiceSignatureVersion": header("X-Service-Signature-Version", true, M{"type": "string", "const": "2"}, "Signature version 2."),
		"ServiceSignature":        header("X-Service-Signature", true, M{"type": "string", "pattern": "^[a-f0-9]{64}$"}, "HMAC-SHA256 over version, caller, method, request target, timestamp, nonce, and body hash."),
		"QuotationTokenQuery":     M{"name": "token", "in": "query", "required": false, "schema": M{"type": "string", "minLength": 32, "writeOnly": true}, "description": "Secure quotation token for emailed links. Authorization bearer transport is preferred because URLs may be logged."},
	}
}

func securitySchemes() M {
	return M{
		"CorporateBearer": M{"type": "http", "scheme": "bearer", "bearerFormat": "access token", "description": "Bearer access token followed by organization membership resolution and organization, branch, and permission enforcement."},
		"QuotationToken":  M{"type": "http", "scheme": "bearer", "bearerFormat": "secure quotation token", "description": "Hashed, expiring, rate-limited token bound to exactly one quotation and version. The raw token is never persisted. Emailed links may alternatively supply the token query parameter."},
		"ServiceHMACv2":   M{"type": "apiKey", "in": "header", "name": "X-Service-Signature", "description": "Shared HMAC signature v2 with verified caller identity, method, request-target, timestamp, nonce, and body-hash binding; caller allowlist, freshness, and single-use nonce are enforced."},
	}
}

func responses() M {
	r := M{}
	for _, code := range []string{"400", "401", "403", "404", "409", "410", "422", "429", "500", "503"} {
		r["Error"+code] = M{"description": errorDescription(code), "content": M{"application/json": M{"schema": ref("ErrorEnvelope")}}}
	}
	return r
}

func tags() []any {
	names := []string{"Customers", "Contacts", "Locations", "Credit", "Opportunities", "Quotations", "Public quotations", "Contracts", "SLAs", "Internal SLA ingestion", "Notification operations"}
	out := make([]any, 0, len(names))
	for _, n := range names {
		out = append(out, M{"name": n})
	}
	return out
}

func tagFor(path string) string {
	switch {
	case strings.HasPrefix(path, "/fleet-partner/public/"):
		return "Public quotations"
	case strings.HasPrefix(path, "/internal/"):
		return "Internal SLA ingestion"
	case strings.Contains(path, "notification-operations"):
		return "Notification operations"
	case strings.Contains(path, "credit-profile"):
		return "Credit"
	case strings.Contains(path, "/contacts"):
		return "Contacts"
	case strings.Contains(path, "/locations"):
		return "Locations"
	case strings.Contains(path, "/customers"):
		return "Customers"
	case strings.Contains(path, "opportunities"):
		return "Opportunities"
	case strings.Contains(path, "quotations"):
		return "Quotations"
	case strings.Contains(path, "contracts"):
		return "Contracts"
	default:
		return "SLAs"
	}
}

func queryFilters(id string) []string {
	return map[string][]string{
		"listCustomers":                {"status", "riskStatus", "customerType", "branchId", "accountOwnerUserId", "createdAfter", "createdBefore", "search"},
		"listContacts":                 {"status", "contactPurpose"},
		"listLocations":                {"locationType", "status"},
		"listOpportunities":            {"stage", "customerId", "branchId", "accountOwnerUserId", "createdAfter", "createdBefore"},
		"listQuotations":               {"status", "customerId", "opportunityId", "branchId", "createdAfter", "createdBefore"},
		"listContracts":                {"status", "customerId", "branchId", "createdAfter", "createdBefore"},
		"listServiceLevelAgreements":   {"status", "customerId", "contractId", "metricCode"},
		"listServiceLevelObservations": {"outcome"},
		"listNotificationOperations":   {"status", "subjectId"},
	}[id]
}

func description(r crmhttp.Route) string {
	base := "Production handler: `" + r.Handler + "`. Feature flag: `" + r.FeatureFlag + "`."
	if r.Permission != "" {
		base += " Required corporate permission: `" + r.Permission + "`."
	}
	if r.IdempotencyRequired {
		base += " Idempotency-Key is required; stable replays return the original result and fingerprint conflicts return 409."
	}
	if r.ExpectedVersionRequired {
		base += " expectedVersion in the request body or If-Match is required; stale versions return 409."
	}
	return base
}

func safeExample(id string) M {
	value := M{"expectedVersion": 7, "reasonCode": "CUSTOMER_REQUEST"}
	switch id {
	case "createCustomer":
		value = M{"customerType": "BUSINESS", "legalName": "Example Logistics Ltd", "displayName": "Example Logistics", "defaultCurrency": "EUR"}
	case "createContact":
		value = M{"fullName": "Example Contact", "roleTitle": "Operations lead", "email": "contact@example.invalid", "preferredChannel": "EMAIL", "contactPurpose": "OPERATIONS", "consentStatus": "GRANTED"}
	case "createLocation":
		value = M{"locationType": "WAREHOUSE", "label": "Example depot", "address": M{"line1": "1 Example Way", "city": "Sample City", "countryCode": "IT"}, "timezone": "Europe/Rome"}
	case "createOpportunity":
		value = M{"customerId": "cus_example_01", "title": "Example regional deliveries", "probabilityBasisPoints": 6500, "serviceCapabilities": []any{"HAULING"}}
	case "createQuotation":
		value = M{"customerId": "cus_example_01", "currency": "EUR", "validFrom": "2026-08-01T00:00:00Z", "validUntil": "2026-08-15T00:00:00Z", "lineItems": []any{M{"description": "Example service", "quantity": "2.500", "unitType": "TRIP", "unitAmountMinor": 12500}}}
	case "submitQuotation":
		value = M{"expectedVersion": 7, "reasonCode": "READY_FOR_REVIEW"}
	case "issueQuotation":
		value = M{"mintCustomerToken": true, "singleUseToken": false, "expectedVersion": 8}
	case "acceptQuotationAsStaff":
		value = M{"acceptedVersionId": "qv_example_01", "acceptanceMethod": "DOCUMENTED_OFFLINE_ACCEPTANCE", "actorReference": "contact_example_01", "expectedVersion": 9}
	case "acceptQuotationAsCustomer":
		value = M{"acceptedVersionId": "qv_example_01"}
	case "createContract":
		value = M{"customerId": "cus_example_01", "quotationId": "quo_example_01", "effectiveFrom": "2026-09-01T00:00:00Z", "serviceScope": M{"summary": "Synthetic scope"}}
	case "activateContract":
		value = M{"signatoryReferences": []any{"contact_example_01"}, "expectedVersion": 4}
	case "createServiceLevelAgreement":
		value = M{"customerId": "cus_example_01", "contractId": "con_example_01", "metricCode": "DELIVERY_ON_TIME", "targetValue": "9500", "measurementUnit": "BASIS_POINTS", "measurementWindow": "CALENDAR_MONTH", "effectiveFrom": "2026-09-01T00:00:00Z"}
	case "ingestServiceLevelObservation":
		value = M{"organizationId": "org_example_01", "slaId": "sla_example_01", "sourceType": "DISPATCH", "sourceReference": "delivery_example_01", "measuredValue": "1", "measurementUnit": "BOOLEAN", "measuredAt": "2026-09-03T10:00:00Z", "windowStart": "2026-09-03T09:00:00Z", "windowEnd": "2026-09-03T10:00:00Z", "sourceVersion": 2, "eventId": "evt_example_01", "payloadHash": "synthetic-non-secret-hash"}
	case "setCreditProfile":
		value = M{"currency": "EUR", "creditLimit": M{"amountMinor": 250000, "currency": "EUR"}, "paymentTermsDays": 30, "reasonCode": "INITIAL_POLICY", "expectedVersion": 0}
	case "applyCreditHold":
		value = M{"holdStatus": "ON_HOLD", "reasonCode": "MANUAL_REVIEW", "expectedVersion": 3}
	}
	return M{"synthetic": M{"summary": "Synthetic, non-secret example", "value": value}}
}

func header(name string, required bool, schema M, desc string) M {
	return M{"name": name, "in": "header", "required": required, "schema": schema, "description": desc}
}
func ref(name string) M { return M{"$ref": "#/components/schemas/" + name} }
func obj(props M, required []string) M {
	v := M{"type": "object", "additionalProperties": false, "properties": props}
	if len(required) > 0 {
		a := make([]any, len(required))
		for i, x := range required {
			a[i] = x
		}
		v["required"] = a
	}
	return v
}
func allOf(base string, props M) M { return M{"allOf": []any{ref(base), obj(props, nil)}} }
func enum(values ...string) M {
	a := make([]any, len(values))
	for i, v := range values {
		a[i] = v
	}
	return M{"type": "string", "enum": a}
}
func historySchema() M {
	return obj(M{"id": M{"type": "string"}, "previousStatus": M{"type": "string"}, "newStatus": M{"type": "string"}, "reasonCode": M{"type": "string"}, "actorType": M{"type": "string"}, "actorId": M{"type": "string"}, "occurredAt": M{"type": "string", "format": "date-time"}}, []string{"id", "newStatus", "actorType", "actorId", "occurredAt"})
}
func openAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = "{" + strings.TrimPrefix(p, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}
func pathParameters(path string) []string {
	var out []string
	for {
		a := strings.Index(path, "{")
		if a < 0 {
			break
		}
		b := strings.Index(path[a:], "}")
		if b < 0 {
			break
		}
		out = append(out, path[a+1:a+b])
		path = path[a+b+1:]
	}
	return out
}
func humanize(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	x := b.String()
	return strings.ToUpper(x[:1]) + x[1:]
}
func errorDescription(code string) string {
	m := map[string]string{"400": "Validation error.", "401": "Authentication required or invalid.", "403": "Permission denied.", "404": "Safe not found or disabled feature.", "409": "Idempotency, version, uniqueness, or state conflict.", "410": "Quotation token expired or no longer valid.", "422": "Domain rule violation.", "429": "Rate limited.", "500": "Safe internal failure.", "503": "Dependency unavailable."}
	return m[code]
}
func operationCount(paths M) int {
	n := 0
	for _, v := range paths {
		for k := range v.(M) {
			if isMethod(k) {
				n++
			}
		}
	}
	return n
}
func isMethod(s string) bool {
	switch s {
	case "get", "post", "put", "patch", "delete":
		return true
	}
	return false
}
