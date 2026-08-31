// src/app/(dashboard)/fleet/breakdowns/[breakdownId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Wrench,
  MapPin,
  Clock,
  CheckCircle2,
  LifeBuoy,
  List,
  AlertCircle,
  Edit3,
  User,
  Calendar,
  Shield,
  FileText,
  Truck,
  ArrowRight,
  Box,
  AlertTriangle,
  History,
  MessageSquare,
} from "lucide-react";

type BreakdownDetail = {
  id: string;
  assetId: string;
  assetName: string;
  assetType: string;
  severity: "Critical" | "High" | "Medium" | "Low";
  status: "Open" | "In Progress" | "Resolved" | "Closed";
  type: "Mechanical" | "Electrical" | "Accident" | "Tyre" | "Other";
  description: string;
  location: string;
  branch: string;
  reportedBy: string;
  assignedTo: string | null;
  reportedAt: string;
  resolvedAt: string | null;
  downtime: string;
  towingRequired: boolean;
  recoveryNotes: string;
  partsReplaced: string[];
  timeline: {
    id: string;
    event: string;
    timestamp: string;
    performedBy: string;
  }[];
};

const BREAKDOWN_DETAIL: BreakdownDetail = {
  id: "BRK-001",
  assetId: "BR-204",
  assetName: "Honda ACE 110",
  assetType: "Motorcycle",
  severity: "Critical",
  status: "In Progress",
  type: "Mechanical",
  description: "Front brake failure - complete brake system failure. Brake pads completely worn down. Rotor damaged. Brake fluid contaminated.",
  location: "Ikeja-Lekki Expressway",
  branch: "Ikeja",
  reportedBy: "Peter Okafor",
  assignedTo: "T. Adeyemi",
  reportedAt: "2026-08-17T14:10:00Z",
  resolvedAt: null,
  downtime: "2 days",
  towingRequired: true,
  recoveryNotes: "Vehicle towed to Ikeja workshop. Awaiting parts delivery.",
  partsReplaced: ["Brake Pads - Front", "Brake Rotor", "Brake Fluid DOT 4"],
  timeline: [
    {
      id: "tl-001",
      event: "Breakdown reported by provider",
      timestamp: "2026-08-17T14:10:00Z",
      performedBy: "Peter Okafor",
    },
    {
      id: "tl-002",
      event: "Towing dispatched",
      timestamp: "2026-08-17T14:25:00Z",
      performedBy: "System",
    },
    {
      id: "tl-003",
      event: "Vehicle arrived at workshop",
      timestamp: "2026-08-17T15:00:00Z",
      performedBy: "T. Adeyemi",
    },
    {
      id: "tl-004",
      event: "Diagnosis completed",
      timestamp: "2026-08-17T15:30:00Z",
      performedBy: "M. Bello",
    },
    {
      id: "tl-005",
      event: "Parts ordered",
      timestamp: "2026-08-17T16:00:00Z",
      performedBy: "F. Daniel",
    },
  ],
};

const SEVERITY_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-4 w-4" /> },
  High: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <AlertCircle className="h-4 w-4" /> },
  Medium: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary", icon: <Clock className="h-4 w-4" /> },
  Low: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <CheckCircle2 className="h-4 w-4" /> },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  "In Progress": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const TYPE_ICONS: Record<string, React.ReactNode> = {
  Mechanical: <Wrench className="h-4 w-4" />,
  Electrical: <AlertCircle className="h-4 w-4" />,
  Accident: <AlertTriangle className="h-4 w-4" />,
  Tyre: <AlertCircle className="h-4 w-4" />,
  Other: <FileText className="h-4 w-4" />,
};

