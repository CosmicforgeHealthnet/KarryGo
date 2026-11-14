// Provider administration reads (availability, branches, vehicles, service
// roles) and the resend-login result.

export interface ProviderResendLoginResult {
  membershipId: string;
  membershipStatus: string;
  notice: string;
  invitationId: string;
  deliveryStatus: string;
  channel: string;
  maskedEmail: string;
  signInMethod: string;
  resendCount: number;
  resendsRemaining: number;
  invitationExpiresAt?: string;
  nextResendAvailableAt: string;
  replayed: boolean;
}

export interface ProviderAvailabilityReason {
  code: string;
  effect: string;
  source: string;
  detail?: Record<string, unknown>;
}

export interface QualificationRef {
  qualificationId: string;
  code: string;
  name: string;
  expiredAt?: string;
  validUntil?: string;
}

export interface ProviderAvailability {
  membershipId: string;
  status: string;
  evaluatedAt: string;
  reasons: ProviderAvailabilityReason[] | null;
  derivation: string;
  inputs: {
    membershipStatus: string;
    approvedLeave?: { leaveId: string; leaveType: string; startAt: string; endAt: string };
    shift: {
      scheduleInForce: boolean;
      onShift: boolean;
      currentShiftAssignmentId?: string;
      currentShiftEndsAt?: string;
      nextShiftStartsAt?: string;
    };
    qualifications: {
      mandatoryCount: number;
      expired: QualificationRef[] | null;
      missing: QualificationRef[] | null;
      expiringSoon: QualificationRef[] | null;
    };
    activeWork: { marketplace: number; private: number; total: number };
    tracking: { freshnessStatus: string; lastObservedAt?: string; sourceService?: string };
  };
}

export interface ProviderBranchLink {
  branchId: string;
  code: string;
  name: string;
  type: string;
  status: string;
  sources: string[];
}

export interface ProviderVehicle {
  assignmentId: string;
  assignmentType: string;
  assignmentStatus: string;
  startAt: string;
  endAt?: string;
  shiftId?: string;
  assetId: string;
  assetCode: string;
  assetType: string;
  branchId: string;
  platformVehicleId?: string;
  registrationReference?: string;
  manufacturer?: string;
  model?: string;
  operationalStatus: string;
  lifecycleStatus: string;
}

export interface ProviderServiceRoles {
  membershipId: string;
  serviceCategory: string;
  providerService: string;
  relationshipType: string;
  employmentType?: string;
  workforceRole?: string;
  workforceStatus?: string;
  capabilities: {
    id: string;
    serviceType: string;
    vehicleClasses: string[];
    operatingZones: string[];
    qualifications: string[];
    validFrom: string;
    validUntil?: string;
    status: string;
    effective: boolean;
  }[];
}
