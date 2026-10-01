export type BreakdownSeverity = 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';

export type TripImpact =
  | 'NONE'
  | 'DELAYED'
  | 'INTERRUPTED'
  | 'CANCELLED';

export type RescueStatus =
  | 'NOT_REQUIRED'
  | 'REQUESTED'
  | 'DISPATCHED'
  | 'ON_SITE'
  | 'COMPLETED'
  | 'FAILED';

export type BreakdownStatus = 'OPEN' | 'IN_PROGRESS' | 'RESOLVED' | 'CLOSED';

export interface Breakdown {
  id: string;
  organizationId: string;
  assetId: string;
  providerMembershipId: string;
  tripReference: string;
  locationReference: string;
  severity: string;
  tripImpact: string;
  rescueStatus: string;
  towingReference: string;
  replacementAssetId: string;
  status: string;
  occurredAt: string;
  createdAt: string;
  updatedAt: string;
  version: number;
}

export interface CreateBreakdownInput {
  assetId: string;
  providerMembershipId?: string;
  tripReference?: string;
  locationReference?: string;
  severity: string;
  tripImpact?: string;
  rescueStatus?: string;
  towingReference?: string;
  replacementAssetId?: string;
  occurredAt?: string;
}

export interface UpdateBreakdownInput {
  rescueStatus?: string;
  towingReference?: string;
  replacementAssetId?: string;
  status?: string;
  expectedVersion: number;
}