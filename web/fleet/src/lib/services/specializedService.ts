// lib/services/specializedService.ts

import apiClient from '@/lib/api/client';

export interface Capability {
  code: string;
  name: string;
  description: string;
  status: 'ENABLED' | 'DISABLED';
  available: boolean;
  category: string;
}

export interface CapabilityResponse {
  capabilities: Capability[];
}

export interface SetCapabilityResponse {
  capability: Capability;
  updatedAt: string;
}

export const specializedService = {
  async listCapabilities(): Promise<Capability[]> {
    const response = await apiClient.get<{ data: Capability[] }>('/specialized/capabilities');
    return response.data.data;
  },

  async enableCapability(capabilityCode: string, reason?: string): Promise<SetCapabilityResponse> {
    const response = await apiClient.post<{ data: SetCapabilityResponse }>(
      `/specialized/capabilities/${capabilityCode}/enable`,
      { reason }
    );
    return response.data.data;
  },

  async disableCapability(capabilityCode: string, reason?: string): Promise<SetCapabilityResponse> {
    const response = await apiClient.post<{ data: SetCapabilityResponse }>(
      `/specialized/capabilities/${capabilityCode}/disable`,
      { reason }
    );
    return response.data.data;
  },
};