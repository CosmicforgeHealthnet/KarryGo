// lib/types/finance.ts

// ── Shared ────────────────────────────────────────────────────────────────

export interface MoneyView {
  amountMinor: number;
  currency: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  nextCursor?: string;
}

// ── financeops enums ──────────────────────────────────────────────────────

export type InvoiceStatus =
  | "DRAFT"
  | "PENDING_APPROVAL"
  | "APPROVED"
  | "ISSUED"
  | "PARTIALLY_PAID"
  | "PAID"
  | "OVERDUE"
  | "CANCELLED"
  | "CREDITED"
  | "WRITTEN_OFF";

export type ReceivableStatus =
  | "OPEN"
  | "PARTIALLY_PAID"
  | "PAID"
  | "OVERDUE"
  | "DISPUTED"
  | "WRITTEN_OFF"
  | "REVERSED";

export type CreditNoteStatus = "ISSUED" | "POSTING" | "POSTED" | "REVERSED";

export type ExpenseStatus =
  | "DRAFT"
  | "SUBMITTED"
  | "PENDING_APPROVAL"
  | "APPROVED"
  | "REJECTED"
  | "POSTING"
  | "POSTED"
  | "REVERSED"
  | "CANCELLED";

export type BudgetStatus =
  | "DRAFT"
  | "PENDING_APPROVAL"
  | "APPROVED"
  | "ACTIVE"
  | "CLOSED"
  | "CANCELLED";

export type ClaimType =
  | "INSURANCE"
  | "CARGO"
  | "VEHICLE"
  | "PROPERTY"
  | "PERSONAL_INJURY"
  | "SECURITY"
  | "OTHER";

export type ClaimStatus =
  | "DRAFT"
  | "SUBMITTED"
  | "UNDER_REVIEW"
  | "MORE_INFORMATION_REQUIRED"
  | "APPROVED"
  | "PARTIALLY_APPROVED"
  | "REJECTED"
  | "SETTLED"
  | "CLOSED"
  | "CANCELLED";

export type CompensationRuleType =
  | "SALARY"
  | "PER_JOB"
  | "PER_SHIFT"
  | "COMMISSION"
  | "ALLOWANCE"
  | "OVERTIME"
  | "BONUS"
  | "MANUAL_ADJUSTMENT"
  | "DEDUCTION";

export type CompensationRuleStatus =
  | "DRAFT"
  | "PENDING_APPROVAL"
  | "APPROVED"
  | "ACTIVE"
  | "INACTIVE"
  | "ARCHIVED";

export type PayrollStatus =
  | "DRAFT"
  | "CALCULATING"
  | "CALCULATED"
  | "PENDING_APPROVAL"
  | "APPROVED"
  | "POSTING"
  | "POSTED"
  | "PARTIALLY_POSTED"
  | "FAILED"
  | "REVERSED"
  | "CLOSED";

export type CompensationItemStatus =
  | "CALCULATED"
  | "REVIEW_REQUIRED"
  | "APPROVED"
  | "POSTING"
  | "POSTED"
  | "REVERSED";

export type CompensationItemSourceType =
  | "COMPLETED_ASSIGNMENT"
  | "PAYROLL_PERIOD"
  | "DECLINE"
  | "DECLINE_COUNT"
  | "DECLINE_RATE"
  | "SAFETY_REPORT"
  | "SAFETY_DECLINE"
  | "PERFORMANCE_SCORE";

export type SettlementStatus =
  | "DRAFT"
  | "CALCULATED"
  | "PENDING_APPROVAL"
  | "APPROVED"
  | "POSTING"
  | "POSTED"
  | "DISPUTED"
  | "REVERSED"
  | "CANCELLED";

// ── financeops rows ───────────────────────────────────────────────────────

