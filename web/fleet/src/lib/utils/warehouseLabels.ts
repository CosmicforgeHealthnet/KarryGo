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
// Warehouse
// ---------------------------------------------------------------------------

const WAREHOUSE_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  ACTIVE:   { label: 'Active',   tone: 'green'   },
  INACTIVE: { label: 'Inactive', tone: 'neutral' },
  ARCHIVED: { label: 'Archived', tone: 'neutral' },
};

export const warehouseStatusLabel = (s: string): string =>
  WAREHOUSE_STATUS_META[s]?.label ?? s;

export const warehouseStatusTone = (s: string): ToneClasses =>
  TONES[WAREHOUSE_STATUS_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Storage location
// ---------------------------------------------------------------------------

const LOCATION_TYPE_LABEL: Record<string, string> = {
  ZONE:       'Zone',
  AISLE:      'Aisle',
  BAY:        'Bay',
  BIN:        'Bin',
  DOCK:       'Dock',
  STAGING:    'Staging',
  QUARANTINE: 'Quarantine',
  COLD:       'Cold storage',
  HAZMAT:     'Hazmat',
  OTHER:      'Other',
};

export const locationTypeLabel = (t: string): string =>
  LOCATION_TYPE_LABEL[t] ?? (t || '—');

const LOCATION_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  ACTIVE:      { label: 'Active',      tone: 'green'   },
  INACTIVE:    { label: 'Inactive',    tone: 'neutral' },
  MAINTENANCE: { label: 'Maintenance', tone: 'amber'   },
};

export const locationStatusLabel = (s: string): string =>
  LOCATION_STATUS_META[s]?.label ?? s;

export const locationStatusTone = (s: string): ToneClasses =>
  TONES[LOCATION_STATUS_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Inventory
// ---------------------------------------------------------------------------

const INVENTORY_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  AVAILABLE:   { label: 'Available',   tone: 'green'   },
  RESERVED:    { label: 'Reserved',    tone: 'blue'    },
  PICKING:     { label: 'Picking',     tone: 'amber'   },
  PACKED:      { label: 'Packed',      tone: 'amber'   },
  DISPATCHED:  { label: 'Dispatched',  tone: 'neutral' },
  QUARANTINED: { label: 'Quarantined', tone: 'red'     },
  EXPIRED:     { label: 'Expired',     tone: 'red'     },
  RETURNED:    { label: 'Returned',    tone: 'blue'    },
};

export const inventoryStatusLabel = (s: string): string =>
  INVENTORY_STATUS_META[s]?.label ?? s;

export const inventoryStatusTone = (s: string): ToneClasses =>
  TONES[INVENTORY_STATUS_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Movement type
// ---------------------------------------------------------------------------

const MOVEMENT_TYPE_LABEL: Record<string, string> = {
  RECEIPT:    'Receipt',
  ADJUSTMENT: 'Adjustment',
  DISPATCH:   'Dispatch',
  DAMAGE:     'Damage',
  EXPIRY:     'Expiry',
  TRANSFER:   'Transfer',
  PUT_AWAY:   'Put away',
  QUARANTINE: 'Quarantine',
  RELEASE:    'Release',
  PICK:       'Pick',
  PACK:       'Pack',
  RETURN:     'Return',
};

export const movementTypeLabel = (t: string): string =>
  MOVEMENT_TYPE_LABEL[t] ?? t;

const MOVEMENT_TYPE_TONE: Record<string, Tone> = {
  RECEIPT:    'green',
  ADJUSTMENT: 'amber',
  DISPATCH:   'blue',
  DAMAGE:     'red',
  EXPIRY:     'red',
  TRANSFER:   'blue',
  PUT_AWAY:   'blue',
  QUARANTINE: 'red',
  RELEASE:    'green',
  PICK:       'amber',
  PACK:       'amber',
  RETURN:     'blue',
};

export const movementTypeTone = (t: string): ToneClasses =>
  TONES[MOVEMENT_TYPE_TONE[t] ?? 'neutral'];

// ---------------------------------------------------------------------------
// Fulfilment
// ---------------------------------------------------------------------------

const FULFILMENT_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  DRAFT:       { label: 'Draft',       tone: 'neutral' },
  RELEASED:    { label: 'Released',    tone: 'blue'    },
  PICKING:     { label: 'Picking',     tone: 'amber'   },
  PICKED:      { label: 'Picked',      tone: 'amber'   },
  PACKING:     { label: 'Packing',     tone: 'amber'   },
  PACKED:      { label: 'Packed',      tone: 'amber'   },
  DISPATCHED:  { label: 'Dispatched',  tone: 'green'   },
  COMPLETED:   { label: 'Completed',   tone: 'green'   },
  CANCELLED:   { label: 'Cancelled',   tone: 'neutral' },
};

export const fulfilmentStatusLabel = (s: string): string =>
  FULFILMENT_STATUS_META[s]?.label ?? s;

export const fulfilmentStatusTone = (s: string): ToneClasses =>
  TONES[FULFILMENT_STATUS_META[s]?.tone ?? 'neutral'];

