// src/app/(dashboard)/fleet/tyres/[tyreId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Circle,
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
  Fuel,
  Wrench,
  Zap,
  Award,
  TrendingUp,
  TrendingDown,
  Circle as CircleIcon,
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
  List,
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

type TyreDetail = {
  id: string;
  assetId: string;
  assetName: string;
  assetType: string;
  position: "FL" | "FR" | "RL" | "RR" | "Spare";
  brand: string;
  size: string;
  condition: "New" | "Good" | "Worn" | "Critical" | "Replaced";
  installedDate: string;
  lastRotation: string;
  mileage: string;
  branch: string;
  status: "Active" | "Inactive";
  serialNumber: string;
  purchaseDate: string;
  warranty: string;
  notes: string;
  history: {
    id: string;
    event: string;
    date: string;
    performedBy: string;
    mileage: string;
  }[];
};

// Mock data
const TYRE_DETAIL: TyreDetail = {
  id: "TYR-001",
  assetId: "BR-204",
  assetName: "Honda ACE 110",
  assetType: "Motorcycle",
  position: "FL",
  brand: "Michelin",
  size: "120/80 R17",
  condition: "Worn",
  installedDate: "2026-03-10",
  lastRotation: "2026-07-15",
  mileage: "8,440 km",
  branch: "Ikeja",
  status: "Active",
  serialNumber: "MCH-2026-4432",
  purchaseDate: "2026-03-08",
  warranty: "24 months / 20,000 km",
  notes: "Front left tyre showing uneven wear. Alignment recommended.",
  history: [
    {
      id: "hist-001",
      event: "Tyre installed",
      date: "2026-03-10",
      performedBy: "T. Adeyemi",
      mileage: "0 km",
    },
    {
      id: "hist-002",
      event: "Tyre rotated",
      date: "2026-07-15",
      performedBy: "M. Bello",
      mileage: "5,200 km",
    },
    {
      id: "hist-003",
      event: "Tyre inspected",
      date: "2026-08-01",
      performedBy: "A. Yusuf",
      mileage: "8,100 km",
    },
    {
      id: "hist-004",
      event: "Wear noted - replace soon",
      date: "2026-08-17",
      performedBy: "F. Daniel",
      mileage: "8,440 km",
    },
  ],
};

const POSITION_LABELS: Record<string, string> = {
  FL: "Front Left",
  FR: "Front Right",
  RL: "Rear Left",
  RR: "Rear Right",
  Spare: "Spare",
};

const POSITION_COLORS: Record<string, string> = {
  FL: "bg-blue-500",
  FR: "bg-blue-400",
  RL: "bg-amber-500",
  RR: "bg-amber-400",
  Spare: "bg-ink-400",
};

const CONDITION_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  New: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  Good: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  Worn: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertCircle className="h-4 w-4" />,
  },
  Critical: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-4 w-4" />,
  },
  Replaced: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <XCircle className="h-4 w-4" />,
  },
};

