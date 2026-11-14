// Incidents and investigations, proof records and compliance checks /
// restrictions. Incidents, investigations, checks and restrictions are
// serialized with Go field names (PascalCase); proof records are camelCase.

export const INCIDENT_SEVERITIES = ['LOW', 'MEDIUM', 'HIGH', 'CRITICAL'] as const;
export const ESCALATION_TRIGGERS = ['SEVERITY', 'REGULATORY', 'CUSTOMER', 'SAFETY_CRITICAL', 'OTHER'] as const;

export interface Incident {
  id: string;
  branchId: string;
  incidentType: string;
  severity: string;
  status: string;
  occurredAt: string;
  locationReference: string;
  routeId: string;
  assignmentId: string;
  tripReference: string;
  externalReference: string;
  description: string;
  reportedBy: string;
  ownerUserId: string;
  resolutionAction: string;
  resolutionReason: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  closedAt: string | null;
  immediateActions: string;
  triagedAt: string | null;
  containmentSummary: string;
  containedAt: string | null;
  escalationReason: string;
  escalationTrigger: string;
  preEscalationStatus: string;
  escalatedAt: string | null;
  closureReason: string;
}

export interface Investigation {
  id: string;
  incidentId: string;
  investigatorUserId: string;
  scope: string;
  evidencePreservation: string;
  status: string;
  findings: string;
  rootCause: string;
  findingsSubmittedAt: string | null;
  approvedBy: string;
  approvedAt: string | null;
  openedBy: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface CorrectiveActionSpec {
  actionType: string;
  ownerUserId: string;
  dueAt: string;
}

export interface ProofRecord {
  id: string;
  policySnapshotReference: string;
  proofType: string;
  orderId?: string;
  shipmentId?: string;
  packageId?: string;
  routeStopId?: string;
  assignmentId?: string;
  canonicalTripReference?: string;
  actorType: string;
  actorReference: string;
  mediaFileId?: string;
  safeValueMetadata: unknown;
  sourceTimestamp: string;
  receivedAt: string;
  source: string;
  verificationStatus: string;
  version: number;
}

export interface ProofVerification {
  id: string;
  status: string;
  authorityType: string;
  authorityReference: string;
  reasonCodes: string[] | null;
  verifiedAt: string;
}

export interface ProofRecordDetail {
  record: ProofRecord;
  effectiveStatus: string;
  verifications: ProofVerification[] | null;
}

export const COMPLIANCE_SUBJECTS = ['ORGANIZATION', 'PROVIDER', 'ASSET', 'ORDER', 'SHIPMENT', 'CARGO', 'WAREHOUSE', 'LOCATION'] as const;

export interface ComplianceCheckResult {
  requirementId: string;
  requirementCode: string;
  severity: string;
  blocksOperation: boolean;
  complianceStatusId: string;
  state: string;
  restrictionReference?: string;
}

export interface ComplianceCheck {
  id: string;
  subjectType: string;
  subjectReference: string;
  sourceType: string;
  sourceReference: string;
  sourceVersion: number;
  overallState: string;
  governingRequirementId: string;
  requirementCount: number;
  results: ComplianceCheckResult[] | null;
  restrictionReferences: string[] | null;
  requestedBy: string;
  createdAt: string;
}

export interface Restriction {
  id: string;
  subjectType: string;
  subjectReference: string;
  reasonCode: string;
  sourceType: string;
  sourceReference: string;
  severity: string;
  state: string;
  effectiveAt: string;
  clearedAt: string | null;
  version: number;
  createdBy: string;
  clearedBy: string;
  clearanceReason: string;
  createdAt: string;
}

// Compliance status of one subject against one requirement (serialized with
// Go field names). States: UNKNOWN, PENDING, COMPLIANT, DUE_SOON, EXPIRED,
// NON_COMPLIANT, RESTRICTED.
export interface ComplianceStatus {
  id: string;
  subjectType: string;
  subjectReference: string;
  requirementId: string;
  state: string;
  dueAt: string | null;
  expiresAt: string | null;
  sourceType: string;
  sourceReference: string;
  sourceVersion: number;
  evaluatedAt: string;
  reasonCodes: unknown;
  restrictionReference: string;
  version: number;
}

export interface ComplianceRequirement {
  id: string;
  code: string;
  name: string;
  subjectType: string;
  requirementType: string;
  requiredDocumentType: string;
  severity: string;
  blocksOperation: boolean;
  status: string;
}
