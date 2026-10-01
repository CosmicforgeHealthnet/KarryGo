export type FuelOrEnergyType = 'PETROL' | 'DIESEL' | 'ELECTRIC' | 'GAS' | 'OTHER';

export type FuelUnit = 'L' | 'LITRE' | 'KWH' | 'KG' | 'UNIT';

export type ApprovalStatus = 'PENDING' | 'APPROVED' | 'REJECTED';

export interface FuelRecord {
  id: string;
  organizationId: string;
  assetId: string;
  providerMembershipId: string;
  fuelOrEnergyType: string;
  quantity: number;
  unit: string;
  odometer: number | null;
  operatingHours: number | null;
  receiptMediaId: string;
  supplierReference: string;
  recordedAt: string;
  approvalStatus: string;
  approvedBy: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateFuelRecordInput {
  assetId: string;
  providerMembershipId?: string;
  fuelOrEnergyType: string;
  quantity: number;
  unit: string;
  odometer?: number | null;
  operatingHours?: number | null;
  receiptMediaId?: string;
  supplierReference?: string;
  recordedAt?: string;
}

export interface FuelActionInput {
  expectedVersion: number;
  reason?: string;
}