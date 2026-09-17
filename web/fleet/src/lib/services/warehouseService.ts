import apiClient from '@/lib/api/client';
import type {
  CreateDockAppointmentInput,
  CreateFulfilmentInput,
  CreateLocationInput,
  CreateReturnInput,
  CreateTerminalTransferInput,
  CreateWarehouseInput,
  CreateYardVisitInput,
  DockAppointment,
  FulfilmentOrder,
  InventoryItem,
  Movement,
  MovementInput,
  ReceiptInput,
  Return,
  StorageLocation,
  TerminalTransfer,
  UpdateLocationInput,
  UpdateWarehouseInput,
  Warehouse,
  YardVisit,
} from '@/lib/types/warehouse';

export const warehouseService = {
  // -------------------------------------------------------------------------
  // Warehouses
  // -------------------------------------------------------------------------

  async listWarehouses(): Promise<Warehouse[]> {
    const res = await apiClient.get<{ data: Warehouse[] }>('/warehouses');
    return res.data.data ?? [];
  },

  async getWarehouse(warehouseId: string): Promise<Warehouse> {
    const res = await apiClient.get<{ data: Warehouse }>(
      `/warehouses/${encodeURIComponent(warehouseId)}`,
    );
    return res.data.data;
  },

  async createWarehouse(
    input: CreateWarehouseInput,
    idempotencyKey: string,
  ): Promise<Warehouse> {
    const res = await apiClient.post<{ data: Warehouse }>(
      '/warehouses',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async updateWarehouse(
    warehouseId: string,
    input: UpdateWarehouseInput,
  ): Promise<Warehouse> {
    const { version, ...body } = input;
    const res = await apiClient.patch<{ data: Warehouse }>(
      `/warehouses/${encodeURIComponent(warehouseId)}`,
      body,
      { headers: { 'If-Match': String(version) } },
    );
    return res.data.data;
  },

  async archiveWarehouse(
    warehouseId: string,
    expectedVersion: number,
  ): Promise<Warehouse> {
    const res = await apiClient.post<{ data: Warehouse }>(
      `/warehouses/${encodeURIComponent(warehouseId)}/archive`,
      { expectedVersion },
      { headers: { 'Idempotency-Key': newWarehouseIdempotencyKey() } },
    );
    return res.data.data;
  },

  // -------------------------------------------------------------------------
  // Storage locations
  // -------------------------------------------------------------------------

  async listLocations(warehouseId: string): Promise<StorageLocation[]> {
    const res = await apiClient.get<{ data: StorageLocation[] }>(
      `/warehouses/${encodeURIComponent(warehouseId)}/locations`,
    );
    return res.data.data ?? [];
  },

  async getLocation(
    warehouseId: string,
    locationId: string,
  ): Promise<StorageLocation> {
    const res = await apiClient.get<{ data: StorageLocation }>(
      `/warehouses/${encodeURIComponent(warehouseId)}/locations/${encodeURIComponent(locationId)}`,
    );
    return res.data.data;
  },

  async createLocation(
    warehouseId: string,
    input: CreateLocationInput,
    idempotencyKey: string,
  ): Promise<StorageLocation> {
    const res = await apiClient.post<{ data: StorageLocation }>(
      `/warehouses/${encodeURIComponent(warehouseId)}/locations`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async updateLocation(
    warehouseId: string,
    locationId: string,
    input: UpdateLocationInput,
  ): Promise<StorageLocation> {
    const { version, ...body } = input;
    const res = await apiClient.patch<{ data: StorageLocation }>(
      `/warehouses/${encodeURIComponent(warehouseId)}/locations/${encodeURIComponent(locationId)}`,
      body,
      { headers: { 'If-Match': String(version) } },
    );
    return res.data.data;
  },

  // -------------------------------------------------------------------------
  // Inventory
  // -------------------------------------------------------------------------

  async listInventory(): Promise<InventoryItem[]> {
    const res = await apiClient.get<{ data: InventoryItem[] }>('/inventory');
    return res.data.data ?? [];
  },

  async getInventoryItem(itemId: string): Promise<InventoryItem> {
    const res = await apiClient.get<{ data: InventoryItem }>(
      `/inventory/${encodeURIComponent(itemId)}`,
    );
    return res.data.data;
  },

  async listItemMovements(itemId: string): Promise<Movement[]> {
    const res = await apiClient.get<{ data: Movement[] }>(
      `/inventory/${encodeURIComponent(itemId)}/movements`,
    );
    return res.data.data ?? [];
  },

  async receiveInventory(
    input: ReceiptInput,
    idempotencyKey: string,
  ): Promise<InventoryItem> {
    const res = await apiClient.post<{ data: InventoryItem }>(
      '/inventory/receipts',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async moveInventory(
    itemId: string,
    input: MovementInput,
    idempotencyKey: string,
  ): Promise<Movement> {
    const res = await apiClient.post<{ data: Movement }>(
      '/inventory/movements',
      { ...input, inventoryItemId: itemId },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(input.expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  // -------------------------------------------------------------------------
  // Fulfilment orders
  // -------------------------------------------------------------------------

  async listFulfilmentOrders(): Promise<FulfilmentOrder[]> {
    const res = await apiClient.get<{ data: FulfilmentOrder[] }>(
      '/fulfilment-orders',
    );
    return res.data.data ?? [];
  },

  async getFulfilmentOrder(orderId: string): Promise<FulfilmentOrder> {
    const res = await apiClient.get<{ data: FulfilmentOrder }>(
      `/fulfilment-orders/${encodeURIComponent(orderId)}`,
    );
    return res.data.data;
  },

  async createFulfilmentOrder(
    input: CreateFulfilmentInput,
    idempotencyKey: string,
  ): Promise<FulfilmentOrder> {
    const res = await apiClient.post<{ data: FulfilmentOrder }>(
      '/fulfilment-orders',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async releaseFulfilment(
    orderId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<FulfilmentOrder> {
    const res = await apiClient.post<{ data: FulfilmentOrder }>(
      `/fulfilment-orders/${encodeURIComponent(orderId)}/release`,
      { expectedVersion },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async pickFulfilment(
    orderId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<FulfilmentOrder> {
    const res = await apiClient.post<{ data: FulfilmentOrder }>(
      `/fulfilment-orders/${encodeURIComponent(orderId)}/pick`,
      { expectedVersion },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async packFulfilment(
    orderId: string,
    input: {
      expectedVersion: number;
      lines: { inventoryItemId: string; packageReference: string }[];
    },
    idempotencyKey: string,
  ): Promise<FulfilmentOrder> {
    const res = await apiClient.post<{ data: FulfilmentOrder }>(
      `/fulfilment-orders/${encodeURIComponent(orderId)}/pack`,
      input,
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(input.expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async dispatchFulfilment(
    orderId: string,
    input: {
      expectedVersion: number;
      shipmentReference?: string;
      manifestReference?: string;
    },
    idempotencyKey: string,
  ): Promise<FulfilmentOrder> {
    const res = await apiClient.post<{ data: FulfilmentOrder }>(
      `/fulfilment-orders/${encodeURIComponent(orderId)}/dispatch`,
      input,
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(input.expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async cancelFulfilment(
    orderId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<FulfilmentOrder> {
    const res = await apiClient.post<{ data: FulfilmentOrder }>(
      `/fulfilment-orders/${encodeURIComponent(orderId)}/cancel`,
      { expectedVersion },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  // -------------------------------------------------------------------------
  // Dock appointments
  // -------------------------------------------------------------------------

  async listDockAppointments(): Promise<DockAppointment[]> {
    const res = await apiClient.get<{ data: DockAppointment[] }>(
      '/dock-appointments',
    );
    return res.data.data ?? [];
  },

  async getDockAppointment(appointmentId: string): Promise<DockAppointment> {
    const res = await apiClient.get<{ data: DockAppointment }>(
      `/dock-appointments/${encodeURIComponent(appointmentId)}`,
    );
    return res.data.data;
  },

  async createDockAppointment(
    input: CreateDockAppointmentInput,
    idempotencyKey: string,
  ): Promise<DockAppointment> {
    const res = await apiClient.post<{ data: DockAppointment }>(
      '/dock-appointments',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  // -------------------------------------------------------------------------
  // Yard visits
  // -------------------------------------------------------------------------

  async listYardVisits(): Promise<YardVisit[]> {
    const res = await apiClient.get<{ data: YardVisit[] }>('/yard-visits');
    return res.data.data ?? [];
  },

  async getYardVisit(visitId: string): Promise<YardVisit> {
    const res = await apiClient.get<{ data: YardVisit }>(
      `/yard-visits/${encodeURIComponent(visitId)}`,
    );
    return res.data.data;
  },

  async createYardVisit(
    input: CreateYardVisitInput,
    idempotencyKey: string,
  ): Promise<YardVisit> {
    const res = await apiClient.post<{ data: YardVisit }>(
      '/yard-visits',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  // -------------------------------------------------------------------------
  // Terminal transfers
  // -------------------------------------------------------------------------

  async listTerminalTransfers(): Promise<TerminalTransfer[]> {
    const res = await apiClient.get<{ data: TerminalTransfer[] }>(
      '/terminal-transfers',
    );
    return res.data.data ?? [];
  },

  async getTerminalTransfer(transferId: string): Promise<TerminalTransfer> {
    const res = await apiClient.get<{ data: TerminalTransfer }>(
      `/terminal-transfers/${encodeURIComponent(transferId)}`,
    );
    return res.data.data;
  },

  async createTerminalTransfer(
    input: CreateTerminalTransferInput,
    idempotencyKey: string,
  ): Promise<TerminalTransfer> {
    const res = await apiClient.post<{ data: TerminalTransfer }>(
      '/terminal-transfers',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async departTransfer(
    transferId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<TerminalTransfer> {
    const res = await apiClient.post<{ data: TerminalTransfer }>(
      `/terminal-transfers/${encodeURIComponent(transferId)}/depart`,
      { expectedVersion },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async arriveTransfer(
    transferId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<TerminalTransfer> {
    const res = await apiClient.post<{ data: TerminalTransfer }>(
      `/terminal-transfers/${encodeURIComponent(transferId)}/arrive`,
      { expectedVersion },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async reconcileTransfer(
    transferId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<TerminalTransfer> {
    const res = await apiClient.post<{ data: TerminalTransfer }>(
      `/terminal-transfers/${encodeURIComponent(transferId)}/reconcile`,
      { expectedVersion },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  // -------------------------------------------------------------------------
  // Returns
  // -------------------------------------------------------------------------

  async listReturns(): Promise<Return[]> {
    const res = await apiClient.get<{ data: Return[] }>('/returns');
    return res.data.data ?? [];
  },

  async getReturn(returnId: string): Promise<Return> {
    const res = await apiClient.get<{ data: Return }>(
      `/returns/${encodeURIComponent(returnId)}`,
    );
    return res.data.data;
  },

  async createReturn(
    input: CreateReturnInput,
    idempotencyKey: string,
  ): Promise<Return> {
    const res = await apiClient.post<{ data: Return }>('/returns', input, {
      headers: { 'Idempotency-Key': idempotencyKey },
    });
    return res.data.data;
  },

  async receiveReturn(
    returnId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Return> {
    const res = await apiClient.post<{ data: Return }>(
      `/returns/${encodeURIComponent(returnId)}/receive`,
      { expectedVersion },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async decideReturn(
    returnId: string,
    input: { expectedVersion: number; disposition: string; reason: string },
    idempotencyKey: string,
  ): Promise<Return> {
    const res = await apiClient.post<{ data: Return }>(
      `/returns/${encodeURIComponent(returnId)}/decide`,
      input,
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(input.expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async completeReturn(
    returnId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Return> {
    const res = await apiClient.post<{ data: Return }>(
      `/returns/${encodeURIComponent(returnId)}/complete`,
      { expectedVersion },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },
};

export function newWarehouseIdempotencyKey(): string {
  if (
    typeof crypto !== 'undefined' &&
    typeof crypto.randomUUID === 'function'
  ) {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}