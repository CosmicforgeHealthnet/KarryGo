"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  CheckCircle2,
  PowerOff,
  Scale,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type { CompensationRuleRow } from "@/lib/types/finance";
import {
  compensationRuleStatusLabel,
  compensationRuleStatusTone,
  compensationRuleTypeLabel,
  compensationRuleTypeTone,
  formatDate,
  formatDateTime,
  formatMinor,
  timeAgo,
} from "@/lib/utils/financeLabels";

export default function CompensationRuleDetailPage() {
  const router = useRouter();
  const params = useParams();
  const ruleId = params.ruleId as string;

  const [rule, setRule] = useState<CompensationRuleRow | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const fetchRule = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await financeService.compensationRules.get(ruleId);
      setRule(result);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to load rule.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [ruleId]);

  useEffect(() => {
    if (ruleId) fetchRule();
  }, [ruleId, fetchRule]);

  const run = async (key: string, fn: () => Promise<CompensationRuleRow>) => {
    setBusy(key);
    setError(null);
    try {
      const updated = await fn();
      setRule(updated);
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

  if (loading && !rule) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading rule…</p>
      </div>
    );
  }

  if (error && !rule) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-surface py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">
          Could not load rule
        </h3>
        <p className="mt-1 max-w-sm px-4 text-center text-sm text-ink-500">
          {error}
        </p>
        <div className="mt-3.5 flex gap-2">
          <button
            onClick={fetchRule}
            className="rounded-lg border border-line bg-surface px-4 py-1.5 text-[11.5px] font-medium text-ink-600 transition hover:bg-surface-2"
          >
            Retry
          </button>
          <button
            onClick={() => router.push("/finance/compensation-rules")}
            className="rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to rules
          </button>
        </div>
      </div>
    );
  }

  if (!rule) return null;

  const statusTone = compensationRuleStatusTone(rule.status);
  const typeTone = compensationRuleTypeTone(rule.rule_type);
  const canApprove =
    rule.status === "DRAFT" || rule.status === "PENDING_APPROVAL";
  const canDeactivate =
    rule.status === "APPROVED" || rule.status === "ACTIVE";
  const isDeduction = rule.rule_type === "DEDUCTION";

  const config = rule.calculation_configuration;
  const extraConfigKeys = Object.keys(config).filter(
    (k) => k !== "amountMinor" && k !== "approvedBasis"
  );

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/finance/compensation-rules")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
            aria-label="Back to rules"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                {rule.name}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {compensationRuleStatusLabel(rule.status)}
              </span>
              <span
                className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${typeTone.bg} ${typeTone.text}`}
              >
                {compensationRuleTypeLabel(rule.rule_type)}
              </span>
              <span className="text-sm text-ink-400">v{rule.version}</span>
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-ink-500">
              <span className="font-mono">{rule.id}</span>
              <span className="text-ink-300">·</span>
              <span className="font-mono">{rule.currency}</span>
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={fetchRule}
            disabled={loading}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          {canApprove && (
            <button
              onClick={() =>
                run("approve", () =>
                  financeService.compensationRules.approve(rule.id, rule.version)
                )
              }
              disabled={busy !== null}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90 disabled:opacity-50"
            >
              {busy === "approve" ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <CheckCircle2 className="h-3.5 w-3.5" />
              )}
              Approve
            </button>
          )}
          {canDeactivate && (
            <button
              onClick={() =>
                run("deactivate", () =>
                  financeService.compensationRules.deactivate(
                    rule.id,
                    rule.version
                  )
                )
              }
              disabled={busy !== null}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-red/30 bg-red-bg/50 px-3.5 text-[11.5px] font-medium text-red transition hover:bg-red-bg disabled:opacity-50"
            >
              {busy === "deactivate" ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <PowerOff className="h-3.5 w-3.5" />
              )}
              Deactivate
            </button>
          )}
        </div>
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
          <div className="rounded-2xl border border-line bg-surface p-6">
            <div className="flex items-center gap-2 text-sm text-ink-500">
              <Scale className="h-4 w-4" />
              <span>Deterministic amount</span>
            </div>
            <div className="mt-2">
              <span
                className={`text-3xl font-bold ${
                  isDeduction ? "text-red" : "text-ink-950"
                }`}
              >
                {isDeduction ? "− " : ""}
                {formatMinor(config.amountMinor, rule.currency)}
              </span>
              <span className="ml-2 text-sm text-ink-400">
                per {compensationRuleTypeLabel(rule.rule_type).toLowerCase()}
              </span>
            </div>
            {config.approvedBasis && (
              <div className="mt-3 rounded-lg bg-surface-2 p-3 text-xs text-ink-600">
                <span className="font-medium">Approved basis: </span>
                {config.approvedBasis}
              </div>
            )}
          </div>

          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Applicability
            </h3>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Roles
                </div>
                {rule.applicable_roles.length === 0 ? (
                  <div className="text-sm text-ink-500">All roles</div>
                ) : (
                  <div className="mt-1 flex flex-wrap gap-1">
                    {rule.applicable_roles.map((r, i) => (
                      <span
                        key={i}
                        className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-700"
                      >
                        {String(r)}
                      </span>
                    ))}
                  </div>
                )}
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
                  Services
                </div>
                {rule.applicable_services.length === 0 ? (
                  <div className="text-sm text-ink-500">All services</div>
                ) : (
                  <div className="mt-1 flex flex-wrap gap-1">
                    {rule.applicable_services.map((s, i) => (
                      <span
                        key={i}
                        className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-700"
                      >
                        {String(s)}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>

          {extraConfigKeys.length > 0 && (
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
                Additional configuration
              </h3>
              <pre className="overflow-x-auto rounded-lg bg-surface-2 p-3 text-xs text-ink-700">
                {JSON.stringify(
                  Object.fromEntries(
                    extraConfigKeys.map((k) => [k, config[k]])
                  ),
                  null,
                  2
                )}
              </pre>
            </div>
          )}
        </div>

        <div className="space-y-5">
          <div className="rounded-2xl border border-line bg-surface p-5">
            <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
              Effective window
            </h3>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">From</span>
                <span className="font-medium text-ink-950">
                  {formatDate(rule.effective_from)}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Until</span>
                <span className="font-medium text-ink-950">
                  {rule.effective_until
                    ? formatDate(rule.effective_until)
                    : "Open-ended"}
                </span>
              </div>
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
                  {rule.version}
                </span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Created by</span>
                <span className="font-mono text-ink-950">{rule.created_by}</span>
              </div>
              {rule.approved_by && (
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Approved by</span>
                  <span className="font-mono text-ink-950">
                    {rule.approved_by}
                  </span>
                </div>
              )}
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Created</span>
                <span className="text-ink-950">
                  {timeAgo(rule.created_at)}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Updated</span>
                <span className="text-ink-950">
                  {timeAgo(rule.updated_at)}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}