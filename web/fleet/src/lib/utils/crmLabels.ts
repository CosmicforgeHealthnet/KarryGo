// lib/utils/crmLabels.ts

import type {
  AcceptanceMethod,
  ConsentStatus,
  ContactChannel,
  ContactPurpose,
  ContractStatus,
  CreditHoldStatus,
  CustomerStatus,
  CustomerType,
  LocationType,
  NotificationStatus,
  OpportunityStage,
  QuotationStatus,
  RecordStatus,
  RiskStatus,
  SLAMeasurementUnit,
  SLAMeasurementWindow,
  SLAMetricCode,
  SLAObservationOutcome,
  SLAStatus,
} from "@/lib/types/crm";

// ── Tone system ───────────────────────────────────────────────────────────

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

export const toneClasses = (t: Tone): ToneClasses => TONES[t];

// ── Customer ──────────────────────────────────────────────────────────────

const CUSTOMER_TYPE_LABEL: Record<CustomerType, string> = {
  INDIVIDUAL: "Individual",
  BUSINESS: "Business",
  GOVERNMENT: "Government",
  NONPROFIT: "Non-profit",
  INTERNAL_BUSINESS_UNIT: "Internal unit",
  OTHER: "Other",
};

export const customerTypeLabel = (t: CustomerType | string): string =>
  CUSTOMER_TYPE_LABEL[t as CustomerType] ?? t;

const CUSTOMER_STATUS_META: Record<CustomerStatus, { label: string; tone: Tone }> = {
  PROSPECT:  { label: "Prospect",  tone: "blue"    },
  ACTIVE:    { label: "Active",    tone: "green"   },
  ON_HOLD:   { label: "On hold",   tone: "amber"   },
  SUSPENDED: { label: "Suspended", tone: "red"     },
  INACTIVE:  { label: "Inactive",  tone: "neutral" },
  ARCHIVED:  { label: "Archived",  tone: "neutral" },
};

export const customerStatusLabel = (s: CustomerStatus | string): string =>
  CUSTOMER_STATUS_META[s as CustomerStatus]?.label ?? s;

export const customerStatusTone = (s: CustomerStatus | string): ToneClasses =>
  TONES[CUSTOMER_STATUS_META[s as CustomerStatus]?.tone ?? "neutral"];

const RISK_STATUS_META: Record<RiskStatus, { label: string; tone: Tone }> = {
  NOT_ASSESSED: { label: "Not assessed", tone: "neutral" },
  LOW:          { label: "Low",          tone: "green"   },
  MEDIUM:       { label: "Medium",       tone: "amber"   },
  HIGH:         { label: "High",         tone: "red"     },
  RESTRICTED:   { label: "Restricted",   tone: "red"     },
};

export const riskStatusLabel = (s: RiskStatus | string): string =>
  RISK_STATUS_META[s as RiskStatus]?.label ?? s;

export const riskStatusTone = (s: RiskStatus | string): ToneClasses =>
  TONES[RISK_STATUS_META[s as RiskStatus]?.tone ?? "neutral"];

// ── Contact ───────────────────────────────────────────────────────────────

const CONTACT_PURPOSE_LABEL: Record<ContactPurpose, string> = {
  GENERAL: "General",
  OPERATIONS: "Operations",
  BILLING: "Billing",
  LEGAL: "Legal",
  SALES: "Sales",
  EMERGENCY: "Emergency",
  OTHER: "Other",
};

export const contactPurposeLabel = (p: ContactPurpose | string): string =>
  CONTACT_PURPOSE_LABEL[p as ContactPurpose] ?? p;

const CHANNEL_LABEL: Record<ContactChannel, string> = {
  EMAIL: "Email",
  PHONE: "Phone",
  SMS: "SMS",
  WHATSAPP: "WhatsApp",
  IN_APP: "In-app",
  NONE: "None",
};

export const channelLabel = (c: ContactChannel | string): string =>
  CHANNEL_LABEL[c as ContactChannel] ?? c;

