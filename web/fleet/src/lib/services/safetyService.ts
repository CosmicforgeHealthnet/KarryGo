import apiClient from '@/lib/api/client';
import { commandHeaders, enc, query } from '@/lib/api/headers';
import type {
  ComplianceCheck,
  ComplianceRequirement,
  ComplianceStatus,
  CorrectiveActionSpec,
  Incident,
  Investigation,
  ProofRecord,
  ProofRecordDetail,
  Restriction,
} from '@/lib/types/safety';

export type IncidentCommand = 'triage' | 'contain' | 'escalate' | 'resolve' | 'close';

// Incident lifecycle (spec 58.11). Every lifecycle command is idempotent and
// carries the incident (or investigation) version the user saw.
export const incidentService = {
  async list(): Promise<Incident[]> {
    return (await apiClient.get<{ data: Incident[] }>('/incidents')).data.data ?? [];
  },
  async get(incidentId: string): Promise<Incident> {
    return (await apiClient.get<{ data: Incident }>(`/incidents/${enc(incidentId)}`)).data.data;
  },
  async create(
    input: { IncidentType: string; Severity: string; Description: string; OccurredAt: string; LocationReference?: string; BranchID?: string; TripReference?: string },
    idempotencyKey: string,
  ): Promise<Incident> {
    return (await apiClient.post<{ data: Incident }>('/incidents', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async command(
    incidentId: string,
    command: IncidentCommand,
    body: Record<string, unknown>,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Incident> {
    const res = await apiClient.post<{ data: Incident }>(
      `/incidents/${enc(incidentId)}/${command}`,
      body,
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async investigations(incidentId: string): Promise<Investigation[]> {
    return (await apiClient.get<{ data: Investigation[] }>(`/incidents/${enc(incidentId)}/investigations`)).data.data ?? [];
  },
  async openInvestigation(
    incidentId: string,
    body: { investigatorUserId: string; scope: string; evidencePreservation?: string },
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<{ investigation: Investigation; incident: Incident }> {
    const res = await apiClient.post<{ data: { investigation: Investigation; incident: Incident } }>(
      `/incidents/${enc(incidentId)}/investigations`,
      body,
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async submitFindings(
    investigationId: string,
    body: { findings: string; rootCause: string },
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Investigation> {
    const res = await apiClient.post<{ data: Investigation }>(
      `/incident-investigations/${enc(investigationId)}/findings`,
      body,
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
  async approveFindings(
    investigationId: string,
    correctiveActions: CorrectiveActionSpec[],
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<{ investigation: Investigation; incident: Incident }> {
    const res = await apiClient.post<{ data: { investigation: Investigation; incident: Incident } }>(
      `/incident-investigations/${enc(investigationId)}/approve`,
      { correctiveActions },
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
};

// Proof records: verify / reject are idempotent decisions.
export const proofService = {
  async list(): Promise<ProofRecord[]> {
    return (await apiClient.get<{ data: ProofRecord[] }>('/proof-records')).data.data ?? [];
  },
  async get(recordId: string): Promise<ProofRecordDetail> {
    return (await apiClient.get<{ data: ProofRecordDetail }>(`/proof-records/${enc(recordId)}`)).data.data;
  },
  async decide(recordId: string, decision: 'verify' | 'reject', reasonCodes: string[], idempotencyKey: string): Promise<ProofRecordDetail> {
    const res = await apiClient.post<{ data: ProofRecordDetail }>(
      `/proof-records/${enc(recordId)}/${decision}`,
      { reasonCodes },
      commandHeaders({ idempotencyKey }),
    );
    return res.data.data;
  },
};

// Compliance checks (evaluate one subject) and the restrictions they raise.
export const complianceService = {
  async runCheck(
    input: { SubjectType: string; SubjectReference: string; SourceType: string; SourceReference?: string; SourceVersion?: number },
    idempotencyKey: string,
  ): Promise<ComplianceCheck> {
    return (await apiClient.post<{ data: ComplianceCheck }>('/compliance/checks', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async listChecks(params: { subjectType?: string; subjectReference?: string } = {}): Promise<ComplianceCheck[]> {
    return (await apiClient.get<{ data: ComplianceCheck[] }>(`/compliance/checks${query(params)}`)).data.data ?? [];
  },
  async getCheck(checkId: string): Promise<ComplianceCheck> {
    return (await apiClient.get<{ data: ComplianceCheck }>(`/compliance/checks/${enc(checkId)}`)).data.data;
  },
  async listRestrictions(params: { subjectType?: string; subjectReference?: string; state?: string } = {}): Promise<Restriction[]> {
    return (await apiClient.get<{ data: Restriction[] }>(`/compliance/restrictions${query(params)}`)).data.data ?? [];
  },
};

// Per-subject compliance status (spec 19.3) and the requirements it refers to.
export const complianceStatusService = {
  async forProvider(membershipId: string): Promise<ComplianceStatus[]> {
    return (await apiClient.get<{ data: ComplianceStatus[] }>(`/providers/${enc(membershipId)}/compliance`)).data.data ?? [];
  },
  async forAsset(assetId: string): Promise<ComplianceStatus[]> {
    return (await apiClient.get<{ data: ComplianceStatus[] }>(`/assets/${enc(assetId)}/compliance`)).data.data ?? [];
  },
  async requirements(): Promise<ComplianceRequirement[]> {
    return (await apiClient.get<{ data: ComplianceRequirement[] }>('/compliance/requirements')).data.data ?? [];
  },
};
