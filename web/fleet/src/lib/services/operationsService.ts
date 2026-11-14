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
  OpenRescueInput,
  Order,
  OrderHistoryEntry,
  OrderInput,
  OrderPatch,
  OrderStop,
  PackageView,
  Page,
  RescueCase,
  RescueReplacementInput,
  ServiceCatalogItem,
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
