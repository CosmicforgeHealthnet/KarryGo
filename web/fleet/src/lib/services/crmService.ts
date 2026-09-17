// lib/services/crmService.ts

import apiClient from "@/lib/api/client";
import type {
  AcceptanceMethod,
  ConsentStatus,
  ContactChannel,
  ContactPurpose,
  ContractActivateResponse,
  ContractHistoryView,
  ContractStatus,
  ContractVersionView,
  ContractView,
  ConversionIntentView,
  CreditHistoryView,
  CreditHoldStatus,
  CreditProfileView,
  CustomerHistoryView,
  CustomerStatus,
  CustomerType,
  CustomerView,
  DocumentView,
  LineItemView,
  LocationType,
  LocationView,
  MoneyView,
  NotificationOperationView,
  ObservationView,
  OpportunityHistoryView,
  OpportunityStage,
  OpportunityView,
  PaginatedResponse,
  QuotationAcceptResponse,
  QuotationHistoryView,
  QuotationIssueResponse,
  QuotationStatus,
  QuotationVersionView,
  QuotationView,
  RecordStatus,
  RiskStatus,
  SLAHistoryView,
  SLAMeasurementUnit,
  SLAMeasurementWindow,
  SLAMetricCode,
  SLAStatus,
  SLAView,
} from "@/lib/types/crm";

// ── Helpers ───────────────────────────────────────────────────────────────

