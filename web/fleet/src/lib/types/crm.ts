// lib/types/crm.ts

// ── Shared value types ────────────────────────────────────────────────────

export interface MoneyView {
  amountMinor: number;
  currency: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  nextCursor?: string;
}

// ── Enums (mirrors backend models.go constants) ───────────────────────────

export type ActorType =
  | "CUSTOMER"
  | "ORGANIZATION_USER"
  | "EXTERNAL_INTEGRATION"
  | "SYSTEM";

export type CustomerType =
  | "INDIVIDUAL"
  | "BUSINESS"
  | "GOVERNMENT"
  | "NONPROFIT"
  | "INTERNAL_BUSINESS_UNIT"
  | "OTHER";

export type CustomerStatus =
  | "PROSPECT"
  | "ACTIVE"
  | "ON_HOLD"
  | "SUSPENDED"
  | "INACTIVE"
  | "ARCHIVED";

export type RiskStatus =
  | "NOT_ASSESSED"
  | "LOW"
  | "MEDIUM"
  | "HIGH"
  | "RESTRICTED";

export type RecordStatus = "ACTIVE" | "INACTIVE" | "ARCHIVED";

export type ContactChannel =
  | "EMAIL"
  | "PHONE"
  | "SMS"
  | "WHATSAPP"
  | "IN_APP"
  | "NONE";

export type ContactPurpose =
  | "GENERAL"
  | "OPERATIONS"
  | "BILLING"
  | "LEGAL"
  | "SALES"
  | "EMERGENCY"
  | "OTHER";

export type ConsentStatus =
  | "UNKNOWN"
  | "GRANTED"
  | "WITHDRAWN"
  | "NOT_REQUIRED";

export type LocationType =
  | "PICKUP"
  | "DELIVERY"
  | "BILLING"
  | "WAREHOUSE"
  | "OFFICE"
  | "DEPOT"
  | "SITE"
  | "PORT"
  | "TERMINAL"
  | "OTHER";

export type OpportunityStage =
  | "LEAD"
  | "QUALIFIED"
  | "NEEDS_ANALYSIS"
  | "PROPOSAL"
  | "NEGOTIATION"
  | "WON"
  | "LOST"
  | "ON_HOLD";

export type QuotationStatus =
  | "DRAFT"
  | "AWAITING_APPROVAL"
  | "APPROVED"
  | "ISSUED"
  | "VIEWED"
  | "ACCEPTED"
  | "REJECTED"
  | "EXPIRED"
  | "CANCELLED"
  | "SUPERSEDED";

export type AcceptanceMethod =
  | "SECURE_CUSTOMER_TOKEN"
  | "AUTHORIZED_COMPANY_USER"
  | "APPROVED_EXTERNAL_INTEGRATION"
  | "DOCUMENTED_OFFLINE_ACCEPTANCE";

export type ConversionStatus = "PENDING_SECTION_15";

export type ContractStatus =
  | "DRAFT"
  | "AWAITING_APPROVAL"
  | "APPROVED"
  | "ACTIVE"
  | "SUSPENDED"
  | "EXPIRED"
  | "TERMINATED"
  | "RENEWAL_PENDING"
  | "SUPERSEDED";

export type SLAStatus =
  | "DRAFT"
  | "ACTIVE"
  | "SUSPENDED"
  | "EXPIRED"
  | "TERMINATED"
  | "SUPERSEDED";

export type SLAMetricCode =
  | "PICKUP_ON_TIME"
  | "DELIVERY_ON_TIME"
  | "ORDER_COMPLETION_TIME"
  | "PACKAGE_DAMAGE_RATE"
  | "PROOF_COMPLETION_TIME"
  | "FAILED_ATTEMPT_RATE";

export type SLAMeasurementUnit =
  | "PERCENT"
  | "BASIS_POINTS"
  | "MINUTES"
  | "HOURS";

export type SLAMeasurementWindow =
  | "DAILY"
  | "WEEKLY"
  | "MONTHLY"
  | "QUARTERLY"
  | "PER_ORDER";

export type SLAObservationOutcome =
  | "MET"
  | "MISSED"
  | "NOT_EVALUATED"
  | "INSUFFICIENT_DATA"
  | "UNKNOWN";

export type CreditHoldStatus =
  | "NO_HOLD"
  | "REVIEW_REQUIRED"
  | "ON_HOLD"
  | "RESTRICTED";

export type NotificationStatus =
  | "PENDING"
  | "PROCESSING"
  | "UNKNOWN"
  | "DELIVERED"
  | "FAILED_RETRYABLE"
  | "FAILED_PERMANENT"
  | "DEAD_LETTER";

export type NotificationIntent =
  | "QUOTATION_ISSUED"
  | "QUOTATION_VIEWED"
  | "QUOTATION_ACCEPTED"
  | "QUOTATION_REJECTED"
  | "QUOTATION_EXPIRED"
  | "CONTRACT_ACTIVATED"
  | "CONTRACT_SUSPENDED"
  | "CONTRACT_TERMINATED"
  | "CONTRACT_RENEWAL_REMINDER"
  | "CONTRACT_EXPIRED"
  | "SLA_ESCALATION"
  | "CREDIT_HOLD"
  | "CREDIT_HOLD_RELEASED";

