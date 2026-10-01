import apiClient from '@/lib/api/client';
import type {
  CreateFuelRecordInput,
  FuelActionInput,
  FuelRecord,
} from '@/lib/types/fuel';

export const fuelService = {
  async list(): Promise<FuelRecord[]> {
    const res = await apiClient.get<{ data: FuelRecord[] }>('/fuel-records');
    return res.data.data ?? [];
  },

  async get(fuelRecordId: string): Promise<FuelRecord> {
    const res = await apiClient.get<{ data: FuelRecord }>(
      `/fuel-records/${encodeURIComponent(fuelRecordId)}`,
    );
    return res.data.data;
  },

  async create(
    input: CreateFuelRecordInput,
    idempotencyKey: string,
  ): Promise<FuelRecord> {
    const res = await apiClient.post<{ data: FuelRecord }>(
      '/fuel-records',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async approve(
    fuelRecordId: string,
    input: FuelActionInput,
    idempotencyKey: string,
  ): Promise<FuelRecord> {
    const { expectedVersion, reason } = input;
    const res = await apiClient.post<{ data: FuelRecord }>(
      `/fuel-records/${encodeURIComponent(fuelRecordId)}/approve`,
      { expectedVersion, reason },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async reject(
    fuelRecordId: string,
    input: FuelActionInput,
    idempotencyKey: string,
  ): Promise<FuelRecord> {
    const { expectedVersion, reason } = input;
    const res = await apiClient.post<{ data: FuelRecord }>(
      `/fuel-records/${encodeURIComponent(fuelRecordId)}/reject`,
      { expectedVersion, reason },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },
};

export function newFuelIdempotencyKey(): string {
  if (
    typeof crypto !== 'undefined' &&
    typeof crypto.randomUUID === 'function'
  ) {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}