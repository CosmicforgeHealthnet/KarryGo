// lib/types/dashboard.ts

export interface DashboardStats {
  // Hero stats
  activeWork: number;
  fleetReady: number;
  exceptions: number;
  totalProviders: number;

  // Map / Live stats
  onRoute: number;
  idle: number;
  inTransit: number;
  offline: number;

  // Performance stats
  totalTrips: number;
  completedTrips: number;
  onTimePerformance: number;
  incidents: number;

  // Additional operational stats
  activeDrivers: number;
  tripsToday: number;

  // KPI trends (percentage change vs previous period)
  fleetReadyTrend: number;
  tripsTodayTrend: number;
  exceptionsTrend: number;
  activeDriversTrend: number;

  // KPI sparklines (last 7 days of daily values)
  fleetReadySpark: number[];
  tripsTodaySpark: number[];
  exceptionsSpark: number[];
  activeDriversSpark: number[];
}

export type DashboardSeverity = 'CRITICAL' | 'HIGH' | 'MEDIUM' | 'LOW';

export interface DashboardAlert {
  id: string;
  alertType: string;
  severity: DashboardSeverity;
  title: string;
  detail: string;
  status: string;
  sourceId: string;
  routePlanId?: string;
  createdAt: string;
}

export interface DashboardTrip {
  id: string;
  routeReference: string;
  routeType: string;
  providerId: string;
  providerName?: string;
  vehicleOrTruckReference: string;
  executionService: string;
  status: string;
  eta?: string;
  route?: string;
  routeText?: string;
  branch?: string;
  latitude?: number;
  longitude?: number;
  speed?: number;
}

export interface WeeklyDataPoint {
  day: string;
  value: number;
}

export interface TopVehicle {
  rank: number;
  vehicle: string;
  trips: number;
  status: string;
}

export interface ActivityItem {
  id: string;
  type: string;
  title: string;
  createdAt: string;
}

export interface SystemStatusItem {
  label: string;
  status: string;
}

export interface DashboardResponse {
  stats: DashboardStats;
  alerts: DashboardAlert[];
  activeTrips: DashboardTrip[];
  topVehicles: TopVehicle[];
  activity: ActivityItem[];
  systemStatus: SystemStatusItem[];
  weeklyVolume: WeeklyDataPoint[];
  weeklyTrend: WeeklyDataPoint[];
  generatedAt: string;
}