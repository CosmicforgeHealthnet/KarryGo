// Staff (office users of the company) — distinct from providers (drivers,
// riders and haulers who carry out the work).

export type StaffStatus = 'ACTIVE' | 'INVITED' | 'SUSPENDED' | 'ENDED' | string;

export interface RoleRef {
  id: string;
  code: string;
  name: string;
}

export interface StaffMember {
  id: string;
  fullName: string;
  jobTitle: string | null;
  email: string;
  phone: string;
  status: StaffStatus;
  isOwner: boolean;
  roles: RoleRef[];
  branchIds: string[];
  version: number;
  invitedAt: string | null;
  activatedAt: string | null;
  suspendedAt: string | null;
  createdAt: string;
}

export type InvitationStatus = 'PENDING' | 'ACCEPTED' | 'EXPIRED' | 'CANCELLED' | string;

export interface StaffInvitation {
  id: string;
  fullName: string;
  email: string;
  phone: string;
  roleIds: string[];
  branchIds: string[];
  status: InvitationStatus;
  deliveryStatus: string;
  expiresAt: string;
  createdAt: string;
}

export interface Role {
  id: string;
  organizationId: string | null;
  code: string;
  name: string;
  description: string;
  isSystem: boolean;
  status: string;
  permissions: string[] | null;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface Permission {
  code: string;
  category: string;
  description: string;
  isActive: boolean;
}

export interface InviteStaffInput {
  fullName: string;
  email: string;
  phone: string;
  roleIds: string[];
  branchIds?: string[];
}

export interface UpdateStaffInput {
  fullName?: string;
  jobTitle?: string | null;
  branchIds?: string[];
  departmentIds?: string[];
}

export interface RoleInput {
  code: string;
  name: string;
  description: string;
  permissions: string[];
}
