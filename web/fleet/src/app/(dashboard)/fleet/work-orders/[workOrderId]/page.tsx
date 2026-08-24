// src/app/(dashboard)/fleet/work-orders/[workOrderId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  Wrench,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Clock,
  Calendar,
  User,
  MapPin,
  FileText,
  Plus,
  Edit3,
  MoreVertical,
  ArrowRight,
  Flame,
  AlertTriangle,
  Timer,
  Truck,
  Check,
  X,
  Send,
  History,
  Shield,
  Clipboard,
} from "lucide-react";

type WorkOrderDetail = {
  id: string;
  assetId: string;
  assetName: string;
  assetType: string;
  reason: string;
  diagnosis: string;
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

const WORK_ORDER_DETAIL: WorkOrderDetail = {
  id: "WO-1917",
  assetId: "BR-204",
  assetName: "Honda ACE 110",
  assetType: "Motorcycle",
  reason: "Front brake failure - safety critical",
  diagnosis: "Brake pads completely worn down. Rotor damaged. Brake fluid contaminated. Complete brake system overhaul required.",
  priority: "Critical",
  state: "In Progress",
  assignedTo: "T. Adeyemi",
  scheduledDate: "Today",
  estimatedHours: "3 hrs",
  actualHours: null,
  branch: "Ikeja",
  downtime: "2 days",
  createdAt: "2026-08-17T10:30:00Z",
  updatedAt: "2026-08-17T14:15:00Z",
  tasks: [
    { id: "task-1", description: "Replace front brake pads", status: "Completed", assignedTo: "T. Adeyemi" },
    { id: "task-2", description: "Machine brake rotor", status: "Completed", assignedTo: "T. Adeyemi" },
    { id: "task-3", description: "Flush and replace brake fluid", status: "In Progress", assignedTo: "T. Adeyemi" },
    { id: "task-4", description: "Test braking system", status: "Pending", assignedTo: "T. Adeyemi" },
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
  Critical: { text: "text-red", badge: "bg-red-bg text-red", icon: <Flame className="h-3.5 w-3.5" /> },
  High: { text: "text-amber", badge: "bg-amber-bg text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Medium: { text: "text-primary", badge: "bg-primary-bg text-primary", icon: <Clock className="h-3.5 w-3.5" /> },
  Low: { text: "text-ink-500", badge: "bg-surface-2 text-ink-500", icon: <Timer className="h-3.5 w-3.5" /> },
};

const STATE_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  "In Progress": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  "Ready for Review": { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const TASK_STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-surface-2", text: "text-ink-500", icon: <Clock className="h-3.5 w-3.5" /> },
  "In Progress": { bg: "bg-amber-bg", text: "text-amber", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
};

export default function WorkOrderDetailPage() {
  const params = useParams();
  const router = useRouter();
  const workOrderId = params.workOrderId as string;

  const [loading, setLoading] = useState(true);
  const [workOrder, setWorkOrder] = useState<WorkOrderDetail | null>(null);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
      setWorkOrder(WORK_ORDER_DETAIL);
      setLoading(false);
    };
    loadData();
  }, [workOrderId]);

  const goBack = () => {
    router.push("/fleet/work-orders");
  };

  const goToReturnToService = () => {
    router.push(`/fleet/work-orders/${workOrderId}/return-to-service`);
  };

  const goToAsset = (assetId: string) => {
    router.push(`/fleet/assets/${assetId}`);
  };

  const goToEdit = () => {
    router.push(`/fleet/work-orders/${workOrderId}/edit`);
  };

  const goToControlTower = () => {
    router.push("/operations/control-tower");
  };

  const goToParts = () => {
    router.push("/fleet/parts");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading work order...</span>
        </div>
      </div>
    );
  }

  if (!workOrder) {
    return (
      <div className="py-12">
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Wrench className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">Work order not found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            The work order you're looking for doesn't exist or has been removed.
          </p>
          <button
            onClick={goBack}
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            Back to work orders
          </button>
        </div>
      </div>
    );
  }

  const priorityStyle = PRIORITY_STYLES[workOrder.priority] || PRIORITY_STYLES.Low;
  const stateStyle = STATE_STYLES[workOrder.state] || STATE_STYLES.Open;
  const isOpen = workOrder.state === "Open" || workOrder.state === "In Progress";
  const isCompleted = workOrder.state === "Completed" || workOrder.state === "Closed";

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2.5 flex-wrap">
            <h1>{workOrder.id}</h1>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${priorityStyle.badge}`}>
              {priorityStyle.icon}
              {workOrder.priority}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${stateStyle.bg} ${stateStyle.text}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${stateStyle.dot}`} />
              {workOrder.state}
            </span>
          </div>
          <p className="mt-0.5 text-sm text-ink-500">
            {workOrder.assetName} · {workOrder.branch} branch
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
          {!isCompleted && (
            <button
              onClick={goToReturnToService}
              className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
            >
              <Shield className="h-3.5 w-3.5" />
              Return to Service
            </button>
          )}
        </div>
      </div>

      {/* ── Stats Row ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <User className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Assigned To</span>
          </div>
          <div className="mt-2 text-sm font-semibold text-ink-950">{workOrder.assignedTo || "Unassigned"}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Calendar className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Scheduled</span>
          </div>
          <div className="mt-2 text-sm font-semibold text-ink-950">{workOrder.scheduledDate}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Est. Hours</span>
          </div>
          <div className="mt-2 text-sm font-semibold text-ink-950">{workOrder.estimatedHours}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Downtime</span>
          </div>
          <div className="mt-2 text-sm font-semibold text-ink-950">{workOrder.downtime}</div>
        </div>
      </div>

      {/* ── Main Grid ── */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.4fr_1fr]">
        {/* ── Left Column ── */}
        <div className="space-y-5">
          {/* Diagnosis */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center gap-2.5 mb-3">
              <FileText className="h-5 w-5 text-ink-400" />
              <h2 className="text-sm font-semibold text-ink-950">Diagnosis</h2>
            </div>
            <div className="rounded-lg bg-surface-2 p-3.5 text-sm text-ink-700">
              {workOrder.diagnosis}
            </div>
            <div className="mt-3 flex items-center gap-2 text-xs text-ink-500">
              <Calendar className="h-3.5 w-3.5" />
              Created: {new Date(workOrder.createdAt).toLocaleString()}
            </div>
          </div>

          {/* Tasks */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-2.5">
                <Clipboard className="h-5 w-5 text-ink-400" />
                <h2 className="text-sm font-semibold text-ink-950">Tasks</h2>
                <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                  {workOrder.tasks.length}
                </span>
              </div>
              <button className="flex items-center gap-1 text-xs font-medium text-primary transition-colors hover:text-primary-hover">
                <Plus className="h-3.5 w-3.5" />
                Add
              </button>
            </div>
            <div className="space-y-2.5">
              {workOrder.tasks.map((task) => {
                const taskStatus = TASK_STATUS_STYLES[task.status] || TASK_STATUS_STYLES.Pending;
                return (
                  <div key={task.id} className="flex items-center gap-3 rounded-lg border border-line p-3">
                    <div className={`rounded-full p-1 ${taskStatus.bg}`}>
                      {taskStatus.icon}
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-ink-950">{task.description}</span>
                        <span className={`text-[10px] font-semibold ${taskStatus.text}`}>
                          {task.status}
                        </span>
                      </div>
                      {task.assignedTo && (
                        <div className="text-xs text-ink-500">Assigned to: {task.assignedTo}</div>
                      )}
                    </div>
                    <button className="text-ink-300 hover:text-ink-600 transition-colors">
                      <MoreVertical className="h-4 w-4" />
                    </button>
                  </div>
                );
              })}
            </div>
          </div>

          {/* Parts Used */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-2.5">
                <Truck className="h-5 w-5 text-ink-400" />
                <h2 className="text-sm font-semibold text-ink-950">Parts Used</h2>
                <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                  {workOrder.partsUsed.length}
                </span>
              </div>
              <button
                onClick={goToParts}
                className="flex items-center gap-1 text-xs font-medium text-primary transition-colors hover:text-primary-hover"
              >
                View All <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
            <div className="space-y-2">
              {workOrder.partsUsed.map((part) => (
                <div key={part.id} className="flex items-center justify-between rounded-lg border border-line p-3">
                  <div>
                    <div className="text-sm font-medium text-ink-950">{part.name}</div>
                    <div className="text-xs text-ink-500">SKU: {part.id}</div>
                  </div>
                  <div className="text-right">
                    <div className="text-sm font-semibold text-ink-950">{part.quantity} {part.unit}</div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* ── Right Column ── */}
        <div className="space-y-5">
          {/* Asset Info */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center gap-2.5 mb-3">
              <Truck className="h-5 w-5 text-ink-400" />
              <h2 className="text-sm font-semibold text-ink-950">Asset</h2>
            </div>
            <div
              onClick={() => goToAsset(workOrder.assetId)}
              className="flex cursor-pointer items-center justify-between rounded-lg border border-line p-3 transition-colors hover:bg-surface-2"
            >
              <div>
                <div className="text-sm font-semibold text-ink-950">{workOrder.assetName}</div>
                <div className="text-xs text-ink-500">{workOrder.assetId} · {workOrder.assetType}</div>
              </div>
              <ArrowRight className="h-4 w-4 text-ink-400" />
            </div>
            <div className="mt-3 flex items-center gap-2 text-xs text-ink-500">
              <MapPin className="h-3.5 w-3.5" />
              {workOrder.branch}
            </div>
          </div>

          {/* Approvals */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center gap-2.5 mb-3">
              <History className="h-5 w-5 text-ink-400" />
              <h2 className="text-sm font-semibold text-ink-950">Approvals</h2>
            </div>
            <div className="space-y-2.5">
              {workOrder.approvals.map((approval) => (
                <div key={approval.id} className="flex items-center gap-3 rounded-lg border border-line p-3">
                  <div className="rounded-full bg-green-bg p-1">
                    <CheckCircle2 className="h-4 w-4 text-green" />
                  </div>
                  <div>
                    <div className="text-xs font-medium text-ink-950">{approval.label}</div>
                    <div className="text-[10px] text-ink-500">{approval.by} · {new Date(approval.timestamp).toLocaleString()}</div>
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
                onClick={goToReturnToService}
                className="flex w-full items-center justify-between rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                <span className="flex items-center gap-2">
                  <Shield className="h-4 w-4" />
                  Return to Service
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={() => goToAsset(workOrder.assetId)}
                className="flex w-full items-center justify-between rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                <span className="flex items-center gap-2">
                  <Truck className="h-4 w-4" />
                  View Asset
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={goToControlTower}
                className="flex w-full items-center justify-between rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                <span className="flex items-center gap-2">
                  <Clock className="h-4 w-4" />
                  Track Progress
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