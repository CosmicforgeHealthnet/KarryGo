"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, Ban, RefreshCw, Search } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { DataTable, EmptyBlock, Loading, PageHeader, SecondaryButton, StatGrid, StatusPill, formatWhen, humanize } from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { complianceService } from "@/lib/services/safetyService";
import { COMPLIANCE_SUBJECTS, type Restriction } from "@/lib/types/safety";

// Restrictions raised by compliance (expired documents, failed checks,
// safety blocks) that stop a provider, asset or other subject from working.
export default function RestrictionsPage() {
  const router = useRouter();
  const [rows, setRows] = useState<Restriction[]>([]);
  const [filter, setFilter] = useState({ subjectType: "", state: "" });
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async (f: { subjectType: string; state: string }) => {
    try {
      setRows(await complianceService.listRestrictions({ subjectType: f.subjectType || undefined, state: f.state || undefined }));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Restrictions could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => load(filter));
  }, [load, filter]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return q === "" ? rows : rows.filter((r) => [r.subjectReference, r.reasonCode, r.sourceType, r.sourceReference].join(" ").toLowerCase().includes(q));
  }, [rows, search]);

  if (loading) return <Loading label="Loading restrictions..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Restrictions"
        description="Compliance blocks on providers, assets and other subjects, and their clearance."
        actions={
          <>
            <SecondaryButton onClick={() => load(filter)}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <SecondaryButton onClick={() => router.push("/compliance/checks")}>Run a check</SecondaryButton>
            <SecondaryButton onClick={() => router.push("/compliance")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Compliance
            </SecondaryButton>
          </>
        }
      />
      <ApiErrorNotice error={error} />
      <StatGrid
        items={[
          { label: "Shown", value: rows.length },
          { label: "Applied", value: rows.filter((r) => r.state === "APPLIED").length, tone: "red" },
          { label: "Clearance pending", value: rows.filter((r) => r.state === "CLEAR_PENDING").length, tone: "amber" },
          { label: "Cleared", value: rows.filter((r) => r.state === "CLEARED").length, tone: "green" },
        ]}
      />
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search by subject or reason..." className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400" />
        </div>
        <select value={filter.subjectType} onChange={(e) => setFilter({ ...filter, subjectType: e.target.value })} aria-label="Subject type" className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm">
          <option value="">All subjects</option>
          {COMPLIANCE_SUBJECTS.map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
        </select>
        <select value={filter.state} onChange={(e) => setFilter({ ...filter, state: e.target.value })} aria-label="State" className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm">
          <option value="">All states</option>
          {["APPLIED", "CLEAR_PENDING", "CLEARED"].map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
        </select>
      </div>
      {filtered.length === 0 ? (
        <EmptyBlock icon={<Ban className="h-8 w-8" />} title="No restrictions" description="Restrictions raised by compliance checks and safety blocks appear here." />
      ) : (
        <DataTable
          rows={filtered}
          rowKey={(r) => r.id}
          columns={[
            { header: "Subject", cell: (r) => <><div className="text-ink-950">{humanize(r.subjectType)}</div><div className="font-mono text-[10px] text-ink-500">{r.subjectReference}</div></> },
            { header: "Reason", cell: (r) => humanize(r.reasonCode) },
            { header: "Source", cell: (r) => `${humanize(r.sourceType)}${r.sourceReference ? ` · ${r.sourceReference}` : ""}` },
            { header: "Severity", cell: (r) => <StatusPill status={r.severity} /> },
            { header: "State", cell: (r) => <StatusPill status={r.state} /> },
            { header: "Since", cell: (r) => formatWhen(r.effectiveAt) },
          ]}
        />
      )}
    </div>
  );
}