const CONSENT_META: Record<ConsentStatus, { label: string; tone: Tone }> = {
  UNKNOWN:      { label: "Pending",      tone: "amber"   },
  GRANTED:      { label: "Consented",    tone: "green"   },
  WITHDRAWN:    { label: "Opted out",    tone: "red"     },
  NOT_REQUIRED: { label: "Not required", tone: "neutral" },
};

export const consentLabel = (s: ConsentStatus | string): string =>
  CONSENT_META[s as ConsentStatus]?.label ?? s;

export const consentTone = (s: ConsentStatus | string): ToneClasses =>
  TONES[CONSENT_META[s as ConsentStatus]?.tone ?? "neutral"];

const RECORD_STATUS_META: Record<RecordStatus, { label: string; tone: Tone }> = {
  ACTIVE:   { label: "Active",   tone: "green"   },
  INACTIVE: { label: "Inactive", tone: "neutral" },
  ARCHIVED: { label: "Archived", tone: "neutral" },
};

export const recordStatusLabel = (s: RecordStatus | string): string =>
  RECORD_STATUS_META[s as RecordStatus]?.label ?? s;

export const recordStatusTone = (s: RecordStatus | string): ToneClasses =>
  TONES[RECORD_STATUS_META[s as RecordStatus]?.tone ?? "neutral"];

// ── Location ──────────────────────────────────────────────────────────────

const LOCATION_TYPE_LABEL: Record<LocationType, string> = {
  PICKUP: "Pickup",
  DELIVERY: "Delivery",
  BILLING: "Billing",
  WAREHOUSE: "Warehouse",
  OFFICE: "Office",
  DEPOT: "Depot",
  SITE: "Site",
  PORT: "Port",
  TERMINAL: "Terminal",
  OTHER: "Other",
};

export const locationTypeLabel = (t: LocationType | string): string =>
  LOCATION_TYPE_LABEL[t as LocationType] ?? t;

// ── Opportunity ───────────────────────────────────────────────────────────

const OPPORTUNITY_STAGE_META: Record<OpportunityStage, { label: string; tone: Tone }> = {
  LEAD:           { label: "Lead",           tone: "blue"    },
  QUALIFIED:      { label: "Qualified",      tone: "blue"    },
  NEEDS_ANALYSIS: { label: "Needs analysis", tone: "amber"   },
  PROPOSAL:       { label: "Proposal",       tone: "purple"  },
  NEGOTIATION:    { label: "Negotiation",    tone: "amber"   },
  WON:            { label: "Won",            tone: "green"   },
  LOST:           { label: "Lost",           tone: "red"     },
  ON_HOLD:        { label: "On hold",        tone: "neutral" },
};

export const opportunityStageLabel = (s: OpportunityStage | string): string =>
  OPPORTUNITY_STAGE_META[s as OpportunityStage]?.label ?? s;

export const opportunityStageTone = (s: OpportunityStage | string): ToneClasses =>
  TONES[OPPORTUNITY_STAGE_META[s as OpportunityStage]?.tone ?? "neutral"];

export const probabilityPercent = (basisPoints: number): string =>
  `${Math.round(basisPoints / 100)}%`;

// ── Quotation ─────────────────────────────────────────────────────────────

const QUOTATION_STATUS_META: Record<QuotationStatus, { label: string; tone: Tone }> = {
  DRAFT:             { label: "Draft",             tone: "blue"    },
  AWAITING_APPROVAL: { label: "Awaiting approval", tone: "amber"   },
  APPROVED:          { label: "Approved",          tone: "blue"    },
  ISSUED:            { label: "Issued",            tone: "amber"   },
  VIEWED:            { label: "Viewed",            tone: "amber"   },
  ACCEPTED:          { label: "Accepted",          tone: "green"   },
  REJECTED:          { label: "Rejected",          tone: "red"     },
  EXPIRED:           { label: "Expired",           tone: "neutral" },
  CANCELLED:         { label: "Cancelled",         tone: "neutral" },
  SUPERSEDED:        { label: "Superseded",        tone: "neutral" },
};

