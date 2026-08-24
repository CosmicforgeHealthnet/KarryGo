// src/app/(dashboard)/fleet/work-orders/[workOrderId]/return-to-service/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Shield,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Clock,
  Calendar,
  User,
  MapPin,
  FileText,
  ArrowRight,
  Truck,
  Check,
  X,
  Send,
  History,
  AlertTriangle,
  Clipboard,
  Wrench,
  Award,
} from "lucide-react";

type WorkOrder = {
  id: string;
  assetId: string;
  assetName: string;
  assetType: string;
  reason: string;
  priority: "Critical" | "High" | "Medium" | "Low";
  state: "Open" | "In Progress" | "Ready for Review" | "Completed" | "Closed";
  assignedTo: string | null;
  scheduledDate: string;
  estimatedHours: string;
  actualHours: string | null;
  branch: string;
  downtime: string;
  createdAt: string;
  updatedAt: string;
  tasks: {
    id: string;
    description: string;
    status: "Pending" | "In Progress" | "Completed";
    assignedTo?: string;
  }[];
  partsUsed: {
    id: string;
    name: string;
    quantity: number;
    unit: string;
  }[];
  approvals: {
    id: string;
    label: string;
    by: string;
    timestamp: string;
  }[];
};

const WORK_ORDER_DETAIL: WorkOrder = {
  id: "WO-1917",
  assetId: "BR-204",
  assetName: "Honda ACE 110",
  assetType: "Motorcycle",
  reason: "Front brake failure - safety critical",
  priority: "Critical",
  state: "Ready for Review",
  assignedTo: "T. Adeyemi",
  scheduledDate: "Today",
  estimatedHours: "3 hrs",
  actualHours: "2.5 hrs",
  branch: "Ikeja",
  downtime: "2 days",
  createdAt: "2026-08-17T10:30:00Z",
  updatedAt: "2026-08-17T14:15:00Z",
  tasks: [
    { id: "task-1", description: "Replace front brake pads", status: "Completed", assignedTo: "T. Adeyemi" },
    { id: "task-2", description: "Machine brake rotor", status: "Completed", assignedTo: "T. Adeyemi" },
    { id: "task-3", description: "Flush and replace brake fluid", status: "Completed", assignedTo: "T. Adeyemi" },
    { id: "task-4", description: "Test braking system", status: "Completed", assignedTo: "T. Adeyemi" },
  ],
  partsUsed: [
    { id: "P-001", name: "Brake Pads - Front", quantity: 2, unit: "Set" },
    { id: "P-007", name: "Brake Rotor", quantity: 1, unit: "Unit" },
    { id: "P-012", name: "Brake Fluid DOT 4", quantity: 1, unit: "Liter" },
  ],
  approvals: [
    { id: "app-1", label: "Work approved", by: "F. Daniel", timestamp: "2026-08-17T11:00:00Z" },
    { id: "app-2", label: "Parts approved", by: "M. Bello", timestamp: "2026-08-17T11:30:00Z" },
  ],
};

