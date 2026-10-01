// lib/services/routeService.ts

import apiClient from '@/lib/api/client';
import {
  CreateRouteInput,
  CreateStopInput,
  EditRouteInput,
  EditStopInput,
  RouteActionInput,
  RouteListFilter,
  RoutePlan,
  RouteStop,
} from '@/lib/types/route';

function buildQuery(filter?: RouteListFilter): string {
  if (!filter) return '';
  const params = new URLSearchParams();
  if (filter.branchId) params.set('branchId', filter.branchId);
  if (filter.status) params.set('status', filter.status);
  if (filter.service) params.set('service', filter.service);
  if (filter.providerId) params.set('providerId', filter.providerId);
  if (filter.vehicleOrTruckReference)
    params.set('vehicleOrTruckReference', filter.vehicleOrTruckReference);
  if (filter.limit != null) params.set('limit', String(filter.limit));
  if (filter.offset != null) params.set('offset', String(filter.offset));
  const qs = params.toString();
  return qs ? `?${qs}` : '';
}

export const routeService = {
  // ---- Routes ----

  async listRoutes(filter?: RouteListFilter): Promise<RoutePlan[]> {
    const res = await apiClient.get<{ data: RoutePlan[] }>(
      `/routes${buildQuery(filter)}`,
    );
    return res.data.data ?? [];
  },

  async getRoute(routeId: string): Promise<RoutePlan> {
    const res = await apiClient.get<{ data: RoutePlan }>(
      `/routes/${encodeURIComponent(routeId)}`,
    );
    return res.data.data;
  },

  async createRoute(
    input: CreateRouteInput,
    idempotencyKey: string,
  ): Promise<RoutePlan> {
    const res = await apiClient.post<{ data: RoutePlan }>(`/routes`, input, {
      headers: { 'Idempotency-Key': idempotencyKey },
    });
    return res.data.data;
  },

  async editRoute(
    routeId: string,
    input: EditRouteInput,
  ): Promise<RoutePlan> {
    const res = await apiClient.patch<{ data: RoutePlan }>(
      `/routes/${encodeURIComponent(routeId)}`,
      input,
    );
    return res.data.data;
  },

  async approveRoute(routeId: string, input: RouteActionInput): Promise<RoutePlan> {
    const res = await apiClient.post<{ data: RoutePlan }>(
      `/routes/${encodeURIComponent(routeId)}/approve`,
      input,
    );
    return res.data.data;
  },

  async publishRoute(
    routeId: string,
    input: RouteActionInput,
    idempotencyKey: string,
  ): Promise<RoutePlan> {
    const res = await apiClient.post<{ data: RoutePlan }>(
      `/routes/${encodeURIComponent(routeId)}/publish`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async optimizeRoute(routeId: string, input: RouteActionInput): Promise<unknown> {
    // The backend returns 202 with an OptimizationRequest envelope. The page
    // does not need to read the fields today.
    const res = await apiClient.post<{ data: unknown }>(
      `/routes/${encodeURIComponent(routeId)}/optimize`,
      input,
    );
    return res.data.data;
  },

  async replanRoute(
    routeId: string,
    input: RouteActionInput,
    idempotencyKey: string,
  ): Promise<RoutePlan> {
    const res = await apiClient.post<{ data: RoutePlan }>(
      `/routes/${encodeURIComponent(routeId)}/replan`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async cancelRoute(routeId: string, input: RouteActionInput): Promise<RoutePlan> {
    const res = await apiClient.post<{ data: RoutePlan }>(
      `/routes/${encodeURIComponent(routeId)}/cancel`,
      input,
    );
    return res.data.data;
  },

  // ---- Stops ----

  async listStops(routeId: string): Promise<RouteStop[]> {
    const res = await apiClient.get<{ data: RouteStop[] }>(
      `/routes/${encodeURIComponent(routeId)}/stops`,
    );
    return res.data.data ?? [];
  },

  async createStop(routeId: string, input: CreateStopInput): Promise<RouteStop> {
    const res = await apiClient.post<{ data: RouteStop }>(
      `/routes/${encodeURIComponent(routeId)}/stops`,
      input,
    );
    return res.data.data;
  },

  async editStop(
    routeId: string,
    stopId: string,
    input: EditStopInput,
  ): Promise<RouteStop> {
    const res = await apiClient.patch<{ data: RouteStop }>(
      `/routes/${encodeURIComponent(routeId)}/stops/${encodeURIComponent(stopId)}`,
      input,
    );
    return res.data.data;
  },

  async deleteStop(routeId: string, stopId: string): Promise<void> {
    await apiClient.delete(
      `/routes/${encodeURIComponent(routeId)}/stops/${encodeURIComponent(stopId)}`,
    );
  },
};

export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}