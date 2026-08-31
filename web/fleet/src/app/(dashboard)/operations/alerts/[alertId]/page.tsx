// src/app/(dashboard)/operations/alerts/[alertId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Bell,
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Clock,
  MapPin,
  RefreshCw,
  Users,
  Navigation,
  Shield,
  Package,
  Truck,
  Flag,
  Eye,
  LifeBuoy,
  Calendar,
  Activity,
  Info,
  XCircle,
  Check,
  User,
  FileText,
  MessageSquare,
  ExternalLink,
} from "lucide-react";
import { ALERT_DETAIL } from "@/lib/mock-alerts-data";

const SEVERITY_STYLES = {
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: AlertCircle },
  High: { bg: "bg-red-bg/70", text: "text-red", dot: "bg-red", icon: AlertTriangle },
  Medium: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: AlertCircle },
  Low: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: Info },
};

const STATUS_STYLES = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Acknowledged: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Dismissed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const TYPE_ICONS = {
  Trip: Navigation,
  Provider: Users,
  Asset: Truck,
  Safety: Shield,
  System: Activity,
  Weather: Clock,
  Customer: Bell,
};

const BTN_PRIMARY =
  "flex items-center justify-center gap-1.5 rounded-xl bg-primary px-3.5 py-2.5 text-[12.5px] font-semibold text-white shadow-sm shadow-primary/25 transition hover:bg-primary-hover hover:shadow-md hover:shadow-primary/30 disabled:opacity-60 disabled:cursor-not-allowed";
const BTN_SECONDARY =
  "flex items-center justify-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 py-2.5 text-[12.5px] font-semibold text-ink-800 shadow-sm backdrop-blur-xl transition hover:bg-white hover:border-primary/30";
const BTN_DANGER =
  "flex items-center justify-center gap-1.5 rounded-xl border border-red/20 bg-red-bg/50 px-3.5 py-2.5 text-[12.5px] font-semibold text-red shadow-sm transition hover:bg-red-bg hover:border-red/40";