const PRIORITY_META: Record<string, { label: string; tone: Tone }> = {
  NORMAL: { label: 'Normal', tone: 'neutral' },
  HIGH:   { label: 'High',   tone: 'amber'   },
  URGENT: { label: 'Urgent', tone: 'red'     },
};

export const priorityLabel = (p: string): string =>
  PRIORITY_META[p]?.label ?? p;

export const priorityTone = (p: string): ToneClasses =>
  TONES[PRIORITY_META[p]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Dock appointments
// ---------------------------------------------------------------------------

const DOCK_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  REQUESTED:  { label: 'Requested',  tone: 'blue'    },
  CONFIRMED:  { label: 'Confirmed',  tone: 'blue'    },
  CHECKED_IN: { label: 'Checked in', tone: 'amber'   },
  COMPLETED:  { label: 'Completed',  tone: 'green'   },
  NO_SHOW:    { label: 'No show',    tone: 'red'     },
  CANCELLED:  { label: 'Cancelled',  tone: 'neutral' },
};

export const dockStatusLabel = (s: string): string =>
  DOCK_STATUS_META[s]?.label ?? s;

export const dockStatusTone = (s: string): ToneClasses =>
  TONES[DOCK_STATUS_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Yard visits
// ---------------------------------------------------------------------------

const YARD_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  GATE_CHECK_IN: { label: 'Gate check-in', tone: 'blue'  },
  IN_YARD:       { label: 'In yard',       tone: 'amber' },
  AT_BAY:        { label: 'At bay',        tone: 'amber' },
  SECURITY_HOLD: { label: 'Security hold', tone: 'red'   },
  RELEASED:      { label: 'Released',      tone: 'blue'  },
  CHECKED_OUT:   { label: 'Checked out',   tone: 'green' },
};

export const yardStatusLabel = (s: string): string =>
  YARD_STATUS_META[s]?.label ?? s;

export const yardStatusTone = (s: string): ToneClasses =>
  TONES[YARD_STATUS_META[s]?.tone ?? 'neutral'];

const YARD_SECURITY_META: Record<string, { label: string; tone: Tone }> = {
  CLEARED: { label: 'Cleared', tone: 'green' },
  HOLD:    { label: 'Hold',    tone: 'red'   },
};

export const yardSecurityLabel = (s: string): string =>
  YARD_SECURITY_META[s]?.label ?? (s || '—');

export const yardSecurityTone = (s: string): ToneClasses =>
  TONES[YARD_SECURITY_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Terminal transfers
// ---------------------------------------------------------------------------

const TRANSFER_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  DRAFT:       { label: 'Draft',       tone: 'neutral' },
  READY:       { label: 'Ready',       tone: 'blue'    },
  DEPARTED:    { label: 'Departed',    tone: 'amber'   },
  IN_TRANSIT:  { label: 'In transit',  tone: 'amber'   },
  ARRIVED:     { label: 'Arrived',     tone: 'blue'    },
  COMPLETED:   { label: 'Completed',   tone: 'green'   },
  DISCREPANCY: { label: 'Discrepancy', tone: 'red'     },
};

export const transferStatusLabel = (s: string): string =>
  TRANSFER_STATUS_META[s]?.label ?? s;

export const transferStatusTone = (s: string): ToneClasses =>
  TONES[TRANSFER_STATUS_META[s]?.tone ?? 'neutral'];

const RECONCILIATION_META: Record<string, { label: string; tone: Tone }> = {
  PENDING:     { label: 'Pending',     tone: 'amber' },
  MATCHED:     { label: 'Matched',     tone: 'green' },
  DISCREPANCY: { label: 'Discrepancy', tone: 'red'   },
};

export const reconciliationLabel = (s: string): string =>
  RECONCILIATION_META[s]?.label ?? (s || '—');

export const reconciliationTone = (s: string): ToneClasses =>
  TONES[RECONCILIATION_META[s]?.tone ?? 'neutral'];

// ---------------------------------------------------------------------------
// Returns
// ---------------------------------------------------------------------------

const RETURN_STATUS_META: Record<string, { label: string; tone: Tone }> = {
  REQUESTED:  { label: 'Requested',  tone: 'blue'  },
  RECEIVED:   { label: 'Received',   tone: 'amber' },
  INSPECTING: { label: 'Inspecting', tone: 'amber' },
  DECIDED:    { label: 'Decided',    tone: 'blue'  },
  COMPLETED:  { label: 'Completed',  tone: 'green' },
};

export const returnStatusLabel = (s: string): string =>
  RETURN_STATUS_META[s]?.label ?? s;

export const returnStatusTone = (s: string): ToneClasses =>
  TONES[RETURN_STATUS_META[s]?.tone ?? 'neutral'];

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

export const formatNumber = (value: number | null | undefined): string => {
  if (value == null || !Number.isFinite(value)) return '—';
  return value.toLocaleString(undefined, { maximumFractionDigits: 2 });
};