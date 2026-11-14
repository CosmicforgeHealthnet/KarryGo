import apiClient from '@/lib/api/client';
import { commandHeaders, enc, query } from '@/lib/api/headers';
import type {
  ApprovalDelegation,
  ApprovalDelegationInput,
  ApprovalPolicy,
  ApprovalPolicyInput,
  ApprovalPolicyVersion,
  ApprovalDecision,
  ApprovalHistoryEntry,
  ApprovalRequest,
  ApprovalRequestDetail,
  CostCentre,
  OperatingLocation,
  Team,
} from '@/lib/types/governance';

// Approval policies and delegations. Creating either is idempotent; policy
// edits and archives carry the version the user saw.
export const approvalPolicyService = {
  async listPolicies(params: { status?: string } = {}): Promise<ApprovalPolicy[]> {
    return (await apiClient.get<{ data: ApprovalPolicy[] }>(`/approval-policies${query(params)}`)).data.data ?? [];
  },
  async getPolicy(policyId: string): Promise<ApprovalPolicy> {
    return (await apiClient.get<{ data: ApprovalPolicy }>(`/approval-policies/${enc(policyId)}`)).data.data;
  },
  async createPolicy(input: ApprovalPolicyInput, idempotencyKey: string): Promise<ApprovalPolicy> {
    return (await apiClient.post<{ data: ApprovalPolicy }>('/approval-policies', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async updatePolicy(policyId: string, input: ApprovalPolicyInput, expectedVersion: number): Promise<ApprovalPolicy> {
    const res = await apiClient.patch<{ data: ApprovalPolicy }>(
      `/approval-policies/${enc(policyId)}`,
      input,
      commandHeaders({ expectedVersion }),
    );
    return res.data.data;
  },
  async archivePolicy(policyId: string, reason: string, expectedVersion: number): Promise<ApprovalPolicy> {
    const res = await apiClient.post<{ data: ApprovalPolicy }>(
      `/approval-policies/${enc(policyId)}/archive`,
      { reason },
      commandHeaders({ expectedVersion }),
    );
    return res.data.data;
  },
  async policyVersions(policyId: string): Promise<ApprovalPolicyVersion[]> {
    return (await apiClient.get<{ data: ApprovalPolicyVersion[] }>(`/approval-policies/${enc(policyId)}/versions`)).data.data ?? [];
  },

  async listDelegations(params: { status?: string } = {}): Promise<ApprovalDelegation[]> {
    return (await apiClient.get<{ data: ApprovalDelegation[] }>(`/approval-delegations${query(params)}`)).data.data ?? [];
  },
  async getDelegation(delegationId: string): Promise<ApprovalDelegation> {
    return (await apiClient.get<{ data: ApprovalDelegation }>(`/approval-delegations/${enc(delegationId)}`)).data.data;
  },
  async createDelegation(input: ApprovalDelegationInput, idempotencyKey: string): Promise<ApprovalDelegation> {
    return (await apiClient.post<{ data: ApprovalDelegation }>('/approval-delegations', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async revokeDelegation(delegationId: string, reason: string, expectedVersion: number): Promise<ApprovalDelegation> {
    const res = await apiClient.post<{ data: ApprovalDelegation }>(
      `/approval-delegations/${enc(delegationId)}/revoke`,
      { reason },
      commandHeaders({ expectedVersion }),
    );
    return res.data.data;
  },
};

type UnitKind = 'cost-centres' | 'teams' | 'locations';
type UnitOf<K extends UnitKind> = K extends 'cost-centres' ? CostCentre : K extends 'teams' ? Team : OperatingLocation;

// Cost centres, teams and operating locations share one shape of API: list,
// create, edit (If-Match) and archive (If-Match).
export const structureUnitService = {
  async list<K extends UnitKind>(kind: K, params: { includeArchived?: boolean } = {}): Promise<UnitOf<K>[]> {
    const res = await apiClient.get<{ data: UnitOf<K>[] }>(`/${kind}${query({ includeArchived: params.includeArchived || undefined })}`);
    return res.data.data ?? [];
  },
  async get<K extends UnitKind>(kind: K, id: string): Promise<UnitOf<K>> {
    return (await apiClient.get<{ data: UnitOf<K> }>(`/${kind}/${enc(id)}`)).data.data;
  },
  async create<K extends UnitKind>(kind: K, input: Record<string, unknown>): Promise<UnitOf<K>> {
    return (await apiClient.post<{ data: UnitOf<K> }>(`/${kind}`, input)).data.data;
  },
  async update<K extends UnitKind>(kind: K, id: string, input: Record<string, unknown>, expectedVersion: number): Promise<UnitOf<K>> {
    return (await apiClient.patch<{ data: UnitOf<K> }>(`/${kind}/${enc(id)}`, input, commandHeaders({ expectedVersion }))).data.data;
  },
  async archive(kind: UnitKind, id: string, expectedVersion: number): Promise<void> {
    await apiClient.post(`/${kind}/${enc(id)}/archive`, {}, commandHeaders({ expectedVersion }));
  },
};

// Approval requests. Approve and reject need a fresh step-up grant
// (approvals.decide); apiClient runs the challenge. A decision carries a
// client decision id (stable across retries of one attempt) and the
// fingerprint the approver saw, so a changed request is refused.
export const approvalRequestService = {
  async list(params: { status?: string; resourceType?: string; action?: string; limit?: number } = {}): Promise<ApprovalRequest[]> {
    const res = await apiClient.get<{ data: { approvals: ApprovalRequest[] | null } }>(`/approvals/requests${query(params)}`);
    return res.data.data?.approvals ?? [];
  },
  async get(approvalId: string): Promise<ApprovalRequestDetail> {
    return (await apiClient.get<{ data: ApprovalRequestDetail }>(`/approvals/requests/${enc(approvalId)}`)).data.data;
  },
  async decisions(approvalId: string): Promise<ApprovalDecision[]> {
    const res = await apiClient.get<{ data: { decisions: ApprovalDecision[] | null } }>(`/approvals/requests/${enc(approvalId)}/decisions`);
    return res.data.data?.decisions ?? [];
  },
  async history(approvalId: string): Promise<ApprovalHistoryEntry[]> {
    const res = await apiClient.get<{ data: { history: ApprovalHistoryEntry[] | null } }>(`/approvals/requests/${enc(approvalId)}/history`);
    return res.data.data?.history ?? [];
  },
  async decide(
    approvalId: string,
    decision: 'approve' | 'reject',
    input: { reason: string; clientDecisionId: string; expectedFingerprint: string },
  ): Promise<ApprovalRequest> {
    const res = await apiClient.post<{ data: ApprovalRequest }>(`/approvals/requests/${enc(approvalId)}/${decision}`, input);
    return res.data.data;
  },
  async cancel(approvalId: string, reason: string): Promise<ApprovalRequest> {
    return (await apiClient.post<{ data: ApprovalRequest }>(`/approvals/requests/${enc(approvalId)}/cancel`, { reason })).data.data;
  },
};
