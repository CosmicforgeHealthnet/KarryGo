import apiClient from '@/lib/api/client';
import { commandHeaders, enc, query } from '@/lib/api/headers';
import type {
  CapabilitySchema,
  CorporateTripDetail,
  CorporateTripPage,
  CustodyEvent,
  CustodyEventInput,
  GeofenceEvent,
  Lead,
  LeadConversion,
  LeadConvertInput,
  LeadHistoryEntry,
  LeadInput,
  LeadPatch,
  Manifest,
  ManifestDetail,
  ManifestInput,
  ManifestItem,
  ManifestPatch,
  OpenRescueInput,
  Order,
  OrderHistoryEntry,
  OrderInput,
  OrderImport,
  OrderImportDetail,
  OrderPatch,
  OrderStop,
  PackageView,
  Page,
  PrivateAssignment,
  RecurringOrderInput,
  RecurringOrderTemplate,
  RescueCase,
  RescueReplacementInput,
  ServiceCatalogInput,
  ServiceCatalogItem,
  ServiceCatalogPatch,
  Shipment,
  SpecializedRecord,
} from '@/lib/types/operations';

type ListEnvelope<T> = { data: T[] | null; meta?: { nextCursor?: string } };
const page = <T,>(body: ListEnvelope<T>): Page<T> => ({ items: body.data ?? [], nextCursor: body.meta?.nextCursor });

