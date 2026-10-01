import apiClient from '@/lib/api/client';
import type {
  CreateProviderInput,
  CreateProviderResult,
  MembershipHistoryEntry,
  ProviderMembership,
  ProvisioningStatus,
} from '@/lib/types/provider';

export const providerService = {
  async list(): Promise<ProviderMembership[]> {
    const res = await apiClient.get<{ data: ProviderMembership[] }>('/providers');
    return res.data.data ?? [];
  },

  async get(membershipId: string): Promise<ProviderMembership> {
    const res = await apiClient.get<{ data: ProviderMembership }>(
      `/providers/${encodeURIComponent(membershipId)}`,
    );
    return res.data.data;
  },

  async create(
    input: CreateProviderInput,
    idempotencyKey: string,
  ): Promise<CreateProviderResult> {
    const res = await apiClient.post<{ data: CreateProviderResult }>(
      '/providers',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async suspend(
    membershipId: string,
    expectedVersion: number,
    reasonCode = 'requested_by_organization',
  ): Promise<{ membershipId: string; status: 'SUSPENDED' }> {
    const res = await apiClient.post<{
      data: { membershipId: string; status: 'SUSPENDED' };
    }>(
      `/providers/${encodeURIComponent(membershipId)}/suspend`,
      { reasonCode },
      { headers: { 'If-Match': String(expectedVersion) } },
    );
    return res.data.data;
  },

  async provisioningStatus(membershipId: string): Promise<ProvisioningStatus> {
    const res = await apiClient.get<{ data: ProvisioningStatus }>(
      `/providers/${encodeURIComponent(membershipId)}/provisioning-status`,
    );
    return res.data.data;
  },

  async history(membershipId: string): Promise<MembershipHistoryEntry[]> {
    const res = await apiClient.get<{ data: MembershipHistoryEntry[] }>(
      `/providers/${encodeURIComponent(membershipId)}/history`,
    );
    return res.data.data ?? [];
  },
};

export function newProviderIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}