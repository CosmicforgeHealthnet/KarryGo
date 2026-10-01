import apiClient from '@/lib/api/client';
import type {
  AddDocumentInput,
  Asset,
  AssetDocument,
  AssetListFilter,
  Assignment,
  CreateAssetInput,
  CreateAssignmentInput,
  UpdateAssetInput,
} from '@/lib/types/asset';

function buildQuery(filter?: AssetListFilter): string {
  if (!filter) return '';
  const params = new URLSearchParams();
  if (filter.branchId) params.set('branchId', filter.branchId);
  if (filter.status) params.set('status', filter.status);
  if (filter.limit != null) params.set('limit', String(filter.limit));
  if (filter.offset != null) params.set('offset', String(filter.offset));
  const qs = params.toString();
  return qs ? `?${qs}` : '';
}

export const assetService = {
  async list(filter?: AssetListFilter): Promise<Asset[]> {
    const res = await apiClient.get<{ data: Asset[] }>(
      `/assets${buildQuery(filter)}`,
    );
    return res.data.data ?? [];
  },

  async get(assetId: string): Promise<Asset> {
    const res = await apiClient.get<{ data: Asset }>(
      `/assets/${encodeURIComponent(assetId)}`,
    );
    return res.data.data;
  },

  async create(
    input: CreateAssetInput,
    idempotencyKey: string,
  ): Promise<Asset> {
    const res = await apiClient.post<{ data: Asset }>('/assets', input, {
      headers: { 'Idempotency-Key': idempotencyKey },
    });
    return res.data.data;
  },

  async update(assetId: string, input: UpdateAssetInput): Promise<Asset> {
    const { version, ...body } = input;
    const res = await apiClient.patch<{ data: Asset }>(
      `/assets/${encodeURIComponent(assetId)}`,
      body,
      { headers: { 'If-Match': String(version) } },
    );
    return res.data.data;
  },

  async archive(
    assetId: string,
    expectedVersion: number,
    reason = 'requested_by_organization',
  ): Promise<Asset> {
    const res = await apiClient.post<{ data: Asset }>(
      `/assets/${encodeURIComponent(assetId)}/archive`,
      { expectedVersion, reason },
      { headers: { 'Idempotency-Key': newAssetIdempotencyKey() } },
    );
    return res.data.data;
  },

  async retire(
    assetId: string,
    expectedVersion: number,
    reason = 'requested_by_organization',
  ): Promise<Asset> {
    const res = await apiClient.post<{ data: Asset }>(
      `/assets/${encodeURIComponent(assetId)}/retire`,
      { expectedVersion, reason },
      { headers: { 'Idempotency-Key': newAssetIdempotencyKey() } },
    );
    return res.data.data;
  },

  async assign(
    assetId: string,
    input: CreateAssignmentInput,
    idempotencyKey: string,
  ): Promise<Assignment> {
    const res = await apiClient.post<{ data: Assignment }>(
      `/assets/${encodeURIComponent(assetId)}/assignments`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async listDocuments(assetId: string): Promise<AssetDocument[]> {
    const res = await apiClient.get<{ data: AssetDocument[] }>(
      `/assets/${encodeURIComponent(assetId)}/documents`,
    );
    return res.data.data ?? [];
  },

  async addDocument(
    assetId: string,
    input: AddDocumentInput,
    idempotencyKey: string,
  ): Promise<AssetDocument> {
    const res = await apiClient.post<{ data: AssetDocument }>(
      `/assets/${encodeURIComponent(assetId)}/documents`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },
};

export function newAssetIdempotencyKey(): string {
  if (
    typeof crypto !== 'undefined' &&
    typeof crypto.randomUUID === 'function'
  ) {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}