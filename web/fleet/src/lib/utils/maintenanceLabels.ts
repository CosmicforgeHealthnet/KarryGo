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

const TRIGGER_LABEL: Record<string, string> = {
  DATE:            'By date',
  MILEAGE:         'By mileage',
  OPERATING_HOURS: 'By operating hours',
  COMBINED:        'Any of date, mileage, hours',
};

export const triggerTypeLabel = (t: string): string =>
  TRIGGER_LABEL[t] ?? t;

const SCHEDULE_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  ACTIVE:   { label: 'Active',   tone: 'green'   },
  PAUSED:   { label: 'Paused',   tone: 'amber'   },
  COMPLETE: { label: 'Complete', tone: 'green'   },
  ARCHIVED: { label: 'Archived', tone: 'neutral' },
};

export const scheduleStatusLabel = (s: string): string =>
  SCHEDULE_STATUS_META[s]?.label ?? s;

export const scheduleStatusTone = (s: string): ToneClasses =>
  TONES[SCHEDULE_STATUS_META[s]?.tone ?? 'neutral'];

const WORK_ORDER_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  DRAFT:            { label: 'Draft',            tone: 'neutral' },
  PENDING_APPROVAL: { label: 'Pending approval', tone: 'amber'   },
  APPROVED:         { label: 'Approved',         tone: 'blue'    },
  IN_PROGRESS:      { label: 'In progress',      tone: 'amber'   },
  COMPLETED:        { label: 'Completed',        tone: 'green'   },
  CANCELLED:        { label: 'Cancelled',        tone: 'neutral' },
};

export const workOrderStatusLabel = (s: string): string =>
  WORK_ORDER_STATUS_META[s]?.label ?? s;

export const workOrderStatusTone = (s: string): ToneClasses =>
  TONES[WORK_ORDER_STATUS_META[s]?.tone ?? 'neutral'];

const PRIORITY_META: Record<string, { label: string; tone: Tone }> = {
  CRITICAL: { label: 'Critical', tone: 'red'   },
  HIGH:     { label: 'High',     tone: 'amber' },
  MEDIUM:   { label: 'Medium',   tone: 'blue'  },
  LOW:      { label: 'Low',      tone: 'neutral' },
};

export const workOrderPriorityLabel = (p: string): string =>
  PRIORITY_META[p]?.label ?? p;

export const workOrderPriorityTone = (p: string): ToneClasses =>
  TONES[PRIORITY_META[p]?.tone ?? 'neutral'];

const SOURCE_LABEL: Record<string, string> = {
  INSPECTION:           'Inspection',
  BREAKDOWN:            'Breakdown',
  MAINTENANCE_SCHEDULE: 'Maintenance schedule',
  MANUAL:               'Manual',
};

export const workOrderSourceLabel = (s: string): string =>
  SOURCE_LABEL[s] ?? (s || '—');

const RETURN_TO_SERVICE_META: Record<string, { label: string; tone: Tone }> = {
  PENDING:  { label: 'Pending',  tone: 'amber'   },
  APPROVED: { label: 'Approved', tone: 'green'   },
  DENIED:   { label: 'Denied',   tone: 'red'     },
};

export const returnToServiceLabel = (s: string): string =>
  RETURN_TO_SERVICE_META[s]?.label ?? s;

export const returnToServiceTone = (s: string): ToneClasses =>
  TONES[RETURN_TO_SERVICE_META[s]?.tone ?? 'neutral'];

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

export const formatDuration = (
  startIso: string | null | undefined,
  endIso: string | null | undefined,
): string => {
  if (!startIso) return '—';
  const start = new Date(startIso).getTime();
  const end = endIso ? new Date(endIso).getTime() : Date.now();
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return '—';
  const ms = end - start;
  const days = Math.floor(ms / 86_400_000);
  const hours = Math.floor((ms % 86_400_000) / 3_600_000);
  if (days > 0) return `${days}d ${hours}h`;
  const minutes = Math.floor((ms % 3_600_000) / 60_000);
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
};

export const formatNumber = (value: number | null | undefined): string => {
  if (value == null || !Number.isFinite(value)) return '—';
  return value.toLocaleString(undefined, { maximumFractionDigits: 1 });
};