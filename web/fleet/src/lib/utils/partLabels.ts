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

const STATUS_META: Record<string, { label: string; tone: Tone }> = {
  ACTIVE:   { label: 'Active',   tone: 'green'   },
  INACTIVE: { label: 'Inactive', tone: 'neutral' },
  ARCHIVED: { label: 'Archived', tone: 'neutral' },
};

export const partStatusLabel = (s: string): string =>
  STATUS_META[s]?.label ?? s;

export const partStatusTone = (s: string): ToneClasses =>
  TONES[STATUS_META[s]?.tone ?? 'neutral'];

const MOVEMENT_META: Record<string, { label: string; tone: Tone }> = {
  RECEIVE:      { label: 'Receive',      tone: 'green'   },
  ISSUE:        { label: 'Issue',        tone: 'red'     },
  ADJUST:       { label: 'Adjust',       tone: 'amber'   },
  TRANSFER_IN:  { label: 'Transfer in',  tone: 'blue'    },
  TRANSFER_OUT: { label: 'Transfer out', tone: 'amber'   },
  RETURN:       { label: 'Return',       tone: 'green'   },
};

export const movementTypeLabel = (t: string): string =>
  MOVEMENT_META[t]?.label ?? t;

export const movementTypeTone = (t: string): ToneClasses =>
  TONES[MOVEMENT_META[t]?.tone ?? 'neutral'];

export const stockStateLabel = (part: {
  currentQuantityProjection: number;
  reorderLevel: number;
}): { label: string; tone: ToneClasses } => {
  const q = part.currentQuantityProjection;
  const r = part.reorderLevel;
  if (q <= 0) return { label: 'Out of stock', tone: TONES.red };
  if (r > 0 && q <= r) return { label: 'Low stock', tone: TONES.amber };
  return { label: 'In stock', tone: TONES.green };
};

export const formatNumber = (value: number | null | undefined): string => {
  if (value == null || !Number.isFinite(value)) return '—';
  return value.toLocaleString(undefined, { maximumFractionDigits: 2 });
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