export default function TyreDetailPage() {
  const params = useParams();
  const router = useRouter();
  const tyreId = params.tyreId as string;

  const [loading, setLoading] = useState(true);
  const [tyre, setTyre] = useState<TyreDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
      setTyre(TYRE_DETAIL);
      setLoading(false);
    };
    loadData();
  }, [tyreId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  // Navigation handlers
  const goBack = () => {
    router.push("/fleet/tyres");
  };

  const goToAsset = (assetId: string) => {
    router.push(`/fleet/assets/${assetId}`);
  };

  const goToEdit = () => {
    router.push(`/fleet/tyres/${tyreId}/edit`);
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading tyre details...</span>
        </div>
      </div>
    );
  }

  if (!tyre) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Circle className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Tyre not found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            The tyre you're looking for doesn't exist or has been removed.
          </p>
          <button
            onClick={goBack}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to tyres
          </button>
        </div>
      </div>
    );
  }

  const conditionStyle = CONDITION_STYLES[tyre.condition] || CONDITION_STYLES.Good;
  const isCritical = tyre.condition === "Critical";
  const isWorn = tyre.condition === "Worn";

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
                  {tyre.id}
                </h1>
                <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${conditionStyle.bg} ${conditionStyle.text}`}>
                  {conditionStyle.icon}
                  {tyre.condition}
                </span>
              </div>
              <p className="mt-0.5 text-[13.5px] text-ink-600">
                {tyre.assetName} · {tyre.brand} {tyre.size}
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
        </div>
      </div>

      {/* ===== STATS ROW ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className={`h-3 w-3 rounded-full ${POSITION_COLORS[tyre.position]}`} />
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Position</div>
              <div className="text-[16px] font-semibold text-ink-950">
                {tyre.position} ({POSITION_LABELS[tyre.position]})
              </div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-blue-bg p-1.5">
              <Gauge className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Mileage</div>
              <div className="text-[16px] font-semibold text-ink-950">{tyre.mileage}</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-green-bg p-1.5">
              <Calendar className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Installed</div>
              <div className="text-[16px] font-semibold text-ink-950">{tyre.installedDate}</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-amber-bg p-1.5">
              <RefreshCw className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Last Rotation</div>
              <div className="text-[16px] font-semibold text-ink-950">{tyre.lastRotation}</div>
            </div>
          </div>
        </div>
      </div>

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.4fr_1fr]">
        {/* LEFT COLUMN */}
        <div className="space-y-5">
          {/* Tyre Details */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <FileText className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Tyre Details</h3>
            </div>
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Brand</div>
                  <div className="mt-0.5 text-[13px] text-ink-950">{tyre.brand}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Size</div>
                  <div className="mt-0.5 text-[13px] text-ink-950">{tyre.size}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Serial Number</div>
                  <div className="mt-0.5 font-mono text-[13px] text-ink-950">{tyre.serialNumber}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Purchase Date</div>
                  <div className="mt-0.5 text-[13px] text-ink-950">{tyre.purchaseDate}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Warranty</div>
                  <div className="mt-0.5 text-[13px] text-ink-950">{tyre.warranty}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</div>
                  <div className="mt-0.5 flex items-center gap-1.5 text-[13px] text-ink-950">
                    <span className={`h-2 w-2 rounded-full ${tyre.status === "Active" ? "bg-green" : "bg-ink-400"}`} />
                    {tyre.status}
                  </div>
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
              onClick={() => goToAsset(tyre.assetId)}
              className="flex cursor-pointer items-center justify-between rounded-xl border border-line p-3 transition hover:bg-surface-2"
            >
              <div>
                <div className="text-[14px] font-semibold text-ink-950">{tyre.assetName}</div>
                <div className="text-[12px] text-ink-500">{tyre.assetId} · {tyre.assetType}</div>
              </div>
              <ArrowRight className="h-4 w-4 text-ink-400" />
            </div>
            <div className="mt-3 flex items-center gap-2 text-[12px] text-ink-500">
              <MapPin className="h-3.5 w-3.5" />
              {tyre.branch}
            </div>
          </div>

          {/* Notes */}
          {tyre.notes && (
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <div className="flex items-center gap-2.5 mb-2">
                <FileText className="h-5 w-5 text-ink-400" />
                <h3 className="text-[15px] font-semibold text-ink-950">Notes</h3>
              </div>
              <p className="text-[13px] text-ink-700">{tyre.notes}</p>
            </div>
          )}
        </div>

        {/* RIGHT COLUMN */}
        <div className="space-y-5">
          {/* Condition Status Card */}
          <div className={`rounded-2xl border p-5 shadow-sm ${
            isCritical
              ? "border-red/30 bg-red-bg/20"
              : isWorn
              ? "border-amber/30 bg-amber-bg/20"
              : "border-green/30 bg-green-bg/20"
          }`}>
            <div className="flex items-center gap-2.5 mb-3">
              <Shield className={`h-5 w-5 ${isCritical ? "text-red" : isWorn ? "text-amber" : "text-green"}`} />
              <h3 className="text-[15px] font-semibold text-ink-950">Condition Status</h3>
            </div>
            <div className="space-y-2">
              <div className="flex items-center justify-between text-[13px]">
                <span className="text-ink-500">Current Condition</span>
                <span className={`font-semibold ${conditionStyle.text}`}>{tyre.condition}</span>
              </div>
              <div className="flex items-center justify-between text-[13px] border-t border-line pt-2">
                <span className="text-ink-500">Mileage Since New</span>
                <span className="font-medium text-ink-950">{tyre.mileage}</span>
              </div>
              <div className="flex items-center justify-between text-[13px] border-t border-line pt-2">
                <span className="text-ink-500">Age</span>
                <span className="font-medium text-ink-950">
                  {Math.floor((new Date().getTime() - new Date(tyre.installedDate).getTime()) / (1000 * 60 * 60 * 24 * 30))} months
                </span>
              </div>
            </div>
          </div>

          {/* History Timeline */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <History className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Service History</h3>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                {tyre.history.length}
              </span>
            </div>
            <div className="space-y-3">
              {tyre.history.map((event, index) => (
                <div key={event.id} className="flex items-start gap-3">
                  <div className="flex flex-col items-center">
                    <div className={`h-3 w-3 rounded-full ${
                      index === 0 ? "bg-amber" : "bg-blue"
                    }`} />
                    {index < tyre.history.length - 1 && (
                      <div className="h-6 w-px bg-line" />
                    )}
                  </div>
                  <div>
                    <div className="text-[13px] font-medium text-ink-950">{event.event}</div>
                    <div className="text-[12px] text-ink-500">
                      {event.performedBy} · {event.date} · {event.mileage}
                    </div>
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
                onClick={() => goToAsset(tyre.assetId)}
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
                  Edit Tyre
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={() => router.push("/fleet/tyres")}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <List className="h-4 w-4" />
                  All Tyres
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
          Back to tyres
        </button>
        <button
          onClick={() => goToAsset(tyre.assetId)}
          className="flex items-center gap-1.5 text-[12px] font-medium text-primary hover:text-primary-hover transition-colors"
        >
          View Asset
          <ArrowRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}