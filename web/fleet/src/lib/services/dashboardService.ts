// lib/services/dashboardService.ts

import apiClient from '@/lib/api/client';
import { DashboardResponse } from '@/lib/types/dashboard';

export const dashboardService = {
  /**
   * Fetch the complete dashboard data in one API call.
   */
  async getDashboard(): Promise<DashboardResponse> {
    const response = await apiClient.get<{ data: DashboardResponse }>('/dashboard');
    return response.data.data;
  },

  /**
   * Refresh dashboard data (bypasses cache).
   */
  async refreshDashboard(): Promise<DashboardResponse> {
    const response = await apiClient.get<{ data: DashboardResponse }>('/dashboard', {
      headers: {
        'Cache-Control': 'no-cache',
        'Pragma': 'no-cache',
      },
    });
    return response.data.data;
  },
};