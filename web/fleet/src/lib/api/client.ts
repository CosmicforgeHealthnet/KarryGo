// lib/api/client.ts

import axios, { AxiosError, AxiosInstance, InternalAxiosRequestConfig } from 'axios';
import { useAuthStore } from '@/lib/stores/authStore';

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

    const isAuthEndpoint =
      originalRequest.url === '/auth/start' ||
      originalRequest.url === '/auth/verify' ||
      originalRequest.url === '/auth/logout';

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