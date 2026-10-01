// lib/services/marketplaceService.ts

import apiClient from '@/lib/api/client';
import {
  Assignment,
  AssignRequestInput,
  AcceptOfferInput,
  CancelAssignmentInput,
  CorporateCandidate,
  CorporateOfferDetail,
  CorporateOfferMetrics,
  CorporateOfferSummary,
  CorporatePageFilter,
  DeclineOfferInput,
  ReassignAssignmentInput,
  StateHistoryEntry,
} from '@/lib/types/marketplace';

// Every corporate marketplace endpoint returns the standard envelope:
//   { data: T, meta: { requestId: string } }
// apiClient's response interceptor already unwraps `response.data`, so
// `response.data` here is that envelope; `response.data.data` is the payload.
//
// Write endpoints require an Idempotency-Key header. The key must be unique
// per logical operation and stable across retries of that same operation.
// Callers pass it explicitly; the page is responsible for generating one.

function buildQuery(filter?: CorporatePageFilter): string {
  if (!filter) return '';
  const params = new URLSearchParams();
  if (filter.status) params.set('status', filter.status);
  if (filter.branchId) params.set('branchId', filter.branchId);
  if (filter.limit != null) params.set('limit', String(filter.limit));
  if (filter.cursorSortValue) params.set('cursorSortValue', filter.cursorSortValue);
  if (filter.cursorLastId) params.set('cursorLastId', filter.cursorLastId);
  const qs = params.toString();
  return qs ? `?${qs}` : '';
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

export const marketplaceService = {
  async listOffers(filter?: CorporatePageFilter): Promise<CorporateOfferSummary[]> {
    const res = await apiClient.get<{ data: CorporateOfferSummary[] }>(
      `/marketplace/offers${buildQuery(filter)}`,
    );
    return res.data.data ?? [];
  },

  async getOffer(offerId: string): Promise<CorporateOfferDetail> {
    const res = await apiClient.get<{ data: CorporateOfferDetail }>(
      `/marketplace/offers/${encodeURIComponent(offerId)}`,
    );
    return res.data.data;
  },

  async getMetrics(): Promise<CorporateOfferMetrics> {
    const res = await apiClient.get<{ data: CorporateOfferMetrics }>(
      `/marketplace/metrics`,
    );
    return res.data.data;
  },

  async listCandidates(projectionId: string): Promise<CorporateCandidate[]> {
    const res = await apiClient.get<{ data: CorporateCandidate[] }>(
      `/marketplace/requests/${encodeURIComponent(projectionId)}/candidates`,
    );
    return res.data.data ?? [];
  },

  async listAssignments(filter?: CorporatePageFilter): Promise<Assignment[]> {
    const res = await apiClient.get<{ data: Assignment[] }>(
      `/marketplace/assignments${buildQuery(filter)}`,
    );
    return res.data.data ?? [];
  },

  async getAssignment(assignmentId: string): Promise<Assignment> {
    const res = await apiClient.get<{ data: Assignment }>(
      `/marketplace/assignments/${encodeURIComponent(assignmentId)}`,
    );
    return res.data.data;
  },

  async getAssignmentHistory(assignmentId: string): Promise<StateHistoryEntry[]> {
    const res = await apiClient.get<{ data: StateHistoryEntry[] }>(
      `/marketplace/assignments/${encodeURIComponent(assignmentId)}/history`,
    );
    return res.data.data ?? [];
  },

  // -------------------------------------------------------------------------
  // Writes
  //
  // Every write takes an idempotencyKey. The caller (the page) is
  // responsible for generating a fresh one when the user initiates the
  // action, and reusing the same one across retries of that same action.
  // -------------------------------------------------------------------------

  async acceptOffer(
    offerId: string,
    input: AcceptOfferInput,
    idempotencyKey: string,
  ): Promise<CorporateOfferSummary> {
    const res = await apiClient.post<{ data: CorporateOfferSummary }>(
      `/marketplace/offers/${encodeURIComponent(offerId)}/accept`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async declineOffer(
    offerId: string,
    input: DeclineOfferInput,
    idempotencyKey: string,
  ): Promise<void> {
    await apiClient.post(
      `/marketplace/offers/${encodeURIComponent(offerId)}/decline`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
  },

  async assignRequest(
    projectionId: string,
    input: AssignRequestInput,
    idempotencyKey: string,
  ): Promise<Assignment> {
    const res = await apiClient.post<{ data: Assignment }>(
      `/marketplace/requests/${encodeURIComponent(projectionId)}/assign`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async cancelAssignment(
    assignmentId: string,
    input: CancelAssignmentInput,
    idempotencyKey: string,
  ): Promise<void> {
    await apiClient.post(
      `/marketplace/assignments/${encodeURIComponent(assignmentId)}/cancel`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
  },

  async reassignAssignment(
    assignmentId: string,
    input: ReassignAssignmentInput,
    idempotencyKey: string,
  ): Promise<Assignment> {
    const res = await apiClient.post<{ data: Assignment }>(
      `/marketplace/assignments/${encodeURIComponent(assignmentId)}/reassign`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },
};

// newIdempotencyKey returns a unique key for one logical write. crypto.randomUUID
// is available in every modern browser; the fallback covers older Safari.
export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}