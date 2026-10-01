import type { Asset } from '@/lib/types/asset';

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

const ASSET_TYPE_LABEL: Record<string, string> = {
  MOTORCYCLE: 'Motorcycle',
  VAN:        'Van',
  TRUCK:      'Truck',
  CAR:        'Car',
  BIKE:       'Bike',
  OTHER:      'Other',
};

const OPERATIONAL_META: Record<string, { label: string; tone: Tone }> = {
  AVAILABLE:         { label: 'Available',         tone: 'green'   },
  ASSIGNED:          { label: 'Assigned',          tone: 'blue'    },
  BLOCKED:           { label: 'Blocked',           tone: 'red'     },
  UNDER_MAINTENANCE: { label: 'Under maintenance', tone: 'amber'   },
  OUT_OF_SERVICE:    { label: 'Out of service',    tone: 'red'     },
  RETIRED:           { label: 'Retired',           tone: 'neutral' },
  ARCHIVED:          { label: 'Archived',          tone: 'neutral' },
};

const LIFECYCLE_META: Record<string, { label: string; tone: Tone }> = {
  ACTIVE:   { label: 'Active',   tone: 'green'   },
  ARCHIVED: { label: 'Archived', tone: 'neutral' },
  RETIRED:  { label: 'Retired',  tone: 'neutral' },
};

const OWNERSHIP_LABEL: Record<string, string> = {
  OWNED:      'Owned',
  LEASED:     'Leased',
  CONTRACTED: 'Contracted',
};

const VERIFICATION_META: Record<string, { label: string; tone: Tone }> = {
  PENDING:  { label: 'Pending',  tone: 'amber' },
  VERIFIED: { label: 'Verified', tone: 'green' },
  REJECTED: { label: 'Rejected', tone: 'red'   },
  EXPIRED:  { label: 'Expired',  tone: 'red'   },
  REVOKED:  { label: 'Revoked',  tone: 'red'   },
};

export const assetTypeLabel = (t: string): string =>
  ASSET_TYPE_LABEL[t] ?? t;

export const operationalStatusLabel = (s: string): string =>
  OPERATIONAL_META[s]?.label ?? s;

export const operationalStatusTone = (s: string): ToneClasses =>
  TONES[OPERATIONAL_META[s]?.tone ?? 'neutral'];

export const lifecycleStatusLabel = (s: string): string =>
  LIFECYCLE_META[s]?.label ?? s;

export const lifecycleStatusTone = (s: string): ToneClasses =>
  TONES[LIFECYCLE_META[s]?.tone ?? 'neutral'];

export const ownershipTypeLabel = (o: string): string =>
  OWNERSHIP_LABEL[o] ?? (o || '—');

export const verificationStatusLabel = (s: string): string =>
  VERIFICATION_META[s]?.label ?? s;

export const verificationStatusTone = (s: string): ToneClasses =>
  TONES[VERIFICATION_META[s]?.tone ?? 'neutral'];

export const assetDisplayName = (
  a: Pick<Asset, 'manufacturer' | 'model'>,
): string => {
  const parts = [a.manufacturer, a.model].filter(Boolean);
  return parts.length > 0 ? parts.join(' ') : '—';
};

export const formatOdometer = (value: number, unit = 'km'): string => {
  if (!Number.isFinite(value)) return '—';
  return `${value.toLocaleString(undefined, { maximumFractionDigits: 0 })} ${unit}`;
};

export const formatOperatingHours = (value: number): string => {
  if (!Number.isFinite(value)) return '—';
  return `${value.toLocaleString(undefined, { maximumFractionDigits: 0 })} h`;
};

export const formatCapacity = (value: number, unit: string): string => {
  if (!Number.isFinite(value) || value <= 0) return '—';
  return `${value.toLocaleString(undefined, { maximumFractionDigits: 1 })} ${unit || ''}`.trim();
};

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