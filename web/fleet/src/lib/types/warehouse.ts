export type WarehouseStatus = 'ACTIVE' | 'INACTIVE' | 'ARCHIVED';

export type LocationType =
  | 'ZONE'
  | 'AISLE'
  | 'BAY'
  | 'BIN'
  | 'DOCK'
  | 'STAGING'
  | 'QUARANTINE'
  | 'COLD'
  | 'HAZMAT'
  | 'OTHER';

export type LocationStatus = 'ACTIVE' | 'INACTIVE' | 'MAINTENANCE';

export type InventoryStatus =
  | 'AVAILABLE'
  | 'RESERVED'
  | 'PICKING'
  | 'PACKED'
  | 'DISPATCHED'
  | 'QUARANTINED'
  | 'EXPIRED'
  | 'RETURNED';

export type MovementType =
  | 'RECEIPT'
  | 'ADJUSTMENT'
  | 'DISPATCH'
  | 'DAMAGE'
  | 'EXPIRY'
  | 'TRANSFER'
  | 'PUT_AWAY'
  | 'QUARANTINE'
  | 'RELEASE'
  | 'PICK'
  | 'PACK'
  | 'RETURN';

export type FulfilmentOrderStatus =
  | 'DRAFT'
  | 'RELEASED'
  | 'PICKING'
  | 'PICKED'
  | 'PACKING'
  | 'PACKED'
  | 'DISPATCHED'
  | 'COMPLETED'
  | 'CANCELLED';

export type FulfilmentPriority = 'NORMAL' | 'HIGH' | 'URGENT';

export type DockStatus =
  | 'REQUESTED'
  | 'CONFIRMED'
  | 'CHECKED_IN'
  | 'COMPLETED'
  | 'NO_SHOW'
  | 'CANCELLED';

export type YardStatus =
  | 'GATE_CHECK_IN'
  | 'IN_YARD'
  | 'AT_BAY'
  | 'SECURITY_HOLD'
  | 'RELEASED'
  | 'CHECKED_OUT';

export type YardSecurityStatus = 'CLEARED' | 'HOLD';

export type TransferStatus =
  | 'DRAFT'
  | 'READY'
  | 'DEPARTED'
  | 'IN_TRANSIT'
  | 'ARRIVED'
  | 'COMPLETED'
  | 'DISCREPANCY';

export type ReconciliationStatus = 'PENDING' | 'MATCHED' | 'DISCREPANCY';

export type ReturnStatus =
  | 'REQUESTED'
  | 'RECEIVED'
  | 'INSPECTING'
  | 'DECIDED'
  | 'COMPLETED';

export interface Warehouse {
  id: string;
  organizationId: string;
  branchId: string;
  code: string;
  name: string;
  siteReference: string;
  addressReference: string;
  timezone: string;
  operatingHours: Record<string, unknown>;
  status: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  archivedAt: string | null;
}

export interface CreateWarehouseInput {
  branchId: string;
  code: string;
  name: string;
  siteReference?: string;
  addressReference: string;
  timezone: string;
  operatingHours?: Record<string, unknown>;
}

export interface UpdateWarehouseInput {
  name?: string;
  siteReference?: string;
  addressReference?: string;
  timezone?: string;
  operatingHours?: Record<string, unknown>;
  status?: string;
  version: number;
}

