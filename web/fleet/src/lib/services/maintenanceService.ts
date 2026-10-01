import apiClient from '@/lib/api/client';
import type {
  CreateMaintenanceScheduleInput,
  CreateWorkOrderInput,
  MaintenanceSchedule,
  UpdateMaintenanceScheduleInput,
  WorkActionInput,
  WorkOrder,
} from '@/lib/types/maintenance';

export const maintenanceService = {
  async listSchedules(): Promise<MaintenanceSchedule[]> {
    const res = await apiClient.get<{ data: MaintenanceSchedule[] }>(
      '/maintenance/schedules',
    );
    return res.data.data ?? [];
  },

  async getSchedule(scheduleId: string): Promise<MaintenanceSchedule> {
    const res = await apiClient.get<{ data: MaintenanceSchedule }>(
      `/maintenance/schedules/${encodeURIComponent(scheduleId)}`,
    );
    return res.data.data;
  },

  async createSchedule(
    input: CreateMaintenanceScheduleInput,
    idempotencyKey: string,
  ): Promise<MaintenanceSchedule> {
    const res = await apiClient.post<{ data: MaintenanceSchedule }>(
      '/maintenance/schedules',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async updateSchedule(
    scheduleId: string,
    input: UpdateMaintenanceScheduleInput,
  ): Promise<MaintenanceSchedule> {
    const { version, ...body } = input;
    const res = await apiClient.patch<{ data: MaintenanceSchedule }>(
      `/maintenance/schedules/${encodeURIComponent(scheduleId)}`,
      body,
      { headers: { 'If-Match': String(version) } },
    );
    return res.data.data;
  },

  async listWorkOrders(): Promise<WorkOrder[]> {
    const res = await apiClient.get<{ data: WorkOrder[] }>(
      '/maintenance/work-orders',
    );
    return res.data.data ?? [];
  },

  async getWorkOrder(workOrderId: string): Promise<WorkOrder> {
    const res = await apiClient.get<{ data: WorkOrder }>(
      `/maintenance/work-orders/${encodeURIComponent(workOrderId)}`,
    );
    return res.data.data;
  },

  async createWorkOrder(
    input: CreateWorkOrderInput,
    idempotencyKey: string,
  ): Promise<WorkOrder> {
    const res = await apiClient.post<{ data: WorkOrder }>(
      '/maintenance/work-orders',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async updateWorkOrder(
    workOrderId: string,
    input: WorkActionInput,
  ): Promise<WorkOrder> {
    const { expectedVersion, ...body } = input;
    const res = await apiClient.patch<{ data: WorkOrder }>(
      `/maintenance/work-orders/${encodeURIComponent(workOrderId)}`,
      body,
      { headers: { 'If-Match': String(expectedVersion) } },
    );
    return res.data.data;
  },

  async approveWorkOrder(
    workOrderId: string,
    input: WorkActionInput,
    idempotencyKey: string,
  ): Promise<WorkOrder> {
    const res = await apiClient.post<{ data: WorkOrder }>(
      `/maintenance/work-orders/${encodeURIComponent(workOrderId)}/approve`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async completeWorkOrder(
    workOrderId: string,
    input: WorkActionInput,
    idempotencyKey: string,
  ): Promise<WorkOrder> {
    const res = await apiClient.post<{ data: WorkOrder }>(
      `/maintenance/work-orders/${encodeURIComponent(workOrderId)}/complete`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async returnToService(
    workOrderId: string,
    input: WorkActionInput,
    idempotencyKey: string,
  ): Promise<WorkOrder> {
    const res = await apiClient.post<{ data: WorkOrder }>(
      `/maintenance/work-orders/${encodeURIComponent(workOrderId)}/return-to-service`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },
};

export function newMaintenanceIdempotencyKey(): string {
  if (
    typeof crypto !== 'undefined' &&
    typeof crypto.randomUUID === 'function'
  ) {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}