// ── Customer views ────────────────────────────────────────────────────────

export interface CustomerView {
  id: string;
  branchId?: string;
  customerType: CustomerType;
  legalName: string;
  displayName: string;
  registrationNumberMasked?: string;
  taxReferenceMasked?: string;
  marketplaceCustomerId?: string;
  status: CustomerStatus;
  riskStatus: RiskStatus;
  accountOwnerUserId?: string;
  defaultCurrency: string;
  paymentTermsReference?: string;
  creditProfileId?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  archivedAt?: string;
}

export interface SensitiveCustomerView extends CustomerView {
  registrationNumber?: string;
  taxReference?: string;
}

export interface CustomerHistoryView {
  id: string;
  changeType: string;
  previousStatus?: string;
  newStatus?: string;
  previousRiskStatus?: string;
  newRiskStatus?: string;
  changedFields: string[];
  reasonCode?: string;
  approvalRequestId?: string;
  actorType: ActorType;
  actorId: string;
  occurredAt: string;
}

// ── Contact views ─────────────────────────────────────────────────────────

export interface ContactView {
  id: string;
  customerId: string;
  fullName: string;
  roleTitle?: string;
  emailMasked?: string;
  phoneMasked?: string;
  preferredChannel: ContactChannel;
  channelDeliverable: boolean;
  contactPurpose: ContactPurpose;
  isPrimary: boolean;
  notificationPreferences?: Record<string, unknown>;
  consentStatus: ConsentStatus;
  consentRecordedAt?: string;
  status: RecordStatus;
  version: number;
  createdAt: string;
  updatedAt: string;
  archivedAt?: string;
}

export interface SensitiveContactView extends ContactView {
  email?: string;
  phone?: string;
  consentSource?: string;
}

// ── Location views ────────────────────────────────────────────────────────

export interface LocationView {
  id: string;
  customerId: string;
  locationType: LocationType;
  label: string;
  city: string;
  stateOrRegion: string;
  countryCode: string;
  timezone: string;
  operatingHours?: Record<string, unknown>;
  contactReference?: string;
  geofenceReference?: string;
  hasCoordinates: boolean;
  hasAccessInstructions: boolean;
  hasSafetyInstructions: boolean;
  status: RecordStatus;
  version: number;
  createdAt: string;
  updatedAt: string;
  archivedAt?: string;
}

export interface SensitiveLocationView extends LocationView {
  addressLine1: string;
  addressLine2?: string;
  postalCode?: string;
  latitude?: number;
  longitude?: number;
  accessInstructions?: string;
  safetyInstructions?: string;
}

// ── Opportunity views ─────────────────────────────────────────────────────

export interface OpportunityView {
  id: string;
  branchId?: string;
  customerId: string;
  title: string;
  description?: string;
  stage: OpportunityStage;
  expectedValue?: MoneyView;
  probabilityBasisPoints: number;
  expectedCloseDate?: string;
  serviceCapabilities: string[];
  accountOwnerUserId?: string;
  nextAction?: string;
  nextActionAt?: string;
  lossReason?: string;
  holdReason?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  closedAt?: string;
}

export interface OpportunityHistoryView {
  id: string;
  previousStage: string;
  newStage: string;
  reasonCode?: string;
  actorType: ActorType;
  actorId: string;
  occurredAt: string;
}

// ── Quotation views ───────────────────────────────────────────────────────

export interface LineItemView {
  id: string;
  description: string;
  quantity: string;
  unitType?: string;
  unitAmountMinor: number;
  lineAmountMinor: number;
  currency: string;
  externalPricingReference?: string;
  manualReason?: string;
  sequence: number;
}

export interface QuotationView {
  id: string;
  branchId?: string;
  customerId: string;
  opportunityId?: string;
  quotationNumber: string;
  status: QuotationStatus;
  total: MoneyView;
  validFrom: string;
  validUntil: string;
  serviceScope?: Record<string, unknown>;
  terms?: Record<string, unknown>;
  currentVersion: number;
  issuedVersionId?: string;
  acceptedVersionId?: string;
  approvalRequestId?: string;
  supersededById?: string;
  lineItems?: LineItemView[];
  version: number;
  createdAt: string;
  updatedAt: string;
  issuedAt?: string;
  resolvedAt?: string;
}

export interface SensitiveQuotationView extends QuotationView {
  internalNotes?: string;
}

export interface QuotationVersionView {
  id: string;
  quotationId: string;
  versionNumber: number;
  customerSnapshot: Record<string, unknown>;
  contactSnapshot?: Record<string, unknown>;
  total: MoneyView;
  lineItems: LineItemView[];
  serviceScope?: Record<string, unknown>;
  terms?: Record<string, unknown>;
  validFrom: string;
  validUntil: string;
  documentReferences: string[];
  approvalRequestId?: string;
  issuedBy: string;
  issuedAt: string;
  contentFingerprint: string;
}

