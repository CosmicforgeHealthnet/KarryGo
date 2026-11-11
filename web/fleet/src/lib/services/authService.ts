// lib/services/authService.ts

import apiClient, { logoutOnServer } from '@/lib/api/client';
import {
  StartOTPRequest,
  StartOTPResponse,
  VerifyOTPRequest,
  VerifyOTPResponse,
  ResendOTPRequest,
  ResendOTPResponse,
  SelectOrganizationRequest,
  SelectOrganizationResponse,
  LogoutResponse,
} from '@/lib/types/auth';

// The service wraps every success body as { data: ..., meta: { requestId } }.
interface Envelope<T> {
  data: T;
}

export const authService = {
  async startOTP(data: StartOTPRequest): Promise<StartOTPResponse> {
    const response = await apiClient.post<Envelope<StartOTPResponse>>('/auth/sign-up/start', data);
    return response.data.data;
  },

  async signIn(identifier: string): Promise<StartOTPResponse> {
    const response = await apiClient.post<Envelope<StartOTPResponse>>('/auth/sign-in/start', { identifier });
    return response.data.data;
  },

  async verifyOTP(data: VerifyOTPRequest): Promise<VerifyOTPResponse> {
    const response = await apiClient.post<VerifyOTPResponse>('/auth/verify', data);
    return response.data;
  },

  async resendOTP(data: ResendOTPRequest): Promise<ResendOTPResponse> {
    const response = await apiClient.post<Envelope<ResendOTPResponse>>('/auth/resend', data);
    return response.data.data;
  },

  async selectOrganization(data: SelectOrganizationRequest): Promise<SelectOrganizationResponse> {
    const response = await apiClient.post<Envelope<SelectOrganizationResponse>>('/auth/select-organization', data);
    return response.data.data;
  },

  async logout(): Promise<LogoutResponse> {
    await logoutOnServer();
    return { loggedOut: true };
  },
};