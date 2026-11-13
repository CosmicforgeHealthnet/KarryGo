'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { ArrowRight, Search } from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { workforceService } from '@/lib/services/workforceService';
import {
  formatDate,
  profileStatusLabel,
  profileStatusTone,
  relationshipTypeLabel,
} from '@/lib/utils/workforceLabels';
import type { Profile } from '@/lib/types/workforce';

// Workforce profiles: the employment record (role, relationship, branch,
// supervisor) for each provider the company manages.
export default function WorkforceProfilesPage() {
  const [rows, setRows] = useState<Profile[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');

  useEffect(() => {
    let cancelled = false;
    workforceService
      .listProfiles({ limit: 100 })
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (!cancelled) setError(err?.response?.data?.error?.message ?? 'Failed to load workforce profiles.');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter((r) => [r.employeeNumber, r.role, r.membershipId, r.branchId].join(' ').toLowerCase().includes(q));
  }, [rows, search]);

  return (
    <div className="space-y-5">
      <div>
        <h1>Workforce Profiles</h1>
        <p className="mt-1.5">Employment records for your managed providers: role, relationship, branch and supervisor.</p>
      </div>

      {error && <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">{error}</div>}

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by role or employee number..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>
        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filtered.length} of {rows.length}
        </span>
      </div>

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">Loading profiles…</div>
      ) : rows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No workforce profiles yet"
          description="Profiles are created for providers your company manages. Add providers first, then manage their schedules and leave here."
          action={
            <Link
              href="/providers"
              className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
            >
              Go to Providers
            </Link>
          }
        />
      ) : (
        <div className="overflow-hidden rounded-lg border border-line bg-surface">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] border-collapse text-sm">
              <thead>
                <tr className="bg-surface-1">
                  {['Employee', 'Role', 'Relationship', 'Status', 'Since', ''].map((h) => (
                    <th key={h} className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {filtered.map((p) => {
                  const tone = profileStatusTone(p.status);
                  return (
                    <tr key={p.id} className="border-b border-line hover:bg-surface-2">
                      <td className="px-4 py-3 text-xs">
                        <div className="font-medium text-ink-950">{p.employeeNumber || '—'}</div>
                        <div className="font-mono text-[10px] text-ink-400">{p.membershipId}</div>
                      </td>
                      <td className="px-4 py-3 text-xs text-ink-600">{p.role || '—'}</td>
                      <td className="px-4 py-3 text-xs text-ink-600">{relationshipTypeLabel(p.relationshipType)}</td>
                      <td className="px-4 py-3">
                        <span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${tone.bg} ${tone.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                          {profileStatusLabel(p.status)}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-xs text-ink-500">{formatDate(p.startDate)}</td>
                      <td className="px-4 py-3 text-right">
                        <Link href={`/workforce/${p.id}`} className="inline-flex items-center gap-1 text-xs font-medium text-ink-400 hover:text-primary">
                          View <ArrowRight className="h-3.5 w-3.5" />
                        </Link>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
