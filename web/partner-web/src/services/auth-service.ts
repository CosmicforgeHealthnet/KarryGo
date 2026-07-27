import { apiClient } from "@/lib/api-client";
import type { AuthAccount, AuthSession } from "@/types";

export const authService = {
  startSignup(businessEmail: string): Promise<void> {
    return apiClient.post<void>("/api/v1/partner/v1/auth/signup/start", {
      business_email: businessEmail,
    });
  },

  resendSignup(businessEmail: string): Promise<void> {
    return apiClient.post<void>("/api/v1/partner/v1/auth/signup/resend", {
      business_email: businessEmail,
    });
  },

  verifySignup(businessEmail: string, otpCode: string): Promise<AuthSession> {
    return apiClient.post<AuthSession>("/api/v1/partner/v1/auth/signup/verify", {
      business_email: businessEmail,
      otp_code: otpCode,
    });
  },

  startLogin(businessEmail: string): Promise<void> {
    return apiClient.post<void>("/api/v1/partner/v1/auth/login/start", {
      business_email: businessEmail,
    });
  },

  verifyLogin(businessEmail: string, otpCode: string): Promise<AuthSession> {
    return apiClient.post<AuthSession>("/api/v1/partner/v1/auth/login/verify", {
      business_email: businessEmail,
      otp_code: otpCode,
    });
  },

  login(email: string, password: string): Promise<AuthSession> {
    return apiClient.post<AuthSession>("/api/v1/partner/v1/auth/login", {
      email,
      password,
    });
  },

  logout(token: string): Promise<void> {
    return apiClient.post<void>("/api/v1/partner/v1/auth/logout", undefined, token);
  },

  me(token: string): Promise<{ account: AuthAccount }> {
    return apiClient.get<{ account: AuthAccount }>("/api/v1/partner/v1/auth/me", token);
  },
};