export interface QuotationHistoryView {
  id: string;
  previousStatus?: string;
  newStatus: string;
  quotationVersion: number;
  reasonCode?: string;
  approvalRequestId?: string;
  actorType: ActorType;
  actorId?: string;
  occurredAt: string;
}

export interface AcceptanceView {
  id: string;
  quotationId: string;
  acceptedVersionId: string;
  actorType: ActorType;
  actorReference?: string;
  acceptanceMethod: AcceptanceMethod;
  evidenceDocumentReference?: string;
  acceptedAt: string;
}

export interface ConversionIntentView {
  id: string;
  quotationId: string;
  acceptedVersionId: string;
  acceptanceId: string;
  conversionType: string;
  status: ConversionStatus;
  createdAt: string;
}

export interface QuotationIssueResponse {
  quotation: QuotationView;
  issuedVersion: QuotationVersionView;
  customerAccessToken?: string;
  customerAccessTokenExpiresAt?: string;
}

export interface QuotationAcceptResponse {
  quotation: QuotationView;
  acceptance: AcceptanceView;
  conversionIntent: ConversionIntentView;
  replayed: boolean;
}

// ── Document views ────────────────────────────────────────────────────────

export interface DocumentView {
  id: string;
  mediaObjectId: string;
  documentPurpose: string;
  isMandatory: boolean;
  statusProjection: string;
  readinessState: string;
  usable: boolean;
  versionId?: string;
  sourceService: string;
  sourceVersion: number;
  syncedAt?: string;
  createdAt: string;
}

// ── Contract views ────────────────────────────────────────────────────────

export interface ContractView {
  id: string;
  branchId?: string;
  customerId: string;
  quotationId?: string;
  contractNumber: string;
  status: ContractStatus;
  effectiveFrom: string;
  effectiveTo?: string;
  serviceScope?: Record<string, unknown>;
  renewalTerms?: Record<string, unknown>;
  terminationTerms?: Record<string, unknown>;
  currentVersion: number;
  activeVersionId?: string;
  approvalRequestId?: string;
  supersedesId?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  activatedAt?: string;
  terminatedAt?: string;
}

export interface SensitiveContractView extends ContractView {
  commercialSnapshot?: Record<string, unknown>;
}

export interface ContractVersionView {
  id: string;
  contractId: string;
  versionNumber: number;
  customerSnapshot: Record<string, unknown>;
  serviceScope?: Record<string, unknown>;
  commercialSnapshot?: Record<string, unknown>;
  effectiveFrom: string;
  effectiveTo?: string;
  renewalTerms?: Record<string, unknown>;
  terminationTerms?: Record<string, unknown>;
  documentReferences: string[];
  signatoryReferences: string[];
  approvalRequestId?: string;
  contentFingerprint: string;
  activatedBy: string;
  activatedAt: string;
}

export interface ContractHistoryView {
  id: string;
  previousStatus?: string;
  newStatus: string;
  reasonCode?: string;
  actorType: ActorType;
  actorId: string;
  occurredAt: string;
}

export interface ContractActivateResponse {
  contract: ContractView;
  activeVersion: ContractVersionView;
}

// ── SLA views ─────────────────────────────────────────────────────────────

export interface SLAView {
  id: string;
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
  status: SLAStatus;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface ObservationView {
  id: string;
  slaId: string;
  sourceType: string;
  sourceReference?: string;
  measuredValue?: string;
  measurementUnit?: string;
  measuredAt: string;
  windowStart: string;
  windowEnd: string;
  outcome: SLAObservationOutcome;
  sourceVersion: number;
  createdAt: string;
}

export interface SLAHistoryView {
  id: string;
  previousStatus?: string;
  newStatus: string;
  reasonCode?: string;
  actorType: ActorType;
  actorId: string;
  occurredAt: string;
}

// ── Credit views ──────────────────────────────────────────────────────────

export interface CreditProfileView {
  id: string;
  customerId: string;
  currency: string;
  paymentTermsDays: number;
  holdStatus: CreditHoldStatus;
  blocking: boolean;
  hasCreditLimit: boolean;
  riskStatusSnapshot: RiskStatus;
  version: number;
  effectiveFrom: string;
  createdAt: string;
  updatedAt: string;
}

export interface SensitiveCreditProfileView extends CreditProfileView {
  creditLimit?: MoneyView;
  holdReason?: string;
  approvedBy?: string;
  approvalRequestId?: string;
}

export interface CreditHistoryView {
  id: string;
  previousHoldStatus?: string;
  newHoldStatus: string;
  previousLimit?: MoneyView;
  newLimit?: MoneyView;
  reasonCode?: string;
  approvalRequestId?: string;
  actorType: ActorType;
  actorId: string;
  occurredAt: string;
}

// ── Notification views ────────────────────────────────────────────────────

export interface NotificationOperationView {
  id: string;
  subjectType: string;
  subjectId: string;
  intentType: NotificationIntent;
  recipientReference?: string;
  channel: string;
  status: NotificationStatus;
  attempts: number;
  nextAttemptAt: string;
  safeErrorCode?: string;
  deadLetterReason?: string;
  createdAt: string;
  updatedAt: string;
}