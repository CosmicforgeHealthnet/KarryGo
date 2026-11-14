// Wire shapes of the CRM lead, private operations, rescue and trip routes
// (fleet-partner-service Sections 14-16). Field names are exactly what the
// server sends; a few older domain types (custody events, specialized
// records) are serialized with Go field names, hence PascalCase.

import type { CustomerView, OpportunityView } from '@/lib/types/crm';

// ── CRM leads ────────────────────────────────────────────────────────────

export const LEAD_STATUSES = ['NEW', 'CONTACTED', 'QUALIFIED', 'DISQUALIFIED', 'CONVERTED'] as const;
export type LeadStatus = (typeof LEAD_STATUSES)[number];

export const LEAD_SOURCES = [
  'WEBSITE', 'REFERRAL', 'EVENT', 'INBOUND_CALL', 'INBOUND_EMAIL', 'OUTBOUND', 'PARTNER', 'IMPORT', 'OTHER',
] as const;

export interface Lead {
  id: string;
  branchId?: string;
  source: string;
  companyName: string;
  status: LeadStatus;
  ownerUserId?: string;
  score?: number;
  nextAction?: string;
  nextActionAt?: string;
  disqualificationReason?: string;
  convertedCustomerId?: string;
  convertedOpportunityId?: string;
  convertedAt?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  // Present only for callers holding the sensitive-data grant.
  contactSnapshot?: Record<string, unknown>;
  notes?: string;
}

export interface LeadInput {
  branchId?: string;
  source: string;
  companyName: string;
  contactSnapshot?: Record<string, unknown>;
  ownerUserId?: string;
  score?: number | null;
  nextAction?: string;
  nextActionAt?: string;
  notes?: string;
}

export interface LeadPatch {
  companyName?: string;
  source?: string;
  contactSnapshot?: Record<string, unknown>;
  score?: number;
  clearScore?: boolean;
  nextAction?: string;
  nextActionAt?: string;
  notes?: string;
}

export interface LeadHistoryEntry {
  id: string;
  changeType: string;
  previousStatus?: string;
  newStatus: string;
  reasonCode?: string;
  actorType: string;
  occurredAt: string;
}

export interface LeadConvertInput {
  customerId?: string;
  newCustomer?: {
    customerType?: string;
    legalName?: string;
    displayName?: string;
    branchId?: string;
    defaultCurrency?: string;
  };
  opportunity?: {
    title: string;
    description?: string;
    expectedValue?: { amountMinor: number; currency: string };
    probabilityBasisPoints?: number;
    expectedCloseDate?: string;
    nextAction?: string;
  };
}

export interface LeadConversion {
  lead: Lead;
  customer: CustomerView;
  customerCreated: boolean;
  opportunity?: OpportunityView;
  replayed: boolean;
}

// ── Orders and shipments ─────────────────────────────────────────────────

export const ORDER_PRIORITIES = ['LOW', 'STANDARD', 'HIGH', 'URGENT'] as const;

export interface Order {
  id: string;
  organizationId: string;
  branchId?: string;
  orderNumber: string;
  customerId: string;
  customerReference?: string;
  source: string;
  sourceReference?: string;
  serviceCatalogItemId: string;
  serviceFields: Record<string, unknown>;
  contractId?: string;
  quotationId?: string;
  requestedScheduleStart?: string;
  requestedScheduleEnd?: string;
  timezone: string;
  priority: string;
  specialInstructions?: string;
  status: string;
  approvalRequestId?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  confirmedAt?: string;
  cancelledAt?: string;
  completedAt?: string;
}

export interface OrderInput {
  branchId?: string;
  customerId: string;
  customerReference?: string;
  serviceCatalogItemId: string;
  serviceFields?: Record<string, unknown>;
  requestedScheduleStart?: string;
  requestedScheduleEnd?: string;
  timezone?: string;
  priority?: string;
  specialInstructions?: string;
}

export interface OrderPatch {
  customerReference?: string;
  requestedScheduleStart?: string;
  requestedScheduleEnd?: string;
  timezone?: string;
  priority?: string;
  specialInstructions?: string;
}

export interface OrderStop {
  id: string;
  orderId: string;
  sequence: number;
  stopType: string;
  customerLocationId?: string;
  latitude?: number;
  longitude?: number;
  timeWindowStart?: string;
  timeWindowEnd?: string;
  timezone: string;
  instructions?: string;
  status: string;
  version: number;
}

