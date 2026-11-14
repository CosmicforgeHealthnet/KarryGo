"use client";

import type { ReactNode } from "react";
import { X } from "lucide-react";

// Small building blocks shared by the operations screens. They reproduce the
// dashboard's existing card, table, pill and button styles (see the branches
// and approval pages) so connected screens look the same as the rest of the
// app; they introduce no new visual language.

export const humanize = (value?: string | null) =>
  value ? value.replace(/_/g, " ").toLowerCase().replace(/^\w/, (c) => c.toUpperCase()) : "—";

export const formatWhen = (iso?: string | null) =>
  iso
    ? new Date(iso).toLocaleString(undefined, { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" })
    : "—";

export const formatMinor = (amountMinor?: number | null, currency?: string | null) => {
  if (amountMinor == null) return "—";
  const major = amountMinor / 100;
  if (!currency) return major.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  try {
    return new Intl.NumberFormat(undefined, { style: "currency", currency }).format(major);
  } catch {
    return `${currency} ${major.toFixed(2)}`;
  }
};

// Converts a decimal amount typed by a person into whole minor units.
export const toMinor = (value: string): number | null => {
  const trimmed = value.trim();
  if (trimmed === "") return null;
  const n = Number(trimmed);
  if (!Number.isFinite(n)) return null;
  return Math.round(n * 100);
};

const TONES: Record<string, { bg: string; text: string; dot: string }> = {
  green: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  amber: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  red: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  primary: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  gray: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const GREEN = ["ACTIVE", "APPROVED", "VERIFIED", "COMPLETED", "RESOLVED", "CONFIRMED", "READY", "CONVERTED", "QUALIFIED", "COMPLIANT", "AVAILABLE", "INSTALLED", "CLEARED", "CLOSED", "DELIVERED", "REPLACEMENT_ACCEPTED", "IN_STOCK", "FRESH", "EFFECTIVE"];
const AMBER = ["PENDING", "DRAFT", "AWAITING_APPROVAL", "OPEN", "TRIAGED", "CONTAINED", "INVESTIGATING", "DEFERRED", "REPAIRING", "REPLACEMENT_PENDING", "CONTACTED", "NEW", "DUE_SOON", "BUSY", "SCHEDULED", "PLANNED", "ASSIGNED", "IN_PROGRESS", "IN_TRANSIT", "STALE", "PENDING_REVIEW", "FINDINGS_SUBMITTED", "CLEAR_PENDING", "DUE_SOON"];
const RED = ["REJECTED", "CANCELLED", "FAILED", "DISQUALIFIED", "CRITICAL", "HIGH", "ESCALATED", "CORRECTIVE_ACTION_REQUIRED", "NON_COMPLIANT", "RESTRICTED", "EXPIRED", "UNAVAILABLE", "BLOCKING", "SCRAPPED", "REVOKED", "APPLIED", "GAPPED", "UNMATCHED"];

export function toneFor(status?: string | null): string {
  const s = (status ?? "").toUpperCase();
  if (GREEN.includes(s)) return "green";
  if (AMBER.includes(s)) return "amber";
  if (RED.includes(s)) return "red";
  return "gray";
}

export function StatusPill({ status, label, tone }: { status?: string | null; label?: string; tone?: string }) {
  const style = TONES[tone ?? toneFor(status)] ?? TONES.gray;
  return (
    <span className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${style.bg} ${style.text}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${style.dot}`} />
      {label ?? humanize(status)}
    </span>
  );
}

export function PageHeader({ title, description, actions }: { title: string; description?: string; actions?: ReactNode }) {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">{title}</h1>
        {description && <p className="mt-1 text-sm text-ink-600">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

type ButtonProps = {
  children: ReactNode;
  onClick?: () => void;
  disabled?: boolean;
  type?: "button" | "submit";
  title?: string;
};

export function PrimaryButton({ children, onClick, disabled, type = "button", title }: ButtonProps) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      title={title}
      className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90 disabled:opacity-60"
    >
      {children}
    </button>
  );
}

export function SecondaryButton({ children, onClick, disabled, type = "button", title }: ButtonProps) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      title={title}
      className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white disabled:opacity-60"
    >
      {children}
    </button>
  );
}

export function DangerButton({ children, onClick, disabled, type = "button", title }: ButtonProps) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      title={title}
      className="flex h-9 items-center gap-1.5 rounded-xl border border-red/30 bg-red-bg px-3.5 text-[11.5px] font-semibold text-red transition hover:bg-red-bg/70 disabled:opacity-60"
    >
      {children}
    </button>
  );
}

export function Loading({ label }: { label: string }) {
  return (
    <div className="py-12 text-center">
      <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
      <p className="mt-3 text-sm text-ink-600">{label}</p>
    </div>
  );
}

export function Notice({ message }: { message: string | null }) {
  if (!message) return null;
  return <div role="status" className="rounded-xl border border-green/30 bg-green-bg px-4 py-3 text-sm text-green">{message}</div>;
}

export function StatGrid({ items }: { items: { label: string; value: ReactNode; tone?: string }[] }) {
  const cols = items.length >= 4 ? "sm:grid-cols-4" : items.length === 3 ? "sm:grid-cols-3" : "sm:grid-cols-2";
  return (
    <div className={`grid grid-cols-2 gap-2.5 ${cols}`}>
      {items.map((item) => (
        <div key={item.label} className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">{item.label}</div>
          <div className={`text-lg font-bold ${item.tone ? TONES[item.tone]?.text ?? "text-ink-950" : "text-ink-950"}`}>{item.value}</div>
        </div>
      ))}
    </div>
  );
}

export function Panel({
  title,
  description,
  actions,
  children,
  onClose,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
  onClose?: () => void;
}) {
  return (
    <section className="space-y-4 rounded-2xl border border-line bg-white p-5 shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h3 className="text-[15px] font-semibold text-ink-950">{title}</h3>
          {description && <p className="mt-0.5 text-xs text-ink-500">{description}</p>}
        </div>
        <div className="flex items-center gap-2">
          {actions}
          {onClose && (
            <button type="button" onClick={onClose} aria-label="Close" className="text-ink-400 hover:text-ink-700">
              <X className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>
      {children}
    </section>
  );
}

export function EmptyBlock({ icon, title, description, action }: { icon?: ReactNode; title: string; description: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14 text-center">
      {icon && <div className="rounded-full bg-surface-2 p-4 text-ink-400">{icon}</div>}
      <h3 className="mt-3 text-[15px] font-semibold text-ink-950">{title}</h3>
      <p className="mt-1 max-w-md text-sm text-ink-500">{description}</p>
      {action && <div className="mt-3.5">{action}</div>}
    </div>
  );
}

export function DataTable<T>({
  columns,
  rows,
  rowKey,
  onRowClick,
  minWidth = 860,
}: {
  columns: { header: string; cell: (row: T) => ReactNode; className?: string }[];
  rows: T[];
  rowKey: (row: T) => string;
  onRowClick?: (row: T) => void;
  minWidth?: number;
}) {
  return (
    <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-sm" style={{ minWidth }}>
          <thead>
            <tr className="bg-surface-2/70">
              {columns.map((c, i) => (
                <th key={`${c.header}-${i}`} className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                  {c.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr
                key={rowKey(row)}
                onClick={onRowClick ? () => onRowClick(row) : undefined}
                className={`border-b border-line transition-colors hover:bg-surface-2 ${onRowClick ? "cursor-pointer" : ""}`}
              >
                {columns.map((c, i) => (
                  <td key={`${c.header}-${i}`} className={`px-4 py-3 text-xs text-ink-600 ${c.className ?? ""}`}>
                    {c.cell(row)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export function KeyValueGrid({ items }: { items: [string, ReactNode][] }) {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
      {items.map(([label, value]) => (
        <div key={label} className="rounded-xl border border-line bg-surface-2/40 p-3">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">{label}</div>
          <div className="mt-1 break-all text-sm font-medium text-ink-950">{value === "" || value == null ? "—" : value}</div>
        </div>
      ))}
    </div>
  );
}

export function Timeline({ entries }: { entries: { id: string; title: ReactNode; detail?: ReactNode; at?: string | null }[] }) {
  if (entries.length === 0) return <p className="text-sm text-ink-500">Nothing recorded yet.</p>;
  return (
    <ol className="space-y-3">
      {entries.map((e) => (
        <li key={e.id} className="flex gap-3">
          <span className="mt-1.5 h-2 w-2 flex-shrink-0 rounded-full bg-primary" />
          <div className="min-w-0">
            <div className="text-sm font-medium text-ink-950">{e.title}</div>
            {e.detail && <div className="text-xs text-ink-500">{e.detail}</div>}
            {e.at && <div className="text-[11px] text-ink-400">{formatWhen(e.at)}</div>}
          </div>
        </li>
      ))}
    </ol>
  );
}
