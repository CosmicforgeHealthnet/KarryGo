"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  FileText,
  Search,
  RefreshCw,
  SlidersHorizontal,
  X,
  ChevronDown,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { financeService } from "@/lib/services/financeService";
import type {
  ProviderStatement,
  ProviderCompensationItemRow,
  PayrollPeriodRow,
} from "@/lib/types/finance";
import {
  compensationItemSourceLabel,
  compensationItemStatusLabel,
  compensationItemStatusTone,
  formatDate,
  formatMinor,
  payrollStatusLabel,
  payrollStatusTone,
} from "@/lib/utils/financeLabels";

type Row = {
  membershipId: string;
  period: PayrollPeriodRow;
  item: ProviderCompensationItemRow;
};

export default function ProviderStatementsPage() {
  const router = useRouter();

  const [search, setSearch] = useState("");
  const [membershipFilter, setMembershipFilter] = useState("");
  const [showFilters, setShowFilters] = useState(false);

  const [statements, setStatements] = useState<ProviderStatement[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchStatements = useCallback(
    async (opts: { silent?: boolean } = {}) => {
      if (opts.silent) setRefreshing(true);
      else setLoading(true);
      setError(null);
      try {
        // The backend exposes statements per provider membership. To render a
        // global view we fetch every period, then for each unique membership
        // referenced by its items, we load its statements and merge.
        const periodsRes = await financeService.payrollPeriods.list();
        const memberships = new Set<string>();
        for (const period of periodsRes.data) {
          const detail = await financeService.payrollPeriods.get(period.id);
          for (const item of detail.items) {
            memberships.add(item.membership_id);
          }
        }

        const statementsList: ProviderStatement[] = [];
        for (const membershipId of memberships) {
          try {
            const s = await financeService.providerStatements.forMembership(
              membershipId
            );
            statementsList.push(s);
          } catch {
            // Skip memberships that error out; continue aggregating the rest.
          }
        }
        setStatements(statementsList);
      } catch (err: unknown) {
        const message =
          (err as { response?: { data?: { error?: { message?: string } } } })
            ?.response?.data?.error?.message ??
          (err as Error)?.message ??
          "Failed to load provider statements.";
        setError(message);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    []
  );

  useEffect(() => {
    fetchStatements();
  }, [fetchStatements]);

  const allRows: Row[] = useMemo(() => {
    const rows: Row[] = [];
    for (const statement of statements) {
      for (const period of statement.periods) {
        for (const item of period.items) {
          rows.push({
            membershipId: statement.membershipId,
            period: period.period,
            item,
          });
        }
      }
    }
    return rows;
  }, [statements]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    const m = membershipFilter.trim().toLowerCase();
    return allRows.filter((row) => {
      if (m && !row.membershipId.toLowerCase().includes(m)) return false;
      if (!q) return true;
      const haystack = [
        row.membershipId,
        row.period.id,
        row.item.id,
        row.item.reason_code,
        row.item.explanation,
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [allRows, search, membershipFilter]);

  const hasActiveFilters =
    membershipFilter.trim() !== "" || search.trim() !== "";

  const clearFilters = () => {
    setSearch("");
    setMembershipFilter("");
  };

  const stats = useMemo(() => {
    const totalProviders = statements.length;
    const totalItems = allRows.length;
    const totalAmount = allRows.reduce(
      (sum, r) => sum + r.item.amount_minor,
      0
    );
    const currency =
      allRows[0]?.item.currency ??
      allRows[0]?.period.currency ??
      "NGN";
    return { totalProviders, totalItems, totalAmount, currency };
  }, [statements, allRows]);

  if (loading && statements.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">
          Loading provider statements…
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Provider Compensation Statements
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Per-provider compensation statement lines across all payroll periods.
          </p>
        </div>
        <button
          onClick={() => fetchStatements({ silent: true })}
          disabled={refreshing}
          className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
          aria-label="Refresh"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
        </button>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => fetchStatements()}
            className="whitespace-nowrap text-xs font-semibold underline"
          >
            Retry
          </button>
        </div>
      )}

      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Providers
          </div>
          <div className="text-lg font-bold text-ink-950">
            {stats.totalProviders}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Statement lines
          </div>
          <div className="text-lg font-bold text-ink-950">
            {stats.totalItems}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Total amount
          </div>
          <div className="text-lg font-bold text-ink-950">
            {formatMinor(stats.totalAmount, stats.currency)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Periods
          </div>
          <div className="text-lg font-bold text-ink-950">
            {
              new Set(
                allRows.map((r) => `${r.period.id}:${r.membershipId}`)
              ).size
            }
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-surface px-4">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by item, period, reason, or explanation…"
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-surface text-ink-600 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {membershipFilter.trim() ? 1 : 0}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-surface px-3 text-sm font-medium text-ink-500 transition hover:bg-surface-2"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {showFilters && (
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="flex flex-wrap items-center gap-6">
            <div className="flex-1 min-w-[200px]">
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Membership ID
              </div>
              <input
                value={membershipFilter}
                onChange={(e) => setMembershipFilter(e.target.value)}
                placeholder="Filter by membership id"
                className="w-full rounded-lg border border-line bg-surface px-3 py-1.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary"
              />
            </div>
            <button
              onClick={clearFilters}
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 transition hover:bg-surface-2"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {filtered.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/reports-empty.png"
            title={
              hasActiveFilters
                ? "No statement lines match your filters"
                : "No statements yet"
            }
            description={
              hasActiveFilters
                ? "Try adjusting your search or clearing the filters."
                : "Statements are generated once payroll is calculated."
            }
            action={
              hasActiveFilters ? (
                <button
                  onClick={clearFilters}
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Clear filters
                </button>
              ) : undefined
            }
          />
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-surface">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[1100px] border-collapse text-sm">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Provider
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Period
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Source
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Rule
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Amount
                  </th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                    Status
                  </th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((row) => {
                  const itemTone = compensationItemStatusTone(row.item.status);
                  const periodTone = payrollStatusTone(row.period.status);
                  const negative = row.item.amount_minor < 0;
                  return (
                    <tr
                      key={row.item.id}
                      onClick={() =>
                        router.push(`/finance/payroll/${row.period.id}`)
                      }
                      className="group cursor-pointer border-b border-line transition-colors hover:bg-surface-2"
                    >
                      <td className="px-4 py-3 font-mono text-[12px] text-ink-600">
                        {row.membershipId}
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-[12px] text-ink-950">
                          {formatDate(row.period.period_start)} –{" "}
                          {formatDate(row.period.period_end)}
                        </div>
                        <span
                          className={`mt-1 inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${periodTone.bg} ${periodTone.text}`}
                        >
                          <span
                            className={`h-1 w-1 rounded-full ${periodTone.dot}`}
                          />
                          {payrollStatusLabel(row.period.status)}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-ink-950">
                          {compensationItemSourceLabel(row.item.source_type)}
                        </div>
                        <div className="font-mono text-[11px] text-ink-500">
                          {row.item.source_reference}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="font-mono text-[12px] text-ink-600">
                          {row.item.rule_id}
                        </div>
                        <div className="text-[11px] text-ink-500">
                          v{row.item.rule_version}
                        </div>
                      </td>
                      <td
                        className={`px-4 py-3 font-semibold ${
                          negative ? "text-red" : "text-ink-950"
                        }`}
                      >
                        {negative ? "− " : ""}
                        {formatMinor(
                          Math.abs(row.item.amount_minor),
                          row.item.currency
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${itemTone.bg} ${itemTone.text}`}
                        >
                          <span
                            className={`h-1 w-1 rounded-full ${itemTone.dot}`}
                          />
                          {compensationItemStatusLabel(row.item.status)}
                        </span>
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