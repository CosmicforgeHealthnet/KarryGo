export type TriggerType = 'DATE' | 'MILEAGE' | 'OPERATING_HOURS' | 'COMBINED';

export type WorkOrderStatus =
  | 'DRAFT'
  | 'PENDING_APPROVAL'
  | 'APPROVED'
  | 'IN_PROGRESS'
  | 'COMPLETED'
  | 'CANCELLED';

export type WorkOrderPriority = 'CRITICAL' | 'HIGH' | 'MEDIUM' | 'LOW';

export type WorkOrderSourceType = 'INSPECTION' | 'BREAKDOWN' | 'MAINTENANCE_SCHEDULE' | 'MANUAL';

export type ReturnToServiceStatus = 'PENDING' | 'APPROVED' | 'DENIED';

export interface MaintenanceSchedule {
  id: string;
  organizationId: string;
  assetId: string;
  triggerType: string;
  intervalValue: number | null;
  nextDueDate: string | null;
  nextDueOdometer: number | null;
  nextDueHours: number | null;
  critical: boolean;
  status: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateMaintenanceScheduleInput {
  assetId: string;
  triggerType: string;
  intervalValue?: number | null;
  nextDueDate?: string | null;
  nextDueOdometer?: number | null;
  nextDueHours?: number | null;
  critical?: boolean;
}

export interface UpdateMaintenanceScheduleInput {
  nextDueDate?: string | null;
  nextDueOdometer?: number | null;
  nextDueHours?: number | null;
  status?: string;
  version: number;
}

export interface WorkOrder {
  id: string;
  organizationId: string;
  assetId: string;
  sourceType: string;
  sourceReference: string;
  priority: string;
  diagnosis: string;
  requiredWork: string;
  status: string;
  approvedBy: string;
  approvedAt: string | null;
  startedAt: string | null;
  completedAt: string | null;
  downtimeStart: string | null;
  downtimeEnd: string | null;
  evidenceMediaIds: string[];
  returnToServiceStatus: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateWorkOrderInput {
  assetId: string;
  sourceType?: string;
  sourceReference?: string;
  priority?: string;
  diagnosis?: string;
  requiredWork?: string;
}

export interface WorkActionInput {
  expectedVersion: number;
  diagnosis?: string;
  requiredWork?: string;
  reason?: string;
  status?: string;
  evidenceMediaIds?: string[];
}