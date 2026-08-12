package usecases

import (
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
)

// Section 14 event types. Every CRM event goes through the existing Fleet
// transactional outbox; there is no second CRM outbox.
const (
	EventCustomerCreated       = "organization.crm_customer.created.v1"
	EventCustomerUpdated       = "organization.crm_customer.updated.v1"
	EventCustomerArchived      = "organization.crm_customer.archived.v1"
	EventCustomerStatusChanged = "organization.crm_customer.status_changed.v1"
	EventCustomerRiskChanged   = "organization.crm_customer.risk_changed.v1"

	EventContactCreated  = "organization.crm_contact.created.v1"
	EventContactUpdated  = "organization.crm_contact.updated.v1"
	EventContactArchived = "organization.crm_contact.archived.v1"

	EventLocationCreated  = "organization.crm_location.created.v1"
	EventLocationUpdated  = "organization.crm_location.updated.v1"
	EventLocationArchived = "organization.crm_location.archived.v1"

	EventOpportunityCreated      = "organization.sales_opportunity.created.v1"
	EventOpportunityUpdated      = "organization.sales_opportunity.updated.v1"
	EventOpportunityStageChanged = "organization.sales_opportunity.stage_changed.v1"

	EventQuotationCreated    = "organization.quotation.created.v1"
	EventQuotationSubmitted  = "organization.quotation.submitted.v1"
	EventQuotationApproved   = "organization.quotation.approved.v1"
	EventQuotationIssued     = "organization.quotation.issued.v1"
	EventQuotationViewed     = "organization.quotation.viewed.v1"
	EventQuotationAccepted   = "organization.quotation.accepted.v1"
	EventQuotationRejected   = "organization.quotation.rejected.v1"
	EventQuotationExpired    = "organization.quotation.expired.v1"
	EventQuotationCancelled  = "organization.quotation.cancelled.v1"
	EventQuotationSuperseded = "organization.quotation.superseded.v1"

	EventContractCreated        = "organization.contract.created.v1"
	EventContractApproved       = "organization.contract.approved.v1"
	EventContractActivated      = "organization.contract.activated.v1"
	EventContractSuspended      = "organization.contract.suspended.v1"
	EventContractTerminated     = "organization.contract.terminated.v1"
	EventContractRenewalPending = "organization.contract.renewal_pending.v1"
	EventContractExpired        = "organization.contract.expired.v1"
	EventContractSuperseded     = "organization.contract.superseded.v1"

	EventSLACreated             = "organization.sla.created.v1"
	EventSLAActivated           = "organization.sla.activated.v1"
	EventSLASuspended           = "organization.sla.suspended.v1"
	EventSLAOutcomeRecorded     = "organization.sla.outcome_recorded.v1"
	EventSLAEscalationRequested = "organization.sla.escalation_requested.v1"

	EventCreditUpdated     = "organization.customer_credit.updated.v1"
	EventCreditHoldChanged = "organization.customer_credit.hold_changed.v1"
)

// The payload builders below are the whole event contract. Each one is written
// field by field rather than serializing a domain model, because a model that
// grows a field would otherwise start publishing it silently — and the fields
// most likely to be added to these models are exactly the ones that must never
// leave: tax references, contact details, coordinates, commercial line items,
// internal notes.
//
// What consumers get is an opaque identifier and a status projection. If a
// consumer needs more, it asks through an authorized API where the caller's
// permissions still apply.