export const quotationStatusLabel = (s: QuotationStatus | string): string =>
  QUOTATION_STATUS_META[s as QuotationStatus]?.label ?? s;

export const quotationStatusTone = (s: QuotationStatus | string): ToneClasses =>
  TONES[QUOTATION_STATUS_META[s as QuotationStatus]?.tone ?? "neutral"];

const ACCEPTANCE_METHOD_LABEL: Record<AcceptanceMethod, string> = {
  SECURE_CUSTOMER_TOKEN: "Secure customer link",
  AUTHORIZED_COMPANY_USER: "Authorized company user",
  APPROVED_EXTERNAL_INTEGRATION: "External integration",
  DOCUMENTED_OFFLINE_ACCEPTANCE: "Documented offline",
};

export const acceptanceMethodLabel = (m: AcceptanceMethod | string): string =>
  ACCEPTANCE_METHOD_LABEL[m as AcceptanceMethod] ?? m;

// ── Contract ──────────────────────────────────────────────────────────────

const CONTRACT_STATUS_META: Record<ContractStatus, { label: string; tone: Tone }> = {
  DRAFT:             { label: "Draft",             tone: "blue"    },
  AWAITING_APPROVAL: { label: "Awaiting approval", tone: "amber"   },
  APPROVED:          { label: "Approved",          tone: "blue"    },
  ACTIVE:            { label: "Active",            tone: "green"   },
  SUSPENDED:         { label: "Suspended",         tone: "amber"   },
  EXPIRED:           { label: "Expired",           tone: "neutral" },
  TERMINATED:        { label: "Terminated",        tone: "red"     },
  RENEWAL_PENDING:   { label: "Renewal pending",   tone: "amber"   },
  SUPERSEDED:        { label: "Superseded",        tone: "neutral" },
};

export const contractStatusLabel = (s: ContractStatus | string): string =>
  CONTRACT_STATUS_META[s as ContractStatus]?.label ?? s;

export const contractStatusTone = (s: ContractStatus | string): ToneClasses =>
  TONES[CONTRACT_STATUS_META[s as ContractStatus]?.tone ?? "neutral"];

// ── SLA ───────────────────────────────────────────────────────────────────

const SLA_STATUS_META: Record<SLAStatus, { label: string; tone: Tone }> = {
  DRAFT:      { label: "Draft",      tone: "blue"    },
  ACTIVE:     { label: "Active",     tone: "green"   },
  SUSPENDED:  { label: "Suspended",  tone: "amber"   },
  EXPIRED:    { label: "Expired",    tone: "neutral" },
  TERMINATED: { label: "Terminated", tone: "red"     },
  SUPERSEDED: { label: "Superseded", tone: "neutral" },
};

export const slaStatusLabel = (s: SLAStatus | string): string =>
  SLA_STATUS_META[s as SLAStatus]?.label ?? s;

export const slaStatusTone = (s: SLAStatus | string): ToneClasses =>
  TONES[SLA_STATUS_META[s as SLAStatus]?.tone ?? "neutral"];

const SLA_METRIC_LABEL: Record<SLAMetricCode, string> = {
  PICKUP_ON_TIME:        "Pickup on-time",
  DELIVERY_ON_TIME:      "Delivery on-time",
  ORDER_COMPLETION_TIME: "Order completion time",
  PACKAGE_DAMAGE_RATE:   "Package damage rate",
  PROOF_COMPLETION_TIME: "Proof completion time",
  FAILED_ATTEMPT_RATE:   "Failed attempt rate",
};

export const slaMetricLabel = (m: SLAMetricCode | string): string =>
  SLA_METRIC_LABEL[m as SLAMetricCode] ?? m;

const SLA_UNIT_LABEL: Record<SLAMeasurementUnit, string> = {
  PERCENT: "%",
  BASIS_POINTS: "bps",
  MINUTES: "min",
  HOURS: "h",
};