const newIdempotencyKey = (): string => {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2)}`;
};

type QueryValue = string | number | undefined;
type QueryBag = Record<string, QueryValue>;

const cleanQuery = <T extends object>(input: T): QueryBag => {
  const out: QueryBag = {};
  for (const [k, v] of Object.entries(input)) {
    if (v !== undefined && v !== null && v !== "") {
      out[k] = v as QueryValue;
    }
  }
  return out;
};

const unwrapList = <T>(envelope: { data: T[]; nextCursor?: string }): PaginatedResponse<T> => ({
  data: envelope.data ?? [],
  nextCursor: envelope.nextCursor,
});

// ── Input types ───────────────────────────────────────────────────────────

export interface ListCustomersParams {
  status?: CustomerStatus;
  riskStatus?: RiskStatus;
  customerType?: CustomerType;
  branchId?: string;
  accountOwnerUserId?: string;
  createdAfter?: string;
  createdBefore?: string;
  search?: string;
  cursor?: string;
  limit?: number;
}

export interface CreateCustomerInput {
  branchId?: string;
  customerType: CustomerType;
  legalName: string;
  displayName: string;
  registrationNumber?: string;
  taxReference?: string;
  marketplaceCustomerId?: string;
  accountOwnerUserId?: string;
  defaultCurrency: string;
  paymentTermsReference?: string;
}

export interface UpdateCustomerInput {
  branchId?: string;
  legalName?: string;
  displayName?: string;
  registrationNumber?: string;
  taxReference?: string;
  marketplaceCustomerId?: string;
  accountOwnerUserId?: string;
  defaultCurrency?: string;
  paymentTermsReference?: string;
}

export interface ListContactsParams {
  status?: RecordStatus;
  contactPurpose?: ContactPurpose;
  cursor?: string;
  limit?: number;
}

export interface CreateContactInput {
  fullName: string;
  roleTitle?: string;
  email?: string;
  phone?: string;
  preferredChannel?: ContactChannel;
  contactPurpose?: ContactPurpose;
  isPrimary?: boolean;
  notificationPreferences?: Record<string, unknown>;
  consentStatus?: ConsentStatus;
  consentSource?: string;
}

export interface UpdateContactInput {
  fullName?: string;
  roleTitle?: string;
  email?: string;
  phone?: string;
  preferredChannel?: ContactChannel;
  contactPurpose?: ContactPurpose;
  notificationPreferences?: Record<string, unknown>;
  consentStatus?: ConsentStatus;
  consentSource?: string;
  status?: "ACTIVE" | "INACTIVE";
}

export interface ListLocationsParams {
  locationType?: LocationType;
  status?: RecordStatus;
  cursor?: string;
  limit?: number;
}

export interface CreateLocationInput {
  locationType: LocationType;
  label: string;
  address: {
    line1: string;
    line2?: string;
    city: string;
    stateOrRegion: string;
    postalCode?: string;
    countryCode: string;
  };
  latitude?: number;
  longitude?: number;
  geofenceReference?: string;
  operatingHours?: Record<string, unknown>;
  accessInstructions?: string;
  safetyInstructions?: string;
  contactReference?: string;
  timezone: string;
}

export interface UpdateLocationInput {
  locationType?: LocationType;
  label?: string;
  address?: {
    line1: string;
    line2?: string;
    city: string;
    stateOrRegion: string;
    postalCode?: string;
    countryCode: string;
  };
  latitude?: number;
  longitude?: number;
  clearCoordinates?: boolean;
  geofenceReference?: string;
  operatingHours?: Record<string, unknown>;
  accessInstructions?: string;
  safetyInstructions?: string;
  contactReference?: string;
  timezone?: string;
  status?: "ACTIVE" | "INACTIVE";
}

export interface SetCreditProfileInput {
  creditLimit?: MoneyView;
  currency?: string;
  paymentTermsDays?: number;
  reasonCode?: string;
}

export interface CreditHoldInput {
  holdStatus?: CreditHoldStatus;
  reasonCode?: string;
}

export interface ListOpportunitiesParams {
  stage?: OpportunityStage;
  customerId?: string;
  branchId?: string;
  accountOwnerUserId?: string;
  createdAfter?: string;
  createdBefore?: string;
  cursor?: string;
  limit?: number;
}

export interface CreateOpportunityInput {
  branchId?: string;
  customerId: string;
  title: string;
  description?: string;
  expectedValue?: MoneyView;
  probabilityBasisPoints?: number;
  expectedCloseDate?: string;
  serviceCapabilities?: string[];
  accountOwnerUserId?: string;
  nextAction?: string;
  nextActionAt?: string;
}

export interface UpdateOpportunityInput {
  title?: string;
  description?: string;
  expectedValue?: MoneyView;
  clearExpectedValue?: boolean;
  probabilityBasisPoints?: number;
  serviceCapabilities?: string[];
  accountOwnerUserId?: string;
  nextAction?: string;
}

export interface ListQuotationsParams {
  status?: QuotationStatus;
  customerId?: string;
  opportunityId?: string;
  branchId?: string;
  createdAfter?: string;
  createdBefore?: string;
  cursor?: string;
  limit?: number;
}

export interface CreateQuotationLineItemInput {
  description: string;
  quantity: string;
  unitType?: string;
  unitAmountMinor: number;
  lineAmountMinor?: number;
  externalPricingReference?: string;
  manualReason?: string;
}

export interface CreateQuotationInput {
  branchId?: string;
  customerId: string;
  opportunityId?: string;
  currency: string;
  validFrom: string;
  validUntil: string;
  serviceScope?: Record<string, unknown>;
  terms?: Record<string, unknown>;
  internalNotes?: string;
  lineItems: CreateQuotationLineItemInput[];
}

export interface UpdateQuotationInput {
  validFrom?: string;
  validUntil?: string;
  serviceScope?: Record<string, unknown>;
  terms?: Record<string, unknown>;
  internalNotes?: string;
  opportunityId?: string;
  lineItems?: CreateQuotationLineItemInput[];
  replaceLineItems?: boolean;
}

export interface IssueQuotationInput {
  mintCustomerToken?: boolean;
  singleUseToken?: boolean;
}

export interface AcceptQuotationInput {
  acceptedVersionId: string;
  acceptanceMethod: AcceptanceMethod;
  actorReference?: string;
  evidenceDocumentReference?: string;
}

export interface RejectQuotationInput {
  reasonCode: string;
  note?: string;
  actorReference?: string;
  evidenceDocumentReference?: string;
}

export interface ListContractsParams {
  status?: ContractStatus;
  customerId?: string;
  branchId?: string;
  createdAfter?: string;
  createdBefore?: string;
  cursor?: string;
  limit?: number;
}

export interface CreateContractInput {
  branchId?: string;
  customerId: string;
  quotationId?: string;
  effectiveFrom: string;
  effectiveTo?: string;
  renewalTerms?: Record<string, unknown>;
  serviceScope?: Record<string, unknown>;
  commercialSnapshot?: Record<string, unknown>;
  terminationTerms?: Record<string, unknown>;
  supersedesId?: string;
}

export interface UpdateContractInput {
  effectiveFrom?: string;
  effectiveTo?: string;
  renewalTerms?: Record<string, unknown>;
  serviceScope?: Record<string, unknown>;
  commercialSnapshot?: Record<string, unknown>;
  terminationTerms?: Record<string, unknown>;
  quotationId?: string;
}

export interface ActivateContractInput {
  signatoryReferences?: string[];
}

export interface ContractLifecycleInput {
  reasonCode: string;
  effectiveAt?: string;
  evidenceDocumentReference?: string;
}

export interface RenewContractInput {
  effectiveFrom: string;
  effectiveTo?: string;
  renewalTerms?: Record<string, unknown>;
  commercialSnapshot?: Record<string, unknown>;
}

export interface ListSLAsParams {
  status?: SLAStatus;
  customerId?: string;
  contractId?: string;
  metricCode?: SLAMetricCode;
  cursor?: string;
  limit?: number;
}

export interface CreateSLAInput {
  customerId: string;
  contractId?: string;
  futureServiceCatalogItemId?: string;
  metricCode: SLAMetricCode;
  targetValue: string;
  measurementUnit: SLAMeasurementUnit;
  measurementWindow: SLAMeasurementWindow;
  escalationPolicy?: Record<string, unknown>;
  effectiveFrom: string;
  effectiveTo?: string;
}

export interface UpdateSLAInput {
  targetValue?: string;
  measurementUnit?: SLAMeasurementUnit;
  measurementWindow?: SLAMeasurementWindow;
  escalationPolicy?: Record<string, unknown>;
  effectiveFrom?: string;
  effectiveTo?: string;
}

export interface AttachDocumentInput {
  mediaObjectId: string;
  documentPurpose: string;
  isMandatory?: boolean;
}

// ── Service ───────────────────────────────────────────────────────────────

const BASE = "";

export const crmService = {
  // ── Customers ───────────────────────────────────────────────────────────

  async listCustomers(
    params: ListCustomersParams = {}
  ): Promise<PaginatedResponse<CustomerView>> {
    const response = await apiClient.get<{
      data: CustomerView[];
      nextCursor?: string;
    }>(`${BASE}/customers`, { params: cleanQuery(params) });
    return unwrapList(response.data);
  },

  async getCustomer(customerId: string): Promise<CustomerView> {
    const response = await apiClient.get<{ data: CustomerView }>(
      `${BASE}/customers/${customerId}`
    );
    return response.data.data;
  },

  async createCustomer(input: CreateCustomerInput): Promise<CustomerView> {
    const response = await apiClient.post<{ data: CustomerView }>(
      `${BASE}/customers`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async updateCustomer(
    customerId: string,
    input: UpdateCustomerInput,
    expectedVersion: number
  ): Promise<CustomerView> {
    const response = await apiClient.patch<{ data: CustomerView }>(
      `${BASE}/customers/${customerId}`,
      { ...input, expectedVersion },
      { headers: { "If-Match": String(expectedVersion) } }
    );
    return response.data.data;
  },

  async archiveCustomer(
    customerId: string,
    expectedVersion: number,
    reasonCode = "archived_by_staff"
  ): Promise<CustomerView> {
    const response = await apiClient.post<{ data: CustomerView }>(
      `${BASE}/customers/${customerId}/archive`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async changeCustomerStatus(
    customerId: string,
    status: CustomerStatus,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<CustomerView> {
    const response = await apiClient.post<{ data: CustomerView }>(
      `${BASE}/customers/${customerId}/status`,
      { status, reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async changeCustomerRisk(
    customerId: string,
    riskStatus: RiskStatus,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<CustomerView> {
    const response = await apiClient.post<{ data: CustomerView }>(
      `${BASE}/customers/${customerId}/risk-status`,
      { riskStatus, reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async getCustomerHistory(
    customerId: string,
    params: { cursor?: string; limit?: number } = {}
  ): Promise<PaginatedResponse<CustomerHistoryView>> {
    const response = await apiClient.get<{
      data: CustomerHistoryView[];
      nextCursor?: string;
    }>(`${BASE}/customers/${customerId}/history`, {
      params: cleanQuery(params),
    });
    return unwrapList(response.data);
  },

  // ── Contacts ────────────────────────────────────────────────────────────

  async listContacts(
    customerId: string,
    params: ListContactsParams = {}
  ): Promise<PaginatedResponse<import("@/lib/types/crm").ContactView>> {
    const response = await apiClient.get<{
      data: import("@/lib/types/crm").ContactView[];
      nextCursor?: string;
    }>(`${BASE}/customers/${customerId}/contacts`, {
      params: cleanQuery(params),
    });
    return unwrapList(response.data);
  },

  async getContact(
    customerId: string,
    contactId: string
  ): Promise<import("@/lib/types/crm").ContactView> {
    const response = await apiClient.get<{
      data: import("@/lib/types/crm").ContactView;
    }>(`${BASE}/customers/${customerId}/contacts/${contactId}`);
    return response.data.data;
  },

  async createContact(
    customerId: string,
    input: CreateContactInput
  ): Promise<import("@/lib/types/crm").ContactView> {
    const response = await apiClient.post<{
      data: import("@/lib/types/crm").ContactView;
    }>(`${BASE}/customers/${customerId}/contacts`, input, {
      headers: { "Idempotency-Key": newIdempotencyKey() },
    });
    return response.data.data;
  },

  async updateContact(
    customerId: string,
    contactId: string,
    input: UpdateContactInput,
    expectedVersion: number
  ): Promise<import("@/lib/types/crm").ContactView> {
    const response = await apiClient.patch<{
      data: import("@/lib/types/crm").ContactView;
    }>(`${BASE}/customers/${customerId}/contacts/${contactId}`, input, {
      headers: { "If-Match": String(expectedVersion) },
    });
    return response.data.data;
  },

  async archiveContact(
    customerId: string,
    contactId: string,
    expectedVersion: number,
    reasonCode = "archived_by_staff"
  ): Promise<import("@/lib/types/crm").ContactView> {
    const response = await apiClient.post<{
      data: import("@/lib/types/crm").ContactView;
    }>(
      `${BASE}/customers/${customerId}/contacts/${contactId}/archive`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async makeContactPrimary(
    customerId: string,
    contactId: string,
    expectedVersion: number
  ): Promise<import("@/lib/types/crm").ContactView> {
    const response = await apiClient.post<{
      data: import("@/lib/types/crm").ContactView;
    }>(
      `${BASE}/customers/${customerId}/contacts/${contactId}/make-primary`,
      { expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  // ── Locations ───────────────────────────────────────────────────────────

  async listLocations(
    customerId: string,
    params: ListLocationsParams = {}
  ): Promise<PaginatedResponse<LocationView>> {
    const response = await apiClient.get<{
      data: LocationView[];
      nextCursor?: string;
    }>(`${BASE}/customers/${customerId}/locations`, {
      params: cleanQuery(params),
    });
    return unwrapList(response.data);
  },

  async getLocation(
    customerId: string,
    locationId: string
  ): Promise<LocationView> {
    const response = await apiClient.get<{ data: LocationView }>(
      `${BASE}/customers/${customerId}/locations/${locationId}`
    );
    return response.data.data;
  },

  async createLocation(
    customerId: string,
    input: CreateLocationInput
  ): Promise<LocationView> {
    const response = await apiClient.post<{ data: LocationView }>(
      `${BASE}/customers/${customerId}/locations`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async updateLocation(
    customerId: string,
    locationId: string,
    input: UpdateLocationInput,
    expectedVersion: number
  ): Promise<LocationView> {
    const response = await apiClient.patch<{ data: LocationView }>(
      `${BASE}/customers/${customerId}/locations/${locationId}`,
      input,
      { headers: { "If-Match": String(expectedVersion) } }
    );
    return response.data.data;
  },

  async archiveLocation(
    customerId: string,
    locationId: string,
    expectedVersion: number,
    reasonCode = "archived_by_staff"
  ): Promise<LocationView> {
    const response = await apiClient.post<{ data: LocationView }>(
      `${BASE}/customers/${customerId}/locations/${locationId}/archive`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  // ── Credit / Risk ───────────────────────────────────────────────────────

  async getCreditProfile(customerId: string): Promise<CreditProfileView> {
    const response = await apiClient.get<{ data: CreditProfileView }>(
      `${BASE}/customers/${customerId}/credit-profile`
    );
    return response.data.data;
  },

  async setCreditProfile(
    customerId: string,
    input: SetCreditProfileInput,
    expectedVersion?: number
  ): Promise<CreditProfileView> {
    const response = await apiClient.put<{ data: CreditProfileView }>(
      `${BASE}/customers/${customerId}/credit-profile`,
      { ...input, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async applyCreditHold(
    customerId: string,
    input: CreditHoldInput,
    expectedVersion: number
  ): Promise<CreditProfileView> {
    const response = await apiClient.post<{ data: CreditProfileView }>(
      `${BASE}/customers/${customerId}/credit-profile/hold`,
      { ...input, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async releaseCreditHold(
    customerId: string,
    expectedVersion: number,
    reasonCode = "hold_released"
  ): Promise<CreditProfileView> {
    const response = await apiClient.post<{ data: CreditProfileView }>(
      `${BASE}/customers/${customerId}/credit-profile/release`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async getCreditHistory(
    customerId: string,
    params: { cursor?: string; limit?: number } = {}
  ): Promise<PaginatedResponse<CreditHistoryView>> {
    const response = await apiClient.get<{
      data: CreditHistoryView[];
      nextCursor?: string;
    }>(`${BASE}/customers/${customerId}/credit-profile/history`, {
      params: cleanQuery(params),
    });
    return unwrapList(response.data);
  },

  // ── Opportunities ───────────────────────────────────────────────────────

  async listOpportunities(
    params: ListOpportunitiesParams = {}
  ): Promise<PaginatedResponse<OpportunityView>> {
    const response = await apiClient.get<{
      data: OpportunityView[];
      nextCursor?: string;
    }>(`${BASE}/sales/opportunities`, { params: cleanQuery(params) });
    return unwrapList(response.data);
  },

  async getOpportunity(opportunityId: string): Promise<OpportunityView> {
    const response = await apiClient.get<{ data: OpportunityView }>(
      `${BASE}/sales/opportunities/${opportunityId}`
    );
    return response.data.data;
  },

  async createOpportunity(
    input: CreateOpportunityInput
  ): Promise<OpportunityView> {
    const response = await apiClient.post<{ data: OpportunityView }>(
      `${BASE}/sales/opportunities`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async updateOpportunity(
    opportunityId: string,
    input: UpdateOpportunityInput,
    expectedVersion: number
  ): Promise<OpportunityView> {
    const response = await apiClient.patch<{ data: OpportunityView }>(
      `${BASE}/sales/opportunities/${opportunityId}`,
      input,
      { headers: { "If-Match": String(expectedVersion) } }
    );
    return response.data.data;
  },

  async markOpportunityWon(
    opportunityId: string,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<OpportunityView> {
    const response = await apiClient.post<{ data: OpportunityView }>(
      `${BASE}/sales/opportunities/${opportunityId}/mark-won`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async markOpportunityLost(
    opportunityId: string,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<OpportunityView> {
    const response = await apiClient.post<{ data: OpportunityView }>(
      `${BASE}/sales/opportunities/${opportunityId}/mark-lost`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async holdOpportunity(
    opportunityId: string,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<OpportunityView> {
    const response = await apiClient.post<{ data: OpportunityView }>(
      `${BASE}/sales/opportunities/${opportunityId}/hold`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async resumeOpportunity(
    opportunityId: string,
    stage: OpportunityStage,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<OpportunityView> {
    const response = await apiClient.post<{ data: OpportunityView }>(
      `${BASE}/sales/opportunities/${opportunityId}/resume`,
      { stage, reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async getOpportunityHistory(
    opportunityId: string,
    params: { cursor?: string; limit?: number } = {}
  ): Promise<PaginatedResponse<OpportunityHistoryView>> {
    const response = await apiClient.get<{
      data: OpportunityHistoryView[];
      nextCursor?: string;
    }>(`${BASE}/sales/opportunities/${opportunityId}/history`, {
      params: cleanQuery(params),
    });
    return unwrapList(response.data);
  },

  // ── Quotations ──────────────────────────────────────────────────────────

  async listQuotations(
    params: ListQuotationsParams = {}
  ): Promise<PaginatedResponse<QuotationView>> {
    const response = await apiClient.get<{
      data: QuotationView[];
      nextCursor?: string;
    }>(`${BASE}/quotations`, { params: cleanQuery(params) });
    return unwrapList(response.data);
  },

  async getQuotation(quotationId: string): Promise<QuotationView> {
    const response = await apiClient.get<{ data: QuotationView }>(
      `${BASE}/quotations/${quotationId}`
    );
    return response.data.data;
  },

  async createQuotation(input: CreateQuotationInput): Promise<QuotationView> {
    const response = await apiClient.post<{ data: QuotationView }>(
      `${BASE}/quotations`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async updateQuotation(
    quotationId: string,
    input: UpdateQuotationInput,
    expectedVersion: number
  ): Promise<QuotationView> {
    const response = await apiClient.patch<{ data: QuotationView }>(
      `${BASE}/quotations/${quotationId}`,
      { ...input, expectedVersion },
      { headers: { "If-Match": String(expectedVersion) } }
    );
    return response.data.data;
  },

  async submitQuotation(
    quotationId: string,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<QuotationView> {
    const response = await apiClient.post<{ data: QuotationView }>(
      `${BASE}/quotations/${quotationId}/submit`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async issueQuotation(
    quotationId: string,
    input: IssueQuotationInput,
    expectedVersion: number
  ): Promise<QuotationIssueResponse> {
    const response = await apiClient.post<{ data: QuotationIssueResponse }>(
      `${BASE}/quotations/${quotationId}/issue`,
      { ...input, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async acceptQuotation(
    quotationId: string,
    input: AcceptQuotationInput,
    expectedVersion?: number
  ): Promise<QuotationAcceptResponse> {
    const response = await apiClient.post<{ data: QuotationAcceptResponse }>(
      `${BASE}/quotations/${quotationId}/accept`,
      { ...input, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async rejectQuotation(
    quotationId: string,
    input: RejectQuotationInput,
    expectedVersion?: number
  ): Promise<QuotationView> {
    const response = await apiClient.post<{ data: QuotationView }>(
      `${BASE}/quotations/${quotationId}/reject`,
      { ...input, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async cancelQuotation(
    quotationId: string,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<QuotationView> {
    const response = await apiClient.post<{ data: QuotationView }>(
      `${BASE}/quotations/${quotationId}/cancel`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async supersedeQuotation(
    quotationId: string,
    replacementQuotationId: string,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<QuotationView> {
    const response = await apiClient.post<{ data: QuotationView }>(
      `${BASE}/quotations/${quotationId}/supersede`,
      { replacementQuotationId, reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async listQuotationVersions(
    quotationId: string,
    params: { limit?: number } = {}
  ): Promise<PaginatedResponse<QuotationVersionView>> {
    const response = await apiClient.get<{
      data: QuotationVersionView[];
      nextCursor?: string;
    }>(`${BASE}/quotations/${quotationId}/versions`, {
      params: cleanQuery(params),
    });
    return unwrapList(response.data);
  },

  async getQuotationVersion(
    quotationId: string,
    versionId: string
  ): Promise<QuotationVersionView> {
    const response = await apiClient.get<{ data: QuotationVersionView }>(
      `${BASE}/quotations/${quotationId}/versions/${versionId}`
    );
    return response.data.data;
  },

  async getQuotationHistory(
    quotationId: string,
    params: { cursor?: string; limit?: number } = {}
  ): Promise<PaginatedResponse<QuotationHistoryView>> {
    const response = await apiClient.get<{
      data: QuotationHistoryView[];
      nextCursor?: string;
    }>(`${BASE}/quotations/${quotationId}/history`, {
      params: cleanQuery(params),
    });
    return unwrapList(response.data);
  },

  async listQuotationDocuments(
    quotationId: string
  ): Promise<PaginatedResponse<DocumentView>> {
    const response = await apiClient.get<{
      data: DocumentView[];
      nextCursor?: string;
    }>(`${BASE}/quotations/${quotationId}/documents`);
    return unwrapList(response.data);
  },

  async attachQuotationDocument(
    quotationId: string,
    input: AttachDocumentInput
  ): Promise<DocumentView> {
    const response = await apiClient.post<{ data: DocumentView }>(
      `${BASE}/quotations/${quotationId}/documents`,
      input
    );
    return response.data.data;
  },

  // ── Contracts ───────────────────────────────────────────────────────────

  async listContracts(
    params: ListContractsParams = {}
  ): Promise<PaginatedResponse<ContractView>> {
    const response = await apiClient.get<{
      data: ContractView[];
      nextCursor?: string;
    }>(`${BASE}/contracts`, { params: cleanQuery(params) });
    return unwrapList(response.data);
  },

  async getContract(contractId: string): Promise<ContractView> {
    const response = await apiClient.get<{ data: ContractView }>(
      `${BASE}/contracts/${contractId}`
    );
    return response.data.data;
  },

  async createContract(input: CreateContractInput): Promise<ContractView> {
    const response = await apiClient.post<{ data: ContractView }>(
      `${BASE}/contracts`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async updateContract(
    contractId: string,
    input: UpdateContractInput,
    expectedVersion: number
  ): Promise<ContractView> {
    const response = await apiClient.patch<{ data: ContractView }>(
      `${BASE}/contracts/${contractId}`,
      input,
      { headers: { "If-Match": String(expectedVersion) } }
    );
    return response.data.data;
  },

  async submitContract(
    contractId: string,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<ContractView> {
    const response = await apiClient.post<{ data: ContractView }>(
      `${BASE}/contracts/${contractId}/submit`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async activateContract(
    contractId: string,
    input: ActivateContractInput,
    expectedVersion: number
  ): Promise<ContractActivateResponse> {
    const response = await apiClient.post<{ data: ContractActivateResponse }>(
      `${BASE}/contracts/${contractId}/activate`,
      { ...input, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async suspendContract(
    contractId: string,
    input: ContractLifecycleInput,
    expectedVersion: number
  ): Promise<ContractView> {
    const response = await apiClient.post<{ data: ContractView }>(
      `${BASE}/contracts/${contractId}/suspend`,
      { ...input, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async resumeContract(
    contractId: string,
    input: ContractLifecycleInput,
    expectedVersion: number
  ): Promise<ContractView> {
    const response = await apiClient.post<{ data: ContractView }>(
      `${BASE}/contracts/${contractId}/resume`,
      { ...input, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async terminateContract(
    contractId: string,
    input: ContractLifecycleInput,
    expectedVersion: number
  ): Promise<ContractView> {
    const response = await apiClient.post<{ data: ContractView }>(
      `${BASE}/contracts/${contractId}/terminate`,
      { ...input, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async renewContract(
    contractId: string,
    input: RenewContractInput,
    expectedVersion: number
  ): Promise<ContractView> {
    const response = await apiClient.post<{ data: ContractView }>(
      `${BASE}/contracts/${contractId}/renew`,
      { ...input, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async listContractVersions(
    contractId: string,
    params: { limit?: number } = {}
  ): Promise<PaginatedResponse<ContractVersionView>> {
    const response = await apiClient.get<{
      data: ContractVersionView[];
      nextCursor?: string;
    }>(`${BASE}/contracts/${contractId}/versions`, {
      params: cleanQuery(params),
    });
    return unwrapList(response.data);
  },

  async getContractVersion(
    contractId: string,
    versionId: string
  ): Promise<ContractVersionView> {
    const response = await apiClient.get<{ data: ContractVersionView }>(
      `${BASE}/contracts/${contractId}/versions/${versionId}`
    );
    return response.data.data;
  },

  async getContractHistory(
    contractId: string,
    params: { cursor?: string; limit?: number } = {}
  ): Promise<PaginatedResponse<ContractHistoryView>> {
    const response = await apiClient.get<{
      data: ContractHistoryView[];
      nextCursor?: string;
    }>(`${BASE}/contracts/${contractId}/history`, {
      params: cleanQuery(params),
    });
    return unwrapList(response.data);
  },

  async listContractDocuments(
    contractId: string
  ): Promise<PaginatedResponse<DocumentView>> {
    const response = await apiClient.get<{
      data: DocumentView[];
      nextCursor?: string;
    }>(`${BASE}/contracts/${contractId}/documents`);
    return unwrapList(response.data);
  },

  async attachContractDocument(
    contractId: string,
    input: AttachDocumentInput
  ): Promise<DocumentView> {
    const response = await apiClient.post<{ data: DocumentView }>(
      `${BASE}/contracts/${contractId}/documents`,
      input
    );
    return response.data.data;
  },

  async replaceContractDocument(
    contractId: string,
    documentId: string,
    input: AttachDocumentInput
  ): Promise<DocumentView> {
    const response = await apiClient.post<{ data: DocumentView }>(
      `${BASE}/contracts/${contractId}/documents/${documentId}/replace`,
      input
    );
    return response.data.data;
  },

  async removeContractDocument(
    contractId: string,
    documentId: string,
    reasonCode: string
  ): Promise<{ removed: boolean; mediaObjectRetained: boolean }> {
    const response = await apiClient.post<{
      data: { removed: boolean; mediaObjectRetained: boolean };
    }>(
      `${BASE}/contracts/${contractId}/documents/${documentId}/remove`,
      { reasonCode }
    );
    return response.data.data;
  },

  // ── SLAs ────────────────────────────────────────────────────────────────

  async listSLAs(
    params: ListSLAsParams = {}
  ): Promise<PaginatedResponse<SLAView>> {
    const response = await apiClient.get<{
      data: SLAView[];
      nextCursor?: string;
    }>(`${BASE}/service-level-agreements`, { params: cleanQuery(params) });
    return unwrapList(response.data);
  },

  async getSLA(slaId: string): Promise<SLAView> {
    const response = await apiClient.get<{ data: SLAView }>(
      `${BASE}/service-level-agreements/${slaId}`
    );
    return response.data.data;
  },

  async createSLA(input: CreateSLAInput): Promise<SLAView> {
    const response = await apiClient.post<{ data: SLAView }>(
      `${BASE}/service-level-agreements`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async updateSLA(
    slaId: string,
    input: UpdateSLAInput,
    expectedVersion: number
  ): Promise<SLAView> {
    const response = await apiClient.patch<{ data: SLAView }>(
      `${BASE}/service-level-agreements/${slaId}`,
      input,
      { headers: { "If-Match": String(expectedVersion) } }
    );
    return response.data.data;
  },

  async activateSLA(
    slaId: string,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<SLAView> {
    const response = await apiClient.post<{ data: SLAView }>(
      `${BASE}/service-level-agreements/${slaId}/activate`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async suspendSLA(
    slaId: string,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<SLAView> {
    const response = await apiClient.post<{ data: SLAView }>(
      `${BASE}/service-level-agreements/${slaId}/suspend`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async terminateSLA(
    slaId: string,
    expectedVersion: number,
    reasonCode?: string
  ): Promise<SLAView> {
    const response = await apiClient.post<{ data: SLAView }>(
      `${BASE}/service-level-agreements/${slaId}/terminate`,
      { reasonCode, expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return response.data.data;
  },

  async listSLAObservations(
    slaId: string,
    params: { outcome?: string; cursor?: string; limit?: number } = {}
  ): Promise<PaginatedResponse<ObservationView>> {
    const response = await apiClient.get<{
      data: ObservationView[];
      nextCursor?: string;
    }>(`${BASE}/service-level-agreements/${slaId}/observations`, {
      params: cleanQuery(params),
    });
    return unwrapList(response.data);
  },

  async getSLAHistory(
    slaId: string,
    params: { cursor?: string; limit?: number } = {}
  ): Promise<PaginatedResponse<SLAHistoryView>> {
    const response = await apiClient.get<{
      data: SLAHistoryView[];
      nextCursor?: string;
    }>(`${BASE}/service-level-agreements/${slaId}/history`, {
      params: cleanQuery(params),
    });
    return unwrapList(response.data);
  },

  // ── Notifications (operator) ────────────────────────────────────────────

  async listNotificationOperations(
    params: { status?: string; subjectId?: string; limit?: number } = {}
  ): Promise<PaginatedResponse<NotificationOperationView>> {
    const response = await apiClient.get<{
      data: NotificationOperationView[];
      nextCursor?: string;
    }>(`${BASE}/crm/notification-operations`, {
      params: cleanQuery(params),
    });
    return unwrapList(response.data);
  },

  async replayNotificationOperation(
    operationId: string
  ): Promise<NotificationOperationView> {
    const response = await apiClient.post<{ data: NotificationOperationView }>(
      `${BASE}/crm/notification-operations/${operationId}/replay`,
      {}
    );
    return response.data.data;
  },
};