// lib/services/controlTowerService.ts

import apiClient from '@/lib/api/client';
import { Board, ControlTowerFilter } from '@/lib/types/controlTower';

function buildQuery(filter?: ControlTowerFilter): string {
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

export const controlTowerService = {
  async getBoard(filter?: ControlTowerFilter): Promise<Board> {
    const res = await apiClient.get<{ data: Board }>(
      `/control-tower${buildQuery(filter)}`,
    );
    return res.data.data;
  },
};