// CRM leads (Section 14). Create, assign, status and convert are idempotent
// commands; edits and every lifecycle command carry the version the user saw.
export const leadService = {
  async list(params: { status?: string; source?: string; cursor?: string; limit?: number } = {}): Promise<Page<Lead>> {
    const res = await apiClient.get<ListEnvelope<Lead>>(`/leads${query(params)}`);
    return page(res.data);
  },
  async get(leadId: string): Promise<Lead> {
    return (await apiClient.get<{ data: Lead }>(`/leads/${enc(leadId)}`)).data.data;
  },
  async create(input: LeadInput, idempotencyKey: string): Promise<Lead> {
    return (await apiClient.post<{ data: Lead }>('/leads', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async update(leadId: string, patch: LeadPatch, expectedVersion: number): Promise<Lead> {
    return (await apiClient.patch<{ data: Lead }>(`/leads/${enc(leadId)}`, patch, commandHeaders({ expectedVersion }))).data.data;
  },
  async assign(leadId: string, ownerUserId: string, expectedVersion: number, idempotencyKey: string): Promise<Lead> {
    const res = await apiClient.post<{ data: Lead }>(
      `/leads/${enc(leadId)}/assign`,
      { ownerUserId },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async changeStatus(leadId: string, status: string, reasonCode: string, expectedVersion: number, idempotencyKey: string): Promise<Lead> {
    const res = await apiClient.post<{ data: Lead }>(
      `/leads/${enc(leadId)}/status`,
      { status, reasonCode },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async convert(leadId: string, input: LeadConvertInput, expectedVersion: number, idempotencyKey: string): Promise<LeadConversion> {
    const res = await apiClient.post<{ data: LeadConversion }>(
      `/leads/${enc(leadId)}/convert`,
      input,
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async history(leadId: string): Promise<LeadHistoryEntry[]> {
    return (await apiClient.get<{ data: LeadHistoryEntry[] }>(`/leads/${enc(leadId)}/history`)).data.data ?? [];
  },
};

// Private orders, shipments, packages and chain of custody (Section 15).
export const privateOpsService = {
  async listOrders(params: { status?: string; customerId?: string; cursor?: string; limit?: number } = {}): Promise<Page<Order>> {
    return page((await apiClient.get<ListEnvelope<Order>>(`/orders${query(params)}`)).data);
  },
  async getOrder(orderId: string): Promise<Order> {
    return (await apiClient.get<{ data: Order }>(`/orders/${enc(orderId)}`)).data.data;
  },
  async createOrder(input: OrderInput, idempotencyKey: string): Promise<Order> {
    return (await apiClient.post<{ data: Order }>('/orders', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async updateOrder(orderId: string, patch: OrderPatch, expectedVersion: number): Promise<Order> {
    return (await apiClient.patch<{ data: Order }>(`/orders/${enc(orderId)}`, patch, commandHeaders({ expectedVersion }))).data.data;
  },
  async orderCommand(
    orderId: string,
    command: 'submit' | 'approve' | 'reject',
    expectedVersion: number,
    idempotencyKey: string,
    reasonCode?: string,
  ): Promise<Order> {
    const body = command === 'reject' ? { reasonCode } : {};
    const res = await apiClient.post<{ data: Order }>(
      `/orders/${enc(orderId)}/${command}`,
      body,
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async orderHistory(orderId: string): Promise<OrderHistoryEntry[]> {
    return (await apiClient.get<{ data: OrderHistoryEntry[] }>(`/orders/${enc(orderId)}/history`)).data.data ?? [];
  },
  async orderStops(orderId: string): Promise<OrderStop[]> {
    return (await apiClient.get<{ data: OrderStop[] }>(`/orders/${enc(orderId)}/stops`)).data.data ?? [];
  },
  async listServiceCatalog(): Promise<ServiceCatalogItem[]> {
    return (await apiClient.get<{ data: ServiceCatalogItem[] }>('/service-catalog')).data.data ?? [];
  },

  async listShipments(params: { status?: string; orderId?: string; cursor?: string; limit?: number } = {}): Promise<Page<Shipment>> {
    return page((await apiClient.get<ListEnvelope<Shipment>>(`/shipments${query(params)}`)).data);
  },
  async getShipment(shipmentId: string): Promise<Shipment> {
    return (await apiClient.get<{ data: Shipment }>(`/shipments/${enc(shipmentId)}`)).data.data;
  },
  async shipmentCommand(
    shipmentId: string,
    command: 'ready' | 'cancel',
    expectedVersion: number,
    idempotencyKey: string,
    reasonCode?: string,
  ): Promise<Shipment> {
    const res = await apiClient.post<{ data: Shipment }>(
      `/shipments/${enc(shipmentId)}/${command}`,
      { reasonCode: reasonCode ?? '' },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async shipmentPackages(shipmentId: string): Promise<PackageView[]> {
    return (await apiClient.get<{ data: PackageView[] }>(`/shipments/${enc(shipmentId)}/packages`)).data.data ?? [];
  },
  async getPackage(packageId: string): Promise<PackageView> {
    return (await apiClient.get<{ data: PackageView }>(`/packages/${enc(packageId)}`)).data.data;
  },

  async listCustodyEvents(subjectType: string, subjectReference: string): Promise<CustodyEvent[]> {
    const res = await apiClient.get<{ data: CustodyEvent[] }>(`/custody-events${query({ subjectType, subjectReference })}`);
    return res.data.data ?? [];
  },
  async getCustodyEvent(eventId: string): Promise<CustodyEvent> {
    return (await apiClient.get<{ data: CustodyEvent }>(`/custody-events/${enc(eventId)}`)).data.data;
  },
  async recordCustodyEvent(input: CustodyEventInput, idempotencyKey: string): Promise<CustodyEvent> {
    return (await apiClient.post<{ data: CustodyEvent }>('/custody-events', input, commandHeaders({ idempotencyKey }))).data.data;
  },

  async capabilitySchema(capabilityCode: string): Promise<CapabilitySchema> {
    return (await apiClient.get<{ data: CapabilitySchema }>(`/capability-schemas/${enc(capabilityCode)}`)).data.data;
  },
  async listSpecializedData(orderId: string): Promise<SpecializedRecord[]> {
    return (await apiClient.get<{ data: SpecializedRecord[] }>(`/orders/${enc(orderId)}/specialized-data`)).data.data ?? [];
  },
  async createSpecializedData(
    orderId: string,
    input: { capabilityCode: string; recordType: string; schemaVersion: number; payload: Record<string, unknown> },
    idempotencyKey: string,
  ): Promise<SpecializedRecord> {
    const res = await apiClient.post<{ data: SpecializedRecord }>(
      `/orders/${enc(orderId)}/specialized-data`,
      input,
      commandHeaders({ idempotencyKey }),
    );
    return res.data.data;
  },
};

// Rescue cases and geofence events (Section 16).
export const rescueService = {
  async list(params: { status?: string; triggerType?: string; branchId?: string } = {}): Promise<RescueCase[]> {
    return (await apiClient.get<{ data: RescueCase[] }>(`/rescue-cases${query({ ...params, limit: 100 })}`)).data.data ?? [];
  },
  async get(rescueCaseId: string): Promise<RescueCase> {
    return (await apiClient.get<{ data: RescueCase }>(`/rescue-cases/${enc(rescueCaseId)}`)).data.data;
  },
  async open(input: OpenRescueInput, idempotencyKey: string): Promise<RescueCase> {
    return (await apiClient.post<{ data: RescueCase }>('/rescue-cases', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async assignReplacement(id: string, input: RescueReplacementInput, expectedVersion: number, idempotencyKey: string): Promise<RescueCase> {
    const res = await apiClient.post<{ data: RescueCase }>(
      `/rescue-cases/${enc(id)}/replacement`,
      input,
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async resolve(id: string, resolutionCode: string, notes: string, expectedVersion: number, idempotencyKey: string): Promise<RescueCase> {
    const res = await apiClient.post<{ data: RescueCase }>(
      `/rescue-cases/${enc(id)}/resolve`,
      { resolutionCode, notes },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async cancel(id: string, reason: string, expectedVersion: number, idempotencyKey: string): Promise<RescueCase> {
    const res = await apiClient.post<{ data: RescueCase }>(
      `/rescue-cases/${enc(id)}/cancel`,
      { reason },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async geofenceEvents(params: { geofenceId?: string; canonicalTripReference?: string; limit?: number } = {}): Promise<GeofenceEvent[]> {
    return (await apiClient.get<{ data: GeofenceEvent[] }>(`/geofence-events${query(params)}`)).data.data ?? [];
  },
};

// Marketplace trip lifecycle projections (status, history, assignment and
// canonical references). Live position stays on /tracking.
export const tripService = {
  async list(params: { status?: string; sourceService?: string; limit?: number; offset?: number } = {}): Promise<CorporateTripPage> {
    const res = await apiClient.get<{ data: CorporateTripPage }>(`/marketplace/trips${query(params)}`);
    return res.data.data ?? { items: [], hasMore: false };
  },
  async get(tripId: string): Promise<CorporateTripDetail> {
    return (await apiClient.get<{ data: CorporateTripDetail }>(`/marketplace/trips/${enc(tripId)}`)).data.data;
  },
};

// Service catalogue (Section 15.1). Create and lifecycle commands are
// idempotent; edits and lifecycle commands carry the version the user saw.
export const serviceCatalogService = {
  async list(params: { status?: string; category?: string; cursor?: string; limit?: number } = {}): Promise<Page<ServiceCatalogItem>> {
    return page((await apiClient.get<ListEnvelope<ServiceCatalogItem>>(`/service-catalog${query(params)}`)).data);
  },
  async get(serviceId: string): Promise<ServiceCatalogItem> {
    return (await apiClient.get<{ data: ServiceCatalogItem }>(`/service-catalog/${enc(serviceId)}`)).data.data;
  },
  async create(input: ServiceCatalogInput, idempotencyKey: string): Promise<ServiceCatalogItem> {
    return (await apiClient.post<{ data: ServiceCatalogItem }>('/service-catalog', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async update(serviceId: string, patch: ServiceCatalogPatch, expectedVersion: number): Promise<ServiceCatalogItem> {
    const res = await apiClient.patch<{ data: ServiceCatalogItem }>(`/service-catalog/${enc(serviceId)}`, patch, commandHeaders({ expectedVersion }));
    return res.data.data;
  },
  async command(
    serviceId: string,
    command: 'activate' | 'deactivate' | 'archive',
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<ServiceCatalogItem> {
    const res = await apiClient.post<{ data: ServiceCatalogItem }>(
      `/service-catalog/${enc(serviceId)}/${command}`,
      { expectedVersion },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
};

// Manifests (Section 15.7).
export const manifestService = {
  async list(params: { status?: string; branchId?: string } = {}): Promise<Manifest[]> {
    return (await apiClient.get<{ data: Manifest[] | null }>(`/manifests${query(params)}`)).data.data ?? [];
  },
  async get(manifestId: string): Promise<ManifestDetail> {
    return (await apiClient.get<{ data: ManifestDetail }>(`/manifests/${enc(manifestId)}`)).data.data;
  },
  async create(input: ManifestInput, idempotencyKey: string): Promise<Manifest> {
    return (await apiClient.post<{ data: Manifest }>('/manifests', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async update(manifestId: string, patch: ManifestPatch, expectedVersion: number): Promise<Manifest> {
    const res = await apiClient.patch<{ data: Manifest }>(
      `/manifests/${enc(manifestId)}`,
      { ...patch, expectedVersion },
      commandHeaders({ expectedVersion }),
    );
    return res.data.data;
  },
  async addItem(manifestId: string, itemType: string, itemId: string, idempotencyKey: string): Promise<ManifestItem> {
    const res = await apiClient.post<{ data: ManifestItem }>(
      `/manifests/${enc(manifestId)}/items`,
      { itemType, itemId },
      commandHeaders({ idempotencyKey }),
    );
    return res.data.data;
  },
  async close(manifestId: string, expectedVersion: number, idempotencyKey: string): Promise<Manifest> {
    const res = await apiClient.post<{ data: { manifest: Manifest } }>(
      `/manifests/${enc(manifestId)}/close`,
      { expectedVersion },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data.manifest;
  },
  async amendSeal(manifestId: string, sealNumber: string, reasonCode: string, expectedVersion: number, idempotencyKey: string): Promise<Manifest> {
    const res = await apiClient.post<{ data: Manifest }>(
      `/manifests/${enc(manifestId)}/seal-amendments`,
      { sealNumber, reasonCode, expectedVersion },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async cancel(manifestId: string, reasonCode: string, expectedVersion: number, idempotencyKey: string): Promise<Manifest> {
    const res = await apiClient.post<{ data: Manifest }>(
      `/manifests/${enc(manifestId)}/cancel`,
      { reasonCode, expectedVersion },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
};

// Recurring order templates (Section 15.9). The backend offers activate,
// deactivate and replay (a dead-lettered occurrence); there is no cancel.
export const recurringOrderService = {
  async list(params: { status?: string } = {}): Promise<RecurringOrderTemplate[]> {
    return (await apiClient.get<{ data: RecurringOrderTemplate[] | null }>(`/recurring-orders${query(params)}`)).data.data ?? [];
  },
  async get(templateId: string): Promise<RecurringOrderTemplate> {
    return (await apiClient.get<{ data: RecurringOrderTemplate }>(`/recurring-orders/${enc(templateId)}`)).data.data;
  },
  async create(input: RecurringOrderInput, idempotencyKey: string): Promise<RecurringOrderTemplate> {
    return (await apiClient.post<{ data: RecurringOrderTemplate }>('/recurring-orders', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async setActive(templateId: string, active: boolean, expectedVersion: number, idempotencyKey: string): Promise<RecurringOrderTemplate> {
    const res = await apiClient.post<{ data: RecurringOrderTemplate }>(
      `/recurring-orders/${enc(templateId)}/${active ? 'activate' : 'deactivate'}`,
      { expectedVersion },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async replay(templateId: string, idempotencyKey: string): Promise<void> {
    await apiClient.post(`/recurring-orders/${enc(templateId)}/replay`, {}, commandHeaders({ idempotencyKey }));
  },
};

// Order imports (Section 15.8, the /orders/imports surface).
export const orderImportService = {
  async list(): Promise<OrderImport[]> {
    return (await apiClient.get<{ data: OrderImport[] | null }>('/orders/imports')).data.data ?? [];
  },
  async get(importId: string): Promise<OrderImportDetail> {
    return (await apiClient.get<{ data: OrderImportDetail }>(`/orders/imports/${enc(importId)}`)).data.data;
  },
  async create(input: { branchId: string; mediaFileId: string; fileFormat: string }, idempotencyKey: string): Promise<OrderImport> {
    return (await apiClient.post<{ data: OrderImport }>('/orders/imports', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async cancel(importId: string, expectedVersion: number, idempotencyKey: string): Promise<OrderImport> {
    const res = await apiClient.post<{ data: OrderImport }>(
      `/orders/imports/${enc(importId)}/cancel`,
      { expectedVersion },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async replay(importId: string, idempotencyKey: string): Promise<void> {
    await apiClient.post(`/orders/imports/${enc(importId)}/replay`, {}, commandHeaders({ idempotencyKey }));
  },
};

// Private (company-provider) assignments. The list is capped at 100 rows and
// has no order filter, so callers narrow it client-side.
export const privateAssignmentService = {
  async list(params: { status?: string } = {}): Promise<PrivateAssignment[]> {
    return (await apiClient.get<{ data: PrivateAssignment[] | null }>(`/private-assignments${query(params)}`)).data.data ?? [];
  },
};
