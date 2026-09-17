// lib/types/auth.ts

export interface StartOTPRequest {
  email: string;
  phone: string;
}

export interface StartOTPResponse {
  challengeId: string;
  deliveryChannel: string;
  maskedDestination: string;
  expiresInSeconds: number;
  resendAfterSeconds: number;
}

export interface VerifyOTPRequest {
  challengeId: string;
  code: string;
  deviceLabel?: string;
}

export interface Tokens {
  accessToken: string;
  refreshToken: string;
  accessExpiresAt: string;
  refreshExpiresAt: string;
  sessionId: string;
}

export interface Organization {
  membershipId: string;
  organizationId: string;
  organizationName: string;
  status: string;
  isOwner: boolean;
}

export interface VerifyOTPResponse {
  data: {
    nextStep: 'ONBOARDING_REQUIRED' | 'ORGANIZATION_SELECTION_REQUIRED' | 'READY' | 'STEP_UP_GRANTED';
    identityId: string;
    tokens?: Tokens;
    organizations?: Organization[];
    stepUp?: {
      reference: string;
      expiresAt: string;
    };
  };
}

export interface ResendOTPRequest {
  challengeId: string;
}

export interface ResendOTPResponse {
  challengeId: string;
  deliveryChannel: string;
  maskedDestination: string;
  expiresInSeconds: number;
  resendAfterSeconds: number;
}

export interface SelectOrganizationRequest {
  membershipId: string;
}

export interface SelectOrganizationResponse {
  accessToken: string;
  refreshToken: string;
  accessExpiresAt: string;
  refreshExpiresAt: string;
  sessionId: string;
}

export interface LogoutResponse {
  loggedOut: boolean;
}

export interface AuthState {
  identityId: string | null;
  accessToken: string | null;
  refreshToken: string | null;
  sessionId: string | null;
  accessExpiresAt: string | null;
  refreshExpiresAt: string | null;
  organizations: Organization[];
  selectedOrganization: Organization | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: null;
}