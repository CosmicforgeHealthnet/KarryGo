// lib/types/alerts.ts

// Types mirroring dispatch.OperationalAlert (internal/features/dispatch/domain.go)
// and the JSON the corporate alerts endpoints emit. Field names match the Go
// struct's JSON tags exactly.

export type AlertSeverity = "CRITICAL" | "HIGH" | "MEDIUM" | "LOW" | string;

export type AlertStatus =
  | "OPEN"
  | "ACKNOWLEDGED"
  | "IN_PROGRESS"
  | "RESOLVED"
  | "DISMISSED"
  | string;

export interface OperationalAlert {
  id: string;
  organizationId: string;
  branchId?: string;
  alertType: string;
  severity: AlertSeverity;
  sourceType: string;
  sourceId: string;
  routePlanId?: string;
  assignmentId?: string;
  shipmentId?: string;
  canonicalTripReference?: string;
  ownerUserId?: string;
  status: AlertStatus;
  slaDueAt: string;
  acknowledgedAt?: string;
  resolvedAt?: string;
  resolutionCode?: string;
  resolutionReason?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

// Query parameters accepted by GET /alerts.
export interface AlertListFilter {
  branchId?: string;
  status?: string;
  alertType?: string;
  limit?: number;
  offset?: number;
}

export interface AcknowledgeAlertInput {
  expectedVersion: number;
}

export interface ResolveAlertInput {
  expectedVersion: number;
  resolutionCode?: string;
  resolutionReason?: string;
}