"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  ChevronDown,
  Clock,
  FileCheck,
  Filter,
  RefreshCw,
  Search,
  ShieldAlert,
  X,
  XCircle,
} from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { formatWhen, humanize } from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { complianceStatusService } from "@/lib/services/safetyService";
import type { ComplianceRequirement, ComplianceStatus } from "@/lib/types/safety";

// The compliance requirement list shared by the provider and asset
// compliance tabs: one row per requirement the subject was evaluated
// against, with the state the compliance service recorded.

const STATE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  COMPLIANT: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  DUE_SOON: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  PENDING: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  EXPIRED: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  NON_COMPLIANT: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  RESTRICTED: { bg: "bg-red-bg", text: "text-red", icon: <ShieldAlert className="h-3.5 w-3.5" /> },
  UNKNOWN: { bg: "bg-surface-2", text: "text-ink-600", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

const STATE_FILTERS = ["All", "COMPLIANT", "DUE_SOON", "PENDING", "EXPIRED", "NON_COMPLIANT", "RESTRICTED", "UNKNOWN"];
const ATTENTION = ["EXPIRED", "NON_COMPLIANT", "RESTRICTED"];

export function ComplianceStatusList({ subject, subjectId }: { subject: "PROVIDER" | "ASSET"; subjectId: string }) {
  const [statuses, setStatuses] = useState<ComplianceStatus[]>([]);
  const [requirements, setRequirements] = useState<ComplianceRequirement[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [selectedState, setSelectedState] = useState("All");
  const [search, setSearch] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const rows = subject === "PROVIDER" ? await complianceStatusService.forProvider(subjectId) : await complianceStatusService.forAsset(subjectId);
      setStatuses(rows);
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Compliance status could not be loaded."));
    } finally {
      setLoading(false);
    }
    // Requirement names are a label lookup; the list still works without them.
    complianceStatusService.requirements().then(setRequirements).catch(() => setRequirements([]));
  }, [subject, subjectId]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const requirementName = useCallback(
    (id: string) => {
      const r = requirements.find((x) => x.id === id);
      return r ? r.name || r.code : id;
    },
    [requirements],
  );

  const filtered = useMemo(
    () =>
      statuses.filter((s) => {
        const matchesSearch = search.trim() === "" || requirementName(s.requirementId).toLowerCase().includes(search.trim().toLowerCase());
        const matchesState = selectedState === "All" || s.state === selectedState;
        return matchesSearch && matchesState;
      }),
    [statuses, search, selectedState, requirementName],
  );

  const compliantCount = statuses.filter((s) => s.state === "COMPLIANT").length;
  const dueCount = statuses.filter((s) => s.state === "DUE_SOON" || s.state === "PENDING").length;
  const attentionCount = statuses.filter((s) => ATTENTION.includes(s.state)).length;

  const hasFilters = selectedState !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedState("All");
  };

  if (loading && statuses.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading compliance...</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <ApiErrorNotice error={error} />

      {/* ── Stats ── */}
      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Compliant</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{compliantCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Due Soon or Pending</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{dueCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <XCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Needs Attention</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{attentionCount}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search requirements..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <button
          onClick={() => {
            const next = (STATE_FILTERS.indexOf(selectedState) + 1) % STATE_FILTERS.length;
            setSelectedState(STATE_FILTERS[next]);
          }}
          className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2 whitespace-nowrap"
        >
          <Filter className="h-3.5 w-3.5 text-ink-400" />
          {selectedState === "All" ? "All" : humanize(selectedState)}
          <ChevronDown className="h-3.5 w-3.5 text-ink-400" />
        </button>

        {hasFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <button
          onClick={load}
          className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2 whitespace-nowrap"
        >
          <RefreshCw className="h-3.5 w-3.5 text-ink-400" />
          Refresh
        </button>

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">{filtered.length} requirements</span>
      </div>

      {/* ── Requirements List ── */}
      {filtered.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-12">
          <div className="rounded-full bg-surface-2 p-3">
            <FileCheck className="h-8 w-8 text-green" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">{statuses.length === 0 ? "No compliance results yet" : "No requirements found"}</h3>
          <p className="mt-1 text-sm text-ink-500">
            {statuses.length === 0
              ? "No requirement has been evaluated for this record yet."
              : "No compliance requirements match your current filters."}
          </p>
          {hasFilters && (
            <button
              onClick={clearFilters}
              className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
            >
              Clear filters
            </button>
          )}
        </div>
      ) : (
        <div className="space-y-2.5">
          {filtered.map((s) => {
            const style = STATE_STYLES[s.state] ?? STATE_STYLES.UNKNOWN;
            return (
              <div
                key={s.id}
                className="flex flex-col gap-2 rounded-lg border border-line bg-white p-3.5 transition-colors hover:bg-surface-2 sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium text-ink-950">{requirementName(s.requirementId)}</span>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${style.bg} ${style.text}`}>
                      {style.icon}
                      {humanize(s.state)}
                    </span>
                    {s.sourceType && (
                      <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-600">{humanize(s.sourceType)}</span>
                    )}
                  </div>
                  <div className="mt-0.5 flex flex-wrap items-center gap-3 text-[10px] text-ink-500">
                    {s.expiresAt && (
                      <span className="flex items-center gap-1">
                        <Clock className="h-3.5 w-3.5" />
                        Expires: {formatWhen(s.expiresAt)}
                      </span>
                    )}
                    {s.dueAt && <span>Due: {formatWhen(s.dueAt)}</span>}
                    <span>Evaluated: {formatWhen(s.evaluatedAt)}</span>
                    {s.restrictionReference && <span>Restriction: {s.restrictionReference}</span>}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