const PRIORITY_STYLES: Record<string, { text: string; badge: string; icon: React.ReactNode }> = {
  Critical: { text: "text-red", badge: "bg-red-bg text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  High: { text: "text-amber", badge: "bg-amber-bg text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Medium: { text: "text-primary", badge: "bg-primary-bg text-primary", icon: <Clock className="h-3.5 w-3.5" /> },
  Low: { text: "text-ink-500", badge: "bg-surface-2 text-ink-500", icon: <Clock className="h-3.5 w-3.5" /> },
};

const TASK_STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-surface-2", text: "text-ink-500", icon: <Clock className="h-3.5 w-3.5" /> },
  "In Progress": { bg: "bg-amber-bg", text: "text-amber", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
};

export default function ReturnToServicePage() {
  const params = useParams();
  const router = useRouter();
  const workOrderId = params.workOrderId as string;

  const [loading, setLoading] = useState(true);
  const [workOrder, setWorkOrder] = useState<WorkOrder | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [decision, setDecision] = useState<"approve" | "deny" | null>(null);
  const [reason, setReason] = useState("");

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
      setWorkOrder(WORK_ORDER_DETAIL);
      setLoading(false);
    };
    loadData();
  }, [workOrderId]);

  const goBack = () => {
    router.push(`/fleet/work-orders/${workOrderId}`);
  };

  const goToAsset = (assetId: string) => {
    router.push(`/fleet/assets/${assetId}`);
  };

  const handleSubmit = async () => {
    if (!decision) return;
    setSubmitting(true);
    await new Promise((resolve) => setTimeout(resolve, 1500));
    setSubmitting(false);
    router.push(`/fleet/work-orders/${workOrderId}`);
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading...</span>
        </div>
      </div>
    );
  }

  if (!workOrder) {
    return (
      <div className="py-12">
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Shield className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">Work order not found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            The work order you're looking for doesn't exist.
          </p>
          <button
            onClick={() => router.push("/fleet/work-orders")}
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            Back to work orders
          </button>
        </div>
      </div>
    );
  }

  const priorityStyle = PRIORITY_STYLES[workOrder.priority] || PRIORITY_STYLES.Low;

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2.5 flex-wrap">
            <h1>Return to Service</h1>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${priorityStyle.badge}`}>
              {priorityStyle.icon}
              {workOrder.priority}
            </span>
            <span className="flex items-center gap-1 rounded-full bg-primary-bg px-2.5 py-0.5 text-xs font-semibold text-primary">
              <span className="h-1.5 w-1.5 rounded-full bg-primary" />
              Ready for Review
            </span>
          </div>
          <p className="mt-0.5 text-sm text-ink-500">
            {workOrder.id} · {workOrder.assetName}
          </p>
        </div>
      </div>

      {/* ── Info Banner ── */}
      <div className="rounded-lg border border-primary/20 bg-primary-bg/20 p-4">
        <div className="flex items-start gap-3">
          <Shield className="h-5 w-5 text-primary flex-shrink-0 mt-0.5" />
          <div>
            <div className="text-sm font-semibold text-ink-950">Return-to-Service Decision Required</div>
            <p className="mt-0.5 text-sm text-ink-600">
              All work has been completed and inspection passed. Review the evidence below and make your decision.
            </p>
          </div>
        </div>
      </div>

      {/* ── Summary Grid ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <div>
              <div className="text-xs font-medium text-ink-500">Tasks</div>
              <div className="text-sm font-semibold text-ink-950">
                {workOrder.tasks.filter(t => t.status === "Completed").length}/{workOrder.tasks.length} Complete
              </div>
            </div>
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <div>
              <div className="text-xs font-medium text-ink-500">Actual Hours</div>
              <div className="text-sm font-semibold text-ink-950">{workOrder.actualHours || "N/A"}</div>
            </div>
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Truck className="h-[18px] w-[18px]" />
            </div>
            <div>
              <div className="text-xs font-medium text-ink-500">Parts Used</div>
              <div className="text-sm font-semibold text-ink-950">{workOrder.partsUsed.length}</div>
            </div>
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <Award className="h-[18px] w-[18px]" />
            </div>
            <div>
              <div className="text-xs font-medium text-ink-500">Status</div>
              <div className="text-sm font-semibold text-green">Ready</div>
            </div>
          </div>
        </div>
      </div>

      {/* ── Completed Tasks ── */}
      <div className="rounded-lg border border-line bg-white p-5">
        <div className="flex items-center gap-2.5 mb-3">
          <Clipboard className="h-5 w-5 text-ink-400" />
          <h2 className="text-sm font-semibold text-ink-950">Completed Work</h2>
          <span className="rounded-full bg-green-bg px-2 py-0.5 text-[10px] font-semibold text-green">
            All {workOrder.tasks.length} tasks complete
          </span>
        </div>
        <div className="space-y-2">
          {workOrder.tasks.map((task) => {
            const taskStatus = TASK_STATUS_STYLES[task.status] || TASK_STATUS_STYLES.Pending;
            return (
              <div key={task.id} className="flex items-center gap-3 rounded-lg border border-line p-3">
                <div className={`rounded-full p-1 ${taskStatus.bg}`}>
                  {taskStatus.icon}
                </div>
                <div className="flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-ink-950">{task.description}</span>
                    <span className={`text-[10px] font-semibold ${taskStatus.text}`}>
                      {task.status}
                    </span>
                  </div>
                  {task.assignedTo && (
                    <div className="text-xs text-ink-500">By: {task.assignedTo}</div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* ── Inspection Passed ── */}
      <div className="rounded-lg border border-green/20 bg-green-bg/20 p-5">
        <div className="flex items-center gap-2.5 mb-3">
          <CheckCircle2 className="h-5 w-5 text-green" />
          <h2 className="text-sm font-semibold text-ink-950">Inspection Passed</h2>
          <span className="rounded-full bg-green-bg px-2.5 py-0.5 text-[10px] font-semibold text-green">
            Verified
          </span>
        </div>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <div className="flex items-center gap-2 rounded-lg border border-green/20 bg-white/50 p-3 text-sm text-ink-700">
            <CheckCircle2 className="h-4 w-4 text-green flex-shrink-0" />
            Brake system fully functional
          </div>
          <div className="flex items-center gap-2 rounded-lg border border-green/20 bg-white/50 p-3 text-sm text-ink-700">
            <CheckCircle2 className="h-4 w-4 text-green flex-shrink-0" />
            All safety checks passed
          </div>
          <div className="flex items-center gap-2 rounded-lg border border-green/20 bg-white/50 p-3 text-sm text-ink-700">
            <CheckCircle2 className="h-4 w-4 text-green flex-shrink-0" />
            Test drive completed successfully
          </div>
          <div className="flex items-center gap-2 rounded-lg border border-green/20 bg-white/50 p-3 text-sm text-ink-700">
            <CheckCircle2 className="h-4 w-4 text-green flex-shrink-0" />
            No remaining safety blocks
          </div>
        </div>
      </div>

      {/* ── Decision ── */}
      <div className="rounded-lg border border-line bg-white p-5">
        <h2 className="text-sm font-semibold text-ink-950 mb-3">Make Decision</h2>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <button
            onClick={() => setDecision("approve")}
            className={`flex items-center gap-3 rounded-lg border-2 p-4 transition-colors ${
              decision === "approve"
                ? "border-green/50 bg-green-bg/20"
                : "border-line hover:border-green/30 hover:bg-green-bg/10"
            }`}
          >
            <div className={`rounded-full p-1.5 ${
              decision === "approve" ? "bg-green text-white" : "bg-green-bg text-green"
            }`}>
              <Check className="h-5 w-5" />
            </div>
            <div className="text-left">
              <div className={`text-sm font-semibold ${
                decision === "approve" ? "text-green" : "text-ink-950"
              }`}>
                Approve Return to Service
              </div>
              <div className="text-xs text-ink-500">Asset cleared for operation</div>
            </div>
          </button>

          <button
            onClick={() => setDecision("deny")}
            className={`flex items-center gap-3 rounded-lg border-2 p-4 transition-colors ${
              decision === "deny"
                ? "border-red/50 bg-red-bg/20"
                : "border-line hover:border-red/30 hover:bg-red-bg/10"
            }`}
          >
            <div className={`rounded-full p-1.5 ${
              decision === "deny" ? "bg-red text-white" : "bg-red-bg text-red"
            }`}>
              <X className="h-5 w-5" />
            </div>
            <div className="text-left">
              <div className={`text-sm font-semibold ${
                decision === "deny" ? "text-red" : "text-ink-950"
              }`}>
                Deny Return to Service
              </div>
              <div className="text-xs text-ink-500">Asset remains blocked</div>
            </div>
          </button>
        </div>

        {decision === "deny" && (
          <div className="mt-3">
            <label className="mb-1.5 block text-xs font-medium text-ink-700">
              Reason for denial <span className="text-red">*</span>
            </label>
            <textarea
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              rows={2}
              placeholder="Explain why the asset cannot return to service..."
              className="w-full rounded-lg border border-line px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 resize-none"
            />
          </div>
        )}

        {decision === "approve" && (
          <div className="mt-3 flex items-center gap-2 rounded-lg bg-green-bg/20 px-3.5 py-2 text-xs text-green-700">
            <CheckCircle2 className="h-4 w-4" />
            Approving will clear all safety blocks and return the asset to service.
          </div>
        )}

        <button
          onClick={handleSubmit}
          disabled={!decision || (decision === "deny" && !reason.trim()) || submitting}
          className={`mt-4 flex w-full items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-semibold text-white transition-colors ${
            decision === "approve"
              ? "bg-green hover:bg-green/90"
              : decision === "deny"
              ? "bg-red hover:bg-red/90"
              : "bg-ink-300 cursor-not-allowed"
          } disabled:opacity-60 disabled:cursor-not-allowed`}
        >
          {submitting ? (
            <>
              <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
              Submitting...
            </>
          ) : (
            <>
              <Send className="h-4 w-4" />
              {decision === "approve" ? "Approve Return to Service" : "Deny Return to Service"}
            </>
          )}
        </button>
      </div>

      {/* ── Navigation ── */}
      <div className="flex flex-col gap-2 border-t border-line pt-4 sm:flex-row sm:justify-between">
        <button
          onClick={goBack}
          className="flex items-center gap-1.5 text-xs font-medium text-ink-500 transition-colors hover:text-ink-900"
        >
          Back to work order
        </button>
        <button
          onClick={() => goToAsset(workOrder.assetId)}
          className="flex items-center gap-1.5 text-xs font-medium text-primary transition-colors hover:text-primary-hover"
        >
          View Asset
          <ArrowRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}