export interface InvoiceRow {
  id: string;
  organization_id: string;
  branch_id: string | null;
  customer_id: string;
  contract_id: string | null;
  quotation_id: string | null;
  currency: string;
  subtotal_minor: number;
  tax_total_minor: number;
  adjustment_total_minor: number;
  total_minor: number;
  due_date: string;
  status: InvoiceStatus;
  issued_at: string | null;
  paid_at: string | null;
  cancelled_at: string | null;
  version: number;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface InvoiceLineRow {
  id: string;
  organization_id: string;
  invoice_id: string;
  source_type: string;
  source_reference: string;
  description: string;
  quantity: number;
  unit_amount_minor: number;
  line_total_minor: number;
  tax_reference: string;
  tax_amount_minor: number;
  adjustment_reference: string;
  created_at: string;
}

export interface InvoiceView extends InvoiceRow {
  lines: InvoiceLineRow[];
}

export interface CreditNoteRow {
  id: string;
  organization_id: string;
  invoice_id: string;
  currency: string;
  amount_minor: number;
  affected_lines: unknown[];
  reason: string;
  status: CreditNoteStatus;
  created_by: string;
  version: number;
  created_at: string;
  posting_reference: string;
}

export interface ReceivableRow {
  id: string;
  organization_id: string;
  invoice_id: string;
  entry_type: "INVOICE";
  original_amount_minor: number;
  paid_amount_minor: number;
  outstanding_amount_minor: number;
  currency: string;
  due_at: string;
  status: ReceivableStatus;
  payment_reference: string;
  wallet_operation_reference: string;
  accounting_reference: string;
  source_version: number;
  created_at: string;
  updated_at: string;
}

export interface ExpenseRow {
  id: string;
  organization_id: string;
  branch_id: string | null;
  category: string;
  cost_centre: string;
  asset_id: string | null;
  provider_membership_id: string | null;
  order_id: string | null;
  shipment_id: string | null;
  work_order_id: string | null;
  incident_id: string | null;
  amount_minor: number;
  currency: string;
  receipt_media_id: string;
  description: string;
  status: ExpenseStatus;
  submitted_by: string;
  approved_by: string;
  approved_at: string | null;
  rejected_at: string | null;
  posting_reference: string;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface ExpenseHistoryRow {
  id: string;
  organization_id: string;
  expense_id: string;
  previous_status: string;
  new_status: string;
  snapshot: Record<string, unknown>;
  actor_reference: string;
  reason: string;
  version: number;
  occurred_at: string;
}

export interface ExpenseReversalRow {
  id: string;
  organization_id: string;
  expense_id: string;
  amount_minor: number;
  currency: string;
  reason: string;
  evidence_media_id: string;
  posting_operation_id: string | null;
  created_by: string;
  created_at: string;
}

export interface BudgetRow {
  id: string;
  organization_id: string;
  cost_centre: string;
  category: string;
  period_start: string;
  period_end: string;
  currency: string;
  approved_amount_minor: number;
  committed_amount_projection: number;
  actual_amount_projection: number;
  status: BudgetStatus;
  approved_by: string;
  approved_at: string | null;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface ClaimRow {
  id: string;
  organization_id: string;
  claim_type: ClaimType;
  incident_id: string | null;
  asset_id: string | null;
  shipment_id: string | null;
  package_id: string | null;
  policy_reference: string;
  amount_reference_minor: number | null;
  currency: string;
  status: ClaimStatus;
  external_claim_reference: string;
  evidence_media_ids: string[];
  submitted_at: string | null;
  approved_amount_minor: number | null;
  settlement_reference: string;
  owner_user_id: string;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface ClaimHistoryRow {
  id: string;
  organization_id: string;
  claim_id: string;
  previous_status: string;
  new_status: string;
  snapshot: Record<string, unknown>;
  actor_reference: string;
  source_type: string;
  source_reference: string;
  source_version: number;
  occurred_at: string;
}

export interface CompensationRuleRow {
  id: string;
  organization_id: string;
  name: string;
  rule_type: CompensationRuleType;
  applicable_roles: unknown[];
  applicable_services: unknown[];
  calculation_configuration: {
    amountMinor: number;
    approvedBasis?: string;
    [key: string]: unknown;
  };
  currency: string;
  effective_from: string;
  effective_until: string | null;
  status: CompensationRuleStatus;
  version: number;
  approved_by: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface CompensationRuleVersionRow {
  id: string;
  organization_id: string;
  rule_id: string;
  rule_version: number;
  snapshot: Record<string, unknown>;
  actor_reference: string;
  created_at: string;
}

export interface PayrollPeriodRow {
  id: string;
  organization_id: string;
  branch_id: string | null;
  period_start: string;
  period_end: string;
  currency: string;
  status: PayrollStatus;
  calculation_version: number;
  calculated_at: string | null;
  approved_by: string;
  approved_at: string | null;
  posted_at: string | null;
  export_reference: string;
  wallet_operation_reference: string;
  version: number;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface ProviderCompensationItemRow {
  id: string;
  organization_id: string;
  payroll_period_id: string;
  membership_id: string;
  source_type: CompensationItemSourceType;
  source_reference: string;
  rule_id: string;
  rule_version: number;
  calculation_version: number;
  amount_minor: number;
  currency: string;
  reason_code: string;
  explanation: string;
  source_inputs: Record<string, unknown>;
  status: CompensationItemStatus;
  adjustment_of: string | null;
  created_at: string;
}

export interface PayrollPeriodView extends PayrollPeriodRow {
  items: ProviderCompensationItemRow[];
}

export interface SubcontractorSettlementRow {
  id: string;
  organization_id: string;
  supplier_reference: string;
  provider_or_carrier_reference: string;
  period_start: string;
  period_end: string;
  currency: string;
  amount_minor: number;
  status: SettlementStatus;
  approved_by: string;
  posting_reference: string;
  payment_reference: string;
  version: number;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface SubcontractorSettlementView extends SubcontractorSettlementRow {
  job_references: string[];
}

export interface ProviderStatementPeriod {
  period: PayrollPeriodRow;
  items: ProviderCompensationItemRow[];
}

export interface ProviderStatement {
  membershipId: string;
  periods: ProviderStatementPeriod[];
}

// ── operator enums ────────────────────────────────────────────────────────

export type OperationStatus =
  | "PENDING"
  | "PROCESSING"
  | "UNKNOWN"
  | "RECONCILING"
  | "SUCCEEDED"
  | "RETRYABLE_FAILURE"
  | "PERMANENT_FAILURE"
  | "DEAD_LETTER";

export type OperationType =
  | "SUBSCRIPTION_INVOICE"
  | "EARNING_ALLOCATION"
  | "SETTLEMENT_REQUEST"
  | "HOLD_CREATE"
  | "HOLD_RELEASE"
  | "COMMISSION_POST"
  | "FLEET_FEE_POST"
  | "REFUND_REQUEST"
  | "REVERSAL_REQUEST"
  | "EXPENSE_POST"
  | "EXPENSE_REVERSAL"
  | "PAYROLL_REVERSAL"
  | "PROVIDER_ALLOCATION"
  | "MANAGED_WORK_SETTLEMENT";

export type ProjectionType =
  | "EARNING_ACCOUNT"
  | "SUBSCRIPTION"
  | "INVOICE"
  | "HOLD"
  | "COMMISSION"
  | "EARNING"
  | "SETTLEMENT"
  | "REFUND"
  | "REVERSAL"
  | "WALLET"
  | "PAYMENT_VERIFICATION";

export type FreshnessState =
  | "CURRENT"
  | "STALE"
  | "UNKNOWN"
  | "GAP_DETECTED"
  | "RECONCILING";

export type ReconciliationStatus =
  | "IDLE"
  | "PENDING"
  | "PROCESSING"
  | "RETRYABLE_FAILURE"
  | "UNKNOWN"
  | "DEAD_LETTER";

// ── operator rows ─────────────────────────────────────────────────────────

export interface Operation {
  id: string;
  organization_id: string;
  branch_id: string;
  subject_type: string;
  subject_id: string;
  operation_type: OperationType;
  payload: Record<string, unknown>;
  payload_version: number;
  expected_version: number;
  idempotency_key: string;
  request_fingerprint: string;
  wallet_reference: string;
  status: OperationStatus;
  attempts: number;
  next_attempt_at: string;
  lease_owner: string;
  lease_expires_at: string | null;
  safe_error_code: string;
  correlation_id: string;
  causation_id: string;
  requested_by_actor_type: string;
  requested_by_actor_id: string;
  version: number;
}

export interface Projection {
  id: string;
  organization_id: string;
  projection_type: ProjectionType;
  subject_type: string;
  subject_id: string;
  wallet_reference: string;
  amount_minor: number | null;
  currency: string;
  safe_status: string;
  source_version: number;
  freshness_state: FreshnessState;
  last_event_id: string;
  last_synced_at: string;
  reconciliation_status: ReconciliationStatus;
  reconciliation_attempts: number;
  reconciliation_next_attempt_at: string;
  reconciliation_lease_owner: string;
  reconciliation_lease_expires_at: string | null;
  reconciliation_safe_error_code: string;
  version: number;
}

export interface OperationHistoryEntry {
  id: string;
  organization_id: string;
  operation_id: string;
  previous_status: string;
  new_status: string;
  safe_reason_code: string;
  actor_type: string;
  actor_id: string;
  occurred_at: string;
}

export interface OperationAuditEntry {
  id: string;
  action: string;
  actor_type: string;
  actor_id: string;
  occurred_at: string;
  [key: string]: unknown;
}

export interface OperationAcceptedResponse {
  id: string;
  operationType: OperationType;
  status: OperationStatus;
  subjectId: string;
  version: number;
  nextAttemptAt: string;
}

// ── Input DTOs ────────────────────────────────────────────────────────────

export interface InvoiceLineInput {
  sourceType: string;
  sourceReference: string;
  description: string;
  quantity: number;
  unitAmountMinor: number;
  taxAmountMinor: number;
  taxReference?: string;
  adjustmentReference?: string;
}

export interface CreateInvoiceInput {
  branchId?: string;
  customerId: string;
  contractId?: string;
  quotationId?: string;
  currency: string;
  dueDate: string;
  lines: InvoiceLineInput[];
}

export interface UpdateInvoiceInput {
  branchId?: string;
  customerId: string;
  contractId?: string;
  quotationId?: string;
  currency: string;
  dueDate: string;
  lines: InvoiceLineInput[];
}

export interface CreditNoteInput {
  amountMinor: number;
  affectedLines?: unknown[];
  reason: string;
}

export interface ExpenseInput {
  branchId?: string;
  category: string;
  costCentre: string;
  assetId?: string;
  providerMembershipId?: string;
  orderId?: string;
  shipmentId?: string;
  workOrderId?: string;
  incidentId?: string;
  amountMinor: number;
  currency: string;
  receiptMediaId?: string;
  description: string;
}

export interface BudgetInput {
  costCentre: string;
  category: string;
  currency: string;
  periodStart: string;
  periodEnd: string;
  approvedAmountMinor: number;
}

export interface CompensationRuleInput {
  name: string;
  ruleType: CompensationRuleType;
  currency: string;
  applicableRoles?: unknown[];
  applicableServices?: unknown[];
  calculationConfiguration: {
    amountMinor: number;
    approvedBasis?: string;
    [key: string]: unknown;
  };
  effectiveFrom: string;
  effectiveUntil?: string | null;
}

export interface PayrollPeriodInput {
  branchId?: string;
  currency: string;
  periodStart: string;
  periodEnd: string;
}

export interface SettlementInput {
  supplierReference: string;
  providerOrCarrierReference: string;
  currency: string;
  periodStart: string;
  periodEnd: string;
  jobReferences: string[];
  amountMinor: number;
}

export interface ClaimInput {
  claimType: ClaimType;
  incidentId?: string;
  assetId?: string;
  shipmentId?: string;
  packageId?: string;
  policyReference?: string;
  amountReferenceMinor?: number | null;
  currency: string;
  evidenceMediaIds?: string[];
  externalClaimReference?: string;
  settlementReference?: string;
  approvedAmountMinor?: number | null;
  ownerUserId?: string;
  status?: ClaimStatus;
}

export interface CommandBody {
  expectedVersion?: number;
  reason?: string;
}

export interface RequestSettlementInput {
  beneficiaryType: "PROVIDER" | "COMPANY";
  beneficiaryReference?: string;
  earningAccountId?: string;
  walletId?: string;
  branchId?: string;
  currency: string;
  expectedVersion?: number;
}

export interface RequestRefundInput {
  originalJournalId: string;
  paymentReference?: string;
  branchId?: string;
  amountMinor: number;
  currency: string;
  reasonCode: string;
  reason?: string;
  expectedVersion?: number;
}

export interface RequestReversalInput {
  originalJournalId: string;
  sourceOperationId?: string;
  branchId?: string;
  amountMinor: number;
  currency: string;
  reasonCode: string;
  reason?: string;
  expectedVersion?: number;
}