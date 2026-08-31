// src/app/(dashboard)/fleet/assets/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Search,
  Plus,
  Truck,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  Activity,
  Shield,
  Calendar,
  Wrench,
  Gauge,
  Car,
  Bike,
  Users,
  TrendingUp,
} from "lucide-react";

type Asset = {
  id: string;
  name: string;
  type: "Motorcycle" | "Van" | "Truck" | "Car";
  branch: string;
  status: "In Service" | "Maintenance" | "Reserved" | "Safety Blocked" | "Workshop";
  compliance: "Compliant" | "Review Due" | "Failed" | "Blocked";
  assignedTo: string | null;
  odometer: string;
  nextService: string;
};

const ASSETS: Asset[] = [
  {
    id: "BR-204",
    name: "Honda ACE 110",
    type: "Motorcycle",
    branch: "Ikeja",
    status: "Safety Blocked",
    compliance: "Failed",
    assignedTo: null,
    odometer: "38,442 km",
    nextService: "Overdue 280 km",
  },
  {
    id: "VN-041",
    name: "Mercedes Sprinter",
    type: "Van",
    branch: "Lekki",
    status: "In Service",
    compliance: "Compliant",
    assignedTo: "Musa Bello",
    odometer: "124,890 km",
    nextService: "Due in 9 days",
  },
  {
    id: "TR-112",
    name: "Scania R500",
    type: "Truck",
    branch: "Apapa",
    status: "Reserved",
    compliance: "Review Due",
    assignedTo: "Kabiru Ahmed",
    odometer: "312,440 km",
    nextService: "Due in 430 km",
  },
  {
    id: "BR-188",
    name: "Yamaha FZ",
    type: "Motorcycle",
    branch: "Yaba",
    status: "In Service",
    compliance: "Compliant",
    assignedTo: "Amina Yusuf",
    odometer: "22,180 km",
    nextService: "Due in 21 days",
  },
  {
    id: "VN-029",
    name: "Ford Transit",
    type: "Van",
    branch: "Ikeja",
    status: "Workshop",
    compliance: "Blocked",
    assignedTo: null,
    odometer: "87,340 km",
    nextService: "Overdue 120 km",
  },
  {
    id: "BR-171",
    name: "Honda CB150X",
    type: "Motorcycle",
    branch: "Lekki",
    status: "In Service",
    compliance: "Compliant",
    assignedTo: "Chinedu Uche",
    odometer: "15,920 km",
    nextService: "Due in 15 days",
  },
  {
    id: "TR-098",
    name: "MAN TGS",
    type: "Truck",
    branch: "Ikeja",
    status: "Maintenance",
    compliance: "Review Due",
    assignedTo: null,
    odometer: "456,720 km",
    nextService: "Due in 3 days",
  },
  {
    id: "BR-135",
    name: "Suzuki GSX-S150",
    type: "Motorcycle",
    branch: "Yaba",
    status: "In Service",
    compliance: "Compliant",
    assignedTo: "Esther Obi",
    odometer: "8,450 km",
    nextService: "Due in 45 days",
  },
];

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

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "In Service", "Maintenance", "Reserved", "Safety Blocked", "Workshop"];
const TYPE_FILTERS = ["All", "Motorcycle", "Van", "Truck", "Car"];

