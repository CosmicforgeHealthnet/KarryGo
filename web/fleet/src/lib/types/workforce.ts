export type ProfileStatus = 'ACTIVE' | 'SUSPENDED' | 'ENDED' | 'ARCHIVED';
export type RelationshipType = 'EMPLOYEE' | 'CONTRACTOR' | 'PARTNER';

export type ShiftStatus =
  | 'DRAFT'
  | 'PUBLISHED'
  | 'IN_PROGRESS'
  | 'COMPLETED'
  | 'CANCELLED';

export type AssignmentStatus =
  | 'PENDING'
  | 'CONFIRMED'
  | 'CHECKED_IN'
  | 'CHECKED_OUT'
  | 'DECLINED'
  | 'CANCELLED';

export type AssignmentType = 'PRIMARY' | 'SECONDARY' | 'TEMPORARY';

export type LeaveType =
  | 'ANNUAL'
  | 'SICK'
  | 'PERSONAL'
  | 'MATERNITY'
  | 'PATERNITY'
  | 'UNPAID'
  | 'OTHER';

export type LeaveStatus =
  | 'PENDING'
  | 'APPROVED'
  | 'REJECTED'
  | 'CANCELLED';

export type QualificationStatus = 'ACTIVE' | 'INACTIVE' | 'ARCHIVED';

export type ProviderQualificationStatus =
  | 'PENDING_VERIFICATION'
  | 'VERIFIED'
  | 'REJECTED'
  | 'REVOKED';

export type CourseStatus = 'ACTIVE' | 'INACTIVE' | 'ARCHIVED';

export type CompletionStatus = 'PASSED' | 'FAILED';

export type PerformanceGenerationStatus =
  | 'PENDING'
  | 'COMPLETE'
  | 'FAILED';

export interface Profile {
  id: string;
  organizationId: string;
  branchId: string;
  membershipId: string;
  employeeNumber: string;
  relationshipType: string;
  role: string;
  supervisorMembershipId: string;
  status: string;
  startDate: string;
  endDate: string | null;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateProfileInput {
  branchId: string;
  membershipId: string;
  employeeNumber?: string;
  relationshipType: string;
  role: string;
  supervisorMembershipId?: string;
  startDate?: string;
}

export interface UpdateProfileInput {
  role?: string;
  supervisorMembershipId?: string;
  status?: string;
  expectedVersion: number;
}

export interface Shift {
  id: string;
  organizationId: string;
  branchId: string;
  name: string;
  role: string;
  startAt: string;
  endAt: string;
  timezone: string;
  requiredCapacity: number;
  status: string;
  plannerUserId: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateShiftInput {
  branchId: string;
  name: string;
  role: string;
  startAt: string;
  endAt: string;
  timezone: string;
  requiredCapacity: number;
}

export interface UpdateShiftInput {
  name?: string;
  startAt?: string;
  endAt?: string;
  status?: string;
  expectedVersion: number;
}

export interface ShiftAssignment {
  id: string;
  organizationId: string;
  shiftId: string;
  membershipId: string;
  assignmentType: string;
  status: string;
  expectedStart: string;
  expectedEnd: string;
  checkedInAt: string | null;
  checkedOutAt: string | null;
  checkInSource: string;
  checkOutSource: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface AssignShiftInput {
  membershipId: string;
  assignmentType?: string;
  expectedStart?: string;
  expectedEnd?: string;
}

export interface Leave {
  id: string;
  organizationId: string;
  membershipId: string;
  leaveType: string;
  startAt: string;
  endAt: string;
  reason: string;
  status: string;
  approvedBy: string;
  approvedAt: string | null;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateLeaveInput {
  membershipId: string;
  leaveType: string;
  startAt: string;
  endAt: string;
  reason?: string;
}

export interface Qualification {
  id: string;
  organizationId: string;
  code: string;
  name: string;
  applicableRoles: string[];
  applicableServices: string[];
  applicableAssetTypes: string[];
  validityPeriodSeconds: number | null;
  evidenceRequired: boolean;
  status: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateQualificationInput {
  code: string;
  name: string;
  applicableRoles?: string[];
  applicableServices?: string[];
  applicableAssetTypes?: string[];
  validityPeriodSeconds?: number | null;
  evidenceRequired?: boolean;
}

export interface UpdateQualificationInput {
  name?: string;
  applicableRoles?: string[];
  applicableServices?: string[];
  applicableAssetTypes?: string[];
  evidenceRequired?: boolean;
  expectedVersion: number;
}

export interface ProviderQualification {
  id: string;
  organizationId: string;
  membershipId: string;
  qualificationId: string;
  evidenceMediaId: string;
  verificationReference: string;
  issuedAt: string | null;
  expiresAt: string | null;
  status: string;
  verifiedBy: string;
  verifiedAt: string | null;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface AddProviderQualificationInput {
  qualificationId: string;
  evidenceMediaId?: string;
  issuedAt?: string | null;
  expiresAt?: string | null;
}

export interface Course {
  id: string;
  organizationId: string;
  code: string;
  title: string;
  providerReference: string;
  requiredRoles: string[];
  requiredServices: string[];
  validityPeriodSeconds: number | null;
  passingScore: number;
  status: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateCourseInput {
  code: string;
  title: string;
  providerReference?: string;
  requiredRoles?: string[];
  requiredServices?: string[];
  validityPeriodSeconds?: number | null;
  passingScore?: number;
}

export interface UpdateCourseInput {
  title?: string;
  passingScore?: number;
  expectedVersion: number;
}

export interface Completion {
  id: string;
  organizationId: string;
  courseId: string;
  membershipId: string;
  score: number;
  status: string;
  certificateMediaId: string;
  completedAt: string;
  expiresAt: string | null;
  verifiedBy: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CompleteCourseInput {
  membershipId: string;
  score: number;
  certificateMediaId?: string;
  completedAt?: string;
}

export interface PerformanceSnapshot {
  id: string;
  organizationId: string;
  membershipId: string;
  periodStart: string;
  periodEnd: string;
  metricInputs: Record<string, unknown>;
  calculatedMetrics: Record<string, unknown>;
  explanation: Record<string, unknown>;
  sourceVersions: Record<string, unknown>;
  generationStatus: string;
  formulaVersion: string;
  generatedAt: string;
  version: number;
}

export interface Page {
  branchId?: string;
  status?: string;
  limit?: number;
  offset?: number;
}