export default function BreakdownDetailPage() {
  const params = useParams();
  const router = useRouter();
  const breakdownId = params.breakdownId as string;

  const [loading, setLoading] = useState(true);
  const [breakdown, setBreakdown] = useState<BreakdownDetail | null>(null);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
      setBreakdown(BREAKDOWN_DETAIL);
      setLoading(false);
    };
    loadData();
  }, [breakdownId]);

  const goBack = () => {
    router.push("/fleet/breakdowns");
  };

  const goToAsset = (assetId: string) => {
    router.push(`/fleet/assets/${assetId}`);
  };

  const goToEdit = () => {
    router.push(`/fleet/breakdowns/${breakdownId}/edit`);
  };

  const goToRescue = () => {
    router.push("/operations/rescue");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading breakdown details...</span>
        </div>
      </div>
    );
  }

  if (!breakdown) {
    return (
      <div className="py-12">
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Wrench className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">Breakdown not found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            The breakdown you're looking for doesn't exist or has been removed.
          </p>
          <button
            onClick={goBack}
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            Back to breakdowns
          </button>
        </div>
      </div>
    );
  }

  const severityStyle = SEVERITY_STYLES[breakdown.severity] || SEVERITY_STYLES.Low;
  const statusStyle = STATUS_STYLES[breakdown.status] || STATUS_STYLES.Open;
  const TypeIcon = TYPE_ICONS[breakdown.type] || <FileText className="h-4 w-4" />;
  const isOpen = breakdown.status === "Open" || breakdown.status === "In Progress";
  const isCritical = breakdown.severity === "Critical";

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2.5 flex-wrap">
            <h1>{breakdown.id}</h1>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
              {severityStyle.icon}
              {breakdown.severity}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
              {breakdown.status}
            </span>
          </div>
          <p className="mt-0.5 text-sm text-ink-500">
            {breakdown.assetName} · {breakdown.location}
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={goToEdit}
            className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
          {isOpen && (
            <button
              onClick={goToRescue}
              className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
            >
              <LifeBuoy className="h-3.5 w-3.5" />
              Rescue
            </button>
          )}
        </div>
      </div>

      {/* ── Stats Row ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Downtime</span>
          </div>
          <div className="mt-2 text-sm font-semibold text-ink-950">{breakdown.downtime}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Calendar className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Reported</span>
          </div>
          <div className="mt-2 text-sm font-semibold text-ink-950">
            {new Date(breakdown.reportedAt).toLocaleString()}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <User className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Reported By</span>
          </div>
          <div className="mt-2 text-sm font-semibold text-ink-950">{breakdown.reportedBy}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              {breakdown.towingRequired ? (
                <AlertCircle className="h-[18px] w-[18px] text-amber" />
              ) : (
                <CheckCircle2 className="h-[18px] w-[18px] text-green" />
              )}
            </div>
            <span className="text-xs font-semibold text-ink-600">Towing</span>
          </div>
          <div className="mt-2 text-sm font-semibold text-ink-950">
            {breakdown.towingRequired ? "Required" : "Not Required"}
          </div>
        </div>
      </div>

      {/* ── Main Grid ── */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.4fr_1fr]">
        {/* ── Left Column ── */}
        <div className="space-y-5">
          {/* Description */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center gap-2.5 mb-3">
              <FileText className="h-5 w-5 text-ink-400" />
              <h2 className="text-sm font-semibold text-ink-950">Description</h2>
            </div>
            <div className="rounded-lg bg-surface-2 p-3.5 text-sm text-ink-700">
              {breakdown.description}
            </div>
            <div className="mt-3 flex items-center gap-2 text-xs text-ink-500">
              <MapPin className="h-3.5 w-3.5" />
              Location: {breakdown.location}
            </div>
          </div>

          {/* Asset Info */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center gap-2.5 mb-3">
              <Truck className="h-5 w-5 text-ink-400" />
              <h2 className="text-sm font-semibold text-ink-950">Asset</h2>
            </div>
            <div
              onClick={() => goToAsset(breakdown.assetId)}
              className="flex cursor-pointer items-center justify-between rounded-lg border border-line p-3 transition-colors hover:bg-surface-2"
            >
              <div>
                <div className="text-sm font-semibold text-ink-950">{breakdown.assetName}</div>
                <div className="text-xs text-ink-500">{breakdown.assetId} · {breakdown.assetType}</div>
              </div>
              <ArrowRight className="h-4 w-4 text-ink-400" />
            </div>
            <div className="mt-3 flex items-center gap-2 text-xs text-ink-500">
              <MapPin className="h-3.5 w-3.5" />
              {breakdown.branch}
            </div>
          </div>

          {/* Recovery Notes */}
          {breakdown.recoveryNotes && (
            <div className="rounded-lg border border-line bg-white p-5">
              <div className="flex items-center gap-2.5 mb-2">
                <MessageSquare className="h-5 w-5 text-ink-400" />
                <h2 className="text-sm font-semibold text-ink-950">Recovery Notes</h2>
              </div>
              <p className="text-sm text-ink-700">{breakdown.recoveryNotes}</p>
            </div>
          )}

          {/* Parts Replaced */}
          {breakdown.partsReplaced && breakdown.partsReplaced.length > 0 && (
            <div className="rounded-lg border border-line bg-white p-5">
              <div className="flex items-center gap-2.5 mb-3">
                <Box className="h-5 w-5 text-ink-400" />
                <h2 className="text-sm font-semibold text-ink-950">Parts Replaced</h2>
                <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                  {breakdown.partsReplaced.length}
                </span>
              </div>
              <div className="flex flex-wrap gap-2">
                {breakdown.partsReplaced.map((part, index) => (
                  <span
                    key={index}
                    className="rounded-full bg-surface-2 px-3 py-1 text-xs font-medium text-ink-700"
                  >
                    {part}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* ── Right Column ── */}
        <div className="space-y-5">
          {/* Status & Assignment */}
          <div className={`rounded-lg border p-5 ${
            isCritical ? "border-red/30 bg-red-bg/10" : "border-line bg-white"
          }`}>
            <div className="flex items-center gap-2.5 mb-3">
              <Shield className={`h-5 w-5 ${isCritical ? "text-red" : "text-ink-400"}`} />
              <h2 className="text-sm font-semibold text-ink-950">Status & Assignment</h2>
            </div>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Severity</span>
                <span className={`font-semibold ${severityStyle.text}`}>{breakdown.severity}</span>
              </div>
              <div className="flex items-center justify-between border-t border-line pt-2">
                <span className="text-ink-500">Status</span>
                <span className={`font-semibold ${statusStyle.text}`}>{breakdown.status}</span>
              </div>
              <div className="flex items-center justify-between border-t border-line pt-2">
                <span className="text-ink-500">Assigned To</span>
                <span className="font-medium text-ink-950">{breakdown.assignedTo || "Unassigned"}</span>
              </div>
              {breakdown.resolvedAt && (
                <div className="flex items-center justify-between border-t border-line pt-2">
                  <span className="text-ink-500">Resolved</span>
                  <span className="font-medium text-ink-950">
                    {new Date(breakdown.resolvedAt).toLocaleString()}
                  </span>
                </div>
              )}
            </div>
          </div>

          {/* Timeline */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center gap-2.5 mb-3">
              <History className="h-5 w-5 text-ink-400" />
              <h2 className="text-sm font-semibold text-ink-950">Timeline</h2>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                {breakdown.timeline.length}
              </span>
            </div>
            <div className="space-y-3">
              {breakdown.timeline.map((event, index) => (
                <div key={event.id} className="flex items-start gap-3">
                  <div className="flex flex-col items-center">
                    <div className={`h-3 w-3 rounded-full ${
                      index === 0 ? "bg-red" : index === breakdown.timeline.length - 1 ? "bg-green" : "bg-primary"
                    }`} />
                    {index < breakdown.timeline.length - 1 && (
                      <div className="h-6 w-px bg-line" />
                    )}
                  </div>
                  <div>
                    <div className="text-sm font-medium text-ink-950">{event.event}</div>
                    <div className="text-xs text-ink-500">
                      {event.performedBy} · {new Date(event.timestamp).toLocaleString()}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Quick Actions */}
          <div className="rounded-lg border border-line bg-white p-5">
            <h2 className="text-sm font-semibold text-ink-950 mb-3">Quick Actions</h2>
            <div className="space-y-2">
              <button
                onClick={() => goToAsset(breakdown.assetId)}
                className="flex w-full items-center justify-between rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                <span className="flex items-center gap-2">
                  <Truck className="h-4 w-4" />
                  View Asset
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              {isOpen && (
                <button
                  onClick={goToRescue}
                  className="flex w-full items-center justify-between rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
                >
                  <span className="flex items-center gap-2">
                    <LifeBuoy className="h-4 w-4" />
                    Create Rescue Case
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              )}
              <button
                onClick={() => router.push("/fleet/breakdowns")}
                className="flex w-full items-center justify-between rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                <span className="flex items-center gap-2">
                  <List className="h-4 w-4" />
                  All Breakdowns
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}