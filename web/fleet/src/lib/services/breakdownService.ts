import apiClient from '@/lib/api/client';
import type {
  Breakdown,
  CreateBreakdownInput,
  UpdateBreakdownInput,
} from '@/lib/types/breakdown';

export const breakdownService = {
  async list(): Promise<Breakdown[]> {
    const res = await apiClient.get<{ data: Breakdown[] }>('/breakdowns');
    return res.data.data ?? [];
  },

  async get(breakdownId: string): Promise<Breakdown> {
    const res = await apiClient.get<{ data: Breakdown }>(
      `/breakdowns/${encodeURIComponent(breakdownId)}`,
    );
    return res.data.data;
  },

  async create(
    input: CreateBreakdownInput,
    idempotencyKey: string,
  ): Promise<Breakdown> {
    const res = await apiClient.post<{ data: Breakdown }>(
      '/breakdowns',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async update(
    breakdownId: string,
    input: UpdateBreakdownInput,
  ): Promise<Breakdown> {
    const { expectedVersion, ...body } = input;
    const res = await apiClient.patch<{ data: Breakdown }>(
      `/breakdowns/${encodeURIComponent(breakdownId)}`,
      body,
      { headers: { 'If-Match': String(expectedVersion) } },
    );
    return res.data.data;
  },
};

export function newBreakdownIdempotencyKey(): string {
  if (
    typeof crypto !== 'undefined' &&
    typeof crypto.randomUUID === 'function'
  ) {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}