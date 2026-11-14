// lib/services/specializedService.ts

import apiClient from '@/lib/api/client';
import { commandHeaders, enc } from '@/lib/api/headers';

// The specialized logistics capability codes the backend recognizes
// (specialized/domain.go). A code only works when its module is enabled for
// the deployment; otherwise the routes answer not-found.
export const SPECIALIZED_CAPABILITY_CODES = [
  'COURIER_LAST_MILE',
  'TAXI_PASSENGER',
  'TRUCKING_HAULAGE',
  'HEAVY_SPECIALIZED',
  'FREIGHT_FORWARDING',
  'COLD_CHAIN',
  'MEDICAL_SENSITIVE',
  'AGRICULTURAL_FOOD',
  'MOVING_RELOCATION',
  'TOWING_RECOVERY',
] as const;

// An organization's capability row (GET /specialized/capabilities),
// serialized with Go field names. Only capabilities the organization has
// configured at least once appear in the list.
export interface Capability {
  ID: string;
  OrganizationID: string;
  CapabilityCode: string;
  Status: 'ENABLED' | 'DISABLED' | string;
  Configuration: Record<string, unknown> | null;
  EnabledAt: string | null;
  DisabledAt: string | null;
  Version: number;
  CreatedAt: string;
  UpdatedAt: string;
}

export const specializedService = {
  async listCapabilities(): Promise<Capability[]> {
    const response = await apiClient.get<{ data: Capability[] | null }>('/specialized/capabilities');
    return response.data.data ?? [];
  },

  // Enabling or disabling needs a reason and an Idempotency-Key. The first
  // change of a capability carries no version; later ones carry the version
  // the user saw.
  async setCapability(
    capabilityCode: string,
    enable: boolean,
    reason: string,
    expectedVersion: number | undefined,
    idempotencyKey: string,
  ): Promise<Capability> {
    const response = await apiClient.post<{ data: Capability }>(
      `/specialized/capabilities/${enc(capabilityCode)}/${enable ? 'enable' : 'disable'}`,
      { reason, expectedVersion: expectedVersion ?? 0 },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return response.data.data;
  },
};
