// src/app/(dashboard)/fleet/fuel/page.tsx

"use client";
export const dynamic = "force-dynamic";


import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Search,
  Plus,
  Fuel,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  ArrowRight,
  Calendar,
  User,
  Truck,
  FileText,
  TrendingUp,
  AlertTriangle,
  DollarSign,
  Droplets,
  Flame,
  Zap,
  Gauge,
} from "lucide-react";

type FuelRecord = {
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
};

const FUEL_RECORDS: FuelRecord[] = [
  {
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
    notes: "Full tank",
  },
  {
    id: "FUEL-002",
    assetId: "TR-112",
    assetName: "Scania R500",
    assetType: "Truck",
    date: "2026-08-17",
    fuelType: "Diesel",
    quantity: 120,
    unit: "Liters",
    cost: "₦112,800",
    odometer: "312,440 km",
    station: "MRS - Apapa",
    location: "Apapa",
    performedBy: "Kabiru Ahmed",
    status: "Pending",
    receipt: "REC-002",
    notes: "Long haul trip",
  },
  {
    id: "FUEL-003",
    assetId: "BR-188",
    assetName: "Yamaha FZ",
    assetType: "Motorcycle",
    date: "2026-08-16",
    fuelType: "Petrol",
    quantity: 8,
    unit: "Liters",
    cost: "₦6,800",
    odometer: "22,180 km",
    station: "NNPC - Yaba",
    location: "Yaba",
    performedBy: "Amina Yusuf",
    status: "Approved",
    receipt: "REC-003",
    notes: "",
  },
  {
    id: "FUEL-004",
    assetId: "VN-029",
    assetName: "Ford Transit",
    assetType: "Van",
    date: "2026-08-16",
    fuelType: "Diesel",
    quantity: 35,
    unit: "Liters",
    cost: "₦32,900",
    odometer: "87,340 km",
    station: "TotalEnergies - Ikeja",
    location: "Ikeja",
    performedBy: "Tunde Adeyemi",
    status: "Reconciling",
    receipt: "REC-004",
    notes: "Meter reading discrepancy",
  },
  {
    id: "FUEL-005",
    assetId: "BR-204",
    assetName: "Honda ACE 110",
    assetType: "Motorcycle",
    date: "2026-08-15",
    fuelType: "Petrol",
    quantity: 6,
    unit: "Liters",
    cost: "₦5,100",
    odometer: "38,442 km",
    station: "Oando - Ikeja",
    location: "Ikeja",
    performedBy: "Peter Okafor",
    status: "Rejected",
    receipt: "REC-005",
    notes: "Receipt missing - rejected",
  },
  {
    id: "FUEL-006",
    assetId: "TR-098",
    assetName: "MAN TGS",
    assetType: "Truck",
    date: "2026-08-15",
    fuelType: "Diesel",
    quantity: 150,
    unit: "Liters",
    cost: "₦141,000",
    odometer: "456,720 km",
    station: "MRS - Ikeja",
    location: "Ikeja",
    performedBy: "Musa Bello",
    status: "Pending",
    receipt: "REC-006",
    notes: "",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Approved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Reconciling: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

const FUEL_TYPE_ICONS: Record<string, React.ReactNode> = {
  Petrol: <Flame className="h-3.5 w-3.5" />,
  Diesel: <Droplets className="h-3.5 w-3.5" />,
  Electric: <Zap className="h-3.5 w-3.5" />,
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Approved", "Pending", "Rejected", "Reconciling"];
const FUEL_TYPES = ["All", "Petrol", "Diesel", "Electric"];

export default function FuelRecordsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedFuelType, setSelectedFuelType] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<FuelRecord[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(FUEL_RECORDS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.assetId.toLowerCase().includes(search.toLowerCase()) ||
      item.assetName.toLowerCase().includes(search.toLowerCase()) ||
      item.station.toLowerCase().includes(search.toLowerCase());

    const matchesBranch = selectedBranch === "All branches" || item.location === selectedBranch;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesFuelType = selectedFuelType === "All" || item.fuelType === selectedFuelType;

    return matchesSearch && matchesBranch && matchesStatus && matchesFuelType;
  });

  const totalCost = data.reduce((acc, d) => {
    const cost = parseInt(d.cost.replace(/[₦,]/g, ""));
    return acc + cost;
  }, 0);

  const pendingCount = data.filter((d) => d.status === "Pending").length;
  const approvedCount = data.filter((d) => d.status === "Approved").length;

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedStatus !== "All" ||
    selectedFuelType !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedStatus("All");
    setSelectedFuelType("All");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading fuel records...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Fuel Records</h1>
          <p className="mt-1.5">
            Fuel and energy records for fleet assets with consumption, cost tracking, and approvals.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={() => router.push("/fleet/fuel/create")}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Record
          </button>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Approved</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{approvedCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Pending</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{pendingCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <DollarSign className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Total Cost</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">₦{totalCost.toLocaleString()}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID, asset, or station..."
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
          <span className="text-ink-400">Fuel:</span>
          {selectedFuelType}
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
                (selectedFuelType !== "All" ? 1 : 0)}
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
          {filteredData.length} records
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
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Fuel Type</span>
              <div className="flex flex-wrap gap-1.5">
                {FUEL_TYPES.map((type) => (
                  <button
                    key={type}
                    onClick={() => setSelectedFuelType(type)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedFuelType === type
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

      {/* ── Fuel Records Cards ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Fuel className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No fuel records found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No fuel records match your current filters. Add a new record to get started.
          </p>
          <button
            onClick={() => router.push("/fleet/fuel/create")}
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            <Plus className="inline h-3.5 w-3.5 mr-1" />
            Add Record
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Pending;
            const FuelIcon = FUEL_TYPE_ICONS[item.fuelType as keyof typeof FUEL_TYPE_ICONS] || <Fuel className="h-3.5 w-3.5" />;
            const isPending = item.status === "Pending";
            const isRejected = item.status === "Rejected";

            return (
              <div
                key={item.id}
                onClick={() => router.push(`/fleet/fuel/${item.id}`)}
                className={`group rounded-lg border p-4 transition-colors cursor-pointer ${
                  isRejected
                    ? "border-red/30 bg-red-bg/10 hover:border-red/50 hover:bg-red-bg/20"
                    : isPending
                    ? "border-amber/30 bg-amber-bg/10 hover:border-amber/50 hover:bg-amber-bg/20"
                    : "border-green/20 bg-white hover:border-green/40 hover:bg-surface-2"
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-10 w-10 items-center justify-center rounded-md ${
                        isRejected
                          ? "bg-red-bg text-red"
                          : isPending
                          ? "bg-amber-bg text-amber"
                          : "bg-green-bg text-green"
                      }`}
                    >
                      <Fuel className="h-4 w-4" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-ink-950">{item.id}</span>
                        <span className="font-mono text-[10px] text-ink-400">{item.assetId}</span>
                      </div>
                      <div className="text-xs text-ink-500">{item.assetName}</div>
                    </div>
                  </div>
                  <span
                    className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                  >
                    {statusStyle.icon}
                    {item.status}
                  </span>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Calendar className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.date}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    {FuelIcon}
                    <span>{item.fuelType}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <DollarSign className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="font-semibold text-ink-950">{item.cost}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Droplets className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.quantity} {item.unit}</span>
                  </div>
                </div>

                <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-ink-500">
                  <span className="flex items-center gap-1">
                    <MapPin className="h-3.5 w-3.5" />
                    {item.location}
                  </span>
                  <span className="text-ink-300">·</span>
                  <span className="truncate max-w-[150px]">{item.station}</span>
                  <span className="text-ink-300">·</span>
                  <span className="flex items-center gap-1">
                    <User className="h-3.5 w-3.5" />
                    {item.performedBy}
                  </span>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[10px] text-ink-400">
                    <Gauge className="h-3.5 w-3.5" />
                    {item.odometer}
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