// lib/types/dispatch.ts

// Types mirroring dispatch.DispatchPlan (internal/features/dispatch/domain.go)
// and the JSON the corporate dispatch-plan endpoints emit.

export interface DispatchPlan {
  id: string;
  organizationId: string;
  branchId: string;
  planDate: string;
  workWindowStart: string;
  workWindowEnd: string;
  timezone: string;
  status: string;
  plannerUserId: string;
  approvedBy?: string;
  approvedAt?: string;
  publishedAt?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface PlanListFilter {
  branchId?: string;
  status?: string;
  limit?: number;
  offset?: number;
}

export interface CreatePlanInput {
  branchId: string;
  planDate: string;          // RFC3339
  workWindowStart: string;   // RFC3339
  workWindowEnd: string;     // RFC3339
  timezone: string;          // IANA, e.g. "Africa/Lagos"
}

export interface PlanActionInput {
  expectedVersion: number;
  reason?: string;
  overrideReason?: string;
}