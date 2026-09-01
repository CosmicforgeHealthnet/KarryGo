// lib/api/client.ts

import axios, { AxiosError, AxiosInstance, InternalAxiosRequestConfig } from 'axios';
import { useAuthStore } from '@/lib/stores/authStore';

const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL || '';

let refreshPromise: Promise<string> | null = null;

const apiClient: AxiosInstance = axios.create({
  baseURL: `${API_BASE_URL}/api`,
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

    if (error.response?.status === 401 && !originalRequest._retry) {
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

  const refreshToken = useAuthStore.getState().refreshToken;
  if (!refreshToken) {
    return null;
  }

  refreshPromise = apiClient
    .post<{ accessToken: string; refreshToken: string; accessExpiresAt: string; refreshExpiresAt: string; sessionId: string }>(
      '/auth/refresh',
      { refreshToken }
    )
    .then((response) => {
      const { accessToken, refreshToken: newRefreshToken, accessExpiresAt, refreshExpiresAt, sessionId } = response.data;
      useAuthStore.getState().setTokensOnly({
        accessToken,
        refreshToken: newRefreshToken,
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

export async function logoutUser(): Promise<void> {
  try {
    const token = useAuthStore.getState().accessToken;
    if (token) {
      await apiClient.post('/auth/logout');
    }
  } catch {
  } finally {
    useAuthStore.getState().logout();
    refreshPromise = null;
  }
}

export default apiClient;