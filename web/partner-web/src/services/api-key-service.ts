import { apiClient } from "@/lib/api-client";
import type { APIKey, CreateAPIKeyInput, IssuedAPIKey, RevokeAPIKeyResult } from "@/types";

export const apiKeyService = {
  async list(token: string): Promise<APIKey[]> {
    const response = await apiClient.get<any>(
      `/api/v1/partner/v1/api-keys`,
      token
    );
    console.log('Raw API response:', response);
    
    // Handle different response structures
    if (Array.isArray(response)) {
      return response;
    }
    if (response && response.data && Array.isArray(response.data)) {
      return response.data;
    }
    if (response && response.keys && Array.isArray(response.keys)) {
      return response.keys;
    }
    return [];
  },

  create(token: string, input: CreateAPIKeyInput): Promise<IssuedAPIKey> {
    return apiClient.post<IssuedAPIKey>(
      `/api/v1/partner/v1/api-keys`,
      input,
      token
    );
  },

  revoke(token: string, id: string): Promise<RevokeAPIKeyResult> {
    return apiClient.delete<RevokeAPIKeyResult>(
      `/api/v1/partner/v1/api-keys/${encodeURIComponent(id)}`,
      token
    );
  },
};