import apiClient from '@/lib/api/client';
import type { RoleBinding, RoleBindingInput } from '@/lib/types/structure';
import type {
  InviteStaffInput,
  Permission,
  Role,
  RoleInput,
  StaffInvitation,
  StaffMember,
  UpdateStaffInput,
} from '@/lib/types/staff';

const id = (value: string) => encodeURIComponent(value);

export const staffService = {
  async list(): Promise<StaffMember[]> {
    const res = await apiClient.get<{ data: StaffMember[] }>('/staff');
    return res.data.data ?? [];
  },

  async get(staffId: string): Promise<StaffMember> {
    const res = await apiClient.get<{ data: StaffMember }>(`/staff/${id(staffId)}`);
    return res.data.data;
  },

  async invitations(): Promise<StaffInvitation[]> {
    const res = await apiClient.get<{ data: StaffInvitation[] }>('/staff/invitations');
    return res.data.data ?? [];
  },

  async invite(input: InviteStaffInput): Promise<void> {
    await apiClient.post('/staff/invitations', input);
  },

  async resendInvitation(invitationId: string): Promise<void> {
    await apiClient.post(`/staff/invitations/${id(invitationId)}/resend`, {});
  },

  async cancelInvitation(invitationId: string): Promise<void> {
    await apiClient.delete(`/staff/invitations/${id(invitationId)}`);
  },

  async suspend(staffId: string, expectedVersion: number): Promise<void> {
    await apiClient.post(`/staff/${id(staffId)}/suspend`, {}, {
      headers: { 'If-Match': String(expectedVersion) },
    });
  },

  async reactivate(staffId: string, expectedVersion: number): Promise<void> {
    await apiClient.post(`/staff/${id(staffId)}/reactivate`, {}, {
      headers: { 'If-Match': String(expectedVersion) },
    });
  },

  async roles(): Promise<Role[]> {
    const res = await apiClient.get<{ data: Role[] }>('/roles');
    return res.data.data ?? [];
  },

  async permissions(): Promise<Permission[]> {
    const res = await apiClient.get<{ data: Permission[] }>('/permissions');
    return res.data.data ?? [];
  },

  async update(staffId: string, input: UpdateStaffInput, expectedVersion: number): Promise<StaffMember> {
    const res = await apiClient.patch<{ data: StaffMember }>(`/staff/${id(staffId)}`, input, {
      headers: { 'If-Match': String(expectedVersion) },
    });
    return res.data.data;
  },

  async roleBindings(staffId: string): Promise<RoleBinding[]> {
    const res = await apiClient.get<{ data: RoleBinding[] }>(`/staff/${id(staffId)}/role-bindings`);
    return res.data.data ?? [];
  },

  // Replaces every role the staff member holds. The server refuses changes
  // to your own roles (permission_escalation_forbidden).
  async replaceRoles(staffId: string, bindings: RoleBindingInput[]): Promise<RoleBinding[]> {
    const res = await apiClient.put<{ data: { staffId: string; bindings: RoleBinding[] } }>(
      `/staff/${id(staffId)}/roles`,
      { bindings },
    );
    return res.data.data?.bindings ?? [];
  },

  async getRole(roleId: string): Promise<Role> {
    const res = await apiClient.get<{ data: Role }>(`/roles/${id(roleId)}`);
    return res.data.data;
  },

  async createRole(input: RoleInput): Promise<Role> {
    const res = await apiClient.post<{ data: Role }>('/roles', input);
    return res.data.data;
  },

  // System roles are managed by the platform (role_is_system_managed).
  async updateRole(roleId: string, input: Omit<RoleInput, 'code'>, expectedVersion: number): Promise<Role> {
    const res = await apiClient.patch<{ data: Role }>(`/roles/${id(roleId)}`, input, {
      headers: { 'If-Match': String(expectedVersion) },
    });
    return res.data.data;
  },

  async archiveRole(roleId: string, expectedVersion: number): Promise<void> {
    await apiClient.post(`/roles/${id(roleId)}/archive`, {}, {
      headers: { 'If-Match': String(expectedVersion) },
    });
  },
};

// A safe, user-facing message for a failed staff request.
export function staffErrorMessage(error: unknown, fallback = 'Something went wrong. Please try again.'): string {
  const e = error as { response?: { status?: number; data?: { error?: { message?: string } } } };
  const status = e?.response?.status;
  if (!e?.response) return "We couldn't connect. Check your connection and try again.";
  if (status === 401) return 'Your session expired. Sign in again to continue.';
  if (status && status >= 500 && status !== 503) return fallback;
  return e.response.data?.error?.message || fallback;
}

export const STAFF_STATUS_LABEL: Record<string, string> = {
  ACTIVE: 'Active',
  INVITED: 'Invited',
  SUSPENDED: 'Suspended',
  ENDED: 'Removed',
  EXPIRED: 'Expired',
  CANCELLED: 'Cancelled',
};

export const INVITATION_STATUS_LABEL: Record<string, string> = {
  PENDING: 'Pending',
  ACCEPTED: 'Accepted',
  EXPIRED: 'Expired',
  CANCELLED: 'Cancelled',
};
