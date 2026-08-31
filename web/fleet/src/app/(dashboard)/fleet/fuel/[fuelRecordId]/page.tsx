// src/app/(dashboard)/fleet/fuel/[fuelRecordId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Fuel,
  MapPin,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Edit3,
  RefreshCw,
  User,
  Calendar,
  Activity,
  Shield,
  FileText,
  Briefcase,
  Users,
  ArrowRight,
  MoreVertical,
  Gauge,
  DollarSign,
  Wrench,
  Zap,
  Award,
  TrendingUp,
  TrendingDown,
  Circle,
  Radar,
  Sparkles,
  Battery,
  Car,
  Bike,
  Navigation,
  Copy,
  Download,
  Printer,
  Clipboard,
  History,
  Box,
  AlertTriangle,
  ShoppingCart,
  Trash2,
  Plus,
  Send,
  BarChart3,
  Truck,
  Droplets,
  Flame,
  Receipt,
  Check,
  X,
} from "lucide-react";

type FuelRecordDetail = {
  id: string;
  assetId: string;
  assetName: string;
  assetType: string;
  date: string;
  fuelType: "Petrol" | "Diesel" | "Electric";
  quantity: number;
  unit: "Liters" | "Gallons" | "kWh";
  cost: string;
  odometer: string;
  station: string;
  location: string;
  performedBy: string;
  status: "Pending" | "Approved" | "Rejected" | "Reconciling";
  receipt: string;
  notes: string;
  createdAt: string;
  updatedAt: string;
  approvals: {
    id: string;
    label: string;
    by: string;
    timestamp: string;
  }[];
};

// Mock data
const FUEL_RECORD_DETAIL: FuelRecordDetail = {
  id: "FUEL-001",
  assetId: "VN-041",
  assetName: "Mercedes Sprinter",
  assetType: "Van",
  date: "2026-08-17",
  fuelType: "Diesel",
  quantity: 45,
  unit: "Liters",
  cost: "₦42,300",
  odometer: "124,890 km",
  station: "TotalEnergies - Lekki",
  location: "Lekki",
  performedBy: "Musa Bello",
  status: "Approved",
  receipt: "REC-001",
  notes: "Full tank. Regular diesel. Requested by dispatch.",
  createdAt: "2026-08-17T10:30:00Z",
  updatedAt: "2026-08-17T14:15:00Z",
  approvals: [
    { id: "app-1", label: "Record submitted", by: "Musa Bello", timestamp: "2026-08-17T10:30:00Z" },
    { id: "app-2", label: "Approved by fleet manager", by: "F. Daniel", timestamp: "2026-08-17T14:15:00Z" },
  ],
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Approved: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-4 w-4" />,
  },
  Rejected: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-4 w-4" />,
  },
  Reconciling: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <AlertCircle className="h-4 w-4" />,
  },
};

const FUEL_TYPE_ICONS: Record<string, React.ReactNode> = {
  Petrol: <Flame className="h-4 w-4" />,
  Diesel: <Droplets className="h-4 w-4" />,
  Electric: <Zap className="h-4 w-4" />,
};