export interface OrderHistoryEntry {
  previousStatus: string;
  newStatus: string;
  reasonCode?: string;
  actorType: string;
  occurredAt: string;
}

export const SERVICE_CATEGORIES = [
  'PACKAGE_DELIVERY', 'COURIER_LAST_MILE', 'SAME_DAY_DELIVERY', 'SCHEDULED_DELIVERY', 'TAXI', 'PASSENGER_TRANSPORT',
  'HAULING', 'TRUCKING', 'HEAVY_HAUL', 'FREIGHT_FORWARDING', 'MOVING_RELOCATION', 'COLD_CHAIN', 'MEDICAL_LOGISTICS',
  'AGRICULTURE_FOOD', 'TOWING_RECOVERY', 'WAREHOUSING', 'FULFILMENT', 'PRIVATE_LOGISTICS', 'OTHER',
] as const;
export const SERVICE_STATUSES = ['DRAFT', 'ACTIVE', 'INACTIVE', 'ARCHIVED'] as const;

// GET /service-catalog and /service-catalog/:serviceId (CatalogItemView).
export interface ServiceCatalogItem {
  id: string;
  organizationId?: string;
  branchId?: string;
  code?: string;
  name: string;
  category?: string;
  description?: string;
  capabilityReference?: string;
  requiredFieldsSchema?: Record<string, unknown>;
  supportedVehicleClasses?: string[];
  supportedPackageTypes?: string[];
  pricingPolicyReference?: string;
  approvalPolicyReference?: string;
  proofPolicyReference?: string;
  status: string;
  version?: number;
  createdAt?: string;
  updatedAt?: string;
  archivedAt?: string;
}

export interface ServiceCatalogInput {
  branchId?: string;
  name: string;
  code: string;
  category: string;
  description?: string;
  capabilityReference?: string;
  requiredFieldsSchema?: Record<string, unknown>;
  supportedVehicleClasses?: string[];
  supportedPackageTypes?: string[];
  pricingPolicyReference?: string;
  approvalPolicyReference?: string;
  proofPolicyReference?: string;
}

// PATCH replaces every editable field, so callers send the full set.
export interface ServiceCatalogPatch {
  name: string;
  description: string;
  capabilityReference: string;
  requiredFieldsSchema: Record<string, unknown>;
  supportedVehicleClasses: string[];
  supportedPackageTypes: string[];
  pricingPolicyReference: string;
  approvalPolicyReference: string;
  proofPolicyReference: string;
}

