import apiClient from '@/lib/api/client';
import type {
  InviteStaffInput,
  Permission,
  Role,
  StaffInvitation,
  StaffMember,
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
