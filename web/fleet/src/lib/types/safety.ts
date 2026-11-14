// Incidents and investigations, proof records and compliance checks /
// restrictions. Incidents, investigations, checks and restrictions are
// serialized with Go field names (PascalCase); proof records are camelCase.

export const INCIDENT_SEVERITIES = ['LOW', 'MEDIUM', 'HIGH', 'CRITICAL'] as const;
export const ESCALATION_TRIGGERS = ['SEVERITY', 'REGULATORY', 'CUSTOMER', 'SAFETY_CRITICAL', 'OTHER'] as const;

export interface Incident {
  ID: string;
  BranchID: string;
  IncidentType: string;
  Severity: string;
  Status: string;
  OccurredAt: string;
  LocationReference: string;
  RouteID: string;
  AssignmentID: string;
  TripReference: string;
  ExternalReference: string;
  Description: string;
  ReportedBy: string;
  OwnerUserID: string;
  ResolutionAction: string;
  ResolutionReason: string;
  Version: number;
  CreatedAt: string;
  UpdatedAt: string;
  ClosedAt: string | null;
  ImmediateActions: string;
  TriagedAt: string | null;
  ContainmentSummary: string;
  ContainedAt: string | null;
  EscalationReason: string;
  EscalationTrigger: string;
  PreEscalationStatus: string;
  EscalatedAt: string | null;
  ClosureReason: string;
}

export interface Investigation {
  ID: string;
  IncidentID: string;
  InvestigatorUserID: string;
  Scope: string;
  EvidencePreservation: string;
  Status: string;
  Findings: string;
  RootCause: string;
  FindingsSubmittedAt: string | null;
  ApprovedBy: string;
  ApprovedAt: string | null;
  OpenedBy: string;
  Version: number;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface CorrectiveActionSpec {
  ActionType: string;
  OwnerUserID: string;
  DueAt: string;
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
  ID: string;
  Status: string;
  AuthorityType: string;
  AuthorityReference: string;
  ReasonCodes: string[] | null;
  VerifiedAt: string;
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
  ID: string;
  SubjectType: string;
  SubjectReference: string;
  SourceType: string;
  SourceReference: string;
  SourceVersion: number;
  OverallState: string;
  GoverningRequirementID: string;
  RequirementCount: number;
  Results: ComplianceCheckResult[] | null;
  RestrictionReferences: string[] | null;
  RequestedBy: string;
  CreatedAt: string;
}

export interface Restriction {
  ID: string;
  SubjectType: string;
  SubjectReference: string;
  ReasonCode: string;
  SourceType: string;
  SourceReference: string;
  Severity: string;
  State: string;
  EffectiveAt: string;
  ClearedAt: string | null;
  Version: number;
  CreatedBy: string;
  ClearedBy: string;
  ClearanceReason: string;
  CreatedAt: string;
}

// Compliance status of one subject against one requirement (serialized with
// Go field names). States: UNKNOWN, PENDING, COMPLIANT, DUE_SOON, EXPIRED,
// NON_COMPLIANT, RESTRICTED.
export interface ComplianceStatus {
  ID: string;
  SubjectType: string;
  SubjectReference: string;
  RequirementID: string;
  State: string;
  DueAt: string | null;
  ExpiresAt: string | null;
  SourceType: string;
  SourceReference: string;
  SourceVersion: number;
  EvaluatedAt: string;
  ReasonCodes: unknown;
  RestrictionReference: string;
  Version: number;
}

export interface ComplianceRequirement {
  ID: string;
  Code: string;
  Name: string;
  SubjectType: string;
  RequirementType: string;
  RequiredDocumentType: string;
  Severity: string;
  BlocksOperation: boolean;
  Status: string;
}
