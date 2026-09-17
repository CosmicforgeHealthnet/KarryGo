"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  CalendarDays,
  Building2,
  FileCheck,
  Clock,
  CheckCircle2,
  Send,
  Power,
  Pause,
  PowerOff,
  RotateCw,
} from "lucide-react";
import { crmService, type ContractLifecycleInput } from "@/lib/services/crmService";
import type {
  ContractView,
  ContractHistoryView,
  ContractVersionView,
} from "@/lib/types/crm";
import {
  contractStatusLabel,
  contractStatusTone,
  formatDate,
  formatDateTime,
  timeAgo,
} from "@/lib/utils/crmLabels";

type ActionKey = "submit" | "activate" | "suspend" | "resume" | "terminate" | "renew";

export default function ContractDetailPage() {
  const router = useRouter();
  const params = useParams();
  const contractId = params.contractId as string;

  const [contract, setContract] = useState<ContractView | null>(null);
  const [history, setHistory] = useState<ContractHistoryView[]>([]);
  const [versions, setVersions] = useState<ContractVersionView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<ActionKey | null>(null);

  const [showLifecycleModal, setShowLifecycleModal] = useState<ActionKey | null>(null);
  const [reasonCode, setReasonCode] = useState("");

  const fetchAll = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [contractResult, historyResult, versionsResult] = await Promise.all([
        crmService.getContract(contractId),
        crmService.getContractHistory(contractId, { limit: 20 }),
        crmService.listContractVersions(contractId, { limit: 10 }),
      ]);
      setContract(contractResult);
      setHistory(historyResult.data);
      setVersions(versionsResult.data);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load contract.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [contractId]);

  useEffect(() => {
    if (contractId) fetchAll();
  }, [contractId, fetchAll]);

  const run = async (key: ActionKey, fn: () => Promise<ContractView | void>) => {
    setBusy(key);
    setError(null);
    try {
      await fn();
      await fetchAll();
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Action failed.";
      setError(message);
    } finally {
      setBusy(null);
    }
  };

  const handleSubmit = () =>
    run("submit", async () => {
      if (!contract) return;
      await crmService.submitContract(contract.id, contract.version);
    });

  const handleActivate = () =>
    run("activate", async () => {
      if (!contract) return;
      await crmService.activateContract(
        contract.id,
        { signatoryReferences: [] },
        contract.version
      );
    });

  const handleLifecycle = (key: ActionKey, fn: (input: ContractLifecycleInput) => Promise<ContractView>) =>
    run(key, async () => {
      if (!contract) return;
      if (!reasonCode.trim()) throw new Error("Reason code is required.");
      await fn({ reasonCode: reasonCode.trim() });
      setShowLifecycleModal(null);
      setReasonCode("");
    });

  const handleRenew = () =>
    run("renew", async () => {
      if (!contract) return;
      const start = new Date();
      const end = new Date();
      end.setFullYear(end.getFullYear() + 1);
      await crmService.renewContract(
        contract.id,
        {
          effectiveFrom: start.toISOString(),
          effectiveTo: end.toISOString(),
        },
        contract.version
      );
    });

  if (loading && !contract) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading contract…</p>
      </div>
    );
  }

  if (error && !contract) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load contract
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <div className="mt-3.5 flex gap-2">
          <button
            onClick={fetchAll}
            className="rounded-lg border border-line bg-surface px-4 py-1.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2"
          >
            Retry
          </button>
          <button
            onClick={() => router.push("/crm/contracts")}
            className="rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to contracts
          </button>
        </div>
      </div>
    );
  }

  if (!contract) return null;

  const statusTone = contractStatusTone(contract.status);
  const canSubmit = contract.status === "DRAFT";
  const canActivate = contract.status === "APPROVED";
  const canSuspend = contract.status === "ACTIVE";
  const canResume = contract.status === "SUSPENDED";
  const canTerminate =
    contract.status !== "TERMINATED" &&
    contract.status !== "EXPIRED" &&
    contract.status !== "SUPERSEDED";
  const canRenew =
    contract.status === "ACTIVE" ||
    contract.status === "SUSPENDED" ||
    contract.status === "RENEWAL_PENDING";

  const activeVersion = contract.activeVersionId
    ? versions.find((v) => v.id === contract.activeVersionId)
    : undefined;

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/crm/contracts")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to contracts"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                {contract.contractNumber}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {contractStatusLabel(contract.status)}
              </span>
              <span className="text-sm text-ink-400">
                v{contract.currentVersion}
              </span>
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-ink-500">
              <span className="flex items-center gap-1.5">
                <Building2 className="h-3.5 w-3.5" />
                <span className="font-mono">{contract.customerId}</span>
              </span>
              <span className="flex items-center gap-1.5">
                <CalendarDays className="h-3.5 w-3.5" />
                {formatDate(contract.effectiveFrom)}
                {contract.effectiveTo ? ` → ${formatDate(contract.effectiveTo)}` : ""}
              </span>
            </div>
          </div>
        </div>
        <button
          onClick={fetchAll}
          disabled={loading}
          className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
          aria-label="Refresh"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
        </button>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => setError(null)}
            className="whitespace-nowrap text-xs font-semibold underline"
          >
            Dismiss
          </button>
        </div>
      )}

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Contract Details
            </h3>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Effective from
                </div>
                <div className="text-sm font-medium text-ink-950">
                  {formatDate(contract.effectiveFrom)}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Effective to
                </div>
                <div className="text-sm font-medium text-ink-950">
                  {contract.effectiveTo ? formatDate(contract.effectiveTo) : "Open"}
                </div>
              </div>
              {contract.quotationId && (
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                    Source quotation
                  </div>
                  <div className="font-mono text-sm font-medium text-ink-950">
                    {contract.quotationId}
                  </div>
                </div>
              )}
              {contract.supersedesId && (
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                    Supersedes
                  </div>
                  <div className="font-mono text-sm font-medium text-ink-950">
                    {contract.supersedesId}
                  </div>
                </div>
              )}
            </div>
          </div>

          {activeVersion && (
            <div className="rounded-2xl border border-line bg-surface p-5">
              <div className="mb-3 flex items-center gap-2">
                <FileCheck className="h-5 w-5 text-green" />
                <h3 className="text-[15px] font-semibold text-ink-950">
                  Active Version
                </h3>
                <span className="rounded-full bg-green-bg px-2 py-0.5 text-[10px] font-semibold text-green">
                  v{activeVersion.versionNumber}
                </span>
              </div>
              <div className="grid grid-cols-2 gap-3 text-sm">
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                    Activated by
                  </div>
                  <div className="text-ink-950">
                    {activeVersion.activatedBy || "—"}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                    Activated at
                  </div>
                  <div className="text-ink-950">
                    {formatDateTime(activeVersion.activatedAt)}
                  </div>
                </div>
                <div className="col-span-2">
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                    Content fingerprint
                  </div>
                  <div className="break-all font-mono text-xs text-ink-600">
                    {activeVersion.contentFingerprint}
                  </div>
                </div>
                {activeVersion.signatoryReferences.length > 0 && (
                  <div className="col-span-2">
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                      Signatories
                    </div>
                    <div className="flex flex-wrap gap-1">
                      {activeVersion.signatoryReferences.map((s, i) => (
                        <span
                          key={i}
                          className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-600"
                        >
                          {s}
                        </span>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}

          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              History
            </h3>
            {history.length === 0 ? (
              <div className="py-6 text-center text-sm text-ink-500">
                No history yet.
              </div>
            ) : (
              <div className="space-y-3">
                {history.map((event, index) => (
                  <div key={event.id} className="flex items-start gap-3">
                    <div className="flex flex-col items-center">
                      <div
                        className={`h-2.5 w-2.5 rounded-full ${
                          event.newStatus === "ACTIVE"
                            ? "bg-green"
                            : event.newStatus === "TERMINATED"
                              ? "bg-red"
                              : event.newStatus === "SUSPENDED"
                                ? "bg-amber"
                                : "bg-ink-400"
                        }`}
                      />
                      {index < history.length - 1 && (
                        <div className="mt-1 h-full w-0.5 bg-line" />
                      )}
                    </div>
                    <div className="flex-1 pb-3">
                      <div className="flex flex-wrap items-center gap-2">
                        {event.previousStatus && (
                          <span className="rounded bg-surface-2 px-2 py-0.5 text-[10px] text-ink-500">
                            {event.previousStatus}
                          </span>
                        )}
                        {event.previousStatus && (
                          <span className="text-ink-400">→</span>
                        )}
                        <span className="rounded bg-primary-bg px-2 py-0.5 text-[10px] font-medium text-primary">
                          {event.newStatus}
                        </span>
                        {event.reasonCode && (
                          <span className="text-[11px] text-ink-500">
                            {event.reasonCode}
                          </span>
                        )}
                      </div>
                      <div className="mt-1 text-xs text-ink-500">
                        {event.actorType} · {formatDateTime(event.occurredAt)} ·{" "}
                        {timeAgo(event.occurredAt)}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Actions
            </h3>
            <div className="space-y-2">
              {canSubmit && (
                <button
                  onClick={handleSubmit}
                  disabled={busy !== null}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2 disabled:opacity-50"
                >
                  <span className="flex items-center gap-2">
                    {busy === "submit" ? (
                      <RefreshCw className="h-4 w-4 animate-spin" />
                    ) : (
                      <Send className="h-4 w-4" />
                    )}
                    Submit for approval
                  </span>
                </button>
              )}
              {canActivate && (
                <button
                  onClick={handleActivate}
                  disabled={busy !== null}
                  className="flex w-full items-center justify-between rounded-xl border border-green/30 bg-green-bg/50 px-4 py-2.5 text-[12.5px] font-medium text-green transition hover:bg-green-bg disabled:opacity-50"
                >
                  <span className="flex items-center gap-2">
                    {busy === "activate" ? (
                      <RefreshCw className="h-4 w-4 animate-spin" />
                    ) : (
                      <Power className="h-4 w-4" />
                    )}
                    Activate contract
                  </span>
                </button>
              )}
              {canSuspend && (
                <button
                  onClick={() => {
                    setReasonCode("");
                    setShowLifecycleModal("suspend");
                  }}
                  disabled={busy !== null}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2 disabled:opacity-50"
                >
                  <span className="flex items-center gap-2">
                    <Pause className="h-4 w-4" />
                    Suspend
                  </span>
                </button>
              )}
              {canResume && (
                <button
                  onClick={() => {
                    setReasonCode("");
                    setShowLifecycleModal("resume");
                  }}
                  disabled={busy !== null}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2 disabled:opacity-50"
                >
                  <span className="flex items-center gap-2">
                    <Power className="h-4 w-4" />
                    Resume
                  </span>
                </button>
              )}
              {canRenew && (
                <button
                  onClick={handleRenew}
                  disabled={busy !== null}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-surface px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-surface-2 disabled:opacity-50"
                >
                  <span className="flex items-center gap-2">
                    {busy === "renew" ? (
                      <RefreshCw className="h-4 w-4 animate-spin" />
                    ) : (
                      <RotateCw className="h-4 w-4" />
                    )}
                    Open renewal
                  </span>
                </button>
              )}
              {canTerminate && (
                <button
                  onClick={() => {
                    setReasonCode("");
                    setShowLifecycleModal("terminate");
                  }}
                  disabled={busy !== null}
                  className="flex w-full items-center justify-between rounded-xl border border-red/30 bg-red-bg/50 px-4 py-2.5 text-[12.5px] font-medium text-red transition hover:bg-red-bg disabled:opacity-50"
                >
                  <span className="flex items-center gap-2">
                    <PowerOff className="h-4 w-4" />
                    Terminate
                  </span>
                </button>
              )}
              {!canSubmit &&
                !canActivate &&
                !canSuspend &&
                !canResume &&
                !canRenew &&
                !canTerminate && (
                  <div className="py-4 text-center text-sm text-ink-500">
                    No actions available in the current state.
                  </div>
                )}
            </div>
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Metadata
            </h3>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Version</span>
                <span className="font-mono font-medium text-ink-950">
                  {contract.version}
                </span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Created</span>
                <span className="text-ink-950">{timeAgo(contract.createdAt)}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Updated</span>
                <span className="text-ink-950">{timeAgo(contract.updatedAt)}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      {showLifecycleModal && contract && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-surface p-6 shadow-2xl">
            <div className="flex items-center gap-2">
              <AlertCircle className="h-5 w-5 text-amber" />
              <h3 className="text-lg font-semibold text-ink-950">
                {showLifecycleModal === "suspend" && "Suspend contract"}
                {showLifecycleModal === "resume" && "Resume contract"}
                {showLifecycleModal === "terminate" && "Terminate contract"}
              </h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              {showLifecycleModal === "terminate"
                ? "This is a terminal action. A reason code is required."
                : "A reason code is required for audit purposes."}
            </p>
            <div className="mt-4">
              <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                Reason code
              </label>
              <input
                value={reasonCode}
                onChange={(e) => setReasonCode(e.target.value)}
                placeholder="e.g. customer_request, payment_issue…"
                className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </div>
            <div className="mt-5 flex gap-2.5">
              <button
                onClick={() => {
                  setShowLifecycleModal(null);
                  setReasonCode("");
                }}
                className="flex-1 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  if (showLifecycleModal === "suspend") {
                    handleLifecycle("suspend", (input) =>
                      crmService.suspendContract(contract.id, input, contract.version)
                    );
                  } else if (showLifecycleModal === "resume") {
                    handleLifecycle("resume", (input) =>
                      crmService.resumeContract(contract.id, input, contract.version)
                    );
                  } else if (showLifecycleModal === "terminate") {
                    handleLifecycle("terminate", (input) =>
                      crmService.terminateContract(contract.id, input, contract.version)
                    );
                  }
                }}
                disabled={busy !== null || !reasonCode.trim()}
                className={`flex-1 rounded-lg px-4 py-2 text-sm font-medium text-white transition disabled:cursor-not-allowed disabled:opacity-50 ${
                  showLifecycleModal === "terminate"
                    ? "bg-red hover:bg-red/90"
                    : "bg-primary hover:bg-primary/90"
                }`}
              >
                {busy === showLifecycleModal ? (
                  <RefreshCw className="mr-1 inline h-4 w-4 animate-spin" />
                ) : null}
                Confirm
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}