func customerEventPayload(customer models.Customer) map[string]any {
	return map[string]any{
		"customerId":     customer.ID,
		"organizationId": customer.OrganizationID,
		"branchId":       customer.BranchID,
		"customerType":   customer.CustomerType,
		"status":         customer.Status,
		"riskStatus":     customer.RiskStatus,
		"currency":       customer.DefaultCurrency,
		"version":        customer.Version,
		"occurredAt":     customer.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func customerUpdatedEventPayload(customer models.Customer, changed []string) map[string]any {
	payload := customerEventPayload(customer)
	// Field names, never field values.
	payload["changedFields"] = changed
	return payload
}

func customerStatusEventPayload(customer models.Customer, previous, reason string) map[string]any {
	payload := customerEventPayload(customer)
	payload["previousStatus"] = previous
	payload["reasonCode"] = reason
	return payload
}

func customerRiskEventPayload(customer models.Customer, previous string) map[string]any {
	payload := customerEventPayload(customer)
	payload["previousRiskStatus"] = previous
	// The reason for a risk change stays out of the event. It is investigation
	// detail, and a consumer that only needs to know a customer is restricted
	// does not need to know why somebody thought so.
	return payload
}

func contactEventPayload(contact models.Contact) map[string]any {
	// No email, no phone, no name. A consumer learns that a contact exists, for
	// what purpose, and whether it is primary.
	return map[string]any{
		"contactId":      contact.ID,
		"customerId":     contact.CustomerID,
		"organizationId": contact.OrganizationID,
		"contactPurpose": contact.ContactPurpose,
		"isPrimary":      contact.IsPrimary,
		"status":         contact.Status,
		"consentStatus":  contact.ConsentStatus,
		"version":        contact.Version,
		"occurredAt":     contact.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func locationEventPayload(location models.Location) map[string]any {
	// No coordinates, no street address, no access or safety instructions. The
	// country and timezone are enough for routing-adjacent consumers to plan;
	// anything finer is operational detail behind a permission.
	return map[string]any{
		"locationId":     location.ID,
		"customerId":     location.CustomerID,
		"organizationId": location.OrganizationID,
		"locationType":   location.LocationType,
		"countryCode":    location.Address.CountryCode,
		"timezone":       location.Timezone,
		"status":         location.Status,
		"version":        location.Version,
		"occurredAt":     location.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func opportunityEventPayload(opportunity models.Opportunity) map[string]any {
	// The expected value is deliberately absent: a sales forecast is commercial
	// information, and no consumer of this event needs it to react.
	return map[string]any{
		"opportunityId":          opportunity.ID,
		"organizationId":         opportunity.OrganizationID,
		"customerId":             opportunity.CustomerID,
		"branchId":               opportunity.BranchID,
		"stage":                  opportunity.Stage,
		"probabilityBasisPoints": opportunity.ProbabilityBasisPoints,
		"version":                opportunity.Version,
		"occurredAt":             opportunity.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func opportunityStageEventPayload(opportunity models.Opportunity, previous,
	reason string) map[string]any {
	payload := opportunityEventPayload(opportunity)
	payload["previousStage"] = previous
	payload["reasonCode"] = reason
	return payload
}

func quotationEventPayload(quotation models.Quotation) map[string]any {
	// The total and currency are included because downstream reporting needs a
	// magnitude. The line items are not: what a customer was quoted line by
	// line is commercial detail that belongs behind an authorized read.
	//
	// internal_notes is absent by construction.
	return map[string]any{
		"quotationId":      quotation.ID,
		"organizationId":   quotation.OrganizationID,
		"customerId":       quotation.CustomerID,
		"branchId":         quotation.BranchID,
		"opportunityId":    quotation.OpportunityID,
		"quotationNumber":  quotation.QuotationNumber,
		"status":           quotation.Status,
		"currency":         quotation.Currency,
		"totalAmountMinor": quotation.TotalAmountMinor,
		"currentVersion":   quotation.CurrentVersion,
		"validUntil":       quotation.ValidUntil.UTC().Format(time.RFC3339Nano),
		"version":          quotation.Version,
		"occurredAt":       quotation.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func quotationStatusEventPayload(quotation models.Quotation, previous,
	reason string) map[string]any {
	payload := quotationEventPayload(quotation)
	payload["previousStatus"] = previous
	payload["reasonCode"] = reason
	return payload
}

// quotationAcceptedEventPayload announces an acceptance and its conversion
// intent.
//
// The intent ID is present so a future Section 15 consumer can find the record
// it will act on. The payload deliberately says PENDING_SECTION_15 and carries
// no order ID: nothing has been ordered, and an event that implied otherwise
// would invite a consumer to behave as though something had.
func quotationAcceptedEventPayload(quotation models.Quotation, acceptance models.Acceptance,
	intent models.ConversionIntent) map[string]any {
	payload := quotationEventPayload(quotation)
	payload["acceptedVersionId"] = acceptance.AcceptedVersionID
	payload["acceptanceMethod"] = acceptance.AcceptanceMethod
	payload["actorType"] = acceptance.ActorType
	payload["acceptedAt"] = acceptance.AcceptedAt.UTC().Format(time.RFC3339Nano)
	payload["conversionIntentId"] = intent.ID
	payload["conversionType"] = intent.ConversionType
	payload["conversionStatus"] = intent.Status
	return payload
}

// quotationViewedEventPayload announces a customer view.
//
// It carries no IP address, no user agent and no token — not even a hash of
// one. A view event is "the customer opened it", not a session record.
func quotationViewedEventPayload(quotation models.Quotation, viewedAt time.Time) map[string]any {
	payload := quotationEventPayload(quotation)
	payload["viewedAt"] = viewedAt.UTC().Format(time.RFC3339Nano)
	return payload
}

func contractEventPayload(contract models.Contract) map[string]any {
	// The commercial snapshot stays out. It is the negotiated substance of the
	// agreement and the single most sensitive thing in the aggregate.
	payload := map[string]any{
		"contractId":     contract.ID,
		"organizationId": contract.OrganizationID,
		"customerId":     contract.CustomerID,
		"branchId":       contract.BranchID,
		"contractNumber": contract.ContractNumber,
		"status":         contract.Status,
		"currentVersion": contract.CurrentVersion,
		"effectiveFrom":  contract.EffectiveFrom.UTC().Format(time.RFC3339Nano),
		"version":        contract.Version,
		"occurredAt":     contract.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if contract.EffectiveTo != nil {
		payload["effectiveTo"] = contract.EffectiveTo.UTC().Format(time.RFC3339Nano)
	}
	if contract.ActiveVersionID != "" {
		payload["activeVersionId"] = contract.ActiveVersionID
	}
	return payload
}

func contractStatusEventPayload(contract models.Contract, previous, reason string) map[string]any {
	payload := contractEventPayload(contract)
	payload["previousStatus"] = previous
	payload["reasonCode"] = reason
	return payload
}

func slaEventPayload(sla models.SLA) map[string]any {
	return map[string]any{
		"slaId":             sla.ID,
		"organizationId":    sla.OrganizationID,
		"customerId":        sla.CustomerID,
		"contractId":        sla.ContractID,
		"metricCode":        sla.MetricCode,
		"measurementUnit":   sla.MeasurementUnit,
		"measurementWindow": sla.MeasurementWindow,
		"status":            sla.Status,
		"version":           sla.Version,
		"occurredAt":        sla.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

// slaOutcomeEventPayload announces a recorded measurement.
//
// The outcome is reported exactly as stored. UNKNOWN and INSUFFICIENT_DATA
// travel as themselves and are never normalized into MET on the way out: a
// consumer building a service report must be able to tell "we met it" from "we
// could not tell".
func slaOutcomeEventPayload(sla models.SLA, observation models.SLAObservation) map[string]any {
	payload := slaEventPayload(sla)
	payload["observationId"] = observation.ID
	payload["outcome"] = observation.Outcome
	payload["sourceType"] = observation.SourceType
	payload["sourceVersion"] = observation.SourceVersion
	payload["windowStart"] = observation.WindowStart.UTC().Format(time.RFC3339Nano)
	payload["windowEnd"] = observation.WindowEnd.UTC().Format(time.RFC3339Nano)
	return payload
}

func slaEscalationEventPayload(sla models.SLA, observationID, caseReference string) map[string]any {
	payload := slaEventPayload(sla)
	payload["observationId"] = observationID
	payload["supportCaseReference"] = caseReference
	// An escalation is a request for attention. It carries no penalty, no
	// credit, no deduction and no wallet reference, because none was created.
	payload["financialEffect"] = "NONE"
	return payload
}

func creditEventPayload(profile models.CreditProfile) map[string]any {
	// The credit limit itself is absent. Whether a customer has a hold is
	// operationally relevant to other systems; how much credit an operator
	// extends them is that operator's commercial position.
	return map[string]any{
		"creditProfileId": profile.ID,
		"organizationId":  profile.OrganizationID,
		"customerId":      profile.CustomerID,
		"currency":        profile.Currency,
		"holdStatus":      profile.HoldStatus,
		"blocking":        models.BlockingHold(profile.HoldStatus),
		"version":         profile.Version,
		"occurredAt":      profile.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func creditHoldEventPayload(profile models.CreditProfile, previous, reason string) map[string]any {
	payload := creditEventPayload(profile)
	payload["previousHoldStatus"] = previous
	payload["reasonCode"] = reason
	return payload
}
