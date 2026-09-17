"use client";

import { useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowLeft,
  RefreshCw,
  AlertCircle,
  Save,
  Scale,
  Plus,
  X,
} from "lucide-react";
import { financeService } from "@/lib/services/financeService";
import type { CompensationRuleType } from "@/lib/types/finance";
import { majorToMinor } from "@/lib/utils/financeLabels";

const CURRENCIES = ["NGN", "USD", "EUR", "GBP", "KES", "GHS", "ZAR"];

const RULE_TYPES: Array<{ value: CompensationRuleType; label: string }> = [
  { value: "SALARY", label: "Salary" },
  { value: "PER_JOB", label: "Per job" },
  { value: "PER_SHIFT", label: "Per shift" },
  { value: "COMMISSION", label: "Commission" },
  { value: "ALLOWANCE", label: "Allowance" },
  { value: "OVERTIME", label: "Overtime" },
  { value: "BONUS", label: "Bonus" },
  { value: "MANUAL_ADJUSTMENT", label: "Manual adjustment" },
  { value: "DEDUCTION", label: "Deduction" },
];

export default function CreateCompensationRulePage() {
  const router = useRouter();

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [name, setName] = useState("");
  const [ruleType, setRuleType] = useState<CompensationRuleType>("PER_JOB");
  const [currency, setCurrency] = useState("NGN");
  const [amountMajor, setAmountMajor] = useState("");
  const [approvedBasis, setApprovedBasis] = useState("");
  const [effectiveFrom, setEffectiveFrom] = useState("");
  const [effectiveUntil, setEffectiveUntil] = useState("");
  const [roles, setRoles] = useState<string[]>([]);
  const [roleInput, setRoleInput] = useState("");
  const [services, setServices] = useState<string[]>([]);
  const [serviceInput, setServiceInput] = useState("");

  const amountMinor = useMemo(() => {
    const trimmed = amountMajor.trim();
    if (!/^-?\d+(\.\d{1,2})?$/.test(trimmed)) return null;
    const n = Number(trimmed);
    if (!Number.isFinite(n) || n === 0) return null;
    return majorToMinor(n);
  }, [amountMajor]);

  const isDeduction = ruleType === "DEDUCTION";
  const periodValid =
    effectiveFrom !== "" &&
    (effectiveUntil === "" || effectiveUntil >= effectiveFrom);

  const canSubmit = useMemo(
    () =>
      name.trim() !== "" &&
      amountMinor !== null &&
      periodValid &&
      (!isDeduction || approvedBasis.trim() !== ""),
    [name, amountMinor, periodValid, isDeduction, approvedBasis]
  );

  const addRole = () => {
    const v = roleInput.trim();
    if (!v || roles.includes(v)) return;
    setRoles((prev) => [...prev, v]);
    setRoleInput("");
  };

  const removeRole = (r: string) =>
    setRoles((prev) => prev.filter((x) => x !== r));

  const addService = () => {
    const v = serviceInput.trim();
    if (!v || services.includes(v)) return;
    setServices((prev) => [...prev, v]);
    setServiceInput("");
  };

  const removeService = (s: string) =>
    setServices((prev) => prev.filter((x) => x !== s));

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving || !canSubmit) return;

    setSaving(true);
    setError(null);
    setSuccess(null);

    const calculationConfiguration: Record<string, unknown> = {
      amountMinor: amountMinor!,
    };
    if (isDeduction) {
      calculationConfiguration.approvedBasis = approvedBasis.trim();
    }

    try {
      const created = await financeService.compensationRules.create({
        name: name.trim(),
        ruleType,
        currency: currency.toUpperCase(),
        applicableRoles: roles,
        applicableServices: services,
        calculationConfiguration,
        effectiveFrom,
        effectiveUntil: effectiveUntil || null,
      });
      setSuccess(`Rule ${created.id} created as draft.`);
      setTimeout(() => {
        router.push(`/finance/compensation-rules/${created.id}`);
      }, 800);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: { message?: string } } } })
          ?.response?.data?.error?.message ??
        (err as Error)?.message ??
        "Failed to create compensation rule.";
      setError(message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-5">
      <div className="flex items-start gap-3">
        <button
          onClick={() => router.push("/finance/compensation-rules")}
          className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          aria-label="Back to compensation rules"
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            New Compensation Rule
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Define a deterministic compensation rule. Amounts drive payroll
            calculation.
          </p>
        </div>
      </div>

      {success && (
        <div className="rounded-lg border border-green/20 bg-green-bg px-4 py-2.5 text-sm text-green">
          {success}
        </div>
      )}

      {error && (
        <div className="flex items-start gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <form onSubmit={handleSubmit}>
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-4 text-[15px] font-semibold text-ink-950">
                Rule details
              </h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Name *
                  </label>
                  <input
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="e.g. Dispatch rider per-trip rate"
                    required
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Rule type *
                  </label>
                  <select
                    value={ruleType}
                    onChange={(e) =>
                      setRuleType(e.target.value as CompensationRuleType)
                    }
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {RULE_TYPES.map((t) => (
                      <option key={t.value} value={t.value}>
                        {t.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Currency *
                  </label>
                  <select
                    value={currency}
                    onChange={(e) => setCurrency(e.target.value)}
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 font-mono text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {CURRENCIES.map((c) => (
                      <option key={c} value={c}>
                        {c}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Amount *
                  </label>
                  <input
                    value={amountMajor}
                    onChange={(e) => setAmountMajor(e.target.value)}
                    placeholder="0.00"
                    required
                    className={`w-full rounded-xl border bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 ${
                      amountMajor.trim() !== "" && amountMinor === null
                        ? "border-red"
                        : "border-line"
                    }`}
                  />
                  <p className="mt-1 text-[11px] text-ink-400">
                    Non-zero. Use negative for deductions.
                  </p>
                </div>
                {isDeduction && (
                  <div>
                    <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                      Approved basis *
                    </label>
                    <input
                      value={approvedBasis}
                      onChange={(e) => setApprovedBasis(e.target.value)}
                      placeholder="Justification for deduction"
                      required
                      className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                    />
                  </div>
                )}
              </div>
            </div>

            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-4 text-[15px] font-semibold text-ink-950">
                Applicability
              </h3>
              <div className="space-y-4">
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Roles (leave empty for all)
                  </label>
                  <div className="flex gap-2">
                    <input
                      value={roleInput}
                      onChange={(e) => setRoleInput(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") {
                          e.preventDefault();
                          addRole();
                        }
                      }}
                      placeholder="e.g. DRIVER"
                      className="flex-1 rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                    />
                    <button
                      type="button"
                      onClick={addRole}
                      className="flex h-10 items-center gap-1 rounded-xl border border-line bg-surface px-3.5 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
                    >
                      <Plus className="h-3.5 w-3.5" />
                      Add
                    </button>
                  </div>
                  {roles.length > 0 && (
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {roles.map((r) => (
                        <span
                          key={r}
                          className="flex items-center gap-1 rounded-full bg-surface-2 px-2.5 py-0.5 text-[11px] text-ink-700"
                        >
                          {r}
                          <button
                            type="button"
                            onClick={() => removeRole(r)}
                            className="text-ink-400 hover:text-red"
                          >
                            <X className="h-3 w-3" />
                          </button>
                        </span>
                      ))}
                    </div>
                  )}
                </div>

                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Services (leave empty for all)
                  </label>
                  <div className="flex gap-2">
                    <input
                      value={serviceInput}
                      onChange={(e) => setServiceInput(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") {
                          e.preventDefault();
                          addService();
                        }
                      }}
                      placeholder="e.g. DISPATCH"
                      className="flex-1 rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                    />
                    <button
                      type="button"
                      onClick={addService}
                      className="flex h-10 items-center gap-1 rounded-xl border border-line bg-surface px-3.5 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
                    >
                      <Plus className="h-3.5 w-3.5" />
                      Add
                    </button>
                  </div>
                  {services.length > 0 && (
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {services.map((s) => (
                        <span
                          key={s}
                          className="flex items-center gap-1 rounded-full bg-surface-2 px-2.5 py-0.5 text-[11px] text-ink-700"
                        >
                          {s}
                          <button
                            type="button"
                            onClick={() => removeService(s)}
                            className="text-ink-400 hover:text-red"
                          >
                            <X className="h-3 w-3" />
                          </button>
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            </div>

            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-4 text-[15px] font-semibold text-ink-950">
                Effective window
              </h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    From *
                  </label>
                  <input
                    type="date"
                    value={effectiveFrom}
                    onChange={(e) => setEffectiveFrom(e.target.value)}
                    required
                    className="w-full rounded-xl border border-line bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                    Until (optional)
                  </label>
                  <input
                    type="date"
                    value={effectiveUntil}
                    onChange={(e) => setEffectiveUntil(e.target.value)}
                    className={`w-full rounded-xl border bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 ${
                      effectiveUntil !== "" &&
                      effectiveFrom !== "" &&
                      effectiveUntil < effectiveFrom
                        ? "border-red"
                        : "border-line"
                    }`}
                  />
                </div>
              </div>
            </div>
          </div>

          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-surface p-5">
              <h3 className="mb-3 text-[15px] font-semibold text-ink-950">
                Summary
              </h3>
              <div className="space-y-2 text-sm">
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Name</span>
                  <span className="truncate font-medium text-ink-950 max-w-[180px]">
                    {name || "—"}
                  </span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Type</span>
                  <span className="font-medium text-ink-950">
                    {RULE_TYPES.find((t) => t.value === ruleType)?.label}
                  </span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Currency</span>
                  <span className="font-mono font-medium text-ink-950">
                    {currency}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-ink-500">Amount</span>
                  <span className="font-mono font-semibold text-ink-950">
                    {amountMinor !== null
                      ? `${(amountMinor / 100).toLocaleString()} ${currency}`
                      : "—"}
                  </span>
                </div>
              </div>
            </div>

            <div className="rounded-2xl border border-line bg-surface p-5">
              <button
                type="submit"
                disabled={saving || !canSubmit}
                className="flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {saving ? (
                  <RefreshCw className="h-4 w-4 animate-spin" />
                ) : (
                  <Scale className="h-4 w-4" />
                )}
                {saving ? "Creating…" : "Create Rule"}
              </button>
              <button
                type="button"
                onClick={() => router.push("/finance/compensation-rules")}
                className="mt-2 flex w-full items-center justify-center gap-2 rounded-xl border border-line bg-surface px-4 py-2.5 text-sm font-medium text-ink-600 transition hover:bg-surface-2"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      </form>
    </div>
  );
}