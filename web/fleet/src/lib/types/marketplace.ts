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
  id: string;
  organizationId: string;
  requestProjectionId: string;
  offerId?: string;
  membershipId: string;
  executingProviderId: string;
  sourceService: string;
  assetReference?: string;
  assignedByType: string;
  assignedById: string;
  status: string;
  responseDeadline: string;
  acceptedAt?: string;
  declinedAt?: string;
  expiredAt?: string;
  cancelledAt?: string;
  readyForExecutionAt?: string;
  startedAt?: string;
  completedAt?: string;
  rootAssignmentId?: string;
  replacesAssignmentId?: string;
  reassignmentAttempt: number;
  reassignmentReason?: string;
  executionStartStatus: string;
  sourceExecutionId?: string;
  acknowledgementState: string;
  acknowledgedAt?: string;
  acknowledgedByActorType?: string;
  acknowledgedByActorId?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface StateHistoryEntry {
  id: string;
  organizationId: string;
  assignmentId: string;
  fromStatus: string;
  toStatus: string;
  actorType: string;
  actorId: string;
  reasonCode?: string;
  metadata?: Record<string, unknown>;
  requestId: string;
  correlationId: string;
  occurredAt: string;
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