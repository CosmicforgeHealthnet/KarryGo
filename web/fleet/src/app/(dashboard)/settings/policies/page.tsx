"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, Lock, Plus, RefreshCw, Save, SlidersHorizontal, Trash2 } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { structureService } from "@/lib/services/structureService";
import type { OrganizationSettings } from "@/lib/types/structure";

type ThresholdRow = { key: string; amount: string };

const titleCase = (value: string) =>
  value.replace(/[_.]+/g, " ").toLowerCase().replace(/\b\w/g, (m) => m.toUpperCase()).trim();

const naira = (minor: number) =>
  (minor / 100).toLocaleString(undefined, { style: "currency", currency: "NGN" });

export default function OrganizationSettingsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [settings, setSettings] = useState<OrganizationSettings | null>(null);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [saveError, setSaveError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const [timeout, setTimeoutSeconds] = useState("");
  const [visibility, setVisibility] = useState("ORGANIZATION");
  const [thresholds, setThresholds] = useState<ThresholdRow[]>([]);

  const apply = (value: OrganizationSettings) => {
    setSettings(value);
    setTimeoutSeconds(String(value.assignmentTimeoutSeconds));
    setVisibility(value.providerVisibility || "ORGANIZATION");
    setThresholds(
      Object.entries(value.approvalThresholds ?? {}).map(([key, amount]) => ({ key, amount: String(amount) })),
    );
  };

  const load = useCallback(async () => {
    try {
      apply(await structureService.getSettings());
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Organization settings could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!settings) return;
    setSaveError(null);
    setNotice(null);
    const approvalThresholds: Record<string, number> = {};
    for (const row of thresholds) {
      const key = row.key.trim();
      if (key === "") continue;
      const amount = Number(row.amount);
      if (!Number.isInteger(amount) || amount < 0) {
        setSaveError({ message: `The threshold for ${key} must be a whole, non-negative amount in kobo.` });
        return;
      }
      approvalThresholds[key] = amount;
    }
    setSaving(true);
    try {
      const saved = await structureService.updateSettings(
        { assignmentTimeoutSeconds: Number(timeout), providerVisibility: visibility, approvalThresholds },
        settings.version,
      );
      apply(saved);
      setNotice("Organization settings saved.");
    } catch (err) {
      setSaveError(describeApiError(err, "The settings could not be saved."));
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading organization settings...</p>
      </div>
    );
  }

  const visibilityPolicy =
    settings && settings.financialVisibilityPolicy && typeof settings.financialVisibilityPolicy === "object"
      ? Object.entries(settings.financialVisibilityPolicy as Record<string, unknown>)
      : [];

  return (
    <div className="mx-auto max-w-4xl space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">Organization settings and policies</h1>
          <p className="mt-1 text-sm text-ink-600">How work is offered to providers and when spending needs approval.</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => load()}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            Refresh
          </button>
          <button
            onClick={() => router.push("/settings")}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
        </div>
      </div>

      <ApiErrorNotice error={error} />

      {settings && (
        <>
          {/* ===== READ-ONLY FACTS ===== */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="flex items-center gap-2 text-[15px] font-semibold text-ink-950">
              <Lock className="h-4 w-4 text-ink-400" />
              Set by the platform
            </h3>
            <dl className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
              <div className="flex items-center justify-between gap-3 border-b border-line py-2">
                <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">Operating mode</dt>
                <dd className="text-sm text-ink-950">{titleCase(settings.operatingMode || "—")}</dd>
              </div>
              <div className="flex items-center justify-between gap-3 border-b border-line py-2">
                <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">Version</dt>
                <dd className="font-mono text-sm text-ink-950">v{settings.version}</dd>
              </div>
            </dl>
            <div className="mt-3 text-xs font-medium uppercase tracking-wide text-ink-500">Financial visibility for managed providers</div>
            {visibilityPolicy.length === 0 ? (
              <p className="mt-1 text-sm text-ink-500">No financial visibility rules are set.</p>
            ) : (
              <div className="mt-2 flex flex-wrap gap-2">
                {visibilityPolicy.map(([key, value]) => (
                  <span
                    key={key}
                    className={`rounded-full px-3 py-1 text-xs font-medium ${value === true ? "bg-green-bg text-green" : "bg-surface-2 text-ink-500"}`}
                  >
                    {titleCase(key)}: {value === true ? "Visible" : "Hidden"}
                  </span>
                ))}
              </div>
            )}
          </div>

          {/* ===== EDITABLE SETTINGS ===== */}
          <form onSubmit={submit} className="space-y-4 rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="flex items-center gap-2 text-[15px] font-semibold text-ink-950">
              <SlidersHorizontal className="h-4 w-4 text-primary" />
              Assignment and approvals
            </h3>
            <ApiErrorNotice error={saveError} />
            {notice && <div className="rounded-xl border border-green/30 bg-green-bg px-4 py-3 text-sm text-green">{notice}</div>}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Assignment timeout (seconds)" required hint="How long a provider has to answer an offer: 30 to 3600 seconds.">
                <input required type="number" min={30} max={3600} value={timeout} onChange={(e) => setTimeoutSeconds(e.target.value)} className={INPUT} />
              </Field>
              <Field label="Provider visibility" required hint="Whether staff see all company providers or only their branch's.">
                <select value={visibility} onChange={(e) => setVisibility(e.target.value)} className={INPUT}>
                  <option value="ORGANIZATION">Whole organization</option>
                  <option value="BRANCH">Own branch only</option>
                </select>
              </Field>
            </div>

            <div>
              <div className="mb-1.5 flex items-center justify-between">
                <span className="text-xs font-medium uppercase tracking-wide text-ink-500">Approval thresholds (kobo)</span>
                <button
                  type="button"
                  onClick={() => setThresholds([...thresholds, { key: "", amount: "0" }])}
                  className="flex items-center gap-1 text-xs font-semibold text-primary hover:text-primary/80"
                >
                  <Plus className="h-3.5 w-3.5" />
                  Add threshold
                </button>
              </div>
              {thresholds.length === 0 ? (
                <p className="text-sm text-ink-500">No approval thresholds are set.</p>
              ) : (
                <div className="space-y-2">
                  {thresholds.map((row, index) => {
                    const amount = Number(row.amount);
                    return (
                      <div key={index} className="grid grid-cols-[1fr_1fr_auto] items-center gap-2">
                        <input
                          value={row.key}
                          onChange={(e) => setThresholds(thresholds.map((t, i) => (i === index ? { ...t, key: e.target.value } : t)))}
                          placeholder="expense"
                          className={INPUT}
                        />
                        <div>
                          <input
                            type="number"
                            min={0}
                            step={1}
                            value={row.amount}
                            onChange={(e) => setThresholds(thresholds.map((t, i) => (i === index ? { ...t, amount: e.target.value } : t)))}
                            className={INPUT}
                          />
                          {Number.isInteger(amount) && amount >= 0 && <p className="mt-0.5 text-[11px] text-ink-400">{naira(amount)}</p>}
                        </div>
                        <button
                          type="button"
                          onClick={() => setThresholds(thresholds.filter((_, i) => i !== index))}
                          title="Remove"
                          className="text-ink-400 transition-colors hover:text-red"
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>

            <button
              type="submit"
              disabled={saving}
              className="flex items-center gap-2 rounded-xl bg-primary px-6 py-2.5 text-sm font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90 disabled:opacity-60"
            >
              <Save className="h-4 w-4" />
              {saving ? "Saving..." : "Save settings"}
            </button>
          </form>
        </>
      )}
    </div>
  );
}