export default function AssetRegisterPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedType, setSelectedType] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Asset[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(ASSETS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      (item.assignedTo && item.assignedTo.toLowerCase().includes(search.toLowerCase()));

    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesType = selectedType === "All" || item.type === selectedType;

    return matchesSearch && matchesBranch && matchesStatus && matchesType;
  });

  const inServiceCount = data.filter((d) => d.status === "In Service").length;
  const blockedCount = data.filter((d) => d.status === "Safety Blocked" || d.status === "Workshop").length;
  const maintenanceCount = data.filter((d) => d.status === "Maintenance").length;

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedStatus !== "All" ||
    selectedType !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedStatus("All");
    setSelectedType("All");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading assets...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Fleet Assets</h1>
          <p className="mt-1.5">
            Operational fleet assets with assignment, safety/compliance state, and maintenance status.
          </p>
        </div>
        <button
          onClick={() => router.push("/fleet/assets/create")}
          className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          Add asset
        </button>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <Activity className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">In Service</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{inServiceCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Wrench className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Maintenance</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{maintenanceCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <Shield className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Blocked</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{blockedCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Gauge className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Fleet Health</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">86%</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search assets..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Status:</span>
          {selectedStatus}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Type:</span>
          {selectedType}
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedBranch !== "All branches" ? 1 : 0) +
                (selectedStatus !== "All" ? 1 : 0) +
                (selectedType !== "All" ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filteredData.length} assets
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</span>
              <div className="flex flex-wrap gap-1.5">
                {STATUS_FILTERS.map((status) => (
                  <button
                    key={status}
                    onClick={() => setSelectedStatus(status)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedStatus === status
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {status}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Type</span>
              <div className="flex flex-wrap gap-1.5">
                {TYPE_FILTERS.map((type) => (
                  <button
                    key={type}
                    onClick={() => setSelectedType(type)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedType === type
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {type}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ── Asset Cards ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Truck className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No assets found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No assets match your current filters. Add a new asset to get started.
          </p>
          <button
            onClick={() => router.push("/fleet/assets/create")}
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            <Plus className="inline h-3.5 w-3.5 mr-1" />
            Add asset
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES["In Service"];
            const complianceStyle = COMPLIANCE_STYLES[item.compliance] || COMPLIANCE_STYLES.Compliant;
            const TypeIcon = TYPE_ICONS[item.type] || <Truck className="h-4 w-4" />;
            const isBlocked = item.status === "Safety Blocked" || item.status === "Workshop";
            const isInService = item.status === "In Service";
            const isMaintenance = item.status === "Maintenance";

            return (
              <div
                key={item.id}
                onClick={() => router.push(`/fleet/assets/${item.id}`)}
                className={`group rounded-lg border p-4 transition-colors cursor-pointer ${
                  isBlocked
                    ? "border-red/30 bg-red-bg/10 hover:border-red/50 hover:bg-red-bg/20"
                    : isMaintenance
                    ? "border-amber/30 bg-amber-bg/10 hover:border-amber/50 hover:bg-amber-bg/20"
                    : isInService
                    ? "border-green/20 bg-white hover:border-green/40 hover:bg-surface-2"
                    : "border-line bg-white hover:border-ink-300 hover:bg-surface-2"
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2.5">
                    <div
                      className={`flex h-10 w-10 items-center justify-center rounded-md ${
                        isBlocked
                          ? "bg-red-bg text-red"
                          : isMaintenance
                          ? "bg-amber-bg text-amber"
                          : isInService
                          ? "bg-green-bg text-green"
                          : "bg-primary-bg text-primary"
                      }`}
                    >
                      {TypeIcon}
                    </div>
                    <div>
                      <div className="flex items-center gap-1.5">
                        <span className="text-sm font-semibold text-ink-950">{item.id}</span>
                        <span className="font-mono text-[10px] text-ink-400">{item.type}</span>
                      </div>
                      <div className="text-xs text-ink-500">{item.name}</div>
                    </div>
                  </div>
                  <div className="flex flex-col items-end gap-0.5">
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                    >
                      <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                      {item.status}
                    </span>
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-medium ${complianceStyle.bg} ${complianceStyle.text}`}
                    >
                      <span className={`h-1 w-1 rounded-full ${complianceStyle.dot}`} />
                      {item.compliance}
                    </span>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.branch}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Gauge className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.odometer}</span>
                  </div>
                  {item.assignedTo ? (
                    <div className="flex items-center gap-1.5 text-xs text-ink-500 col-span-2">
                      <Users className="h-3.5 w-3.5 flex-shrink-0" />
                      <span className="truncate">{item.assignedTo}</span>
                    </div>
                  ) : (
                    <div className="flex items-center gap-1.5 text-xs text-ink-400 col-span-2">
                      <Users className="h-3.5 w-3.5 flex-shrink-0" />
                      <span>Unassigned</span>
                    </div>
                  )}
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[10px] text-ink-500">
                    <Calendar className="h-3.5 w-3.5" />
                    {item.nextService}
                  </div>
                  <div className="flex items-center gap-1 text-xs font-medium text-ink-400 transition-colors group-hover:text-primary">
                    View <ArrowRight className="h-3.5 w-3.5" />
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}