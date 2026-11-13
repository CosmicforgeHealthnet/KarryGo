// Organization structure: branches, departments and organization settings
// (spec Sections 7-9). Money thresholds are whole minor units (kobo).

export type BranchType = 'BRANCH' | 'HUB' | 'DEPOT' | 'WAREHOUSE' | 'OFFICE';
export type BranchStatus = 'ACTIVE' | 'INACTIVE' | 'ARCHIVED';

export interface Branch {
  id: string;
  branchId: string;
  organizationId: string;
  code: string;
  name: string;
  type: BranchType | string;
  address: string;
  latitude: number | null;
  longitude: number | null;
  operatingHours: string;
  timezone: string;
  status: BranchStatus | string;
  version: number;
  createdAt: string;
  updatedAt: string;
  archivedAt: string | null;
}

export interface BranchInput {
  code?: string;
  name: string;
  type: string;
  address?: string;
  latitude?: number | null;
  longitude?: number | null;
  operatingHours?: string;
  timezone?: string;
  status?: string;
}

export interface Department {
  id: string;
  departmentId: string;
  organizationId: string;
  branchId: string | null;
  parentId: string | null;
  costCentreId: string | null;
  code: string;
  name: string;
  status: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  archivedAt: string | null;
}

export interface DepartmentInput {
  code?: string;
  name: string;
  branchId?: string | null;
  parentId?: string | null;
  costCentreId?: string | null;
}

export type ProviderVisibility = 'ORGANIZATION' | 'BRANCH';

export interface OrganizationSettings {
  organizationId: string;
  assignmentTimeoutSeconds: number;
  operatingMode: string;
  approvalThresholds: Record<string, number>;
  providerVisibility: ProviderVisibility | string;
  financialVisibilityPolicy: unknown;
  version: number;
  updatedAt: string;
}

export interface OrganizationSettingsPatch {
  assignmentTimeoutSeconds?: number;
  providerVisibility?: string;
  approvalThresholds?: Record<string, number>;
}

export type RoleScopeType = 'ORGANIZATION' | 'BRANCH' | 'DEPARTMENT';

export interface RoleBinding {
  id: string;
  organizationUserId: string;
  roleId: string;
  scopeType: RoleScopeType | string;
  scopeId: string;
  effectiveFrom: string;
  expiresAt: string | null;
  grantedBy: string | null;
  version: number;
  createdAt: string;
}

export interface RoleBindingInput {
  roleId: string;
  scopeType: RoleScopeType;
  scopeId?: string;
  expiresAt?: string | null;
}
