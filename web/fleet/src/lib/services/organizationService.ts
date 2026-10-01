// lib/services/organizationService.ts

import apiClient from '@/lib/api/client';
import type { Organization } from '@/lib/stores/organizationStore';

// NOTE: The Organization type is defined in organizationStore.ts and imported
// here. Do not redeclare it — that caused a status-type mismatch across the app.

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

export interface OnboardingStatusResponse {
  organizationId: string;
  organizationStatus: string;
  organizationVersion: number;
  stages: Array<{
    stage: string;
    status: string;
    completedAt: string | null;
    missingFields: string[];
    blockingIssues: string[];
  }>;
  allowedNextActions: string[];
  updatedAt: string;
  verification?: {
    status: string;
    submissionVersion: number;
    issues: unknown[];
  };
  walletProvisioning?: {
    status: string;
  };
}

export interface Representative {
  id: string;
  organizationId: string;
  fullName: string;
  roleTitle: string;
  email: string;
  phone: string;
  authorityType: string;
  authorityDocumentFileId?: string;
  identityReference?: string;
  isOnboardingOwner: boolean;
  status: string;
  effectiveFrom: string;
  effectiveTo?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateRepresentativeRequest {
  fullName: string;
  roleTitle: string;
  email: string;
  phone: string;
  authorityType: string;
  authorityDocumentFileId?: string;
  identityReference?: string;
  isOnboardingOwner?: boolean;
}

export interface UpdateRepresentativeRequest {
  fullName: string;
  roleTitle: string;
  email: string;
  phone: string;
  authorityType: string;
  authorityDocumentFileId?: string;
  status?: string;
  effectiveTo?: string;
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
    const response = await apiClient.get<{ data: Organization }>('/organizations/current');
    return response.data.data;
  },

  async updateOrganization(organizationId: string, data: Partial<CreateOrganizationRequest>): Promise<Organization> {
    const response = await apiClient.put<{ data: Organization }>(`/organizations/${organizationId}`, data);
    return response.data.data;
  },

  async markStageComplete(organizationId: string, stage: string): Promise<{ stage: string; status: string }> {
    const response = await apiClient.put<{ stage: string; status: string }>(
      `/organizations/${organizationId}/stages/${stage}`,
      { status: 'COMPLETE' }
    );
    return response.data;
  },

  // ---- Representatives ----

  async createRepresentative(organizationId: string, data: CreateRepresentativeRequest): Promise<Representative> {
    const response = await apiClient.post<{ data: Representative }>(
      `/organizations/${organizationId}/representatives`,
      data
    );
    return response.data.data;
  },

  async listRepresentatives(organizationId: string): Promise<Representative[]> {
    const response = await apiClient.get<{ data: Representative[] }>(
      `/organizations/${organizationId}/representatives`
    );
    return response.data.data;
  },

  async updateRepresentative(
    organizationId: string,
    representativeId: string,
    data: UpdateRepresentativeRequest
  ): Promise<Representative> {
    const response = await apiClient.put<{ data: Representative }>(
      `/organizations/${organizationId}/representatives/${representativeId}`,
      data
    );
    return response.data.data;
  },

  // ---- Onboarding Status ----

  async getOnboardingStatus(organizationId: string): Promise<OnboardingStatusResponse> {
    const response = await apiClient.get<{ data: OnboardingStatusResponse }>(
      `/organizations/${organizationId}/onboarding-status`
    );
    return response.data.data;
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

  // ---- Capabilities ----

  async getCapabilities(organizationId: string): Promise<Record<string, string>> {
    const response = await apiClient.get<{ data: Record<string, string> }>(
      `/organizations/${organizationId}/capabilities`
    );
    return response.data.data || {};
  },

  async updateCapabilities(organizationId: string, capabilities: string[]): Promise<{ capabilities: string[] }> {
    const response = await apiClient.put<{ data: { capabilities: string[] } }>(
      `/organizations/${organizationId}/capabilities`,
      { capabilities }
    );
    return response.data.data;
  },

  // ---- Profile (Headquarters) ----

  async getProfile(organizationId: string): Promise<any> {
    const response = await apiClient.get<{ data: any }>(
      `/organizations/${organizationId}/profile`
    );
    return response.data.data || null;
  },

  async updateProfile(organizationId: string, data: any): Promise<any> {
    const response = await apiClient.put<{ data: any }>(
      `/organizations/${organizationId}/profile`,
      data
    );
    return response.data.data;
  },

  async listDocuments(organizationId: string): Promise<Document[]> {
    const response = await apiClient.get<{ data: Document[] }>(`/organizations/${organizationId}/documents`);
    return response.data.data;
  },

  async attachDocument(organizationId: string, documentType: string, fileId: string): Promise<{ documentId: string; status: string }> {
    const response = await apiClient.post<{ documentId: string; status: string }>(
      `/organizations/${organizationId}/documents`,
      { documentType, fileId }
    );
    return response.data;
  },

  // ---- Document Upload (Private Media Contract) ----

  async requestUploadAuthorization(
    organizationId: string,
    data: {
      documentType: string;
      uploadPurpose?: string;
      allowedMimeTypes?: string[];
      maxSizeBytes?: number;
      idempotencyKey: string;
    }
  ): Promise<{
    authorizationId: string;
    mediaObjectId: string;
    uploadToken: string;
    uploadPath: string;
    expiresAt: string;
    maxSizeBytes: number;
    allowedMimeTypes: string[];
  }> {
    const response = await apiClient.post<{
      data: {
        authorizationId: string;
        mediaObjectId: string;
        uploadToken: string;
        uploadPath: string;
        expiresAt: string;
        maxSizeBytes: number;
        allowedMimeTypes: string[];
      };
    }>(
      `/organizations/${organizationId}/documents/upload-authorization`,
      {
        documentType: data.documentType,
        uploadPurpose: data.uploadPurpose,
        allowedMimeTypes: data.allowedMimeTypes,
        maxSizeBytes: data.maxSizeBytes,
      },
      {
        headers: {
          'Idempotency-Key': data.idempotencyKey,
        },
      }
    );
    return response.data.data;
  },

  async completeDocumentUpload(
    organizationId: string,
    data: {
      mediaObjectId: string;
      authorizationId: string;
      uploadToken: string;
      declaredMime: string;
      declaredSize: number;
      declaredChecksum: string;
      file: File;
    }
  ) {
    const formData = new FormData();
    formData.append('mediaObjectId', data.mediaObjectId);
    formData.append('authorizationId', data.authorizationId);
    formData.append('uploadToken', data.uploadToken);
    formData.append('declaredMime', data.declaredMime);
    formData.append('declaredSize', String(data.declaredSize));
    formData.append('declaredChecksum', data.declaredChecksum);
    formData.append('file', data.file);

    const response = await apiClient.post(
      `/organizations/${organizationId}/documents/upload-content`,
      formData,
      { headers: { 'Content-Type': undefined } }
    );
    return response.data.data;
  },
};