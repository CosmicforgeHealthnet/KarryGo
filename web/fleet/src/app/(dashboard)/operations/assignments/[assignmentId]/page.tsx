// src/app/(dashboard)/operations/assignments/[assignmentId]/page.tsx

"use client";

import { useState } from "react";
import Link from "next/link";
import {
  User,
  Truck,
  MapPin,
  Calendar,
  Clock,
  AlertCircle,
  CheckCircle2,
  XCircle,
  FileText,
  MessageSquare,
  Phone,
  Mail,
  MoreVertical,
  Flag,
  Shield,
  Package,
  ArrowRight,
} from "lucide-react";
import { ASSIGNMENT_DETAIL } from "@/lib/mock-marketplace-data";

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  "Pending response": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Accepted: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Declined: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  "In Progress": { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
};

const BTN_DANGER =
  "flex items-center justify-center gap-2 rounded-xl border border-red/20 bg-red-bg/50 px-4 py-3 text-[13px] font-semibold text-red shadow-sm transition hover:bg-red-bg hover:border-red/40";

export default function AssignmentOverviewTab() {
  const a = ASSIGNMENT_DETAIL;
  const [cancelled, setCancelled] = useState(false);

  const statusStyle = STATUS_STYLES[a.status] || STATUS_STYLES["Pending response"];
  const isPending = a.status === "Pending response";
  const isActive = ["Pending response", "Accepted", "In Progress"].includes(a.status);

  return (
    <div className="space-y-5">
      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.6fr_1fr]">
        {/* ===== LEFT COLUMN ===== */}
        <div className="space-y-5">
          {/* Assignment Details */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-4">
              <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary/10 text-primary">
                <FileText className="h-4 w-4" />
              </div>
              <div>
                <div className="text-[15px] font-semibold text-ink-950">Assignment Details</div>
                <div className="text-[12px] text-ink-500">
                  Offer: <span className="font-mono">{a.offerId}</span>
                </div>
              </div>
              <span
                className={`ml-auto flex items-center gap-1.5 rounded-full px-3 py-1 text-[11px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                {a.status}
              </span>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Provider</div>
                <div className="mt-1 flex items-center gap-2">
                  <div className="flex h-7 w-7 items-center justify-center rounded-full bg-primary/10 text-[10px] font-bold text-primary">
                    {a.provider.split(" ").map((n: string) => n[0]).join("")}
                  </div>
                  <span className="text-[14px] font-medium text-ink-950">{a.provider}</span>
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Assigned Asset</div>
                <div className="mt-1 flex items-center gap-2">
                  <Truck className="h-4 w-4 text-ink-400" />
                  <span className="font-mono text-[14px] font-medium text-ink-950">{a.asset}</span>
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</div>
                <div className="mt-1 flex items-center gap-2">
                  <MapPin className="h-4 w-4 text-ink-400" />
                  <span className="text-[14px] font-medium text-ink-950">{a.branch}</span>
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Created</div>
                <div className="mt-1 flex items-center gap-2">
                  <Calendar className="h-4 w-4 text-ink-400" />
                  <span className="text-[14px] font-medium text-ink-950">{a.created}</span>
                </div>
              </div>
              <div className="col-span-2">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Response Deadline</div>
                <div className="mt-1 flex items-center gap-2">
                  <Clock className="h-4 w-4 text-amber" />
                  <span className="rounded-full bg-amber-bg px-3 py-1 text-[13px] font-bold text-amber">
                    {a.deadline}
                  </span>
                  {isPending && (
                    <span className="text-[12px] text-ink-500">
                      Provider must respond by this time
                    </span>
                  )}
                </div>
              </div>
            </div>

            {a.instructions && (
              <div className="mt-4 border-t border-line pt-4">
                <div className="flex items-center gap-2">
                  <MessageSquare className="h-4 w-4 text-ink-400" />
                  <span className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Instructions</span>
                </div>
                <p className="mt-1.5 text-[14px] text-ink-700">{a.instructions}</p>
              </div>
            )}
          </div>

          {/* Provider Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <Phone className="h-4 w-4 text-ink-400" />
              <span className="text-[13px] font-semibold text-ink-950">Contact Provider</span>
            </div>
            <div className="flex flex-wrap gap-2.5">
              <button className="flex items-center gap-2 rounded-xl border border-line bg-white/70 px-4 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-white">
                <Phone className="h-4 w-4" />
                Call
              </button>
              <button className="flex items-center gap-2 rounded-xl border border-line bg-white/70 px-4 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-white">
                <MessageSquare className="h-4 w-4" />
                Message
              </button>
              <button className="flex items-center gap-2 rounded-xl border border-line bg-white/70 px-4 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-white">
                <Mail className="h-4 w-4" />
                Email
              </button>
            </div>
          </div>
        </div>

        {/* ===== RIGHT COLUMN ===== */}
        <div className="space-y-5">
          {/* Provider Response */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary/10 text-primary">
                <Shield className="h-4 w-4" />
              </div>
              <span className="text-[15px] font-semibold text-ink-950">Provider Response</span>
            </div>

            {isPending ? (
              <>
                <div className="flex items-start gap-3 rounded-xl bg-amber-bg/30 p-3.5">
                  <Clock className="h-5 w-5 text-amber flex-shrink-0 mt-0.5" />
                  <div>
                    <div className="text-[13px] font-medium text-ink-950">Awaiting Response</div>
                    <p className="mt-0.5 text-[13px] text-ink-600">
                      Waiting for {a.provider} to accept or decline. Execution cannot begin until they respond.
                    </p>
                  </div>
                </div>

                <div className="mt-4 flex flex-col gap-2.5">
                  <button className="flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-4 py-3 text-[13px] font-semibold text-white transition hover:bg-primary-hover">
                    <Clock className="h-4 w-4" />
                    Nudge provider
                  </button>
                  {!cancelled ? (
                    <button
                      onClick={() => setCancelled(true)}
                      className={BTN_DANGER}
                    >
                      <XCircle className="h-4 w-4" />
                      Cancel assignment
                    </button>
                  ) : (
                    <span className="flex w-full items-center justify-center gap-2 rounded-xl bg-surface-2 px-4 py-3 text-[13px] font-medium text-ink-500">
                      <XCircle className="h-4 w-4" />
                      Assignment cancelled
                    </span>
                  )}
                </div>
              </>
            ) : a.status === "Accepted" ? (
              <div className="flex items-start gap-3 rounded-xl bg-green-bg/30 p-3.5">
                <CheckCircle2 className="h-5 w-5 text-green flex-shrink-0 mt-0.5" />
                <div>
                  <div className="text-[13px] font-medium text-ink-950">Provider Accepted</div>
                  <p className="mt-0.5 text-[13px] text-ink-600">
                    {a.provider} has accepted the assignment. Trip execution will begin shortly.
                  </p>
                </div>
              </div>
            ) : a.status === "Declined" ? (
              <div className="flex items-start gap-3 rounded-xl bg-red-bg/30 p-3.5">
                <XCircle className="h-5 w-5 text-red flex-shrink-0 mt-0.5" />
                <div>
                  <div className="text-[13px] font-medium text-ink-950">Provider Declined</div>
                  <p className="mt-0.5 text-[13px] text-ink-600">
                    {a.provider} has declined this assignment. Reassignment is required.
                  </p>
                </div>
              </div>
            ) : null}
          </div>

          {/* Quick Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="text-[13px] font-semibold text-ink-950 mb-3">Quick Actions</div>
            <div className="flex flex-col gap-2">
              <Link
                href={`/operations/marketplace/${a.offerId}`}
                className="flex items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[13px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Package className="h-4 w-4" />
                  View original offer
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </Link>
              {isActive && (
                <Link
                  href={`/operations/trips`}
                  className="flex items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[13px] font-medium text-ink-700 transition hover:bg-white"
                >
                  <span className="flex items-center gap-2">
                    <Truck className="h-4 w-4" />
                    View related trip
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </Link>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}