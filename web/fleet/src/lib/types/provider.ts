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