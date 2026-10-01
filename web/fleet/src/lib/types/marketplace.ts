// lib/types/marketplace.ts

// Types mirroring the Go corporate marketplace DTOs in
// internal/features/marketplace/usecases/corporate.go and the underlying
// models package. Field names match the JSON tags the backend emits.

export interface CorporateOfferSummary {
  offerId: string;
  requestProjectionId: string;
  sourceService: string;
  serviceType: string;
  status: string;
  operatingMode: string;
  offeredAt: string;
  expiresAt: string;
  pickupSummary: Record<string, unknown>;
  dropoffSummary: Record<string, unknown>;
  operationalRequirements: Record<string, unknown>;
  version: number;
}

export interface CorporateOfferAcceptanceView {
  acceptanceId: string;
  acceptanceSource: string;
  sourceReservationId: string;
  acceptedAt: string;
}

export interface CorporateOfferDetail {
  offer: CorporateOfferSummary;
  acceptance?: CorporateOfferAcceptanceView;
}

export interface CorporateOfferMetrics {
  openOffers: number;
  expiringSoon: number;
  acceptedToday: number;
  declinedToday: number;
}

export interface CorporateCandidate {
  membershipId: string;
  providerId: string;
  sourceService: string;
  serviceCategory: string;
  status: string;
  eligible: boolean;
  reason?: string;
}

// Assignment mirrors models.Assignment from the Go backend. Timestamps are
// RFC3339 strings; optional fields use `*T` in Go and become `T | undefined`
// here (absent from JSON when nil).

export interface Assignment {
  ID: string;
  OrganizationID: string;
  RequestProjectionID: string;
  OfferID?: string;
  MembershipID: string;
  ExecutingProviderID: string;
  SourceService: string;
  AssetReference?: string;
  AssignedByType: string;
  AssignedByID: string;
  Status: string;
  ResponseDeadline: string;
  AcceptedAt?: string;
  DeclinedAt?: string;
  ExpiredAt?: string;
  CancelledAt?: string;
  ReadyForExecutionAt?: string;
  StartedAt?: string;
  CompletedAt?: string;
  RootAssignmentID?: string;
  ReplacesAssignmentID?: string;
  ReassignmentAttempt: number;
  ReassignmentReason?: string;
  ExecutionStartStatus: string;
  SourceExecutionID?: string;
  AcknowledgementState: string;
  AcknowledgedAt?: string;
  AcknowledgedByActorType?: string;
  AcknowledgedByActorID?: string;
  Version: number;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface StateHistoryEntry {
  ID: string;
  OrganizationID: string;
  AssignmentID: string;
  FromStatus: string;
  ToStatus: string;
  ActorType: string;
  ActorID: string;
  ReasonCode?: string;
  Metadata?: Record<string, unknown>;
  RequestID: string;
  CorrelationID: string;
  OccurredAt: string;
}

// Write inputs.

export interface AcceptOfferInput {
  expectedVersion: number;
}

export interface DeclineOfferInput {
  reasonCode: string;
  note?: string;
}

export interface AssignRequestInput {
  membershipId: string;
  assetReference?: string;
  responseTimeoutSeconds?: number;
  expectedRequestVersion?: number;
}

export interface CancelAssignmentInput {
  reason: string;
  expectedVersion: number;
}

export interface ReassignAssignmentInput {
  membershipId: string;
  assetReference?: string;
  responseTimeoutSeconds?: number;
  reason: string;
}

// Pagination filter matching usecases.CorporatePageFilter.

export interface CorporatePageFilter {
  status?: string;
  branchId?: string;
  limit?: number;
  cursorSortValue?: string;
  cursorLastId?: string;
}