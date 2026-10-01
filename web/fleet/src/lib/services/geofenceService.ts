// lib/services/geofenceService.ts

import apiClient from '@/lib/api/client';
import {
  CreateGeofenceInput,
  Geofence,
  GeofenceListFilter,
} from '@/lib/types/geofence';

function buildQuery(filter?: GeofenceListFilter): string {
  if (!filter) return '';
  const params = new URLSearchParams();
  if (filter.branchId) params.set('branchId', filter.branchId);
  if (filter.limit != null) params.set('limit', String(filter.limit));
  if (filter.offset != null) params.set('offset', String(filter.offset));
  const qs = params.toString();
  return qs ? `?${qs}` : '';
}

export const geofenceService = {
  async listGeofences(filter?: GeofenceListFilter): Promise<Geofence[]> {
    const res = await apiClient.get<{ data: Geofence[] }>(
      `/geofences${buildQuery(filter)}`,
    );
    return res.data.data ?? [];
  },

  async getGeofence(id: string): Promise<Geofence> {
    const res = await apiClient.get<{ data: Geofence }>(
      `/geofences/${encodeURIComponent(id)}`,
    );
    return res.data.data;
  },

  async createGeofence(
    input: CreateGeofenceInput,
    idempotencyKey: string,
  ): Promise<Geofence> {
    const res = await apiClient.post<{ data: Geofence }>(
      `/geofences`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async activateGeofence(id: string, expectedVersion: number): Promise<Geofence> {
    const res = await apiClient.post<{ data: Geofence }>(
      `/geofences/${encodeURIComponent(id)}/activate`,
      { expectedVersion },
    );
    return res.data.data;
  },

  async deactivateGeofence(id: string, expectedVersion: number): Promise<Geofence> {
    const res = await apiClient.post<{ data: Geofence }>(
      `/geofences/${encodeURIComponent(id)}/deactivate`,
      { expectedVersion },
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