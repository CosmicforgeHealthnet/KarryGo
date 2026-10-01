export type AssetType =
  | 'MOTORCYCLE'
  | 'VAN'
  | 'TRUCK'
  | 'CAR'
  | 'BIKE'
  | 'OTHER';

export type OperationalStatus =
  | 'AVAILABLE'
  | 'ASSIGNED'
  | 'BLOCKED'
  | 'UNDER_MAINTENANCE'
  | 'OUT_OF_SERVICE'
  | 'RETIRED'
  | 'ARCHIVED';

export type LifecycleStatus = 'ACTIVE' | 'ARCHIVED' | 'RETIRED';

export type OwnershipType = 'OWNED' | 'LEASED' | 'CONTRACTED';

export type AssignmentType = 'PRIMARY' | 'SECONDARY' | 'TEMPORARY';

export type VerificationStatus =
  | 'PENDING'
  | 'VERIFIED'
  | 'REJECTED'
  | 'EXPIRED'
  | 'REVOKED';

export interface Asset {
  id: string;
  organizationId: string;
  branchId: string;
  assetCode: string;
  assetType: string;
  platformVehicleId: string;
  ownershipType: string;
  registrationReference: string;
  manufacturer: string;
  model: string;
  year: number | null;
  capacity: number;
  capacityUnit: string;
  odometer: number;
  operatingHours: number;
  operationalStatus: string;
  lifecycleStatus: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  archivedAt: string | null;
  retiredAt: string | null;
}

export interface CreateAssetInput {
  branchId: string;
  assetCode: string;
  assetType: string;
  platformVehicleId?: string;
  ownershipType?: string;
  registrationReference?: string;
  manufacturer?: string;
  model?: string;
  year?: number | null;
  capacity?: number;
  capacityUnit?: string;
  odometer?: number;
  operatingHours?: number;
}

export interface UpdateAssetInput {
  manufacturer?: string;
  model?: string;
  capacity?: number;
  odometer?: number;
  operatingHours?: number;
  version: number;
}

export interface Assignment {
  id: string;
  organizationId: string;
  assetId: string;
  providerMembershipId: string;
  shiftId: string;
  assignmentType: string;
  status: string;
  startAt: string;
  endAt: string | null;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateAssignmentInput {
  providerMembershipId: string;
  shiftId?: string;
  assignmentType?: string;
  startAt?: string;
  endAt?: string;
}

export interface AssetDocument {
  id: string;
  organizationId: string;
  assetId: string;
  mediaFileId: string;
  documentType: string;
  issuedAt: string | null;
  expiresAt: string | null;
  verificationReference: string;
  verificationStatus: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface AddDocumentInput {
  mediaFileId: string;
  documentType: string;
  issuedAt?: string;
  expiresAt?: string;
}

export interface AssetEligibility {
  eligible: boolean;
  hardBlocks: string[];
  warnings: string[];
  reasons: string[];
}

export interface AssetListFilter {
  branchId?: string;
  status?: string;
  limit?: number;
  offset?: number;
}