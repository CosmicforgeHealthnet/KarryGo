// lib/services/authService.ts

import apiClient from '@/lib/api/client';
import {
  StartOTPRequest,
  StartOTPResponse,
  VerifyOTPRequest,
  VerifyOTPResponse,
  ResendOTPRequest,
  ResendOTPResponse,
  RefreshTokenRequest,
  RefreshTokenResponse,
  SelectOrganizationRequest,
  SelectOrganizationResponse,
  LogoutResponse,
} from '@/lib/types/auth';

export const authService = {
  async startOTP(data: StartOTPRequest): Promise<StartOTPResponse> {
    const response = await apiClient.post<StartOTPResponse>('/auth/start', data);
    return response.data;
  },

  async verifyOTP(data: VerifyOTPRequest): Promise<VerifyOTPResponse> {
    const response = await apiClient.post<VerifyOTPResponse>('/auth/verify', data);
    return response.data;
  },

  async resendOTP(data: ResendOTPRequest): Promise<ResendOTPResponse> {
    const response = await apiClient.post<ResendOTPResponse>('/auth/resend', data);
    return response.data;
  },

  async refreshToken(data: RefreshTokenRequest): Promise<RefreshTokenResponse> {
    const response = await apiClient.post<RefreshTokenResponse>('/auth/refresh', data);
    return response.data;
  },

  async selectOrganization(data: SelectOrganizationRequest): Promise<SelectOrganizationResponse> {
    const response = await apiClient.post<SelectOrganizationResponse>('/auth/select-organization', data);
    return response.data;
  },

  async logout(): Promise<LogoutResponse> {
    const response = await apiClient.post<LogoutResponse>('/auth/logout');
    return response.data;
  },
};