// lib/services/dispatchService.ts

import apiClient from '@/lib/api/client';
import {
  CreatePlanInput,
  DispatchPlan,
  PlanActionInput,
  PlanListFilter,
} from '@/lib/types/dispatch';

function buildQuery(filter?: PlanListFilter): string {
  if (!filter) return '';
  const params = new URLSearchParams();
  if (filter.branchId) params.set('branchId', filter.branchId);
  if (filter.status) params.set('status', filter.status);
  if (filter.limit != null) params.set('limit', String(filter.limit));
  if (filter.offset != null) params.set('offset', String(filter.offset));
  const qs = params.toString();
  return qs ? `?${qs}` : '';
}

export const dispatchService = {
  async listPlans(filter?: PlanListFilter): Promise<DispatchPlan[]> {
    const res = await apiClient.get<{ data: DispatchPlan[] }>(
      `/dispatch/plans${buildQuery(filter)}`,
    );
    return res.data.data ?? [];
  },

  async getPlan(planId: string): Promise<DispatchPlan> {
    const res = await apiClient.get<{ data: DispatchPlan }>(
      `/dispatch/plans/${encodeURIComponent(planId)}`,
    );
    return res.data.data;
  },

  async createPlan(
    input: CreatePlanInput,
    idempotencyKey: string,
  ): Promise<DispatchPlan> {
    const res = await apiClient.post<{ data: DispatchPlan }>(
      `/dispatch/plans`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async approvePlan(
    planId: string,
    input: PlanActionInput,
    idempotencyKey: string,
  ): Promise<DispatchPlan> {
    const res = await apiClient.post<{ data: DispatchPlan }>(
      `/dispatch/plans/${encodeURIComponent(planId)}/approve`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async publishPlan(
    planId: string,
    input: PlanActionInput,
    idempotencyKey: string,
  ): Promise<DispatchPlan> {
    const res = await apiClient.post<{ data: DispatchPlan }>(
      `/dispatch/plans/${encodeURIComponent(planId)}/publish`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async cancelPlan(
    planId: string,
    input: PlanActionInput,
    idempotencyKey: string,
  ): Promise<DispatchPlan> {
    const res = await apiClient.post<{ data: DispatchPlan }>(
      `/dispatch/plans/${encodeURIComponent(planId)}/cancel`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },
};

export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}