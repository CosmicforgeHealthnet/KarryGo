export type MembershipStatus =
  | 'INVITED'
  | 'PENDING_PROVIDER_ACCEPTANCE'
  | 'PENDING_SETUP'
  | 'ACTIVE'
  | 'SUSPENDED'
  | 'ENDING'
  | 'REMOVED'
  | 'DECLINED'
  | 'EXPIRED'
  | 'CANCELLED';

export type ServiceCategory = 'DISPATCH' | 'TAXI' | 'HAULING';

export type ProviderService =
  | 'driver-dispatch-delivery-service'
  | 'driver-taxi-service'
  | 'driver-hauling-service';

export type OnboardingSource =
  | 'ORGANIZATION_SPONSORED'
  | 'EXISTING_PROVIDER_LINKED';

export type RelationshipType = 'EMPLOYEE' | 'CONTRACTOR' | 'PARTNER';

export type ProvisioningOverallStatus =
  | 'IN_PROGRESS'
  | 'COMPLETED'
  | 'BLOCKED'
  | 'RECONCILIATION_REQUIRED'
  | 'FAILED_PERMANENT';

export interface ProviderMembership {
  membershipId: string;
  serviceCategory: ServiceCategory;
  providerService: ProviderService;
  providerId: string | null;
  onboardingSource: OnboardingSource;
  relationshipType: RelationshipType;
  homeBranchId: string | null;
  status: MembershipStatus;
  joinedAt: string | null;
  version: number;
}

export interface ProvisioningStatus {
  overallStatus: ProvisioningOverallStatus;
  providerResolutionStatus: string;
  providerIdentityStatus: string;
  associationStatus: string;
  subWalletStatus: string;
  notificationStatus: string;
}

export interface MembershipHistoryEntry {
  id: string;
  fromStatus: MembershipStatus | '';
  toStatus: MembershipStatus;
  actorType: string;
  actorId: string;
  reasonCode: string;
  occurredAt: string;
}

export interface CreateProviderInput {
  legalName: string;
  email: string;
  phone: string;
  serviceCategory: ServiceCategory;
  homeBranchId?: string;
  relationshipType?: RelationshipType;
  employmentType?: string;
  employeeNumber?: string;
}

export interface CreateProviderResult {
  membershipId: string;
  status: MembershipStatus;
  invitationSent: boolean;
}

export interface ProviderListFilter {
  status?: MembershipStatus;
  serviceCategory?: ServiceCategory;
  onboardingSource?: OnboardingSource;
  relationshipType?: RelationshipType;
  search?: string;
}
export type EmploymentType = 'FULL_TIME' | 'PART_TIME' | 'CASUAL';

export interface UpdateProviderInput {
  homeBranchId?: string;
  relationshipType?: RelationshipType;
  employmentType?: EmploymentType | '';
  employeeNumber?: string;
}

export interface ProviderCapability {
  id?: string;
  serviceType: string;
  vehicleClasses: string[];
  operatingZones: string[];
  qualifications: string[];
  validFrom?: string;
  validUntil: string | null;
  status?: string;
}

export type AssignmentMode = 'OFFER' | 'BROADCAST' | 'DIRECTED_OFFER';

export interface ProviderPolicy {
  membershipId: string;
  assignmentMode: AssignmentMode | string;
  // Always true: a managed provider can always decline company work.
  providerDeclineRequired: boolean;
  visibilityPolicy: 'ORGANIZATION' | 'BRANCH' | string;
  shiftRestrictions: Record<string, unknown> | null;
  zoneRestrictions: Record<string, unknown> | null;
  workloadRestrictions: Record<string, unknown> | null;
  version: number;
  updatedAt: string;
}

export interface ProviderPolicyInput {
  assignmentMode: string;
  visibilityPolicy: string;
  shiftRestrictions: Record<string, unknown>;
  zoneRestrictions: Record<string, unknown>;
  workloadRestrictions: Record<string, unknown>;
}

export interface ProviderDecline {
  assignmentId: string;
  workOrigin: 'MARKETPLACE' | 'PRIVATE' | string;
  reasonCode: string;
  safetyRelated: boolean;
  declinedAt: string;
}
