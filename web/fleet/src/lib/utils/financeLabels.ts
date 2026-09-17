// lib/utils/financeLabels.ts

import type {
  BudgetStatus,
  ClaimStatus,
  ClaimType,
  CompensationItemSourceType,
  CompensationItemStatus,
  CompensationRuleStatus,
  CompensationRuleType,
  CreditNoteStatus,
  ExpenseStatus,
  FreshnessState,
  InvoiceStatus,
  OperationStatus,
  OperationType,
  PayrollStatus,
  ProjectionType,
  ReceivableStatus,
  ReconciliationStatus,
  SettlementStatus,
} from "@/lib/types/finance";

export type Tone = "blue" | "green" | "amber" | "red" | "neutral" | "purple";

export interface ToneClasses {
  bg: string;
  text: string;
  dot: string;
}

export const TONES: Record<Tone, ToneClasses> = {
  blue:    { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  green:   { bg: "bg-green-bg",   text: "text-green",   dot: "bg-green"   },
  amber:   { bg: "bg-amber-bg",   text: "text-amber",   dot: "bg-amber"   },
  red:     { bg: "bg-red-bg",     text: "text-red",     dot: "bg-red"     },
  neutral: { bg: "bg-surface-2",  text: "text-ink-500", dot: "bg-ink-400" },
  purple:  { bg: "bg-surface-2",  text: "text-ink-700", dot: "bg-ink-500" },
};

const pick = <T extends string>(
  map: Record<string, { label: string; tone: Tone }>,
  key: T | string
): { label: string; tone: Tone } =>
  map[key] ?? { label: key, tone: "neutral" };

// ── Invoice ───────────────────────────────────────────────────────────────

const INVOICE_STATUS: Record<InvoiceStatus, { label: string; tone: Tone }> = {
  DRAFT:            { label: "Draft",            tone: "blue"    },
  PENDING_APPROVAL: { label: "Pending approval", tone: "amber"   },
  APPROVED:         { label: "Approved",         tone: "blue"    },
  ISSUED:           { label: "Issued",           tone: "amber"   },
  PARTIALLY_PAID:   { label: "Partially paid",   tone: "amber"   },
  PAID:             { label: "Paid",             tone: "green"   },
  OVERDUE:          { label: "Overdue",          tone: "red"     },
  CANCELLED:        { label: "Cancelled",        tone: "neutral" },
  CREDITED:         { label: "Credited",         tone: "purple"  },
  WRITTEN_OFF:      { label: "Written off",      tone: "neutral" },
};

export const invoiceStatusLabel = (s: InvoiceStatus | string): string =>
  pick(INVOICE_STATUS, s).label;
export const invoiceStatusTone = (s: InvoiceStatus | string): ToneClasses =>
  TONES[pick(INVOICE_STATUS, s).tone];

// ── Receivable ────────────────────────────────────────────────────────────

const RECEIVABLE_STATUS: Record<ReceivableStatus, { label: string; tone: Tone }> = {
  OPEN:           { label: "Open",           tone: "blue"    },
  PARTIALLY_PAID: { label: "Partially paid", tone: "amber"   },
  PAID:           { label: "Paid",           tone: "green"   },
  OVERDUE:        { label: "Overdue",        tone: "red"     },
  DISPUTED:       { label: "Disputed",       tone: "amber"   },
  WRITTEN_OFF:    { label: "Written off",    tone: "neutral" },
  REVERSED:       { label: "Reversed",       tone: "neutral" },
};

export const receivableStatusLabel = (s: ReceivableStatus | string): string =>
  pick(RECEIVABLE_STATUS, s).label;
export const receivableStatusTone = (s: ReceivableStatus | string): ToneClasses =>
  TONES[pick(RECEIVABLE_STATUS, s).tone];

// ── Credit note ───────────────────────────────────────────────────────────

const CREDIT_NOTE_STATUS: Record<CreditNoteStatus, { label: string; tone: Tone }> = {
  ISSUED:   { label: "Issued",   tone: "blue"    },
  POSTING:  { label: "Posting",  tone: "amber"   },
  POSTED:   { label: "Posted",   tone: "green"   },
  REVERSED: { label: "Reversed", tone: "neutral" },
};

export const creditNoteStatusLabel = (s: CreditNoteStatus | string): string =>
  pick(CREDIT_NOTE_STATUS, s).label;
export const creditNoteStatusTone = (s: CreditNoteStatus | string): ToneClasses =>
  TONES[pick(CREDIT_NOTE_STATUS, s).tone];

// ── Expense ───────────────────────────────────────────────────────────────

const EXPENSE_STATUS: Record<ExpenseStatus, { label: string; tone: Tone }> = {
  DRAFT:            { label: "Draft",            tone: "blue"    },
  SUBMITTED:        { label: "Submitted",        tone: "amber"   },
  PENDING_APPROVAL: { label: "Pending approval", tone: "amber"   },
  APPROVED:         { label: "Approved",         tone: "green"   },
  REJECTED:         { label: "Rejected",         tone: "red"     },
  POSTING:          { label: "Posting",          tone: "amber"   },
  POSTED:           { label: "Posted",           tone: "green"   },
  REVERSED:         { label: "Reversed",         tone: "neutral" },
  CANCELLED:        { label: "Cancelled",        tone: "neutral" },
};

export const expenseStatusLabel = (s: ExpenseStatus | string): string =>
  pick(EXPENSE_STATUS, s).label;
export const expenseStatusTone = (s: ExpenseStatus | string): ToneClasses =>
  TONES[pick(EXPENSE_STATUS, s).tone];

// ── Budget ────────────────────────────────────────────────────────────────

const BUDGET_STATUS: Record<BudgetStatus, { label: string; tone: Tone }> = {
  DRAFT:            { label: "Draft",            tone: "blue"    },
  PENDING_APPROVAL: { label: "Pending approval", tone: "amber"   },
  APPROVED:         { label: "Approved",         tone: "blue"    },
  ACTIVE:           { label: "Active",           tone: "green"   },
  CLOSED:           { label: "Closed",           tone: "neutral" },
  CANCELLED:        { label: "Cancelled",        tone: "neutral" },
};

export const budgetStatusLabel = (s: BudgetStatus | string): string =>
  pick(BUDGET_STATUS, s).label;
export const budgetStatusTone = (s: BudgetStatus | string): ToneClasses =>
  TONES[pick(BUDGET_STATUS, s).tone];

// ── Claim ─────────────────────────────────────────────────────────────────

const CLAIM_TYPE: Record<ClaimType, { label: string; tone: Tone }> = {
  INSURANCE:       { label: "Insurance",       tone: "blue"    },
  CARGO:           { label: "Cargo",           tone: "amber"   },
  VEHICLE:         { label: "Vehicle",         tone: "purple"  },
  PROPERTY:        { label: "Property",        tone: "blue"    },
  PERSONAL_INJURY: { label: "Personal injury", tone: "red"     },
  SECURITY:        { label: "Security",        tone: "red"     },
  OTHER:           { label: "Other",           tone: "neutral" },
};

export const claimTypeLabel = (t: ClaimType | string): string =>
  pick(CLAIM_TYPE, t).label;
export const claimTypeTone = (t: ClaimType | string): ToneClasses =>
  TONES[pick(CLAIM_TYPE, t).tone];

const CLAIM_STATUS: Record<ClaimStatus, { label: string; tone: Tone }> = {
  DRAFT:                     { label: "Draft",                     tone: "blue"    },
  SUBMITTED:                 { label: "Submitted",                 tone: "amber"   },
  UNDER_REVIEW:              { label: "Under review",              tone: "amber"   },
  MORE_INFORMATION_REQUIRED: { label: "Info required",             tone: "amber"   },
  APPROVED:                  { label: "Approved",                  tone: "green"   },
  PARTIALLY_APPROVED:        { label: "Partially approved",        tone: "amber"   },
  REJECTED:                  { label: "Rejected",                  tone: "red"     },
  SETTLED:                   { label: "Settled",                   tone: "green"   },
  CLOSED:                    { label: "Closed",                    tone: "neutral" },
  CANCELLED:                 { label: "Cancelled",                 tone: "neutral" },
};

export const claimStatusLabel = (s: ClaimStatus | string): string =>
  pick(CLAIM_STATUS, s).label;
export const claimStatusTone = (s: ClaimStatus | string): ToneClasses =>
  TONES[pick(CLAIM_STATUS, s).tone];

// ── Compensation rule ─────────────────────────────────────────────────────

const COMP_RULE_TYPE: Record<CompensationRuleType, { label: string; tone: Tone }> = {
  SALARY:            { label: "Salary",            tone: "blue"    },
  PER_JOB:           { label: "Per job",           tone: "green"   },
  PER_SHIFT:         { label: "Per shift",         tone: "blue"    },
  COMMISSION:        { label: "Commission",        tone: "purple"  },
  ALLOWANCE:         { label: "Allowance",         tone: "blue"    },
  OVERTIME:          { label: "Overtime",          tone: "amber"   },
  BONUS:             { label: "Bonus",             tone: "green"   },
  MANUAL_ADJUSTMENT: { label: "Manual adjustment", tone: "amber"   },
  DEDUCTION:         { label: "Deduction",         tone: "red"     },
};

export const compensationRuleTypeLabel = (t: CompensationRuleType | string): string =>
  pick(COMP_RULE_TYPE, t).label;
export const compensationRuleTypeTone = (t: CompensationRuleType | string): ToneClasses =>
  TONES[pick(COMP_RULE_TYPE, t).tone];

const COMP_RULE_STATUS: Record<CompensationRuleStatus, { label: string; tone: Tone }> = {
  DRAFT:            { label: "Draft",            tone: "blue"    },
  PENDING_APPROVAL: { label: "Pending approval", tone: "amber"   },
  APPROVED:         { label: "Approved",         tone: "green"   },
  ACTIVE:           { label: "Active",           tone: "green"   },
  INACTIVE:         { label: "Inactive",         tone: "neutral" },
  ARCHIVED:         { label: "Archived",         tone: "neutral" },
};

export const compensationRuleStatusLabel = (s: CompensationRuleStatus | string): string =>
  pick(COMP_RULE_STATUS, s).label;
export const compensationRuleStatusTone = (s: CompensationRuleStatus | string): ToneClasses =>
  TONES[pick(COMP_RULE_STATUS, s).tone];

// ── Payroll ───────────────────────────────────────────────────────────────

const PAYROLL_STATUS: Record<PayrollStatus, { label: string; tone: Tone }> = {
  DRAFT:            { label: "Draft",            tone: "blue"    },
  CALCULATING:      { label: "Calculating",      tone: "amber"   },
  CALCULATED:       { label: "Calculated",       tone: "amber"   },
  PENDING_APPROVAL: { label: "Pending approval", tone: "amber"   },
  APPROVED:         { label: "Approved",         tone: "green"   },
  POSTING:          { label: "Posting",          tone: "amber"   },
  POSTED:           { label: "Posted",           tone: "green"   },
  PARTIALLY_POSTED: { label: "Partially posted", tone: "amber"   },
  FAILED:           { label: "Failed",           tone: "red"     },
  REVERSED:         { label: "Reversed",         tone: "neutral" },
  CLOSED:           { label: "Closed",           tone: "neutral" },
};

export const payrollStatusLabel = (s: PayrollStatus | string): string =>
  pick(PAYROLL_STATUS, s).label;
export const payrollStatusTone = (s: PayrollStatus | string): ToneClasses =>
  TONES[pick(PAYROLL_STATUS, s).tone];

const COMP_ITEM_STATUS: Record<CompensationItemStatus, { label: string; tone: Tone }> = {
  CALCULATED:      { label: "Calculated",      tone: "amber"   },
  REVIEW_REQUIRED: { label: "Review required", tone: "amber"   },
  APPROVED:        { label: "Approved",        tone: "green"   },
  POSTING:         { label: "Posting",         tone: "amber"   },
  POSTED:          { label: "Posted",          tone: "green"   },
  REVERSED:        { label: "Reversed",        tone: "neutral" },
};

export const compensationItemStatusLabel = (s: CompensationItemStatus | string): string =>
  pick(COMP_ITEM_STATUS, s).label;
export const compensationItemStatusTone = (s: CompensationItemStatus | string): ToneClasses =>
  TONES[pick(COMP_ITEM_STATUS, s).tone];

const COMP_ITEM_SOURCE: Record<CompensationItemSourceType, string> = {
  COMPLETED_ASSIGNMENT: "Completed assignment",
  PAYROLL_PERIOD:       "Payroll period",
  DECLINE:              "Decline",
  DECLINE_COUNT:        "Decline count",
  DECLINE_RATE:         "Decline rate",
  SAFETY_REPORT:        "Safety report",
  SAFETY_DECLINE:       "Safety decline",
  PERFORMANCE_SCORE:    "Performance score",
};

export const compensationItemSourceLabel = (s: CompensationItemSourceType | string): string =>
  COMP_ITEM_SOURCE[s as CompensationItemSourceType] ?? s;

// ── Settlement ────────────────────────────────────────────────────────────

const SETTLEMENT_STATUS: Record<SettlementStatus, { label: string; tone: Tone }> = {
  DRAFT:            { label: "Draft",            tone: "blue"    },
  CALCULATED:       { label: "Calculated",       tone: "amber"   },
  PENDING_APPROVAL: { label: "Pending approval", tone: "amber"   },
  APPROVED:         { label: "Approved",         tone: "green"   },
  POSTING:          { label: "Posting",          tone: "amber"   },
  POSTED:           { label: "Posted",           tone: "green"   },
  DISPUTED:         { label: "Disputed",         tone: "amber"   },
  REVERSED:         { label: "Reversed",         tone: "neutral" },
  CANCELLED:        { label: "Cancelled",        tone: "neutral" },
};

export const settlementStatusLabel = (s: SettlementStatus | string): string =>
  pick(SETTLEMENT_STATUS, s).label;
export const settlementStatusTone = (s: SettlementStatus | string): ToneClasses =>
  TONES[pick(SETTLEMENT_STATUS, s).tone];

// ── Operator: operation ───────────────────────────────────────────────────

const OPERATION_STATUS: Record<OperationStatus, { label: string; tone: Tone }> = {
  PENDING:           { label: "Pending",           tone: "blue"    },
  PROCESSING:        { label: "Processing",        tone: "amber"   },
  UNKNOWN:           { label: "Unknown",           tone: "amber"   },
  RECONCILING:       { label: "Reconciling",       tone: "purple"  },
  SUCCEEDED:         { label: "Succeeded",         tone: "green"   },
  RETRYABLE_FAILURE: { label: "Retrying",          tone: "amber"   },
  PERMANENT_FAILURE: { label: "Failed",            tone: "red"     },
  DEAD_LETTER:       { label: "Dead letter",       tone: "red"     },
};

export const operationStatusLabel = (s: OperationStatus | string): string =>
  pick(OPERATION_STATUS, s).label;
export const operationStatusTone = (s: OperationStatus | string): ToneClasses =>
  TONES[pick(OPERATION_STATUS, s).tone];

const OPERATION_TYPE: Record<OperationType, string> = {
  SUBSCRIPTION_INVOICE:    "Subscription invoice",
  EARNING_ALLOCATION:      "Earning allocation",
  SETTLEMENT_REQUEST:      "Settlement",
  HOLD_CREATE:             "Hold create",
  HOLD_RELEASE:            "Hold release",
  COMMISSION_POST:         "Commission post",
  FLEET_FEE_POST:          "Fleet fee post",
  REFUND_REQUEST:          "Refund",
  REVERSAL_REQUEST:        "Reversal",
  EXPENSE_POST:            "Expense post",
  EXPENSE_REVERSAL:        "Expense reversal",
  PAYROLL_REVERSAL:        "Payroll reversal",
  PROVIDER_ALLOCATION:     "Provider allocation",
  MANAGED_WORK_SETTLEMENT: "Managed work settlement",
};

export const operationTypeLabel = (t: OperationType | string): string =>
  OPERATION_TYPE[t as OperationType] ?? t;

// ── Operator: projection ──────────────────────────────────────────────────

const PROJECTION_TYPE: Record<ProjectionType, string> = {
  EARNING_ACCOUNT:     "Earning account",
  SUBSCRIPTION:        "Subscription",
  INVOICE:             "Invoice",
  HOLD:                "Hold",
  COMMISSION:          "Commission",
  EARNING:             "Earning",
  SETTLEMENT:          "Settlement",
  REFUND:              "Refund",
  REVERSAL:            "Reversal",
  WALLET:              "Wallet",
  PAYMENT_VERIFICATION: "Payment verification",
};

export const projectionTypeLabel = (t: ProjectionType | string): string =>
  PROJECTION_TYPE[t as ProjectionType] ?? t;

const FRESHNESS: Record<FreshnessState, { label: string; tone: Tone }> = {
  CURRENT:      { label: "Current",      tone: "green"   },
  STALE:        { label: "Stale",        tone: "amber"   },
  UNKNOWN:      { label: "Unknown",      tone: "neutral" },
  GAP_DETECTED: { label: "Gap detected", tone: "red"     },
  RECONCILING:  { label: "Reconciling",  tone: "purple"  },
};

export const freshnessLabel = (f: FreshnessState | string): string =>
  pick(FRESHNESS, f).label;
export const freshnessTone = (f: FreshnessState | string): ToneClasses =>
  TONES[pick(FRESHNESS, f).tone];

const RECONCILIATION: Record<ReconciliationStatus, { label: string; tone: Tone }> = {
  IDLE:              { label: "Idle",       tone: "green"   },
  PENDING:           { label: "Pending",    tone: "amber"   },
  PROCESSING:        { label: "Processing", tone: "amber"   },
  RETRYABLE_FAILURE: { label: "Retrying",   tone: "amber"   },
  UNKNOWN:           { label: "Unknown",    tone: "amber"   },
  DEAD_LETTER:       { label: "Dead letter", tone: "red"   },
};

export const reconciliationStatusLabel = (s: ReconciliationStatus | string): string =>
  pick(RECONCILIATION, s).label;
export const reconciliationStatusTone = (s: ReconciliationStatus | string): ToneClasses =>
  TONES[pick(RECONCILIATION, s).tone];

// ── Formatters ────────────────────────────────────────────────────────────

export const formatMoney = (
  money: { amountMinor: number; currency: string } | undefined | null,
  locale = "en-NG"
): string => {
  if (!money) return "—";
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: money.currency,
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(money.amountMinor / 100);
};

export const formatMinor = (
  amountMinor: number | undefined | null,
  currency: string,
  locale = "en-NG"
): string => {
  if (amountMinor === undefined || amountMinor === null) return "—";
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency,
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(amountMinor / 100);
};

export const majorToMinor = (major: number): number =>
  Math.round(major * 100);

export const minorToMajor = (minor: number): number => minor / 100;

export const formatDate = (iso: string | null | undefined): string => {
  if (!iso) return "—";
  const d = new Date(iso);
  return Number.isFinite(d.getTime())
    ? d.toLocaleDateString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric",
      })
    : "—";
};

export const formatDateTime = (iso: string | null | undefined): string => {
  if (!iso) return "—";
  const d = new Date(iso);
  return Number.isFinite(d.getTime())
    ? d.toLocaleString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "—";
};

export const timeAgo = (iso: string | null | undefined): string => {
  if (!iso) return "—";
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return "—";
  const sec = Math.max(1, Math.floor((Date.now() - then) / 1000));
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const day = Math.floor(hr / 24);
  if (day < 30) return `${day}d ago`;
  const mo = Math.floor(day / 30);
  if (mo < 12) return `${mo}mo ago`;
  return `${Math.floor(mo / 12)}y ago`;
};

export const initialsOf = (value: string | null | undefined): string => {
  if (!value) return "—";
  const parts = value.trim().split(/\s+/).slice(0, 2);
  const initials = parts.map((p) => p[0]?.toUpperCase() ?? "").join("");
  return initials || "—";
};