export default function AlertDetailPage() {
  const params = useParams();
  const router = useRouter();
  const alertId = params.alertId as string;

  const [loading, setLoading] = useState(true);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [alert, setAlert] = useState(ALERT_DETAIL);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setAlert(ALERT_DETAIL);
      setLoading(false);
    };
    loadData();
  }, [alertId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const handleAcknowledge = () => {
    // In production, this would call an API
    setAlert({ ...alert, status: "Acknowledged" });
  };

  const handleResolve = () => {
    // In production, this would call an API
    setAlert({ ...alert, status: "Resolved" });
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading alert...</span>
        </div>
      </div>
    );
  }

  if (!alert) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <AlertCircle className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Alert not found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            The alert you're looking for doesn't exist or has been removed.
          </p>
          <Link
            href="/operations/alerts"
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to alerts
          </Link>
        </div>
      </div>
    );
  }

  const severityStyle = SEVERITY_STYLES[alert.severity as keyof typeof SEVERITY_STYLES] || SEVERITY_STYLES.Low;
  const statusStyle = STATUS_STYLES[alert.status as keyof typeof STATUS_STYLES] || STATUS_STYLES.Open;
  const SeverityIcon = severityStyle.icon;
  const TypeIcon = TYPE_ICONS[alert.type as keyof typeof TYPE_ICONS] || Bell;
  const isOpen = alert.status === "Open";
  const isResolved = alert.status === "Resolved" || alert.status === "Dismissed";

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      {/* ── Header ─────────────────────────────────────────── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <Link
              href="/operations/alerts"
              className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-white/70 text-ink-600 transition hover:bg-white hover:text-ink-900"
            >
              <ArrowLeft className="h-4 w-4" />
            </Link>
            <div>
              <div className="flex items-center gap-2.5">
                <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                  {alert.id}
                </h1>
                <span
                  className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}
                >
                  <SeverityIcon className="h-3.5 w-3.5" />
                  {alert.severity}
                </span>
                <span
                  className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                >
                  <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                  {alert.status}
                </span>
              </div>
              <p className="mt-0.5 text-[13.5px] text-ink-600">
                {alert.type} alert · {alert.branch} branch · {alert.source}
              </p>
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          {!isResolved && (
            <>
              <button
                onClick={handleAcknowledge}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
              >
                <Flag className="h-3.5 w-3.5" />
                Acknowledge
              </button>
              <button
                onClick={handleResolve}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-green px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-green/20 transition hover:bg-green/90"
              >
                <Check className="h-3.5 w-3.5" />
                Resolve
              </button>
            </>
          )}
          {isResolved && (
            <span className="flex h-9 items-center gap-1.5 rounded-xl bg-green-bg px-3.5 text-[11.5px] font-semibold text-green">
              <CheckCircle2 className="h-3.5 w-3.5" />
              Resolved
            </span>
          )}
        </div>
      </div>

      {/* ── Main content ────────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        {/* Left column - 2/3 */}
        <div className="space-y-5 lg:col-span-2">
          {/* Alert details */}
          <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
            <div className="mb-3 flex items-center gap-2">
              <TypeIcon className="h-4 w-4 text-ink-400" />
              <h3 className="text-[13px] font-semibold text-ink-950">Alert details</h3>
            </div>
            <div className="space-y-3">
              <div>
                <div className="text-[11px] font-medium uppercase tracking-wide text-ink-500">Title</div>
                <p className="mt-0.5 text-[14px] font-semibold text-ink-950">{alert.title}</p>
              </div>
              <div>
                <div className="text-[11px] font-medium uppercase tracking-wide text-ink-500">Description</div>
                <p className="mt-0.5 text-[13px] text-ink-700">{alert.description}</p>
              </div>
              <div className="grid grid-cols-2 gap-3 border-t border-line pt-3">
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">Source</div>
                  <div className="mt-0.5 text-[13px] font-medium text-ink-950">{alert.source}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">Branch</div>
                  <div className="mt-0.5 text-[13px] font-medium text-ink-950">{alert.branch}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">Created</div>
                  <div className="mt-0.5 text-[13px] font-medium text-ink-950">
                    {new Date(alert.createdAt).toLocaleString()}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">SLA</div>
                  <div className={`mt-0.5 text-[13px] font-medium ${alert.sla === "Breached" ? "text-red" : "text-ink-950"}`}>
                    {alert.sla}
                  </div>
                </div>
              </div>
              {alert.assignedTo && (
                <div className="border-t border-line pt-3">
                  <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">Assigned to</div>
                  <div className="mt-0.5 flex items-center gap-2">
                    <div className="flex h-7 w-7 items-center justify-center rounded-full bg-primary-bg text-[10px] font-bold text-primary">
                      {alert.assignedTo.split(" ").map((n) => n[0]).join("")}
                    </div>
                    <span className="text-[13px] font-medium text-ink-950">{alert.assignedTo}</span>
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* Action history */}
          <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
            <div className="mb-3 flex items-center gap-2">
              <Clock className="h-4 w-4 text-ink-400" />
              <h3 className="text-[13px] font-semibold text-ink-950">Action history</h3>
            </div>
            <div className="space-y-3">
              {alert.actions.map((action, index) => (
                <div key={index} className="flex gap-3">
                  <div className="flex flex-col items-center">
                    <div className="h-2 w-2 rounded-full bg-primary-bg mt-1.5" />
                    {index < alert.actions.length - 1 && (
                      <div className="h-full w-px bg-line" />
                    )}
                  </div>
                  <div>
                    <div className="text-[13px] font-medium text-ink-950">{action.label}</div>
                    <div className="text-[11.5px] text-ink-500">{action.timestamp}</div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Right column - 1/3 */}
        <div className="space-y-4">
          {/* Quick actions */}
          <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
            <h3 className="mb-2.5 text-[13px] font-semibold text-ink-950">Quick actions</h3>
            <div className="space-y-2">
              {!isResolved && (
                <>
                  <button
                    onClick={handleAcknowledge}
                    className="flex w-full items-center gap-2 rounded-lg bg-primary px-3 py-2 text-[12px] font-semibold text-white transition hover:bg-primary/90"
                  >
                    <Flag className="h-3.5 w-3.5" />
                    Acknowledge alert
                  </button>
                  <button
                    onClick={handleResolve}
                    className="flex w-full items-center gap-2 rounded-lg bg-green px-3 py-2 text-[12px] font-semibold text-white transition hover:bg-green/90"
                  >
                    <Check className="h-3.5 w-3.5" />
                    Resolve alert
                  </button>
                </>
              )}
              <Link
                href={`/operations/rescue`}
                className="flex w-full items-center gap-2 rounded-lg border border-line bg-white/70 px-3 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-white"
              >
                <LifeBuoy className="h-3.5 w-3.5" />
                Create rescue case
              </Link>
            </div>
          </div>

          {/* Related resources */}
          {(alert.tripId || alert.assignmentId || alert.providerId || alert.assetId) && (
            <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
              <h3 className="mb-2.5 text-[13px] font-semibold text-ink-950">Related resources</h3>
              <div className="space-y-2">
                {alert.tripId && (
                  <Link
                    href={`/operations/trips/${alert.tripId}`}
                    className="flex items-center gap-2 rounded-lg border border-line bg-white/70 px-3 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-white"
                  >
                    <Navigation className="h-3.5 w-3.5" />
                    Trip: {alert.tripId}
                    <ExternalLink className="h-3.5 w-3.5 ml-auto text-ink-400" />
                  </Link>
                )}
                {alert.assignmentId && (
                  <Link
                    href={`/operations/assignments/${alert.assignmentId}`}
                    className="flex items-center gap-2 rounded-lg border border-line bg-white/70 px-3 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-white"
                  >
                    <FileText className="h-3.5 w-3.5" />
                    Assignment: {alert.assignmentId}
                    <ExternalLink className="h-3.5 w-3.5 ml-auto text-ink-400" />
                  </Link>
                )}
                {alert.providerId && (
                  <Link
                    href={`/providers/${alert.providerId}`}
                    className="flex items-center gap-2 rounded-lg border border-line bg-white/70 px-3 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-white"
                  >
                    <User className="h-3.5 w-3.5" />
                    Provider: {alert.providerId}
                    <ExternalLink className="h-3.5 w-3.5 ml-auto text-ink-400" />
                  </Link>
                )}
                {alert.assetId && (
                  <Link
                    href={`/fleet/assets/${alert.assetId}`}
                    className="flex items-center gap-2 rounded-lg border border-line bg-white/70 px-3 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-white"
                  >
                    <Truck className="h-3.5 w-3.5" />
                    Asset: {alert.assetId}
                    <ExternalLink className="h-3.5 w-3.5 ml-auto text-ink-400" />
                  </Link>
                )}
              </div>
            </div>
          )}

          {/* Metadata */}
          <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
            <div className="space-y-2 text-[12px]">
              <div className="flex justify-between">
                <span className="text-ink-500">Alert ID</span>
                <span className="font-mono font-medium text-ink-950">{alert.id}</span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Created</span>
                <span className="font-medium text-ink-950">
                  {new Date(alert.createdAt).toLocaleString()}
                </span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Last updated</span>
                <span className="font-medium text-ink-950">{secondsAgo}s ago</span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Severity</span>
                <span className={`font-medium ${severityStyle.text}`}>{alert.severity}</span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Status</span>
                <span className={`font-medium ${statusStyle.text}`}>{alert.status}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}