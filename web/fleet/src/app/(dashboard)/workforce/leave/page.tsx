'use client';

import { useEffect, useMemo, useState, type FormEvent } from 'react';
import Link from 'next/link';
import {
  Calendar,
  CheckCircle2,
  Clock,
  Plus,
  Search,
  SlidersHorizontal,
  X,
  XCircle,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import {
  newWorkforceIdempotencyKey,
  workforceService,
} from '@/lib/services/workforceService';
import {
  formatDate,
  leaveStatusLabel,
  leaveStatusTone,
  leaveTypeLabel,
} from '@/lib/utils/workforceLabels';
import type { Leave } from '@/lib/types/workforce';

const STATUS_FILTERS = [
  'ALL',
  'PENDING',
  'APPROVED',
  'REJECTED',
  'CANCELLED',
] as const;

const LEAVE_TYPES = [
  'ANNUAL',
  'SICK',
  'PERSONAL',
  'MATERNITY',
  'PATERNITY',
  'UNPAID',
  'OTHER',
] as const;

export default function LeavePage() {
  const [rows, setRows] = useState<Leave[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<(typeof STATUS_FILTERS)[number]>('ALL');
  const [showFilters, setShowFilters] = useState(false);
  const [showCreate, setShowCreate] = useState(false);

  const [createForm, setCreateForm] = useState({
    membershipId: '',
    leaveType: 'ANNUAL' as string,
    startAt: '',
    endAt: '',
    reason: '',
  });

  const load = () => {
    setLoading(true);
    setError(null);
    workforceService
      .listLeave({ limit: 100 })
      .then(setRows)
      .catch((err) => {
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load leave requests.',
        );
      })
      .finally(() => setLoading(false));
  };

  useEffect(load, []);

  const stats = useMemo(() => {
    let pending = 0;
    let approved = 0;
    let rejected = 0;
    for (const r of rows) {
      if (r.status === 'PENDING') pending++;
      else if (r.status === 'APPROVED') approved++;
      else if (r.status === 'REJECTED') rejected++;
    }
    return { pending, approved, rejected };
  }, [rows]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter((row) => {
      if (status !== 'ALL' && row.status !== status) return false;
      if (q === '') return true;
      const haystack = [
        row.id,
        row.membershipId,
        row.leaveType,
        row.reason,
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [rows, search, status]);

  const hasActiveFilters = status !== 'ALL' || search.trim() !== '';

  const clearFilters = () => {
    setSearch('');
    setStatus('ALL');
  };

  async function runAction(
    leaveId: string,
    fn: (
      id: string,
      version: number,
      key: string,
    ) => Promise<Leave>,
  ) {
    const row = rows.find((r) => r.id === leaveId);
    if (!row) return;
    setBusyId(leaveId);
    setError(null);
    try {
      await fn(leaveId, row.version, newWorkforceIdempotencyKey());
      load();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Action failed.',
      );
    } finally {
      setBusyId(null);
    }
  }

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await workforceService.createLeave(
        {
          membershipId: createForm.membershipId.trim(),
          leaveType: createForm.leaveType,
          startAt: new Date(createForm.startAt).toISOString(),
          endAt: new Date(createForm.endAt).toISOString(),
          reason: createForm.reason.trim() || undefined,
        },
        newWorkforceIdempotencyKey(),
      );
      setShowCreate(false);
      setCreateForm({
        membershipId: '',
        leaveType: 'ANNUAL',
        startAt: '',
        endAt: '',
        reason: '',
      });
      load();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to create leave request.',
      );
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Leave Management</h1>
          <p className="mt-1.5">
            Leave requests, approvals and unavailability records.
          </p>
        </div>
        <button
          onClick={() => setShowCreate((v) => !v)}
          className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          Request leave
        </button>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-3 gap-3.5">
        <StatCard label="Pending" value={stats.pending} tone="amber" />
        <StatCard label="Approved" value={stats.approved} tone="green" />
        <StatCard label="Rejected" value={stats.rejected} tone="red" />
      </div>

      {showCreate && (
        <form
          onSubmit={handleCreate}
          className="space-y-3 rounded-lg border border-line bg-surface p-4"
        >
          <h3 className="flex items-center gap-2 text-sm font-semibold text-ink-950">
            <Calendar className="h-4 w-4" />
            Request leave
          </h3>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label="Membership ID" required>
              <input
                required
                value={createForm.membershipId}
                onChange={(e) =>
                  setCreateForm({
                    ...createForm,
                    membershipId: e.target.value,
                  })
                }
                placeholder="pmb_..."
                className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </Field>
            <Field label="Type" required>
              <select
                required
                value={createForm.leaveType}
                onChange={(e) =>
                  setCreateForm({
                    ...createForm,
                    leaveType: e.target.value,
                  })
                }
                className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              >
                {LEAVE_TYPES.map((t) => (
                  <option key={t} value={t}>
                    {leaveTypeLabel(t)}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Start" required>
              <input
                required
                type="datetime-local"
                value={createForm.startAt}
                onChange={(e) =>
                  setCreateForm({
                    ...createForm,
                    startAt: e.target.value,
                  })
                }
                className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </Field>
            <Field label="End" required>
              <input
                required
                type="datetime-local"
                value={createForm.endAt}
                onChange={(e) =>
                  setCreateForm({ ...createForm, endAt: e.target.value })
                }
                className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </Field>
          </div>

          <Field label="Reason">
            <input
              value={createForm.reason}
              onChange={(e) =>
                setCreateForm({ ...createForm, reason: e.target.value })
              }
              className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>

          <div className="flex gap-2">
            <button
              type="submit"
              className="rounded-md bg-primary px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover"
            >
              Submit
            </button>
            <button
              type="button"
              onClick={() => setShowCreate(false)}
              className="rounded-md border border-line bg-surface px-3.5 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by membership, type or reason..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <button
          onClick={() => setShowFilters((v) => !v)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || hasActiveFilters
              ? 'border-primary bg-primary-bg text-primary'
              : 'border-line bg-surface text-ink-700 hover:bg-surface-2'
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-ink-500 transition-colors hover:bg-surface-2 whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filtered.length} of {rows.length}
        </span>
      </div>

      {showFilters && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
            Status
          </span>
          <div className="flex flex-wrap gap-1.5">
            {STATUS_FILTERS.map((s) => (
              <button
                key={s}
                onClick={() => setStatus(s)}
                className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                  status === s
                    ? 'bg-primary text-white'
                    : 'bg-surface-2 text-ink-600 hover:bg-surface-3'
                }`}
              >
                {s === 'ALL' ? 'All' : leaveStatusLabel(s)}
              </button>
            ))}
          </div>
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading leave records…
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No leave records yet"
          description="Request leave to begin tracking unavailability."
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No records match your filters"
          description="Try adjusting your search or clearing the filters."
          action={
            <button
              onClick={clearFilters}
              className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
            >
              Clear filters
            </button>
          }
        />
      ) : (
        <div className="space-y-2.5">
          {filtered.map((row) => {
            const tone = leaveStatusTone(row.status);
            const isPending = row.status === 'PENDING';
            const isApproved = row.status === 'APPROVED';
            const busy = busyId === row.id;

            return (
              <div
                key={row.id}
                className="flex flex-col gap-3 rounded-lg border border-line bg-surface p-4 sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-semibold text-ink-950">
                      {leaveTypeLabel(row.leaveType)}
                    </span>
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${tone.bg} ${tone.text}`}
                    >
                      <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                      {leaveStatusLabel(row.status)}
                    </span>
                    <Link
                      href={`/providers/${row.membershipId}`}
                      className="font-mono text-[10px] text-ink-400 hover:text-primary"
                    >
                      {row.membershipId}
                    </Link>
                  </div>
                  <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-ink-500">
                    <span className="flex items-center gap-1">
                      <Calendar className="h-3.5 w-3.5" />
                      {formatDate(row.startAt)} – {formatDate(row.endAt)}
                    </span>
                    {row.reason && (
                      <>
                        <span className="text-ink-300">·</span>
                        <span className="truncate max-w-[280px]">
                          {row.reason}
                        </span>
                      </>
                    )}
                  </div>
                </div>

                <div className="flex flex-wrap items-center gap-2">
                  {isPending && (
                    <>
                      <button
                        disabled={busy}
                        onClick={() =>
                          runAction(row.id, (id, v, k) =>
                            workforceService.approveLeave(id, v, k),
                          )
                        }
                        className="flex items-center gap-1 rounded-md bg-green px-2.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-green/90 disabled:opacity-60"
                      >
                        <CheckCircle2 className="h-3.5 w-3.5" />
                        Approve
                      </button>
                      <button
                        disabled={busy}
                        onClick={() =>
                          runAction(row.id, (id, v, k) =>
                            workforceService.rejectLeave(id, v, k),
                          )
                        }
                        className="flex items-center gap-1 rounded-md bg-red px-2.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-red/90 disabled:opacity-60"
                      >
                        <XCircle className="h-3.5 w-3.5" />
                        Reject
                      </button>
                    </>
                  )}
                  {isApproved && (
                    <button
                      disabled={busy}
                      onClick={() =>
                        runAction(row.id, (id, v, k) =>
                          workforceService.cancelLeave(id, v, k),
                        )
                      }
                      className="rounded-md border border-line bg-surface px-2.5 py-1.5 text-xs font-medium text-ink-600 transition-colors hover:bg-surface-2 disabled:opacity-60"
                    >
                      Cancel
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function StatCard({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: 'green' | 'amber' | 'red';
}) {
  const cls = {
    green: 'bg-green-bg text-green',
    amber: 'bg-amber-bg text-amber',
    red: 'bg-red-bg text-red',
  }[tone];

  return (
    <div className="rounded-lg border border-line bg-surface p-4">
      <div className="flex items-center gap-2.5">
        <div className={`flex h-8 w-8 items-center justify-center rounded-md ${cls}`}>
          <span className="text-sm font-bold">{value}</span>
        </div>
        <span className="text-xs font-semibold text-ink-600">{label}</span>
      </div>
    </div>
  );
}

function Field({
  label,
  required,
  children,
}: {
  label: string;
  required?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
        {label}
        {required && <span className="ml-1 text-red">*</span>}
      </label>
      {children}
    </div>
  );
}