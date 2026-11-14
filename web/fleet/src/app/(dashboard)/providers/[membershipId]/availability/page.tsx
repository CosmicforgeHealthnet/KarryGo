"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { Activity, AlertTriangle, Calendar, CheckCircle2, Clock, Shield, Wifi, WifiOff, XCircle } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { providerService } from "@/lib/services/providerService";
import type { ProviderAvailability } from "@/lib/types/providerAdmin";

const humanize = (value?: string) =>
  value ? value.replace(/_/g, " ").toLowerCase().replace(/^\w/, (c) => c.toUpperCase()) : "—";
const formatWhen = (iso?: string) =>
  iso ? new Date(iso).toLocaleString(undefined, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }) : "—";

const STATUS_STYLES: Record<string, { bg: string; text: string }> = {
  AVAILABLE: { bg: "bg-green-bg", text: "text-green" },
  BUSY: { bg: "bg-amber-bg", text: "text-amber" },
  UNAVAILABLE: { bg: "bg-red-bg", text: "text-red" },
};

const EFFECT_STYLES: Record<string, string> = {
  BLOCKING: "border-red/30 bg-red-bg/20 text-red",
  BUSY: "border-amber/30 bg-amber-bg/20 text-amber",
  INFORMATIONAL: "border-line bg-surface-2 text-ink-600",
};

// Whether the provider can take company work right now, derived by the
// server from membership, leave, shift, qualifications, active work and
// tracking, with each reason that decided it.
export default function AvailabilityTab() {
  const params = useParams();
  const membershipId = params.membershipId as string;
  const [data, setData] = useState<ProviderAvailability | null>(null);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    providerService
      .availability(membershipId)
      .then(setData)
      .catch((e) => setError(describeApiError(e, "Availability could not be loaded.")))
      .finally(() => setLoading(false));
  }, [membershipId]);

  if (loading) {
    return <div className="rounded-lg border border-line bg-surface px-4 py-12 text-center text-sm text-ink-500">Loading availability…</div>;
  }
  if (!data) return <ApiErrorNotice error={error} />;

  const style = STATUS_STYLES[data.status] ?? STATUS_STYLES.UNAVAILABLE;
  const inputs = data.inputs;
  const quals = inputs.qualifications;
  const reasons = data.reasons ?? [];

  return (
    <div className="space-y-5">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className={`rounded-full p-2 ${style.bg}`}>
              {data.status === "AVAILABLE" ? <Wifi className={`h-4 w-4 ${style.text}`} /> : <WifiOff className={`h-4 w-4 ${style.text}`} />}
            </div>
            <div>
              <div className="text-xs font-medium uppercase tracking-wide text-ink-500">Availability</div>
              <div className={`text-lg font-semibold ${style.text}`}>{humanize(data.status)}</div>
            </div>
          </div>
          <p className="mt-3 text-xs text-ink-500">Evaluated {formatWhen(data.evaluatedAt)} · membership {humanize(inputs.membershipStatus)}</p>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-primary-bg p-2">
              <Activity className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-xs font-medium uppercase tracking-wide text-ink-500">Active work</div>
              <div className="text-lg font-semibold text-ink-950">{inputs.activeWork.total}</div>
            </div>
          </div>
          <p className="mt-3 text-xs text-ink-500">
            {inputs.activeWork.marketplace} marketplace · {inputs.activeWork.private} company · tracking {humanize(inputs.tracking.freshnessStatus)}
            {inputs.tracking.lastObservedAt ? ` (${formatWhen(inputs.tracking.lastObservedAt)})` : ""}
          </p>
        </div>
      </div>

      <div className="rounded-lg border border-line bg-white p-4">
        <h3 className="mb-3 text-sm font-semibold text-ink-950">Why</h3>
        {reasons.length === 0 ? (
          <p className="flex items-center gap-2 text-sm text-green">
            <CheckCircle2 className="h-4 w-4" /> Nothing is stopping this provider from taking work.
          </p>
        ) : (
          <ul className="space-y-2">
            {reasons.map((r, i) => (
              <li key={`${r.code}-${i}`} className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-sm ${EFFECT_STYLES[r.effect] ?? EFFECT_STYLES.INFORMATIONAL}`}>
                {r.effect === "BLOCKING" ? <XCircle className="h-4 w-4" /> : <AlertTriangle className="h-4 w-4" />}
                <span className="font-medium">{humanize(r.code)}</span>
                <span className="text-xs opacity-80">· {humanize(r.source)}</span>
              </li>
            ))}
          </ul>
        )}
        {data.derivation && <p className="mt-3 text-xs text-ink-500">{data.derivation}</p>}
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="mb-2 flex items-center gap-2 text-sm font-semibold text-ink-950">
            <Clock className="h-4 w-4 text-ink-400" /> Shift
          </div>
          <p className="text-sm text-ink-700">
            {inputs.shift.scheduleInForce ? (inputs.shift.onShift ? `On shift until ${formatWhen(inputs.shift.currentShiftEndsAt)}` : "Off shift") : "No published shift schedule in force"}
          </p>
          {inputs.shift.nextShiftStartsAt && <p className="text-xs text-ink-500">Next shift {formatWhen(inputs.shift.nextShiftStartsAt)}</p>}
          {inputs.approvedLeave && (
            <p className="mt-2 flex items-center gap-1.5 text-xs text-primary">
              <Calendar className="h-3.5 w-3.5" /> {humanize(inputs.approvedLeave.leaveType)} leave {formatWhen(inputs.approvedLeave.startAt)} – {formatWhen(inputs.approvedLeave.endAt)}
            </p>
          )}
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="mb-2 flex items-center gap-2 text-sm font-semibold text-ink-950">
            <Shield className="h-4 w-4 text-ink-400" /> Qualifications
          </div>
          <p className="text-sm text-ink-700">{quals.mandatoryCount} mandatory</p>
          {[
            ["Expired", quals.expired ?? [], "text-red"],
            ["Missing", quals.missing ?? [], "text-red"],
            ["Expiring soon", quals.expiringSoon ?? [], "text-amber"],
          ].map(([label, list, tone]) =>
            (list as { qualificationId: string; name: string; code: string }[]).length > 0 ? (
              <p key={label as string} className={`text-xs ${tone}`}>
                {label as string}: {(list as { name: string; code: string }[]).map((q) => q.name || q.code).join(", ")}
              </p>
            ) : null,
          )}
        </div>
      </div>
    </div>
  );
}
