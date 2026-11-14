"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, CheckCircle2, FileCheck, RefreshCw, Search, XCircle } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  DangerButton,
  DataTable,
  EmptyBlock,
  KeyValueGrid,
  Loading,
  Notice,
  PageHeader,
  Panel,
  PrimaryButton,
  SecondaryButton,
  StatGrid,
  StatusPill,
  Timeline,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { proofService } from "@/lib/services/safetyService";
import type { ProofRecord, ProofRecordDetail } from "@/lib/types/safety";

// Proof records (photos, signatures, codes, scans) captured during work, and
// the organization's verification of them.
export default function ProofRecordsPage() {
  const router = useRouter();
  const [rows, setRows] = useState<ProofRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");

  const [detail, setDetail] = useState<ProofRecordDetail | null>(null);
  const [reasons, setReasons] = useState("");
  const [keys, setKeys] = useState({ verify: newIdempotencyKey(), reject: newIdempotencyKey() });
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setRows(await proofService.list());
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Proof records could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const open = async (r: ProofRecord) => {
    setActionError(null);
    setNotice(null);
    try {
      setDetail(await proofService.get(r.id));
      setKeys({ verify: newIdempotencyKey(), reject: newIdempotencyKey() });
    } catch (e) {
      setActionError(describeApiError(e, "The proof record could not be loaded."));
    }
  };

  const decide = async (decision: "verify" | "reject") => {
    if (!detail) return;
    setBusy(true);
    setActionError(null);
    try {
      const codes = reasons.split(",").map((s) => s.trim()).filter(Boolean);
      setDetail(await proofService.decide(detail.record.id, decision, codes, keys[decision]));
      setKeys((k) => ({ ...k, [decision]: newIdempotencyKey() }));
      setReasons("");
      setNotice(decision === "verify" ? "Proof verified." : "Proof rejected.");
      await load();
    } catch (e) {
      setActionError(describeApiError(e, "The decision could not be recorded."));
    } finally {
      setBusy(false);
    }
  };

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter(
      (r) =>
        (status === "" || r.verificationStatus === status) &&
        (q === "" || [r.id, r.proofType, r.orderId ?? "", r.shipmentId ?? "", r.canonicalTripReference ?? "", r.actorReference].join(" ").toLowerCase().includes(q)),
    );
  }, [rows, search, status]);

  if (loading) return <Loading label="Loading proof records..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Proof Records"
        description="Proof captured at pickup and delivery, and its verification."
        actions={
          <>
            <SecondaryButton onClick={load}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <SecondaryButton onClick={() => router.push("/compliance")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Compliance
            </SecondaryButton>
          </>
        }
      />
      <ApiErrorNotice error={error} />
      <Notice message={notice} />
      <StatGrid
        items={[
          { label: "Total", value: rows.length },
          { label: "Pending", value: rows.filter((r) => r.verificationStatus === "PENDING").length, tone: "amber" },
          { label: "Verified", value: rows.filter((r) => r.verificationStatus === "VERIFIED").length, tone: "green" },
          { label: "Rejected", value: rows.filter((r) => r.verificationStatus === "REJECTED").length, tone: "red" },
        ]}
      />

      {detail && (
        <Panel title={`Proof ${detail.record.id}`} description={humanize(detail.record.proofType)} onClose={() => setDetail(null)}>
          <ApiErrorNotice error={actionError} />
          <KeyValueGrid
            items={[
              ["Status", <StatusPill key="s" status={detail.effectiveStatus} />],
              ["Order", detail.record.orderId || "—"],
              ["Shipment", detail.record.shipmentId || "—"],
              ["Trip", detail.record.canonicalTripReference || "—"],
              ["Captured by", `${humanize(detail.record.actorType)} ${detail.record.actorReference}`],
              ["Captured at", formatWhen(detail.record.sourceTimestamp)],
              ["Received", formatWhen(detail.record.receivedAt)],
              ["Media", detail.record.mediaFileId || "—"],
              ["Policy", detail.record.policySnapshotReference],
            ]}
          />
          <Timeline
            entries={(detail.verifications ?? []).map((v) => ({
              id: v.id,
              title: `${humanize(v.status)} by ${humanize(v.authorityType)} ${v.authorityReference}`,
              detail: (v.reasonCodes ?? []).join(", ") || undefined,
              at: v.verifiedAt,
            }))}
          />
          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-[240px] flex-1">
              <Field label="Reason codes" hint="Comma separated; needed when rejecting">
                <input value={reasons} onChange={(e) => setReasons(e.target.value)} placeholder="SIGNATURE_ILLEGIBLE" className={INPUT} />
              </Field>
            </div>
            <PrimaryButton disabled={busy} onClick={() => decide("verify")}>
              <CheckCircle2 className="h-3.5 w-3.5" /> Verify
            </PrimaryButton>
            <DangerButton disabled={busy} onClick={() => decide("reject")}>
              <XCircle className="h-3.5 w-3.5" /> Reject
            </DangerButton>
          </div>
        </Panel>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search by order, shipment, trip..." className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400" />
        </div>
        <select value={status} onChange={(e) => setStatus(e.target.value)} aria-label="Status" className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm">
          <option value="">All statuses</option>
          {["PENDING", "VERIFIED", "REJECTED"].map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
        </select>
      </div>

      {filtered.length === 0 ? (
        <EmptyBlock icon={<FileCheck className="h-8 w-8" />} title="No proof records" description="Proof captured by providers during company work appears here." />
      ) : (
        <DataTable
          rows={filtered}
          rowKey={(r) => r.id}
          onRowClick={open}
          columns={[
            { header: "Proof", cell: (r) => <span className="font-mono text-ink-950">{r.id}</span> },
            { header: "Type", cell: (r) => humanize(r.proofType) },
            { header: "For", cell: (r) => r.orderId || r.shipmentId || r.canonicalTripReference || "—" },
            { header: "Captured by", cell: (r) => humanize(r.actorType) },
            { header: "Status", cell: (r) => <StatusPill status={r.verificationStatus} /> },
            { header: "Captured", cell: (r) => formatWhen(r.sourceTimestamp) },
          ]}
        />
      )}
    </div>
  );
}