export interface Shipment {
  id: string;
  organizationId: string;
  branchId?: string;
  orderId: string;
  originStopId: string;
  destinationStopId: string;
  cargoProfile: Record<string, unknown>;
  passengerProfile: Record<string, unknown>;
  requiredVehicleClass?: string;
  requiredServiceCapability?: string;
  temperatureRequirements: Record<string, unknown>;
  declaredValueAmountMinor: number;
  declaredValueCurrency: string;
  packageCount: number;
  status: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface PackageView {
  id: string;
  shipmentId: string;
  branchId?: string;
  packageCode: string;
  barcodeValue: string;
  qrValue: string;
  packageType: string;
  condition: string;
  status: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface Page<T> {
  items: T[];
  nextCursor?: string;
}

// ── Chain of custody ─────────────────────────────────────────────────────

export const CUSTODY_SUBJECTS = ['PACKAGE', 'SHIPMENT', 'ORDER', 'ASSET', 'SPECIALIZED_RECORD'] as const;

// Serialized with Go field names.
export interface CustodyEvent {
  id: string;
  subjectType: string;
  subjectReference: string;
  packageId?: string;
  shipmentId?: string;
  fromActorType: string;
  fromActorReference: string;
  toActorType: string;
  toActorReference: string;
  fromLocationReference?: string;
  toLocationReference?: string;
  condition?: string;
  correctionOfEventId?: string;
  source?: string;
  occurredAt: string;
  receivedAt?: string;
  evidenceMediaIds?: string[] | null;
}

export interface CustodyEventInput {
  subjectType: string;
  subjectReference: string;
  fromActorType: string;
  fromActorReference: string;
  toActorType: string;
  toActorReference: string;
  fromLocationReference?: string;
  toLocationReference?: string;
  condition?: string;
  occurredAt: string;
}

// ── Specialized logistics ────────────────────────────────────────────────

export interface SchemaField {
  name: string;
  kind: string;
  required: boolean;
  enum?: string[];
  minimum?: number;
  fields?: SchemaField[];
}

export interface CapabilitySchema {
  capabilityCode: string;
  schemaId: string;
  version: number;
  supportedVersions: number[];
  restrictedVisibility: boolean;
  executionAdapter: string;
  requiredProofTypes: string[];
  financialValues: string;
  recordTypes: { recordType: string; fields: SchemaField[]; additionalProperties: boolean; rules?: string[] }[];
}

// Serialized with Go field names.
export interface SpecializedRecord {
  id: string;
  capabilityCode: string;
  recordType: string;
  subjectType: string;
  subjectReference: string;
  status: string;
  restrictedVisibility: boolean;
  payload: Record<string, unknown> | null;
  schemaVersion: number;
  schemaId: string;
  version: number;
  createdAt: string;
}

// ── Rescue and geofence events ───────────────────────────────────────────

export const RESCUE_TRIGGERS = [
  'BREAKDOWN', 'PROVIDER_UNAVAILABLE', 'DELAY', 'FAILED_DELIVERY', 'UNSAFE_VEHICLE', 'ACCIDENT', 'CARGO_TRANSFER', 'OTHER',
] as const;
export const RESCUE_SEVERITIES = ['LOW', 'MEDIUM', 'HIGH', 'CRITICAL'] as const;
export const RESCUE_RESOLUTIONS = [
  'REPLACEMENT_COMPLETED', 'RECOVERED_ON_SITE', 'CARGO_TRANSFERRED', 'TRIP_CANCELLED_AT_SOURCE', 'NO_ACTION_REQUIRED',
] as const;

export interface RescueCase {
  id: string;
  branchId: string;
  triggerType: string;
  severity: string;
  status: string;
  privateAssignmentId?: string;
  canonicalTripReference?: string;
  assetId?: string;
  breakdownId?: string;
  reason: string;
  assignedToUserId?: string;
  currentAssignmentId?: string;
  replacementAssignmentId?: string;
  replacementAssignmentStatus?: string;
  replacementMembershipId?: string;
  replacementAssetReference?: string;
  replacementAttempts: number;
  alertId?: string;
  resolutionCode?: string;
  resolvedAt?: string;
  cancelledAt?: string;
  createdBy: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface OpenRescueInput {
  branchId?: string;
  triggerType: string;
  severity: string;
  privateAssignmentId?: string;
  canonicalTripReference?: string;
  assetId?: string;
  breakdownId?: string;
  reason: string;
}

export interface RescueReplacementInput {
  providerMembershipId: string;
  providerId?: string;
  vehicleOrTruckReference?: string;
  executionService?: string;
}

export interface GeofenceEvent {
  id: string;
  geofenceId: string;
  geofenceName: string;
  branchId?: string;
  canonicalTripReference: string;
  eventRule: string;
  sourceVersion: number;
  confidence: string;
  occurredAt: string;
  recordedAt: string;
  dataFreshness: string;
  ageSeconds: number;
}

// ── Marketplace trip lifecycle ───────────────────────────────────────────

export interface CorporateTrip {
  tripId: string;
  sourceService: string;
  platformTripId: string;
  platformRequestId?: string;
  platformBookingId?: string;
  assignmentId?: string;
  organizationMembershipId?: string;
  executingProviderId: string;
  serviceType: string;
  tripStatus: string;
  sourceVersion: number;
  freshnessState: string;
  lastEventAt?: string;
  lastReconciledAt?: string;
  completedAt?: string;
  cancelledAt?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CorporateTripPage {
  items: CorporateTrip[];
  hasMore: boolean;
  nextOffset?: number;
}

export interface CorporateTripDetail {
  trip: CorporateTrip;
  history: { id: string; fromStatus: string; toStatus: string; sourceVersion: number; sourceEventId?: string; occurredAt: string }[];
  deliveryOutcome?: {
    recipientConfirmedAt?: string;
    confirmationMethod?: string;
    recipientMatchedExpected: boolean;
    failedAt?: string;
    failureReasonCode?: string;
    failureReasonCategory?: string;
    retryEligible: boolean;
  };
  assignment?: { assignmentId: string; status: string; startedAt?: string; completedAt?: string };
}

// ── Manifests (serialized with Go field names) ───────────────────────────

export const MANIFEST_ITEM_TYPES = ['SHIPMENT', 'PACKAGE', 'ORDER_STOP'] as const;
export const MANIFEST_STATUSES = ['DRAFT', 'OPEN', 'CLOSED', 'DISPATCHED', 'COMPLETED', 'CANCELLED'] as const;

export interface Manifest {
  id: string;
  organizationId: string;
  branchId: string;
  manifestReference: string;
  routeReference: string;
  futureRoutePlanId: string;
  vehicleReference: string;
  providerMembershipId: string;
  sealNumber: string;
  status: string;
  version: number;
  createdBy: string;
  updatedBy: string;
  createdAt: string;
  updatedAt: string;
  closedAt: string | null;
  dispatchedAt: string | null;
  completedAt: string | null;
  cancelledAt: string | null;
}

export interface ManifestItem {
  id: string;
  manifestId: string;
  itemType: string;
  itemId: string;
  shipmentId: string;
  addedBy: string;
  createdAt: string;
}

export interface ManifestDetail {
  manifest: Manifest;
  items: ManifestItem[] | null;
}

export interface ManifestInput {
  branchId: string;
  routeReference?: string;
  vehicleReference?: string;
  providerMembershipId?: string;
  sealNumber?: string;
}

export interface ManifestPatch {
  routeReference: string;
  vehicleReference: string;
  providerMembershipId: string;
  sealNumber: string;
}

// ── Recurring order templates (serialized with Go field names) ───────────

export const RECURRENCE_TYPES = ['DAILY', 'WEEKLY', 'MONTHLY', 'CUSTOM_RRULE'] as const;
export const RECURRING_STATUSES = ['DRAFT', 'ACTIVE', 'INACTIVE', 'COMPLETED', 'CANCELLED'] as const;

export interface RecurringTemplateData {
  customerReference: string;
  serviceFields: Record<string, unknown> | null;
  priority: string;
  specialInstructions: string;
  durationMinutes: number;
}

export interface RecurringOrderTemplate {
  id: string;
  organizationId: string;
  branchId: string;
  customerId: string;
  serviceCatalogItemId: string;
  contractId: string;
  data: RecurringTemplateData;
  recurrenceType: string;
  normalizedRrule: string;
  timezone: string;
  startDate: string;
  endDate: string | null;
  nextRunAt: string;
  missedRunPolicy: string;
  generatedOrderInitialState: string;
  status: string;
  version: number;
  attempts: number;
  lastErrorCode: string;
  createdAt: string;
  updatedAt: string;
  activatedAt: string | null;
  deactivatedAt: string | null;
}

export interface RecurringOrderInput {
  branchId: string;
  customerId: string;
  serviceCatalogItemId: string;
  recurrenceType: string;
  rrule?: string;
  timezone: string;
  startDate?: string;
  endDate?: string;
  templateData: {
    customerReference?: string;
    priority?: string;
    specialInstructions?: string;
    durationMinutes?: number;
  };
}

// ── Order imports, legacy engine (serialized with Go field names) ────────

export interface OrderImport {
  id: string;
  organizationId: string;
  branchId: string;
  mediaFileId: string;
  fileName: string;
  fileFormat: string;
  contentType: string;
  status: string;
  sizeBytes: number;
  totalRows: number;
  validRows: number;
  invalidRows: number;
  importedRows: number;
  failedRows: number;
  errorReportMediaId: string;
  version: number;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  completedAt: string | null;
  cancelledAt: string | null;
  attempts: number;
}

export interface OrderImportRow {
  id: string;
  importId: string;
  rowNumber: number;
  rowReference: string;
  status: string;
  generatedOrderId: string;
  safeErrorCode: string;
  safeErrorMessage: string;
  data: { customerId: string; customerReference: string; serviceCatalogItemId: string; priority: string } | null;
  createdAt: string;
}

export interface OrderImportDetail {
  import: OrderImport;
  rows: OrderImportRow[] | null;
}

// ── Private assignments (serialized with Go field names) ─────────────────

export interface PrivateAssignment {
  id: string;
  branchId: string;
  workOrigin: string;
  privateOrderId: string;
  shipmentId: string;
  routeStopId: string;
  providerMembershipId: string;
  providerId: string;
  vehicleOrTruckReference: string;
  executionService: string;
  status: string;
  responseDeadline: string;
  acceptedAt: string | null;
  declinedAt: string | null;
  expiredAt: string | null;
  cancelledAt: string | null;
  declineReason: string;
  replacementAssignmentId: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}
