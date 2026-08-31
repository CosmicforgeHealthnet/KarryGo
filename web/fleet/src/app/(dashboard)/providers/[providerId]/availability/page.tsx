// src/app/(dashboard)/providers/[providerId]/availability/page.tsx

"use client";

import { useState } from "react";
import { PROVIDER_DETAIL, AVAILABILITY_BLOCKERS } from "@/lib/mock-provider-data";
import {
  Clock,
  CheckCircle2,
  AlertCircle,
  XCircle,
  Calendar,
  User,
  Truck,
  MapPin,
  Shield,
  AlertTriangle,
  ChevronRight,
  Activity,
  Wifi,
  WifiOff,
  FileText,
} from "lucide-react";

const AVAILABILITY_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Online: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Busy: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Offline: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Leave: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
};

const BLOCKER_ICONS = {
  Qualification: Shield,
  Training: Activity,
  Document: FileText,
  Safety: AlertTriangle,
};

const BLOCKER_STYLES = {
  critical: "border-red/30 bg-red-bg/20 text-red",
  warning: "border-amber/30 bg-amber-bg/20 text-amber",
};

export default function AvailabilityTab() {
  const provider = PROVIDER_DETAIL;

  const availabilityStyle = AVAILABILITY_STYLES[provider.availability] || AVAILABILITY_STYLES.Offline;
  const isAvailable = provider.availability === "Online" && provider.eligibility === "Eligible";

  return (
    <div className="space-y-5">
      {/* ── Status Cards ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {/* Availability Status */}
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className={`rounded-full p-2 ${availabilityStyle.bg}`}>
              {provider.availability === "Online" ? (
                <Wifi className="h-4 w-4 text-green" />
              ) : provider.availability === "Busy" ? (
                <Activity className="h-4 w-4 text-amber" />
              ) : (
                <WifiOff className="h-4 w-4 text-ink-500" />
              )}
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Current Status</div>
              <div className="flex items-center gap-2">
                <span className={`text-base font-semibold ${availabilityStyle.text}`}>
                  {provider.availability}
                </span>
                <span className={`h-1.5 w-1.5 rounded-full ${availabilityStyle.dot}`} />
              </div>
            </div>
          </div>
          <div className="mt-3 flex items-center gap-2 text-xs text-ink-500">
            <Clock className="h-3.5 w-3.5" />
            Last active: {provider.lastActive}
          </div>
        </div>

        {/* Eligibility Status */}
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className={`rounded-full p-2 ${
              provider.eligibility === "Eligible" ? "bg-green-bg" :
              provider.eligibility === "Warning" ? "bg-amber-bg" :
              "bg-red-bg"
            }`}>
              {provider.eligibility === "Eligible" ? (
                <CheckCircle2 className="h-4 w-4 text-green" />
              ) : provider.eligibility === "Warning" ? (
                <AlertCircle className="h-4 w-4 text-amber" />
              ) : (
                <XCircle className="h-4 w-4 text-red" />
              )}
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Eligibility</div>
              <div className={`text-base font-semibold ${
                provider.eligibility === "Eligible" ? "text-green" :
                provider.eligibility === "Warning" ? "text-amber" :
                "text-red"
              }`}>
                {provider.eligibility}
              </div>
            </div>
          </div>
          <div className="mt-3 flex items-center gap-2 text-xs text-ink-500">
            {isAvailable ? (
              <span className="flex items-center gap-1 text-green">
                <CheckCircle2 className="h-3.5 w-3.5" />
                Available for assignments
              </span>
            ) : (
              <span className="flex items-center gap-1 text-amber">
                <AlertCircle className="h-3.5 w-3.5" />
                Currently unavailable
              </span>
            )}
          </div>
        </div>
      </div>

      {/* ── Blockers ── */}
      <div className="rounded-lg border border-line bg-white p-4">
        <div className="flex items-center gap-2.5 mb-3">
          <Shield className="h-5 w-5 text-ink-400" />
          <h3 className="text-sm font-semibold text-ink-950">Availability Blockers</h3>
          <span className="rounded-full bg-amber-bg px-2 py-0.5 text-[10px] font-semibold text-amber">
            {AVAILABILITY_BLOCKERS.length}
          </span>
        </div>

        {AVAILABILITY_BLOCKERS.length === 0 ? (
          <div className="flex items-center gap-2 rounded-lg bg-green-bg/30 px-4 py-3 text-sm text-green">
            <CheckCircle2 className="h-4 w-4" />
            No blockers — provider is fully available
          </div>
        ) : (
          <div className="space-y-2.5">
            {AVAILABILITY_BLOCKERS.map((blocker) => {
              const Icon = BLOCKER_ICONS[blocker.kind] || AlertCircle;
              const style = BLOCKER_STYLES[blocker.severity] || BLOCKER_STYLES.warning;

              return (
                <div
                  key={blocker.id}
                  className={`flex items-start gap-3 rounded-lg border p-3 ${style}`}
                >
                  <div className={`rounded-full p-1.5 ${
                    blocker.severity === "critical" ? "bg-red-bg" : "bg-amber-bg"
                  }`}>
                    <Icon className={`h-4 w-4 ${
                      blocker.severity === "critical" ? "text-red" : "text-amber"
                    }`} />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-ink-950">
                        {blocker.kind}
                      </span>
                      <span className={`text-[10px] font-semibold ${
                        blocker.severity === "critical" ? "text-red" : "text-amber"
                      }`}>
                        {blocker.severity}
                      </span>
                    </div>
                    <p className="mt-0.5 text-xs text-ink-600">{blocker.detail}</p>
                  </div>
                  <button className="text-xs font-medium text-primary whitespace-nowrap transition-colors hover:text-primary-hover">
                    Resolve →
                  </button>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* ── Schedule Preview ── */}
      <div className="rounded-lg border border-line bg-white p-4">
        <div className="flex items-center gap-2.5 mb-3">
          <Calendar className="h-5 w-5 text-ink-400" />
          <h3 className="text-sm font-semibold text-ink-950">Today's Schedule</h3>
        </div>
        <div className="space-y-2">
          <div className="flex items-center justify-between rounded-lg border border-line bg-surface-2/50 px-3 py-2">
            <div className="flex items-center gap-2">
              <div className="h-2 w-2 rounded-full bg-green" />
              <span className="text-sm text-ink-700">Morning shift</span>
            </div>
            <span className="text-xs text-ink-500">06:00 — 10:00</span>
          </div>
          <div className="flex items-center justify-between rounded-lg border border-line bg-surface-2/50 px-3 py-2">
            <div className="flex items-center gap-2">
              <div className="h-2 w-2 rounded-full bg-amber" />
              <span className="text-sm text-ink-700">Afternoon shift</span>
            </div>
            <span className="text-xs text-ink-500">12:00 — 16:00</span>
          </div>
          <div className="flex items-center justify-between rounded-lg border border-dashed border-line bg-white/50 px-3 py-2">
            <div className="flex items-center gap-2">
              <div className="h-2 w-2 rounded-full bg-ink-300" />
              <span className="text-sm text-ink-400">Evening shift</span>
            </div>
            <span className="text-xs text-ink-400">16:00 — 20:00 (Unassigned)</span>
          </div>
        </div>
      </div>
    </div>
  );
}