export interface StorageLocation {
  id: string;
  organizationId: string;
  warehouseId: string;
  parentLocationId: string;
  code: string;
  name: string;
  locationType: string;
  capacity: number | null;
  capacityUnit: string;
  restrictions: Record<string, unknown>;
  status: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateLocationInput {
  code: string;
  name: string;
  locationType: string;
  parentLocationId?: string;
  capacity?: number | null;
  capacityUnit?: string;
  restrictions?: Record<string, unknown>;
}

export interface UpdateLocationInput {
  parentLocationId?: string;
  name?: string;
  capacity?: number | null;
  capacityUnit?: string;
  restrictions?: Record<string, unknown>;
  status?: string;
  version: number;
}

export interface InventoryItem {
  id: string;
  organizationId: string;
  warehouseId: string;
  locationId: string;
  ownerType: string;
  ownerReference: string;
  sku: string;
  batchReference: string;
  lotReference: string;
  serialReference: string;
  quantityProjection: number;
  reservedQuantity: number;
  unit: string;
  status: string;
  expiryAt: string | null;
  condition: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface Movement {
  id: string;
  organizationId: string;
  inventoryItemId: string;
  movementType: string;
  fromLocationId: string;
  toLocationId: string;
  quantity: number;
  reasonCode: string;
  actorReference: string;
  fulfilmentOrderId: string;
  terminalTransferId: string;
  returnId: string;
  correlationId: string;
  projectionBefore: number;
  projectionAfter: number;
  createdAt: string;
}

export interface ReceiptInput {
  warehouseId: string;
  locationId: string;
  ownerType: string;
  ownerReference: string;
  sku: string;
  batchReference?: string;
  lotReference?: string;
  serialReference?: string;
  quantity: number;
  unit: string;
  condition?: string;
  expiryAt?: string | null;
  packageId?: string;
  shipmentId?: string;
  fromActorType?: string;
  fromActorReference?: string;
  evidenceMediaId?: string;
}

export interface MovementInput {
  movementType: string;
  quantity: number;
  reasonCode: string;
  expectedVersion: number;
  toLocationId?: string;
  increase?: boolean;
  fulfilmentOrderId?: string;
  terminalTransferId?: string;
  returnId?: string;
}

export interface FulfilmentLine {
  id: string;
  organizationId: string;
  fulfilmentOrderId: string;
  inventoryItemId: string;
  requestedQuantity: number;
  pickedQuantity: number;
  packageReference: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface FulfilmentOrder {
  id: string;
  organizationId: string;
  warehouseId: string;
  customerReference: string;
  sourceOrderId: string;
  priority: string;
  status: string;
  requestedDispatchAt: string | null;
  assignedTeamReference: string;
  packingRequirements: Record<string, unknown>;
  proofPolicyReference: string;
  shipmentReference: string;
  manifestReference: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  completedAt: string | null;
  lines?: FulfilmentLine[];
}

export interface CreateFulfilmentInput {
  warehouseId: string;
  customerReference: string;
  sourceOrderId?: string;
  priority?: string;
  requestedDispatchAt?: string | null;
  assignedTeamReference?: string;
  packingRequirements?: Record<string, unknown>;
  proofPolicyReference?: string;
  lines: {
    inventoryItemId: string;
    requestedQuantity: number;
  }[];
}

export interface DockAppointment {
  id: string;
  organizationId: string;
  warehouseId: string;
  dockLocationId: string;
  carrierReference: string;
  providerReference: string;
  vehicleOrTruckReference: string;
  appointmentType: string;
  windowStart: string;
  windowEnd: string;
  status: string;
  checkedInAt: string | null;
  loadingStartedAt: string | null;
  loadingCompletedAt: string | null;
  checkedOutAt: string | null;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateDockAppointmentInput {
  warehouseId: string;
  dockLocationId: string;
  carrierReference?: string;
  providerReference?: string;
  vehicleOrTruckReference: string;
  appointmentType?: string;
  windowStart: string;
  windowEnd: string;
}

export interface YardVisit {
  id: string;
  organizationId: string;
  warehouseId: string;
  providerReference: string;
  vehicleOrTruckReference: string;
  gateReference: string;
  bayReference: string;
  sealReference: string;
  securityStatus: string;
  status: string;
  checkedInAt: string | null;
  enteredYardAt: string | null;
  exitedYardAt: string | null;
  checkedOutAt: string | null;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateYardVisitInput {
  warehouseId: string;
  providerReference: string;
  vehicleOrTruckReference: string;
  gateReference?: string;
  bayReference?: string;
  sealReference?: string;
}

export interface TerminalTransfer {
  id: string;
  organizationId: string;
  originWarehouseId: string;
  destinationWarehouseId: string;
  manifestId: string;
  routeReference: string;
  status: string;
  departedAt: string | null;
  arrivedAt: string | null;
  originSnapshotReference: string;
  destinationSnapshotReference: string;
  reconciliationStatus: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateTerminalTransferInput {
  originWarehouseId: string;
  destinationWarehouseId: string;
  manifestId: string;
  routeReference?: string;
}

export interface Return {
  id: string;
  organizationId: string;
  warehouseId: string;
  sourceOrderId: string;
  sourceShipmentId: string;
  packageId: string;
  reasonCode: string;
  condition: string;
  disposition: string;
  financialReference: string;
  status: string;
  inventoryItemId: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  completedAt: string | null;
}

export interface CreateReturnInput {
  warehouseId: string;
  sourceOrderId?: string;
  sourceShipmentId?: string;
  packageId?: string;
  reasonCode: string;
  condition?: string;
  financialReference?: string;
  inventoryItemId?: string;
}