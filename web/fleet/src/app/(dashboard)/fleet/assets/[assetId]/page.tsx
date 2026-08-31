// src/app/(dashboard)/fleet/assets/[assetId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Truck,
  MapPin,
  Clock,
  CheckCircle2,
  AlertCircle,
  Edit3,
  User,
  Calendar,
  Shield,
  FileText,
  Users,
  ArrowRight,
  Gauge,
  Fuel,
  Wrench,
  Zap,
  Car,
  Bike,
  Clipboard,
  Package,
  AlertTriangle,
  Circle,
} from "lucide-react";

type AssetDetail = {
  id: string;
  name: string;
  type: "Motorcycle" | "Van" | "Truck" | "Car";
  branch: string;
  status: "In Service" | "Maintenance" | "Reserved" | "Safety Blocked" | "Workshop";
  compliance: "Compliant" | "Review Due" | "Failed" | "Blocked";
  assignedTo: string | null;
  odometer: string;
  lastInspection: string;
  nextService: string;
  ytdCost: string;
  fuelEfficiency: string;
  vin: string;
  plateNumber: string;
  year: string;
  color: string;
  engine: string;
  lastLocation: string;
  lastUpdated: string;
  description?: string;
};

const ASSET_DETAIL: AssetDetail = {
  id: "BR-204",
  name: "Honda ACE 110",
  type: "Motorcycle",
  branch: "Ikeja",
  status: "Safety Blocked",
  compliance: "Failed",
  assignedTo: null,
  odometer: "38,442 km",
  lastInspection: "Today · Failed",
  nextService: "Overdue 280 km",
  ytdCost: "₦48,200",
  fuelEfficiency: "42 km/L",
  vin: "JH2SDH3A1KK123456",
  plateNumber: "LAG-204-GG",
  year: "2023",
  color: "Red",
  engine: "110cc, 8.5 HP",
  lastLocation: "Ikeja Depot",
  lastUpdated: "12 sec ago",
  description: "Honda ACE 110 is a reliable commuter motorcycle used for last-mile delivery. Currently undergoing safety inspection due to brake failure report.",
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  "In Service": { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Maintenance: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Reserved: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  "Safety Blocked": { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Workshop: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const COMPLIANCE_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Compliant: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  "Review Due": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Failed: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Blocked: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const TYPE_ICONS = {
  Motorcycle: <Bike className="h-4 w-4" />,
  Van: <Car className="h-4 w-4" />,
  Truck: <Truck className="h-4 w-4" />,
  Car: <Car className="h-4 w-4" />,
};

export default function AssetDetailPage() {
  const params = useParams();
  const router = useRouter();
  const assetId = params.assetId as string;

  const [loading, setLoading] = useState(true);
  const [asset, setAsset] = useState<AssetDetail>(ASSET_DETAIL);

  // ── Navigation Handlers ──
  const goToAssignments = () => {
    router.push(`/fleet/assets/${assetId}/assignments`);
  };

  const goToCompliance = () => {
    router.push(`/fleet/assets/${assetId}/compliance`);
  };

  const goToDocuments = () => {
    router.push(`/fleet/assets/${assetId}/documents`);
  };

  const goToWorkOrders = () => {
    router.push("/fleet/work-orders");
  };

  const goToParts = () => {
    router.push("/fleet/parts");
  };

  const goToFuel = () => {
    router.push("/fleet/fuel");
  };

  const goToTyres = () => {
    router.push("/fleet/tyres");
  };

  const goToBreakdowns = () => {
    router.push("/fleet/breakdowns");
  };

  const goToInspections = () => {
    router.push("/fleet/inspections");
  };

  const goToMaintenance = () => {
    router.push("/fleet/maintenance/schedules");
  };

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
      setAsset(ASSET_DETAIL);
      setLoading(false);
    };
    loadData();
  }, [assetId]);

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading asset...</span>
        </div>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[asset.status] || STATUS_STYLES["In Service"];
  const complianceStyle = COMPLIANCE_STYLES[asset.compliance] || COMPLIANCE_STYLES.Compliant;
  const TypeIcon = TYPE_ICONS[asset.type] || <Truck className="h-4 w-4" />;
  const isBlocked = asset.status === "Safety Blocked" || asset.status === "Workshop";
  const isInService = asset.status === "In Service";

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2.5 flex-wrap">
            <h1>{asset.id}</h1>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
              {asset.status}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${complianceStyle.bg} ${complianceStyle.text}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${complianceStyle.dot}`} />
              {asset.compliance}
            </span>
          </div>
          <p className="mt-0.5 text-sm text-ink-500">
            {asset.name} · {asset.branch} branch
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
        </div>
      </div>

      {/* ── Main Grid ── */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.4fr_1fr]">
        {/* ── Left Column ── */}
        <div className="space-y-5">
          {/* Hero */}
          <div className={`rounded-lg border p-5 ${
            isBlocked ? "border-red/30 bg-red-bg/10" :
            isInService ? "border-green/30 bg-green-bg/10" :
            "border-line bg-white"
          }`}>
            <div className="flex items-center gap-4">
              <div className={`flex h-14 w-14 items-center justify-center rounded-lg ${
                isBlocked ? "bg-red-bg text-red" : isInService ? "bg-green-bg text-green" : "bg-primary-bg text-primary"
              }`}>
                {TypeIcon}
              </div>
              <div>
                <div className="text-base font-semibold text-ink-950">{asset.name}</div>
                <div className="flex flex-wrap items-center gap-2 text-sm text-ink-500">
                  <span>{asset.type}</span>
                  <span className="text-ink-300">·</span>
                  <span>{asset.year}</span>
                  <span className="text-ink-300">·</span>
                  <span className="font-mono">{asset.plateNumber}</span>
                </div>
              </div>
              <div className="ml-auto text-right">
                <div className="text-[10px] text-ink-400">Last updated</div>
                <div className="text-xs font-medium text-ink-600">{asset.lastUpdated}</div>
              </div>
            </div>
          </div>

          {/* Specs */}
          <div className="grid grid-cols-2 gap-3.5">
            <div className="rounded-lg border border-line bg-white p-4">
              <div className="flex items-center gap-2">
                <Gauge className="h-4 w-4 text-ink-400" />
                <span className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Odometer</span>
              </div>
              <div className="mt-1 text-base font-semibold text-ink-950">{asset.odometer}</div>
            </div>
            <div className="rounded-lg border border-line bg-white p-4">
              <div className="flex items-center gap-2">
                <Fuel className="h-4 w-4 text-ink-400" />
                <span className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Fuel Efficiency</span>
              </div>
              <div className="mt-1 text-base font-semibold text-ink-950">{asset.fuelEfficiency}</div>
            </div>
            <div className="rounded-lg border border-line bg-white p-4">
              <div className="flex items-center gap-2">
                <Calendar className="h-4 w-4 text-ink-400" />
                <span className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Next Service</span>
              </div>
              <div className="mt-1 text-base font-semibold text-ink-950">{asset.nextService}</div>
            </div>
            <div className="rounded-lg border border-line bg-white p-4">
              <div className="flex items-center gap-2">
                <Zap className="h-4 w-4 text-ink-400" />
                <span className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">YTD Cost</span>
              </div>
              <div className="mt-1 text-base font-semibold text-ink-950">{asset.ytdCost}</div>
            </div>
          </div>

          {/* Details */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="flex items-center gap-2.5 mb-3">
              <FileText className="h-4 w-4 text-ink-400" />
              <span className="text-sm font-semibold text-ink-950">Asset Details</span>
            </div>
            <div className="grid grid-cols-2 gap-3 text-sm">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">VIN</div>
                <div className="mt-0.5 font-mono text-xs text-ink-950">{asset.vin}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Plate Number</div>
                <div className="mt-0.5 font-mono text-xs text-ink-950">{asset.plateNumber}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Color</div>
                <div className="mt-0.5 text-xs text-ink-950">{asset.color}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Engine</div>
                <div className="mt-0.5 text-xs text-ink-950">{asset.engine}</div>
              </div>
            </div>
          </div>

          {asset.description && (
            <div className="rounded-lg border border-line bg-white p-4">
              <p className="text-sm text-ink-700">{asset.description}</p>
            </div>
          )}
        </div>

        {/* ── Right Column ── */}
        <div className="space-y-5">
          {/* Safety & Compliance */}
          <div className={`rounded-lg border p-4 ${
            isBlocked ? "border-red/30 bg-red-bg/10" : isInService ? "border-green/30 bg-green-bg/10" : "border-line bg-white"
          }`}>
            <div className="flex items-center gap-2.5 mb-3">
              <Shield className={`h-4 w-4 ${isBlocked ? "text-red" : isInService ? "text-green" : "text-ink-400"}`} />
              <span className="text-sm font-semibold text-ink-950">Safety & Compliance</span>
            </div>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Status</span>
                <span className={`font-semibold ${statusStyle.text}`}>{asset.status}</span>
              </div>
              <div className="flex items-center justify-between border-t border-line pt-2">
                <span className="text-ink-500">Compliance</span>
                <span className={`font-semibold ${complianceStyle.text}`}>{asset.compliance}</span>
              </div>
              <div className="flex items-center justify-between border-t border-line pt-2">
                <span className="text-ink-500">Last Inspection</span>
                <span className="font-medium text-ink-950">{asset.lastInspection}</span>
              </div>
              <div className="flex items-center justify-between border-t border-line pt-2">
                <span className="text-ink-500">Location</span>
                <span className="font-medium text-ink-950">{asset.lastLocation}</span>
              </div>
            </div>
          </div>

          {/* Assignment */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="flex items-center gap-2.5 mb-3">
              <Users className="h-4 w-4 text-ink-400" />
              <span className="text-sm font-semibold text-ink-950">Assignment</span>
            </div>
            {asset.assignedTo ? (
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary-bg text-sm font-bold text-primary">
                  {asset.assignedTo.split(" ").map((n) => n[0]).join("")}
                </div>
                <div>
                  <div className="text-sm font-semibold text-ink-950">{asset.assignedTo}</div>
                  <div className="text-xs text-ink-500">Assigned provider</div>
                </div>
              </div>
            ) : (
              <div className="flex flex-col items-center gap-2 py-3 text-center">
                <div className="rounded-full bg-surface-2 p-3">
                  <User className="h-5 w-5 text-ink-400" />
                </div>
                <span className="text-sm text-ink-500">No provider assigned</span>
                <button className="text-xs font-semibold text-primary transition-colors hover:text-primary-hover">
                  Assign provider →
                </button>
              </div>
            )}
          </div>

          {/* Quick Actions */}
          <div className="rounded-lg border border-line bg-white p-4">
            <span className="text-sm font-semibold text-ink-950 mb-3 block">Quick Actions</span>
            <div className="space-y-3">
              {/* Main actions */}
              <div className="space-y-2">
                <button
                  onClick={goToAssignments}
                  className="flex w-full items-center justify-between rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
                >
                  <span className="flex items-center gap-2">
                    <Clipboard className="h-4 w-4" />
                    View Assignments
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
                <button
                  onClick={goToCompliance}
                  className="flex w-full items-center justify-between rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
                >
                  <span className="flex items-center gap-2">
                    <Shield className="h-4 w-4" />
                    Compliance Status
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
                <button
                  onClick={goToDocuments}
                  className="flex w-full items-center justify-between rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
                >
                  <span className="flex items-center gap-2">
                    <FileText className="h-4 w-4" />
                    Documents
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              </div>

              {/* Divider */}
              <div className="border-t border-line" />

              {/* Fleet module navigation */}
              <div className="grid grid-cols-2 gap-2">
                <button
                  onClick={goToWorkOrders}
                  className="flex items-center justify-center gap-1.5 rounded-md border border-line bg-white px-3 py-2 text-xs font-medium text-ink-600 transition-colors hover:bg-surface-2"
                >
                  <Wrench className="h-3.5 w-3.5" />
                  Work Orders
                </button>
                <button
                  onClick={goToParts}
                  className="flex items-center justify-center gap-1.5 rounded-md border border-line bg-white px-3 py-2 text-xs font-medium text-ink-600 transition-colors hover:bg-surface-2"
                >
                  <Package className="h-3.5 w-3.5" />
                  Parts
                </button>
                <button
                  onClick={goToFuel}
                  className="flex items-center justify-center gap-1.5 rounded-md border border-line bg-white px-3 py-2 text-xs font-medium text-ink-600 transition-colors hover:bg-surface-2"
                >
                  <Fuel className="h-3.5 w-3.5" />
                  Fuel
                </button>
                <button
                  onClick={goToTyres}
                  className="flex items-center justify-center gap-1.5 rounded-md border border-line bg-white px-3 py-2 text-xs font-medium text-ink-600 transition-colors hover:bg-surface-2"
                >
                  <Circle className="h-3.5 w-3.5" />
                  Tyres
                </button>
                <button
                  onClick={goToBreakdowns}
                  className="flex items-center justify-center gap-1.5 rounded-md border border-line bg-white px-3 py-2 text-xs font-medium text-ink-600 transition-colors hover:bg-surface-2"
                >
                  <AlertTriangle className="h-3.5 w-3.5" />
                  Breakdowns
                </button>
                <button
                  onClick={goToInspections}
                  className="flex items-center justify-center gap-1.5 rounded-md border border-line bg-white px-3 py-2 text-xs font-medium text-ink-600 transition-colors hover:bg-surface-2"
                >
                  <Clipboard className="h-3.5 w-3.5" />
                  Inspections
                </button>
                <button
                  onClick={goToMaintenance}
                  className="flex items-center justify-center gap-1.5 rounded-md border border-line bg-white px-3 py-2 text-xs font-medium text-ink-600 transition-colors hover:bg-surface-2"
                >
                  <Calendar className="h-3.5 w-3.5" />
                  Maintenance
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}