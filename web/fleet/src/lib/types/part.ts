export type PartStatus = 'ACTIVE' | 'INACTIVE' | 'ARCHIVED';

export type StockMovementType =
  | 'RECEIVE'
  | 'ISSUE'
  | 'ADJUST'
  | 'TRANSFER_IN'
  | 'TRANSFER_OUT'
  | 'RETURN';

export interface Part {
  id: string;
  organizationId: string;
  branchId: string;
  sku: string;
  name: string;
  stockUnit: string;
  warrantyInformation: string;
  status: string;
  currentQuantityProjection: number;
  reorderLevel: number;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreatePartInput {
  branchId: string;
  sku: string;
  name: string;
  stockUnit?: string;
  reorderLevel?: number;
  warrantyInformation?: string;
}

export interface UpdatePartInput {
  name?: string;
  reorderLevel?: number;
  status?: string;
  version: number;
}

export interface StockMovement {
  id: string;
  organizationId: string;
  partId: string;
  locationReference: string;
  movementType: string;
  quantity: number;
  reason: string;
  workOrderId: string;
  correlationId: string;
  createdBy: string;
  createdAt: string;
}

export interface CreateStockMovementInput {
  partId: string;
  locationReference?: string;
  movementType: string;
  quantity: number;
  reason: string;
  workOrderId?: string;
}