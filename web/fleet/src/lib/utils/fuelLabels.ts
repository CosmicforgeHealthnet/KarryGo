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

const FUEL_TYPE_LABEL: Record<string, string> = {
  PETROL:   'Petrol',
  DIESEL:   'Diesel',
  ELECTRIC: 'Electric',
  GAS:      'Gas',
  OTHER:    'Other',
};

export const fuelOrEnergyTypeLabel = (t: string): string =>
  FUEL_TYPE_LABEL[t] ?? t;

const UNIT_LABEL: Record<string, string> = {
  L:     'L',
  LITRE: 'L',
  KWH:   'kWh',
  KG:    'kg',
  UNIT:  'unit',
};

export const fuelUnitLabel = (u: string): string => UNIT_LABEL[u] ?? u;

const APPROVAL_META: Record<string, { label: string; tone: Tone }> = {
  PENDING:  { label: 'Pending',  tone: 'amber' },
  APPROVED: { label: 'Approved', tone: 'green' },
  REJECTED: { label: 'Rejected', tone: 'red'   },
};

export const approvalStatusLabel = (s: string): string =>
  APPROVAL_META[s]?.label ?? s;

export const approvalStatusTone = (s: string): ToneClasses =>
  TONES[APPROVAL_META[s]?.tone ?? 'neutral'];

export const validUnitsForType = (type: string): string[] => {
  switch (type) {
    case 'PETROL':
    case 'DIESEL':
      return ['L', 'LITRE'];
    case 'ELECTRIC':
      return ['KWH'];
    case 'GAS':
      return ['KG', 'L'];
    case 'OTHER':
      return ['UNIT'];
    default:
      return [];
  }
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