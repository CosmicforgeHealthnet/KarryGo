// src/app/(dashboard)/providers/[providerId]/membership/page.tsx

"use client";

import { useState } from "react";
import { PROVIDER_DETAIL, MEMBERSHIP_EVENTS } from "@/lib/mock-provider-data";
import {
  Users,
  UserCheck,
  UserX,
  Clock,
  Calendar,
  CheckCircle2,
  AlertCircle,
  Mail,
  Link2,
  RefreshCw,
  UserPlus,
  UserMinus,
  Shield,
  ArrowRight,
  MoreVertical,
} from "lucide-react";

const MEMBERSHIP_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Inactive: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Suspended: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

export default function MembershipTab() {
  const provider = PROVIDER_DETAIL;
  const membershipStyle = MEMBERSHIP_STYLES[provider.membershipStatus] || MEMBERSHIP_STYLES.Inactive;

  return (
    <div className="space-y-5">
      {/* ── Status Cards ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className={`rounded-full p-2 ${membershipStyle.bg}`}>
              {provider.membershipStatus === "Active" ? (
                <UserCheck className="h-4 w-4 text-green" />
              ) : provider.membershipStatus === "Suspended" ? (
                <UserX className="h-4 w-4 text-red" />
              ) : (
                <Users className="h-4 w-4 text-amber" />
              )}
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Membership Status</div>
              <div className="flex items-center gap-2">
                <span className={`text-base font-semibold ${membershipStyle.text}`}>
                  {provider.membershipStatus}
                </span>
                <span className={`h-1.5 w-1.5 rounded-full ${membershipStyle.dot}`} />
              </div>
            </div>
          </div>
          <div className="mt-3 flex items-center gap-2 text-xs text-ink-500">
            <Calendar className="h-3.5 w-3.5" />
            Joined: {provider.joinedAt}
          </div>
        </div>

        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-primary-bg p-2">
              <Shield className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Provider Type</div>
              <div className="text-base font-semibold text-ink-950">{provider.service}</div>
            </div>
          </div>
          <div className="mt-3 flex items-center gap-2 text-xs text-ink-500">
            <CheckCircle2 className="h-3.5 w-3.5 text-green" />
            Consent confirmed
          </div>
        </div>
      </div>

      {/* ── Timeline ── */}
      <div className="rounded-lg border border-line bg-white p-4">
        <div className="flex items-center gap-2.5 mb-4">
          <Clock className="h-5 w-5 text-ink-400" />
          <h3 className="text-sm font-semibold text-ink-950">Membership Timeline</h3>
          <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
            {MEMBERSHIP_EVENTS.length} events
          </span>
        </div>

        <div className="space-y-0">
          {MEMBERSHIP_EVENTS.map((event, index) => {
            const isFirst = index === 0;
            const isLast = index === MEMBERSHIP_EVENTS.length - 1;

            return (
              <div key={event.id} className="relative flex gap-3.5 pb-4 last:pb-0">
                {!isLast && (
                  <div className="absolute left-[14px] top-[28px] bottom-0 w-px bg-line" />
                )}
                <div className={`flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full ${
                  isFirst ? "bg-primary text-white" : "bg-surface-2 text-ink-500"
                }`}>
                  {isFirst ? (
                    <CheckCircle2 className="h-3.5 w-3.5" />
                  ) : (
                    <Clock className="h-3.5 w-3.5" />
                  )}
                </div>
                <div className="flex-1 min-w-0 pt-0.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-semibold text-ink-950">
                      {event.label}
                    </span>
                    <span className="text-xs text-ink-500">{event.time}</span>
                  </div>
                  <p className="text-xs text-ink-600">{event.detail}</p>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* ── Actions ── */}
      <div className="rounded-lg border border-line bg-white p-4">
        <div className="flex flex-wrap gap-2.5">
          <button className="flex items-center gap-2 rounded-md border border-line bg-white px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
            <Mail className="h-4 w-4" />
            Send Invitation
          </button>
          <button className="flex items-center gap-2 rounded-md border border-line bg-white px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
            <Link2 className="h-4 w-4" />
            Copy Invite Link
          </button>
          {provider.membershipStatus === "Active" ? (
            <button className="flex items-center gap-2 rounded-md border border-red/30 bg-red-bg px-4 py-2 text-sm font-medium text-red transition-colors hover:bg-red-bg/70">
              <UserMinus className="h-4 w-4" />
              Suspend Membership
            </button>
          ) : (
            <button className="flex items-center gap-2 rounded-md border border-green/30 bg-green-bg px-4 py-2 text-sm font-medium text-green transition-colors hover:bg-green-bg/70">
              <UserCheck className="h-4 w-4" />
              Activate Membership
            </button>
          )}
        </div>
      </div>
    </div>
  );
}