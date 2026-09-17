export type Tone = 'blue' | 'green' | 'amber' | 'red' | 'neutral';

export interface ToneClasses {
  bg: string;
  text: string;
  dot: string;
}

const TONES: Record<Tone, ToneClasses> = {
  blue:    { bg: 'bg-primary-bg', text: 'text-primary', dot: 'bg-primary' },
  green:   { bg: 'bg-green-bg',   text: 'text-green',   dot: 'bg-green'   },
  amber:   { bg: 'bg-amber-bg',   text: 'text-amber',   dot: 'bg-amber'   },
  red:     { bg: 'bg-red-bg',     text: 'text-red',     dot: 'bg-red'     },
  neutral: { bg: 'bg-surface-2',  text: 'text-ink-500', dot: 'bg-ink-400' },
};

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

const PROFILE_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  ACTIVE:    { label: 'Active',    tone: 'green'   },
  SUSPENDED: { label: 'Suspended', tone: 'amber'   },
  ENDED:     { label: 'Ended',     tone: 'neutral' },
  ARCHIVED:  { label: 'Archived',  tone: 'neutral' },
};

export const profileStatusLabel = (s: string): string =>
  PROFILE_STATUS_META[s]?.label ?? s;

export const profileStatusTone = (s: string): ToneClasses =>
  TONES[PROFILE_STATUS_META[s]?.tone ?? 'neutral'];

const RELATIONSHIP_LABEL: Record<string, string> = {
  EMPLOYEE:   'Employee',
  CONTRACTOR: 'Contractor',
  PARTNER:    'Partner',
};

export const relationshipTypeLabel = (r: string): string =>
  RELATIONSHIP_LABEL[r] ?? (r || '—');

// ---------------------------------------------------------------------------
// Shift
// ---------------------------------------------------------------------------

const SHIFT_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  DRAFT:       { label: 'Draft',       tone: 'neutral' },
  PUBLISHED:   { label: 'Published',   tone: 'blue'    },
  IN_PROGRESS: { label: 'In progress', tone: 'amber'   },
  COMPLETED:   { label: 'Completed',   tone: 'green'   },
  CANCELLED:   { label: 'Cancelled',   tone: 'neutral' },
};

export const shiftStatusLabel = (s: string): string =>
  SHIFT_STATUS_META[s]?.label ?? s;

export const shiftStatusTone = (s: string): ToneClasses =>
  TONES[SHIFT_STATUS_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Shift assignment
// ---------------------------------------------------------------------------

const ASSIGNMENT_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  PENDING:     { label: 'Pending',     tone: 'blue'    },
  CONFIRMED:   { label: 'Confirmed',   tone: 'green'   },
  CHECKED_IN:  { label: 'Checked in',  tone: 'amber'   },
  CHECKED_OUT: { label: 'Checked out', tone: 'green'   },
  DECLINED:    { label: 'Declined',    tone: 'red'     },
  CANCELLED:   { label: 'Cancelled',   tone: 'neutral' },
};

export const assignmentStatusLabel = (s: string): string =>
  ASSIGNMENT_STATUS_META[s]?.label ?? s;

export const assignmentStatusTone = (s: string): ToneClasses =>
  TONES[ASSIGNMENT_STATUS_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Leave
// ---------------------------------------------------------------------------

const LEAVE_TYPE_LABEL: Record<string, string> = {
  ANNUAL:    'Annual leave',
  SICK:      'Sick leave',
  PERSONAL:  'Personal leave',
  MATERNITY: 'Maternity leave',
  PATERNITY: 'Paternity leave',
  UNPAID:    'Unpaid leave',
  OTHER:     'Other',
};

export const leaveTypeLabel = (t: string): string =>
  LEAVE_TYPE_LABEL[t] ?? t;

const LEAVE_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  PENDING:   { label: 'Pending',   tone: 'amber'   },
  APPROVED:  { label: 'Approved',  tone: 'green'   },
  REJECTED:  { label: 'Rejected',  tone: 'red'     },
  CANCELLED: { label: 'Cancelled', tone: 'neutral' },
};

