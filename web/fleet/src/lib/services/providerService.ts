import apiClient from '@/lib/api/client';
import type {
  CreateProviderInput,
  CreateProviderResult,
  MembershipHistoryEntry,
  ProviderCapability,
  ProviderDecline,
  ProviderMembership,
  ProviderPolicy,
  ProviderPolicyInput,
  ProvisioningStatus,
  UpdateProviderInput,
} from '@/lib/types/provider';
import type {
  ProviderAvailability,
  ProviderBranchLink,
  ProviderResendLoginResult,
  ProviderServiceRoles,
  ProviderVehicle,
} from '@/lib/types/providerAdmin';

const mid = (membershipId: string) => encodeURIComponent(membershipId);

export const providerService = {
  async list(): Promise<ProviderMembership[]> {
    const res = await apiClient.get<{ data: ProviderMembership[] }>('/providers');
    return res.data.data ?? [];
  },

  // One page of providers; pass the previous page's nextCursor to continue.
  async listPage(
    cursor?: string,
    limit = 50,
  ): Promise<{ items: ProviderMembership[]; nextCursor?: string }> {
    const params = new URLSearchParams({ limit: String(limit) });
    if (cursor) params.set('cursor', cursor);
    const res = await apiClient.get<{ data: ProviderMembership[]; meta?: { nextCursor?: string } }>(
      `/providers?${params.toString()}`,
    );
    return { items: res.data.data ?? [], nextCursor: res.data.meta?.nextCursor };
  },

  // Re-sends the invitation or first-login notice; the key keeps a retry
  // of the same attempt from sending twice.
  async resendLogin(membershipId: string, idempotencyKey: string): Promise<ProviderResendLoginResult> {
    const res = await apiClient.post<{ data: ProviderResendLoginResult }>(
      `/providers/${mid(membershipId)}/resend-login`,
      {},
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async availability(membershipId: string): Promise<ProviderAvailability> {
    const res = await apiClient.get<{ data: ProviderAvailability }>(`/providers/${mid(membershipId)}/availability`);
    return res.data.data;
  },

  async branches(membershipId: string): Promise<ProviderBranchLink[]> {
    const res = await apiClient.get<{ data: ProviderBranchLink[] }>(`/providers/${mid(membershipId)}/branches`);
    return res.data.data ?? [];
  },

  async vehicles(membershipId: string, includeHistory = false): Promise<ProviderVehicle[]> {
    const res = await apiClient.get<{ data: ProviderVehicle[] }>(
      `/providers/${mid(membershipId)}/vehicles${includeHistory ? '?includeHistory=true' : ''}`,
    );
    return res.data.data ?? [];
  },

  async serviceRoles(membershipId: string): Promise<ProviderServiceRoles> {
    const res = await apiClient.get<{ data: ProviderServiceRoles }>(`/providers/${mid(membershipId)}/service-roles`);
    return res.data.data;
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

  async update(
    membershipId: string,
    input: UpdateProviderInput,
    expectedVersion: number,
  ): Promise<ProviderMembership> {
    const res = await apiClient.patch<{ data: ProviderMembership }>(
      `/providers/${mid(membershipId)}`,
      input,
      { headers: { 'If-Match': String(expectedVersion) } },
    );
    return res.data.data;
  },

  async reactivate(membershipId: string, expectedVersion: number): Promise<void> {
    await apiClient.post(`/providers/${mid(membershipId)}/reactivate`, {}, {
      headers: { 'If-Match': String(expectedVersion) },
    });
  },

  async remove(
    membershipId: string,
    expectedVersion: number,
    reasonCode: string,
  ): Promise<{ membershipId: string; status: string }> {
    const res = await apiClient.post<{ data: { membershipId: string; status: string } }>(
      `/providers/${mid(membershipId)}/remove`,
      { reasonCode },
      { headers: { 'If-Match': String(expectedVersion) } },
    );
    return res.data.data;
  },

  async capabilities(membershipId: string): Promise<ProviderCapability[]> {
    const res = await apiClient.get<{ data: ProviderCapability[] }>(
      `/providers/${mid(membershipId)}/capabilities`,
    );
    return res.data.data ?? [];
  },

  async replaceCapabilities(
    membershipId: string,
    capabilities: ProviderCapability[],
  ): Promise<ProviderCapability[]> {
    const res = await apiClient.put<{ data: { capabilities: ProviderCapability[] } }>(
      `/providers/${mid(membershipId)}/capabilities`,
      {
        capabilities: capabilities.map((c) => ({
          serviceType: c.serviceType,
          vehicleClasses: c.vehicleClasses,
          operatingZones: c.operatingZones,
          qualifications: c.qualifications,
          validUntil: c.validUntil,
        })),
      },
    );
    return res.data.data?.capabilities ?? [];
  },

  async policy(membershipId: string): Promise<ProviderPolicy> {
    const res = await apiClient.get<{ data: ProviderPolicy }>(
      `/providers/${mid(membershipId)}/policies`,
    );
    return res.data.data;
  },

  // providerDeclineRequired is never sent: the platform fixes it to true.
  async replacePolicy(membershipId: string, input: ProviderPolicyInput): Promise<ProviderPolicy> {
    const res = await apiClient.put<{ data: ProviderPolicy }>(
      `/providers/${mid(membershipId)}/policies`,
      input,
    );
    return res.data.data;
  },

  async declines(membershipId: string): Promise<ProviderDecline[]> {
    const res = await apiClient.get<{ data: ProviderDecline[] }>(
      `/providers/${mid(membershipId)}/declines`,
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