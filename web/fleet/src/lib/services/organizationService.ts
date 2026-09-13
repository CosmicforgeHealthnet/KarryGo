// lib/services/organizationService.ts

import apiClient from '@/lib/api/client';

export interface Organization {
  organizationId: string;
  legalName: string;
  tradingName: string;
  status: string;
  countryCode: string;
  timezone: string;
  defaultCurrency: string;
  version: number;
}

export interface CreateOrganizationRequest {
  legalName: string;
  tradingName: string;
  registrationNumber: string;
  countryCode: string;
  organizationType: string;
  timezone: string;
  defaultCurrency: string;
  ownerName: string;
}

export interface CreateOrganizationResponse {
  organizationId: string;
  status: string;
  version: number;
}

export interface VerificationStatusResponse {
  status: string;
  submissionVersion: number;
  issues: Array<{
    id: string;
    type: string;
    message: string;
    severity: string;
  }>;
}

export interface WalletProvisioningResponse {
  status: string;
  attempts: number;
}

export interface Document {
  id: string;
  documentType: string;
  fileId: string;
  status: string;
  uploadedAt: string;
}

export interface SubmitResponse {
  verificationRequestId: string;
  submissionVersion: number;
  status: string;
}

export const organizationService = {
  async createOrganization(data: CreateOrganizationRequest): Promise<CreateOrganizationResponse> {
    const response = await apiClient.post<CreateOrganizationResponse>('/organizations', data, {
      headers: {
        'Idempotency-Key': crypto.randomUUID(),
      },
    });
    return response.data;
  },

  async getCurrentOrganization(): Promise<Organization> {
    const response = await apiClient.get<Organization>('/organizations/current');
    return response.data;
  },

  async getOnboardingStatus(organizationId: string): Promise<any> {
    const response = await apiClient.get(`/organizations/${organizationId}/onboarding-status`);
    return response.data;
  },

  async getVerificationStatus(organizationId: string): Promise<VerificationStatusResponse> {
    const response = await apiClient.get<VerificationStatusResponse>(`/organizations/${organizationId}/verification`);
    return response.data;
  },

  async getWalletProvisioning(organizationId: string): Promise<WalletProvisioningResponse> {
    const response = await apiClient.get<WalletProvisioningResponse>(`/organizations/${organizationId}/wallet-provisioning`);
    return response.data;
  },

  async retryWalletProvisioning(organizationId: string): Promise<{ status: string }> {
    const response = await apiClient.post<{ status: string }>(`/organizations/${organizationId}/wallet-provisioning/retry`);
    return response.data;
  },

  async submitOrganization(organizationId: string): Promise<SubmitResponse> {
    const response = await apiClient.post<SubmitResponse>(`/organizations/${organizationId}/submit`, {}, {
      headers: {
        'Idempotency-Key': crypto.randomUUID(),
      },
    });
    return response.data;
  },

  async listDocuments(organizationId: string): Promise<Document[]> {
    const response = await apiClient.get<Document[]>(`/organizations/${organizationId}/documents`);
    return response.data;
  },

  async attachDocument(organizationId: string, documentType: string, fileId: string): Promise<{ documentId: string; status: string }> {
    const response = await apiClient.post<{ documentId: string; status: string }>(
      `/organizations/${organizationId}/documents`,
      { documentType, fileId }
    );
    return response.data;
  },
};