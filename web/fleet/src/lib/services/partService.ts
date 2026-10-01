import apiClient from '@/lib/api/client';
import type {
  CreatePartInput,
  CreateStockMovementInput,
  Part,
  StockMovement,
  UpdatePartInput,
} from '@/lib/types/part';

export const partService = {
  async list(): Promise<Part[]> {
    const res = await apiClient.get<{ data: Part[] }>('/inventory/parts');
    return res.data.data ?? [];
  },

  async get(partId: string): Promise<Part> {
    const res = await apiClient.get<{ data: Part }>(
      `/inventory/parts/${encodeURIComponent(partId)}`,
    );
    return res.data.data;
  },

  async create(
    input: CreatePartInput,
    idempotencyKey: string,
  ): Promise<Part> {
    const res = await apiClient.post<{ data: Part }>(
      '/inventory/parts',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async update(partId: string, input: UpdatePartInput): Promise<Part> {
    const { version, ...body } = input;
    const res = await apiClient.patch<{ data: Part }>(
      `/inventory/parts/${encodeURIComponent(partId)}`,
      body,
      { headers: { 'If-Match': String(version) } },
    );
    return res.data.data;
  },

  async listMovements(): Promise<StockMovement[]> {
    const res = await apiClient.get<{ data: StockMovement[] }>(
      '/inventory/stock-movements',
    );
    return res.data.data ?? [];
  },

  async createMovement(
    input: CreateStockMovementInput,
    idempotencyKey: string,
  ): Promise<StockMovement> {
    const res = await apiClient.post<{ data: StockMovement }>(
      '/inventory/stock-movements',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },
};

export function newPartIdempotencyKey(): string {
  if (
    typeof crypto !== 'undefined' &&
    typeof crypto.randomUUID === 'function'
  ) {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}