export default function FuelRecordDetailPage() {
  const params = useParams();
  const router = useRouter();
  const fuelRecordId = params.fuelRecordId as string;

  const [loading, setLoading] = useState(true);
  const [record, setRecord] = useState<FuelRecordDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [approving, setApproving] = useState(false);
  const [rejecting, setRejecting] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
      setRecord(FUEL_RECORD_DETAIL);
      setLoading(false);
    };
    loadData();
  }, [fuelRecordId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  // Navigation handlers
  const goBack = () => {
    router.push("/fleet/fuel");
  };

  const goToAsset = (assetId: string) => {
    router.push(`/fleet/assets/${assetId}`);
  };

  const goToEdit = () => {
    router.push(`/fleet/fuel/${fuelRecordId}/edit`);
  };

  const handleApprove = async () => {
    setApproving(true);
    await new Promise((resolve) => setTimeout(resolve, 1200));
    setApproving(false);
    router.push("/fleet/fuel");
  };

  const handleReject = async () => {
    setRejecting(true);
    await new Promise((resolve) => setTimeout(resolve, 1200));
    setRejecting(false);
    router.push("/fleet/fuel");
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading fuel record...</span>
        </div>
      </div>
    );
  }

  if (!record) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Fuel className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Fuel record not found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            The fuel record you're looking for doesn't exist or has been removed.
          </p>
          <button
            onClick={goBack}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to fuel records
          </button>
        </div>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[record.status] || STATUS_STYLES.Pending;
  const FuelIcon = FUEL_TYPE_ICONS[record.fuelType] || <Fuel className="h-4 w-4" />;
  const isPending = record.status === "Pending";
  const isApproved = record.status === "Approved";

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <button
              onClick={goBack}
              className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-white/70 text-ink-600 transition hover:bg-white hover:text-ink-900"
            >
              <ArrowLeft className="h-4 w-4" />
            </button>
            <div>
              <div className="flex items-center gap-2.5">
                <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                  {record.id}
                </h1>
                <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                  {statusStyle.icon}
                  {record.status}
                </span>
              </div>
              <p className="mt-0.5 text-[13.5px] text-ink-600">
                {record.assetName} · {record.location}
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
          <button
            onClick={goToEdit}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
          {isPending && (
            <>
              <button
                onClick={handleApprove}
                disabled={approving}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-green px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-green/20 transition hover:bg-green/90 disabled:opacity-60"
              >
                {approving ? (
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                ) : (
                  <Check className="h-3.5 w-3.5" />
                )}
                Approve
              </button>
              <button
                onClick={handleReject}
                disabled={rejecting}
                className="flex h-9 items-center gap-1.5 rounded-xl bg-red px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-red/20 transition hover:bg-red/90 disabled:opacity-60"
              >
                {rejecting ? (
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                ) : (
                  <X className="h-3.5 w-3.5" />
                )}
                Reject
              </button>
            </>
          )}
        </div>
      </div>

      {/* ===== STATS ROW ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-primary-bg p-1.5">
              <DollarSign className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Cost</div>
              <div className="text-[16px] font-semibold text-ink-950">{record.cost}</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-blue-bg p-1.5">
              <Droplets className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Quantity</div>
              <div className="text-[16px] font-semibold text-ink-950">{record.quantity} {record.unit}</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-amber-bg p-1.5">
              <Gauge className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Odometer</div>
              <div className="text-[16px] font-semibold text-ink-950">{record.odometer}</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-green-bg p-1.5">
              <Calendar className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Date</div>
              <div className="text-[16px] font-semibold text-ink-950">{record.date}</div>
            </div>
          </div>
        </div>
      </div>

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.4fr_1fr]">
        {/* LEFT COLUMN */}
        <div className="space-y-5">
          {/* Record Details */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <FileText className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Record Details</h3>
            </div>
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Fuel Type</div>
                  <div className="mt-0.5 flex items-center gap-1.5 text-[13px] text-ink-950">
                    {FuelIcon}
                    {record.fuelType}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Station</div>
                  <div className="mt-0.5 text-[13px] text-ink-950">{record.station}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Performed By</div>
                  <div className="mt-0.5 flex items-center gap-1.5 text-[13px] text-ink-950">
                    <User className="h-4 w-4 text-ink-400" />
                    {record.performedBy}
                  </div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Receipt</div>
                  <div className="mt-0.5 flex items-center gap-1.5 text-[13px] text-ink-950">
                    <Receipt className="h-4 w-4 text-ink-400" />
                    {record.receipt}
                  </div>
                </div>
                <div className="col-span-2">
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Notes</div>
                  <div className="mt-0.5 text-[13px] text-ink-700">{record.notes || "No notes"}</div>
                </div>
              </div>
            </div>
          </div>

          {/* Asset Info */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <Truck className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Asset</h3>
            </div>
            <div
              onClick={() => goToAsset(record.assetId)}
              className="flex cursor-pointer items-center justify-between rounded-xl border border-line p-3 transition hover:bg-surface-2"
            >
              <div>
                <div className="text-[14px] font-semibold text-ink-950">{record.assetName}</div>
                <div className="text-[12px] text-ink-500">{record.assetId} · {record.assetType}</div>
              </div>
              <ArrowRight className="h-4 w-4 text-ink-400" />
            </div>
            <div className="mt-3 flex items-center gap-2 text-[12px] text-ink-500">
              <MapPin className="h-3.5 w-3.5" />
              {record.location}
            </div>
          </div>
        </div>

        {/* RIGHT COLUMN */}
        <div className="space-y-5">
          {/* Approvals Timeline */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <History className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Approval History</h3>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                {record.approvals.length}
              </span>
            </div>
            <div className="space-y-3">
              {record.approvals.map((approval, index) => (
                <div key={approval.id} className="flex items-start gap-3">
                  <div className="flex flex-col items-center">
                    <div className={`h-3 w-3 rounded-full ${
                      index === 0 ? "bg-amber" : "bg-green"
                    }`} />
                    {index < record.approvals.length - 1 && (
                      <div className="h-6 w-px bg-line" />
                    )}
                  </div>
                  <div>
                    <div className="text-[13px] font-medium text-ink-950">{approval.label}</div>
                    <div className="text-[12px] text-ink-500">{approval.by} · {new Date(approval.timestamp).toLocaleString()}</div>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Quick Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Quick Actions</h3>
            <div className="space-y-2">
              <button
                onClick={() => goToAsset(record.assetId)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Truck className="h-4 w-4" />
                  View Asset
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={goToEdit}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Edit3 className="h-4 w-4" />
                  Edit Record
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={() => router.push("/fleet/fuel")}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <BarChart3 className="h-4 w-4" />
                  All Fuel Records
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* ===== NAVIGATION FOOTER ===== */}
      <div className="flex flex-col gap-2 border-t border-line pt-4 sm:flex-row sm:justify-between">
        <button
          onClick={goBack}
          className="flex items-center gap-1.5 text-[12px] font-medium text-ink-500 hover:text-ink-900 transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to fuel records
        </button>
        <button
          onClick={() => goToAsset(record.assetId)}
          className="flex items-center gap-1.5 text-[12px] font-medium text-primary hover:text-primary-hover transition-colors"
        >
          View Asset
          <ArrowRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}