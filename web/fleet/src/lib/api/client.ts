// lib/api/client.ts

import axios, { AxiosError, AxiosInstance, InternalAxiosRequestConfig } from 'axios';
import { useAuthStore } from '@/lib/stores/authStore';
import { isLifecycleRefusal } from '@/lib/auth/organizationDestination';
import {
  STEP_UP_HEADER,
  dropStepUpGrant,
  readStepUpGrant,
  stepUpRefusal,
  stepUpVerifyQuery,
  type StartedStepUp,
} from '@/lib/auth/stepUp';

// Announced when the backend refuses an action because the company is not
// yet approved (or needs corrections). The dashboard explains it to the user.
export const LIFECYCLE_REFUSAL_EVENT = 'fleet:organization-lifecycle-refusal';

const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL || '';
const AUTH_COOKIE_SCOPED_BASE = '/fleet-partner/v1/auth';
const REFRESH_COOKIE_SCOPED_PATH = `${AUTH_COOKIE_SCOPED_BASE}/refresh`;
const LOGOUT_COOKIE_SCOPED_PATH = `${AUTH_COOKIE_SCOPED_BASE}/logout`;

let refreshPromise: Promise<string> | null = null;

const apiClient: AxiosInstance = axios.create({
  baseURL: `${API_BASE_URL}/api`,
  withCredentials: true,
  headers: {
    'Content-Type': 'application/json',
  },
  timeout: 30000,
});

apiClient.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const token = useAuthStore.getState().accessToken;
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

apiClient.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const originalRequest = error.config as InternalAxiosRequestConfig & { _retry?: boolean };

    const isAuthEndpoint = /^\/auth\/(?:start|sign-up\/start|sign-in\/start|verify|resend|refresh|logout)$/.test(originalRequest.url ?? '');

    const hasAuthHeader = !!originalRequest.headers?.Authorization;

    if (
      error.response?.status === 401 &&
      !originalRequest._retry &&
      !isAuthEndpoint &&
      hasAuthHeader
    ) {
      originalRequest._retry = true;

      try {
        const newToken = await refreshAccessToken();
        if (newToken) {
          originalRequest.headers.Authorization = `Bearer ${newToken}`;
          return apiClient(originalRequest);
        }
      } catch {
        await logoutUser();
        window.location.href = '/sign-in';
        return Promise.reject(error);
      }
    }

    // A high-risk action needs a fresh step-up grant (see lib/auth/stepUp).
    // Retry once with a stored grant for the refused action class; otherwise
    // start a challenge and send the person to the verification page, which
    // stores the grant and returns here so the action can be tried again.
    const refusal = stepUpRefusal(error);
    if (refusal && typeof window !== 'undefined') {
      const storage = window.sessionStorage;
      const sent = originalRequest.headers?.[STEP_UP_HEADER];
      const stored = readStepUpGrant(storage, refusal.actionClass);
      if (stored && stored !== sent) {
        originalRequest.headers[STEP_UP_HEADER] = stored;
        return apiClient(originalRequest);
      }
      if (sent) {
        dropStepUpGrant(storage, refusal.actionClass);
      }
      try {
        const started = await apiClient.post<{ data: StartedStepUp }>('/auth/step-up/start', {
          actionClass: refusal.actionClass,
        });
        window.location.assign(`/verify?${stepUpVerifyQuery(refusal.actionClass, started.data.data)}`);
      } catch {
        // The challenge could not start; the original refusal is shown.
      }
      return Promise.reject(error);
    }

    if (isLifecycleRefusal(error) && typeof window !== 'undefined') {
      const body = (error.response?.data as { error?: { code?: string; message?: string } })?.error;
      window.dispatchEvent(
        new CustomEvent(LIFECYCLE_REFUSAL_EVENT, { detail: { code: body?.code, message: body?.message } }),
      );
    }

    return Promise.reject(error);
  }
);

export async function refreshAccessToken(): Promise<string | null> {
  if (refreshPromise) {
    return refreshPromise;
  }

  refreshPromise = axios
    .post(`${API_BASE_URL}${REFRESH_COOKIE_SCOPED_PATH}`, {}, { withCredentials: true })
    .then((response) => {
      const { accessToken, accessExpiresAt, refreshExpiresAt, sessionId } = response.data.data;
      useAuthStore.getState().setTokensOnly({
        accessToken,
        accessExpiresAt,
        refreshExpiresAt,
        sessionId,
      });
      return accessToken;
    })
    .catch((error) => {
      useAuthStore.getState().logout();
      throw error;
    })
    .finally(() => {
      refreshPromise = null;
    });

  return refreshPromise;
}

export async function logoutOnServer(): Promise<void> {
  await axios.post(`${API_BASE_URL}${LOGOUT_COOKIE_SCOPED_PATH}`, {}, { withCredentials: true });
}

export async function logoutUser(): Promise<void> {
  try {
    await logoutOnServer();
  } catch {
  } finally {
    useAuthStore.getState().logout();
    refreshPromise = null;
  }
}

export default apiClient;