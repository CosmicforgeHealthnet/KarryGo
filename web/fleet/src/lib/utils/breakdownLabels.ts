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

const SEVERITY_META: Record<string, { label: string; tone: Tone }> = {
  LOW:      { label: 'Low',      tone: 'neutral' },
  MEDIUM:   { label: 'Medium',   tone: 'blue'    },
  HIGH:     { label: 'High',     tone: 'amber'   },
  CRITICAL: { label: 'Critical', tone: 'red'     },
};

export const severityLabel = (s: string): string =>
  SEVERITY_META[s]?.label ?? s;

export const severityTone = (s: string): ToneClasses =>
  TONES[SEVERITY_META[s]?.tone ?? 'neutral'];

const STATUS_META: Record<string, { label: string; tone: Tone }> = {
  OPEN:        { label: 'Open',        tone: 'red'     },
  IN_PROGRESS: { label: 'In progress', tone: 'amber'   },
  RESOLVED:    { label: 'Resolved',    tone: 'green'   },
  CLOSED:      { label: 'Closed',      tone: 'neutral' },
};

export const breakdownStatusLabel = (s: string): string =>
  STATUS_META[s]?.label ?? s;

export const breakdownStatusTone = (s: string): ToneClasses =>
  TONES[STATUS_META[s]?.tone ?? 'neutral'];

const TRIP_IMPACT_META: Record<string, { label: string; tone: Tone }> = {
  NONE:        { label: 'No impact',  tone: 'neutral' },
  DELAYED:     { label: 'Delayed',    tone: 'amber'   },
  INTERRUPTED: { label: 'Interrupted', tone: 'red'    },
  CANCELLED:   { label: 'Cancelled',  tone: 'red'     },
};

export const tripImpactLabel = (t: string): string =>
  TRIP_IMPACT_META[t]?.label ?? (t || '—');

export const tripImpactTone = (t: string): ToneClasses =>
  TONES[TRIP_IMPACT_META[t]?.tone ?? 'neutral'];

const RESCUE_META: Record<string, { label: string; tone: Tone }> = {
  NOT_REQUIRED: { label: 'Not required', tone: 'neutral' },
  REQUESTED:    { label: 'Requested',    tone: 'amber'   },
  DISPATCHED:   { label: 'Dispatched',   tone: 'blue'    },
  ON_SITE:      { label: 'On site',      tone: 'blue'    },
  COMPLETED:    { label: 'Completed',    tone: 'green'   },
  FAILED:       { label: 'Failed',       tone: 'red'     },
};

export const rescueStatusLabel = (s: string): string =>
  RESCUE_META[s]?.label ?? (s || '—');

export const rescueStatusTone = (s: string): ToneClasses =>
  TONES[RESCUE_META[s]?.tone ?? 'neutral'];

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
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start)
    return '—';
  const ms = end - start;
  const days = Math.floor(ms / 86_400_000);
  const hours = Math.floor((ms % 86_400_000) / 3_600_000);
  if (days > 0) return `${days}d ${hours}h`;
  const minutes = Math.floor((ms % 3_600_000) / 60_000);
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
};