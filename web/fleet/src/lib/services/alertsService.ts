// lib/services/alertsService.ts

import apiClient from '@/lib/api/client';
import {
  AcknowledgeAlertInput,
  AlertListFilter,
  OperationalAlert,
  ResolveAlertInput,
} from '@/lib/types/alerts';

// Every corporate endpoint returns { data: T, meta: { requestId } }. The
// apiClient response interceptor already unwraps response.data, so
// res.data here is that envelope and res.data.data is the payload.
//
// Write endpoints (acknowledge, resolve) require an Idempotency-Key header
// and an expectedVersion — the backend uses both to guarantee that a retried
// call replays the original outcome rather than moving the alert twice.

function buildQuery(filter?: AlertListFilter): string {
  if (!filter) return '';
  const params = new URLSearchParams();
  if (filter.branchId) params.set('branchId', filter.branchId);
  if (filter.status) params.set('status', filter.status);
  if (filter.alertType) params.set('alertType', filter.alertType);
  if (filter.limit != null) params.set('limit', String(filter.limit));
  if (filter.offset != null) params.set('offset', String(filter.offset));
  const qs = params.toString();
  return qs ? `?${qs}` : '';
}

export const alertsService = {
  async listAlerts(filter?: AlertListFilter): Promise<OperationalAlert[]> {
    const res = await apiClient.get<{ data: OperationalAlert[] }>(
      `/alerts${buildQuery(filter)}`,
    );
    return res.data.data ?? [];
  },

  async getAlert(alertId: string): Promise<OperationalAlert> {
    const res = await apiClient.get<{ data: OperationalAlert }>(
      `/alerts/${encodeURIComponent(alertId)}`,
    );
    return res.data.data;
  },

  async acknowledgeAlert(
    alertId: string,
    input: AcknowledgeAlertInput,
    idempotencyKey: string,
  ): Promise<OperationalAlert> {
    const res = await apiClient.post<{ data: OperationalAlert }>(
      `/alerts/${encodeURIComponent(alertId)}/acknowledge`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async resolveAlert(
    alertId: string,
    input: ResolveAlertInput,
    idempotencyKey: string,
  ): Promise<OperationalAlert> {
    const res = await apiClient.post<{ data: OperationalAlert }>(
      `/alerts/${encodeURIComponent(alertId)}/resolve`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },
};

// newIdempotencyKey mints a key for one logical write action. crypto.randomUUID
// exists on every modern browser; the fallback covers older Safari.
export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}