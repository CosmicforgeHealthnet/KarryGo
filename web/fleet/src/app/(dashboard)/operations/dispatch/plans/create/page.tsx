// src/app/(dashboard)/operations/dispatch/plans/create/page.tsx

"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { AlertCircle, X } from "lucide-react";
import {
  dispatchService,
  newIdempotencyKey,
} from "@/lib/services/dispatchService";

// IANA timezone options for the markets this service runs in.
// Keep this list small and expand when a new market is added; a free-text
// timezone field is a footgun (one typo means a plan silently in UTC).
const TIMEZONES = [
  { value: "Africa/Lagos", label: "Lagos (WAT)" },
  { value: "Africa/Accra", label: "Accra (GMT)" },
  { value: "Africa/Nairobi", label: "Nairobi (EAT)" },
  { value: "Africa/Johannesburg", label: "Johannesburg (SAST)" },
  { value: "UTC", label: "UTC" },
];

function todayIso(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

// Turn a <input type="date"> value and an <input type="time"> value into an
// RFC3339 timestamp in the browser's local timezone offset. The backend
// stores timestamptz, so what matters is the instant, not the string shape.
function toRfc3339(dateStr: string, timeStr: string): string | null {
  if (!dateStr || !timeStr) return null;
  const [y, m, d] = dateStr.split("-").map(Number);
  const [hh, mm] = timeStr.split(":").map(Number);
  if (!y || !m || !d || Number.isNaN(hh) || Number.isNaN(mm)) return null;
  const dt = new Date(y, m - 1, d, hh, mm, 0, 0);
  if (!Number.isFinite(dt.getTime())) return null;
  return dt.toISOString();
}

export default function CreatePlanPage() {
  const router = useRouter();

  const [branchId, setBranchId] = useState("");
  const [planDate, setPlanDate] = useState(todayIso());
  const [windowStart, setWindowStart] = useState("08:00");
  const [windowEnd, setWindowEnd] = useState("12:00");
  const [timezone, setTimezone] = useState("Africa/Lagos");

  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  // One key per page load. Retries reuse it.
  const idempotencyKey = useMemo(() => newIdempotencyKey(), []);

  const formValid =
    branchId.trim() !== "" &&
    planDate !== "" &&
    windowStart !== "" &&
    windowEnd !== "" &&
    timezone !== "";

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!formValid) return;

    const planDateRfc = toRfc3339(planDate, "00:00");
    const startRfc = toRfc3339(planDate, windowStart);
    const endRfc = toRfc3339(planDate, windowEnd);

    if (!planDateRfc || !startRfc || !endRfc) {
      setSubmitError("Date or time is invalid.");
      return;
    }
    if (new Date(endRfc) <= new Date(startRfc)) {
      setSubmitError("Work window end must be after start.");
      return;
    }

    setSubmitting(true);
    setSubmitError(null);

    try {
      const plan = await dispatchService.createPlan(
        {
          branchId: branchId.trim(),
          planDate: planDateRfc,
          workWindowStart: startRfc,
          workWindowEnd: endRfc,
          timezone,
        },
        idempotencyKey,
      );
      router.push(`/operations/dispatch/plans/${plan.id}`);
    } catch (err: any) {
      const message =
        err?.response?.data?.error?.message ??
        err?.message ??
        "Could not create the plan.";
      setSubmitError(message);
      setSubmitting(false);
    }
  };

  return (
    <div className="space-y-5">
      <div>
        <h1>Create dispatch plan</h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Define a planning window for a branch. The plan is created in draft
          and can be approved and published afterwards.
        </p>
      </div>

      {submitError && (
        <div className="flex items-start gap-2 rounded-lg border border-red/20 bg-red-bg px-4 py-3 text-sm text-red">
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          <span>{submitError}</span>
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="rounded-lg border border-line bg-surface p-4">
          <label className="mb-2 block text-xs font-semibold text-ink-700">
            Branch ID <span className="text-red">*</span>
          </label>
          <input
            type="text"
            required
            value={branchId}
            onChange={(e) => setBranchId(e.target.value)}
            placeholder="e.g. brn_ikeja"
            className="w-full rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
          />
          <p className="mt-1.5 text-xs text-ink-500">
            The branch the plan belongs to. Must be a branch your account can access.
          </p>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="rounded-lg border border-line bg-surface p-4">
            <label className="mb-2 block text-xs font-semibold text-ink-700">
              Plan date <span className="text-red">*</span>
            </label>
            <input
              type="date"
              required
              value={planDate}
              onChange={(e) => setPlanDate(e.target.value)}
              className="w-full rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </div>

          <div className="rounded-lg border border-line bg-surface p-4">
            <label className="mb-2 block text-xs font-semibold text-ink-700">
              Timezone <span className="text-red">*</span>
            </label>
            <select
              required
              value={timezone}
              onChange={(e) => setTimezone(e.target.value)}
              className="w-full appearance-none rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
            >
              {TIMEZONES.map((tz) => (
                <option key={tz.value} value={tz.value}>{tz.label}</option>
              ))}
            </select>
          </div>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="rounded-lg border border-line bg-surface p-4">
            <label className="mb-2 block text-xs font-semibold text-ink-700">
              Work window start <span className="text-red">*</span>
            </label>
            <input
              type="time"
              required
              value={windowStart}
              onChange={(e) => setWindowStart(e.target.value)}
              className="w-full rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </div>

          <div className="rounded-lg border border-line bg-surface p-4">
            <label className="mb-2 block text-xs font-semibold text-ink-700">
              Work window end <span className="text-red">*</span>
            </label>
            <input
              type="time"
              required
              value={windowEnd}
              onChange={(e) => setWindowEnd(e.target.value)}
              className="w-full rounded-lg border border-line bg-surface px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </div>
        </div>

        <div className="flex flex-col-reverse gap-2.5 sm:flex-row sm:justify-end">
          <Link
            href="/operations/dispatch/plans"
            className="flex items-center justify-center gap-2 rounded-lg border border-line bg-surface px-5 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <X className="h-4 w-4" />
            Cancel
          </Link>
          <button
            type="submit"
            disabled={submitting || !formValid}
            className="flex items-center justify-center gap-2 rounded-lg bg-primary px-5 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting ? (
              <>
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                Creating…
              </>
            ) : (
              "Create plan"
            )}
          </button>
        </div>
      </form>
    </div>
  );
}