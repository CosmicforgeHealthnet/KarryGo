// lib/stores/dashboardStore.ts

import { create } from 'zustand';
import {
  DashboardResponse,
  DashboardStats,
  DashboardAlert,
  DashboardTrip,
  TopVehicle,
  ActivityItem,
  SystemStatusItem,
  WeeklyDataPoint,
} from '@/lib/types/dashboard';

interface DashboardState {
  // Data
  stats: DashboardStats | null;
  alerts: DashboardAlert[];
  activeTrips: DashboardTrip[];
  topVehicles: TopVehicle[];
  activity: ActivityItem[];
  systemStatus: SystemStatusItem[];
  weeklyVolume: WeeklyDataPoint[];
  weeklyTrend: WeeklyDataPoint[];
  generatedAt: string | null;

  // UI State
  isLoading: boolean;
  error: string | null;
  isRefreshing: boolean;
  lastFetched: number | null;

  // Actions
  setDashboard: (data: DashboardResponse) => void;
  setLoading: (isLoading: boolean) => void;
  setRefreshing: (isRefreshing: boolean) => void;
  setError: (error: string | null) => void;
  clearDashboard: () => void;
  shouldRefetch: () => boolean;
}

const CACHE_TTL = 30000; // 30 seconds

const initialState = {
  stats: null,
  alerts: [],
  activeTrips: [],
  topVehicles: [],
  activity: [],
  systemStatus: [],
  weeklyVolume: [],
  weeklyTrend: [],
  generatedAt: null,
  isLoading: false,
  error: null,
  isRefreshing: false,
  lastFetched: null,
};

export const useDashboardStore = create<DashboardState>((set, get) => ({
  ...initialState,

  setDashboard: (data: DashboardResponse) => {
    set({
      stats: data.stats,
      alerts: data.alerts,
      activeTrips: data.activeTrips,
      topVehicles: data.topVehicles,
      activity: data.activity,
      systemStatus: data.systemStatus,
      weeklyVolume: data.weeklyVolume,
      weeklyTrend: data.weeklyTrend,
      generatedAt: data.generatedAt,
      lastFetched: Date.now(),
      isLoading: false,
      isRefreshing: false,
      error: null,
    });
  },

  setLoading: (isLoading: boolean) => set({ isLoading }),

  setRefreshing: (isRefreshing: boolean) => set({ isRefreshing }),

  setError: (error: string | null) =>
    set({ error, isLoading: false, isRefreshing: false }),

  clearDashboard: () => set(initialState),

  shouldRefetch: () => {
    const { lastFetched, isLoading } = get();
    if (isLoading) return false;
    if (!lastFetched) return true;
    return Date.now() - lastFetched > CACHE_TTL;
  },
}));