export const leaveStatusLabel = (s: string): string =>
  LEAVE_STATUS_META[s]?.label ?? s;

export const leaveStatusTone = (s: string): ToneClasses =>
  TONES[LEAVE_STATUS_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Qualification
// ---------------------------------------------------------------------------

const QUALIFICATION_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  ACTIVE:   { label: 'Active',   tone: 'green'   },
  INACTIVE: { label: 'Inactive', tone: 'neutral' },
  ARCHIVED: { label: 'Archived', tone: 'neutral' },
};

export const qualificationStatusLabel = (s: string): string =>
  QUALIFICATION_STATUS_META[s]?.label ?? s;

export const qualificationStatusTone = (s: string): ToneClasses =>
  TONES[QUALIFICATION_STATUS_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Provider qualification
// ---------------------------------------------------------------------------

const PROVIDER_QUALIFICATION_STATUS_META: Record<
  string,
  { label: string; tone: Tone }
> = {
  PENDING_VERIFICATION: { label: 'Pending verification', tone: 'amber'   },
  VERIFIED:             { label: 'Verified',             tone: 'green'   },
  REJECTED:             { label: 'Rejected',             tone: 'red'     },
  REVOKED:              { label: 'Revoked',              tone: 'neutral' },
};

export const providerQualificationStatusLabel = (s: string): string =>
  PROVIDER_QUALIFICATION_STATUS_META[s]?.label ?? s;

export const providerQualificationStatusTone = (s: string): ToneClasses =>
  TONES[PROVIDER_QUALIFICATION_STATUS_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Course
// ---------------------------------------------------------------------------

const COURSE_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  ACTIVE:   { label: 'Active',   tone: 'green'   },
  INACTIVE: { label: 'Inactive', tone: 'neutral' },
  ARCHIVED: { label: 'Archived', tone: 'neutral' },
};

export const courseStatusLabel = (s: string): string =>
  COURSE_STATUS_META[s]?.label ?? s;

export const courseStatusTone = (s: string): ToneClasses =>
  TONES[COURSE_STATUS_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Completion
// ---------------------------------------------------------------------------

const COMPLETION_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  PASSED: { label: 'Passed', tone: 'green' },
  FAILED: { label: 'Failed', tone: 'red'   },
};

export const completionStatusLabel = (s: string): string =>
  COMPLETION_STATUS_META[s]?.label ?? s;

export const completionStatusTone = (s: string): ToneClasses =>
  TONES[COMPLETION_STATUS_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------

export const formatDate = (iso: string | null | undefined): string => {
  if (!iso) return '—';
  const d = new Date(iso);
  return Number.isFinite(d.getTime())
    ? d.toLocaleDateString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      })
    : '—';
};

export const formatDateTime = (iso: string | null | undefined): string => {
  if (!iso) return '—';
  const d = new Date(iso);
  return Number.isFinite(d.getTime())
    ? d.toLocaleString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
    : '—';
};

export const formatTimeRange = (
  startIso: string | null | undefined,
  endIso: string | null | undefined,
): string => {
  if (!startIso || !endIso) return '—';
  const start = new Date(startIso);
  const end = new Date(endIso);
  if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime())) {
    return '—';
  }
  const fmt = (d: Date) =>
    d.toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
    });
  return `${fmt(start)} – ${fmt(end)}`;
};

export const formatDuration = (
  startIso: string | null | undefined,
  endIso: string | null | undefined,
): string => {
  if (!startIso || !endIso) return '—';
  const start = new Date(startIso).getTime();
  const end = new Date(endIso).getTime();
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) {
    return '—';
  }
  const minutes = Math.round((end - start) / 60_000);
  const hours = Math.floor(minutes / 60);
  const mins = minutes % 60;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
};