// src/app/(dashboard)/operations/assignments/[assignmentId]/history/page.tsx

"use client";

import { RESPONSE_HISTORY } from "@/lib/mock-marketplace-data";
import {
  Clock,
  CheckCircle2,
  XCircle,
  Send,
  Eye,
  Flag,
  UserCheck,
  UserX,
  RefreshCw,
  Shield,
  Package,
  Truck,
  User,
  Calendar,
} from "lucide-react";

const getActionIcon = (label: string) => {
  const lower = label.toLowerCase();
  if (lower.includes("accept")) return { icon: CheckCircle2, color: "text-green", bg: "bg-green-bg" };
  if (lower.includes("declin") || lower.includes("reject")) return { icon: XCircle, color: "text-red", bg: "bg-red-bg" };
  if (lower.includes("create") || lower.includes("send")) return { icon: Send, color: "text-primary", bg: "bg-primary-bg" };
  if (lower.includes("view") || lower.includes("open")) return { icon: Eye, color: "text-amber", bg: "bg-amber-bg" };
  if (lower.includes("flag") || lower.includes("report")) return { icon: Flag, color: "text-red", bg: "bg-red-bg" };
  if (lower.includes("assign") || lower.includes("nudge")) return { icon: UserCheck, color: "text-primary", bg: "bg-primary-bg" };
  if (lower.includes("expire") || lower.includes("timeout")) return { icon: Clock, color: "text-ink-500", bg: "bg-surface-2" };
  if (lower.includes("cancel")) return { icon: UserX, color: "text-red", bg: "bg-red-bg" };
  if (lower.includes("reassign")) return { icon: RefreshCw, color: "text-amber", bg: "bg-amber-bg" };
  if (lower.includes("safety")) return { icon: Shield, color: "text-red", bg: "bg-red-bg" };
  if (lower.includes("dispatch")) return { icon: Package, color: "text-primary", bg: "bg-primary-bg" };
  if (lower.includes("haul")) return { icon: Truck, color: "text-amber", bg: "bg-amber-bg" };
  return { icon: Clock, color: "text-ink-500", bg: "bg-surface-2" };
};

const getBadgeColor = (label: string) => {
  const lower = label.toLowerCase();
  if (lower.includes("accept")) return "bg-green-bg text-green";
  if (lower.includes("declin") || lower.includes("reject")) return "bg-red-bg text-red";
  if (lower.includes("create") || lower.includes("send")) return "bg-primary-bg text-primary";
  if (lower.includes("expire") || lower.includes("timeout")) return "bg-amber-bg text-amber";
  if (lower.includes("cancel")) return "bg-red-bg text-red";
  if (lower.includes("reassign")) return "bg-amber-bg text-amber";
  if (lower.includes("safety")) return "bg-red-bg text-red";
  if (lower.includes("view") || lower.includes("open")) return "bg-amber-bg text-amber";
  return "bg-surface-2 text-ink-500";
};

export default function AssignmentHistoryTab() {
  return (
    <div className="space-y-5">
      {/* ── Timeline ── */}
      <div className="rounded-lg border border-line bg-white p-5">
        <div className="flex flex-col gap-0">
          {RESPONSE_HISTORY.map((ev, i) => {
            const { icon: Icon, color, bg } = getActionIcon(ev.label);
            const isFirst = i === 0;
            const isLast = i === RESPONSE_HISTORY.length - 1;

            return (
              <div
                key={ev.id}
                className={`relative flex gap-4 ${
                  !isLast ? "pb-6" : "pb-0"
                }`}
              >
                {/* Timeline line */}
                {!isLast && (
                  <div className="absolute left-[18px] top-[34px] bottom-0 w-px bg-line" />
                )}

                {/* Icon circle */}
                <div className="relative z-10 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full">
                  <div className={`flex h-full w-full items-center justify-center rounded-full ${bg}`}>
                    <Icon className={`h-4 w-4 ${color}`} />
                  </div>
                  {isFirst && (
                    <span className="absolute -top-1 -right-1 flex h-4 w-4 items-center justify-center rounded-full bg-green text-[8px] font-bold text-white">
                      NEW
                    </span>
                  )}
                </div>

                {/* Content */}
                <div className="min-w-0 flex-1 pt-0.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-semibold text-ink-950">
                      {ev.label}
                    </span>
                    <span
                      className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${getBadgeColor(ev.label)}`}
                    >
                      {ev.detail}
                    </span>
                  </div>
                  <div className="mt-1 flex flex-wrap items-center gap-2 text-sm text-ink-500">
                    <div className="flex items-center gap-1">
                      <User className="h-3.5 w-3.5" />
                      {ev.actor}
                    </div>
                    <span className="text-ink-300">·</span>
                    <div className="flex items-center gap-1">
                      <Calendar className="h-3.5 w-3.5" />
                      {ev.time}
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}