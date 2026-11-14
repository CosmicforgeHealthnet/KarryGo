// Asset history, operating costs, meter readings, defects and tyres
// (fleet-partner-service Section 18).

export interface AssetHistoryEntry {
  id: string;
  previousOperationalStatus: string;
  newOperationalStatus: string;
  previousLifecycleStatus: string;
  newLifecycleStatus: string;
  reason: string;
  actorId: string;
  version: number;
  occurredAt: string;
}

export const COST_TYPES = ['MAINTENANCE', 'FUEL', 'PARTS', 'TYRES', 'OTHER'] as const;

export interface CostEntry {
  id: string;
  assetId: string;
  costType: string;
  sourceType: string;
  sourceReference?: string;
  amountMinor: number;
  currency: string;
  incurredAt: string;
  description?: string;
  recordedBy: string;
  createdAt: string;
}

export interface CostHistory {
  assetId: string;
  entries: CostEntry[];
  totals: { costType: string; currency: string; amountMinor: number }[];
}

export interface CostInput {
  costType: string;
  amountMinor: number;
  currency: string;
  incurredAt: string;
  description?: string;
}

export const METER_TYPES = ['ODOMETER', 'ENGINE_HOURS'] as const;

export interface MeterReading {
  id: string;
  assetId: string;
  meterType: string;
  reading: number;
  previousReading: number;
  source: string;
  sourceReference?: string;
  recordedAt: string;
  anomaly: boolean;
  anomalyReasons: string[] | null;
  recordedBy: string;
  createdAt: string;
  dueScheduleIds?: string[];
}

export const DEFECT_SEVERITIES = ['LOW', 'MEDIUM', 'HIGH', 'CRITICAL'] as const;

export interface Defect {
  id: string;
  branchId: string;
  assetId: string;
  sourceType: string;
  sourceReference?: string;
  itemCode?: string;
  description: string;
  severity: string;
  critical: boolean;
  status: string;
  workOrderId?: string;
  deferredUntil?: string;
  deferralReason?: string;
  resolutionNotes?: string;
  resolvedBy?: string;
  resolvedAt?: string;
  reportedBy: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface DefectAction {
  reason?: string;
  notes?: string;
  deferredUntil?: string;
  workOrderId?: string;
}

export interface Tyre {
  id: string;
  branchId: string;
  serialNumber: string;
  brand?: string;
  size?: string;
  status: string;
  assetId?: string;
  position?: string;
  installedAt?: string;
  lastTreadDepthMm?: number;
  minTreadDepthMm: number;
  version: number;
  createdAt: string;
  updatedAt: string;
  lowTreadDefectId?: string;
}

export interface RegisterTyreInput {
  branchId?: string;
  serialNumber: string;
  brand?: string;
  size?: string;
  minTreadDepthMm?: number;
  treadDepthMm?: number;
}

export interface TyreAction {
  assetId?: string;
  position?: string;
  toPosition?: string;
  reason?: string;
  scrap?: boolean;
  treadDepthMm?: number;
  odometer?: number;
}
