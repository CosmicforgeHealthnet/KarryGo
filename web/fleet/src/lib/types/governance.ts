// Approval policies, delegations and organization structure units
// (cost centres, teams, operating locations).

export interface ApprovalPolicyStep {
  quorum: number;
  eligibleRoleIds: string[];
}

export interface ApprovalPolicy {
  id: string;
  code: string;
  name: string;
  description: string;
  resourceType: string;
  action: string;
  scopeType: string;
  branchId: string | null;
  thresholdAmountMinor: number | null;
  thresholdCurrency: string | null;
  steps: ApprovalPolicyStep[];
  requesterMayApprove: boolean;
  separationOfDuties: boolean;
  expirySeconds: number;
  origin: string;
  status: string;
  version: number;
  createdBy: string;
  updatedBy: string;
  createdAt: string;
  updatedAt: string;
  archivedAt: string | null;
}

export interface ApprovalPolicyInput {
  code?: string;
  name?: string;
  description?: string;
  resourceType?: string;
  action?: string;
  branchId?: string | null;
  thresholdAmountMinor?: number | null;
  thresholdCurrency?: string | null;
  clearThreshold?: boolean;
  steps?: ApprovalPolicyStep[];
  requesterMayApprove?: boolean;
  separationOfDuties?: boolean;
  expirySeconds?: number;
  reason?: string;
}

export interface ApprovalPolicyVersion {
  id: string;
  policyId: string;
  version: number;
  status: string;
  rules: unknown;
  changedBy: string;
  changeReason: string;
  createdAt: string;
}

export interface ApprovalDelegation {
  id: string;
  delegatorId: string;
  delegateId: string;
  resourceType: string | null;
  action: string | null;
  branchId: string | null;
  effectiveFrom: string;
  expiresAt: string;
  reason: string;
  status: string;
  revokedAt: string | null;
  revokedBy: string | null;
  revocationReason: string | null;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface ApprovalDelegationInput {
  delegateId: string;
  resourceType?: string;
  action?: string;
  branchId?: string;
  effectiveFrom?: string;
  expiresAt: string;
  reason: string;
}

export interface CostCentre {
  id: string;
  branchId: string | null;
  code: string;
  name: string;
  budgetOwnerUserId: string | null;
  status: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface Team {
  id: string;
  branchId: string | null;
  departmentId: string | null;
  code: string;
  name: string;
  supervisorUserId: string | null;
  status: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export const LOCATION_TYPES = [
  'OFFICE', 'DEPOT', 'YARD', 'PARKING', 'LOADING_BAY', 'PICKUP_POINT', 'DROPOFF_POINT', 'SERVICE_POINT', 'FUEL_STATION', 'OTHER',
] as const;

export interface OperatingLocation {
  id: string;
  branchId: string | null;
  code: string;
  name: string;
  locationType: string;
  address: Record<string, unknown> | null;
  latitude: number | null;
  longitude: number | null;
  operatingHours: Record<string, unknown> | null;
  status: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}