export const slaUnitLabel = (u: SLAMeasurementUnit | string): string =>
  SLA_UNIT_LABEL[u as SLAMeasurementUnit] ?? u;

const SLA_WINDOW_LABEL: Record<SLAMeasurementWindow, string> = {
  DAILY: "Daily",
  WEEKLY: "Weekly",
  MONTHLY: "Monthly",
  QUARTERLY: "Quarterly",
  PER_ORDER: "Per order",
};

export const slaWindowLabel = (w: SLAMeasurementWindow | string): string =>
  SLA_WINDOW_LABEL[w as SLAMeasurementWindow] ?? w;

const OUTCOME_META: Record<SLAObservationOutcome, { label: string; tone: Tone }> = {
  MET:               { label: "Met",               tone: "green"   },
  MISSED:            { label: "Missed",            tone: "red"     },
  NOT_EVALUATED:     { label: "Not evaluated",     tone: "neutral" },
  INSUFFICIENT_DATA: { label: "Insufficient data", tone: "amber"   },
  UNKNOWN:           { label: "Unknown",           tone: "neutral" },
};

export const outcomeLabel = (o: SLAObservationOutcome | string): string =>
  OUTCOME_META[o as SLAObservationOutcome]?.label ?? o;

export const outcomeTone = (o: SLAObservationOutcome | string): ToneClasses =>
  TONES[OUTCOME_META[o as SLAObservationOutcome]?.tone ?? "neutral"];

// ── Credit ────────────────────────────────────────────────────────────────

const HOLD_STATUS_META: Record<CreditHoldStatus, { label: string; tone: Tone }> = {
  NO_HOLD:         { label: "No hold",         tone: "green"   },
  REVIEW_REQUIRED: { label: "Review required", tone: "amber"   },
  ON_HOLD:         { label: "On hold",         tone: "red"     },
  RESTRICTED:      { label: "Restricted",      tone: "red"     },
};

export const holdStatusLabel = (s: CreditHoldStatus | string): string =>
  HOLD_STATUS_META[s as CreditHoldStatus]?.label ?? s;

export const holdStatusTone = (s: CreditHoldStatus | string): ToneClasses =>
  TONES[HOLD_STATUS_META[s as CreditHoldStatus]?.tone ?? "neutral"];

// ── Notification ──────────────────────────────────────────────────────────

const NOTIFICATION_STATUS_META: Record<NotificationStatus, { label: string; tone: Tone }> = {
  PENDING:            { label: "Pending",            tone: "blue"    },
  PROCESSING:         { label: "Processing",         tone: "blue"    },
  UNKNOWN:            { label: "Unknown",            tone: "amber"   },
  DELIVERED:          { label: "Delivered",          tone: "green"   },
  FAILED_RETRYABLE:   { label: "Retrying",           tone: "amber"   },
  FAILED_PERMANENT:   { label: "Failed",             tone: "red"     },
  DEAD_LETTER:        { label: "Dead letter",        tone: "red"     },
};

export const notificationStatusLabel = (s: NotificationStatus | string): string =>
  NOTIFICATION_STATUS_META[s as NotificationStatus]?.label ?? s;

export const notificationStatusTone = (s: NotificationStatus | string): ToneClasses =>
  TONES[NOTIFICATION_STATUS_META[s as NotificationStatus]?.tone ?? "neutral"];

// ── Money ─────────────────────────────────────────────────────────────────

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

// ── Dates ─────────────────────────────────────────────────────────────────

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
  const yr = Math.floor(mo / 12);
  return `${yr}y ago`;
};

// ── Misc ──────────────────────────────────────────────────────────────────

export const initialsOf = (name: string | null | undefined): string => {
  if (!name) return "—";
  const parts = name.trim().split(/\s+/).slice(0, 2);
  const initials = parts.map((p) => p[0]?.toUpperCase() ?? "").join("");
  return initials || "—";
};