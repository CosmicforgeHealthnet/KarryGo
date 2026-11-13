import apiClient from '@/lib/api/client';
import type {
  Branch,
  BranchInput,
  Department,
  DepartmentInput,
  OrganizationSettings,
  OrganizationSettingsPatch,
} from '@/lib/types/structure';

const id = (value: string) => encodeURIComponent(value);
const ifMatch = (version: number) => ({ headers: { 'If-Match': String(version) } });

// Branches, departments and organization settings. Writes carry the version
// the user saw, so a change made by someone else in between is refused
// rather than silently overwritten.
export const structureService = {
  async listBranches(): Promise<Branch[]> {
    const res = await apiClient.get<{ data: Branch[] }>('/branches');
    return res.data.data ?? [];
  },

  async getBranch(branchId: string): Promise<Branch> {
    const res = await apiClient.get<{ data: Branch }>(`/branches/${id(branchId)}`);
    return res.data.data;
  },

  async createBranch(input: BranchInput): Promise<Branch> {
    const res = await apiClient.post<{ data: Branch }>('/branches', input);
    return res.data.data;
  },

  async updateBranch(branchId: string, input: BranchInput, expectedVersion: number): Promise<Branch> {
    const res = await apiClient.patch<{ data: Branch }>(`/branches/${id(branchId)}`, input, ifMatch(expectedVersion));
    return res.data.data;
  },

  async archiveBranch(branchId: string, expectedVersion: number): Promise<void> {
    await apiClient.post(`/branches/${id(branchId)}/archive`, {}, ifMatch(expectedVersion));
  },

  async listDepartments(params?: { branchId?: string; includeArchived?: boolean }): Promise<Department[]> {
    const q = new URLSearchParams();
    if (params?.branchId) q.set('branchId', params.branchId);
    if (params?.includeArchived) q.set('includeArchived', 'true');
    const qs = q.toString();
    const res = await apiClient.get<{ data: Department[] }>(`/departments${qs ? `?${qs}` : ''}`);
    return res.data.data ?? [];
  },

  async getDepartment(departmentId: string): Promise<Department> {
    const res = await apiClient.get<{ data: Department }>(`/departments/${id(departmentId)}`);
    return res.data.data;
  },

  async createDepartment(input: DepartmentInput): Promise<Department> {
    const res = await apiClient.post<{ data: Department }>('/departments', input);
    return res.data.data;
  },

  async updateDepartment(departmentId: string, input: DepartmentInput, expectedVersion: number): Promise<Department> {
    const res = await apiClient.patch<{ data: Department }>(
      `/departments/${id(departmentId)}`,
      input,
      ifMatch(expectedVersion),
    );
    return res.data.data;
  },

  async archiveDepartment(departmentId: string, expectedVersion: number): Promise<void> {
    await apiClient.post(`/departments/${id(departmentId)}/archive`, {}, ifMatch(expectedVersion));
  },

  async getSettings(): Promise<OrganizationSettings> {
    const res = await apiClient.get<{ data: OrganizationSettings }>('/organization-settings');
    return res.data.data;
  },

  async updateSettings(patch: OrganizationSettingsPatch, expectedVersion: number): Promise<OrganizationSettings> {
    const res = await apiClient.patch<{ data: OrganizationSettings }>('/organization-settings', patch, ifMatch(expectedVersion));
    return res.data.data;
  },
};

export const BRANCH_TYPES = ['BRANCH', 'HUB', 'DEPOT', 'WAREHOUSE', 'OFFICE'] as const;

export const BRANCH_TYPE_LABEL: Record<string, string> = {
  BRANCH: 'Branch',
  HUB: 'Hub',
  DEPOT: 'Depot',
  WAREHOUSE: 'Warehouse',
  OFFICE: 'Office',
};

export const STRUCTURE_STATUS_LABEL: Record<string, string> = {
  ACTIVE: 'Active',
  INACTIVE: 'Inactive',
  ARCHIVED: 'Archived',
};
