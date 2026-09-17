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

const INSPECTION_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  DRAFT:       { label: 'Draft',       tone: 'neutral' },
  IN_PROGRESS: { label: 'In progress', tone: 'amber'   },
  COMPLETED:   { label: 'Completed',   tone: 'green'   },
  CANCELLED:   { label: 'Cancelled',   tone: 'neutral' },
};

export const inspectionStatusLabel = (s: string): string =>
  INSPECTION_STATUS_META[s]?.label ?? s;

export const inspectionStatusTone = (s: string): ToneClasses =>
  TONES[INSPECTION_STATUS_META[s]?.tone ?? 'neutral'];

const OVERALL_RESULT_META: Record<string, { label: string; tone: Tone }> = {
  PASS:                 { label: 'Pass',                 tone: 'green'   },
  PASS_WITH_ADVISORY:   { label: 'Pass with advisory',   tone: 'blue'    },
  FAIL:                 { label: 'Fail',                 tone: 'amber'   },
  CRITICAL_FAIL:        { label: 'Critical fail',        tone: 'red'     },
};

export const overallResultLabel = (r: string): string =>
  OVERALL_RESULT_META[r]?.label ?? (r || '—');

export const overallResultTone = (r: string): ToneClasses =>
  TONES[OVERALL_RESULT_META[r]?.tone ?? 'neutral'];

const TEMPLATE_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  ACTIVE:   { label: 'Active',   tone: 'green'   },
  INACTIVE: { label: 'Inactive', tone: 'neutral' },
  DRAFT:    { label: 'Draft',    tone: 'neutral' },
  ARCHIVED: { label: 'Archived', tone: 'neutral' },
};

export const templateStatusLabel = (s: string): string =>
  TEMPLATE_STATUS_META[s]?.label ?? s;

export const templateStatusTone = (s: string): ToneClasses =>
  TONES[TEMPLATE_STATUS_META[s]?.tone ?? 'neutral'];

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