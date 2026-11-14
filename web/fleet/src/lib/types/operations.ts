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

export interface ServiceCatalogItem {
  id: string;
  code?: string;
  name: string;
  status: string;
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
  ID: string;
  SubjectType: string;
  SubjectReference: string;
  PackageID?: string;
  ShipmentID?: string;
  FromActorType: string;
  FromActorReference: string;
  ToActorType: string;
  ToActorReference: string;
  FromLocationReference?: string;
  ToLocationReference?: string;
  Condition?: string;
  CorrectionOfEventID?: string;
  Source?: string;
  OccurredAt: string;
  ReceivedAt?: string;
  EvidenceMediaIDs?: string[] | null;
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
  ID: string;
  CapabilityCode: string;
  RecordType: string;
  SubjectType: string;
  SubjectReference: string;
  Status: string;
  RestrictedVisibility: boolean;
  Payload: Record<string, unknown> | null;
  SchemaVersion: number;
  SchemaID: string;
  Version: number;
  CreatedAt: string;
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
