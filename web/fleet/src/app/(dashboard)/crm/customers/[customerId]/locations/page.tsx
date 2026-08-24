// src/app/(dashboard)/crm/customers/[customerId]/locations/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  MapPin,
  Search,
  RefreshCw,
  Filter,
  SlidersHorizontal,
  X,
  ChevronDown,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  Sparkles,
  MoreHorizontal,
  Plus,
  Building2,
  Home,
  Truck,
  Package,
  Edit3,
  Trash2,
  Navigation,
  CalendarDays,
  Phone,
  Mail,
  User,
} from "lucide-react";

type Location = {
  id: string;
  name: string;
  type: "Pickup" | "Delivery" | "Billing" | "Operating";
  address: string;
  city: string;
  state: string;
  country: string;
  coordinates: string | null;
  hours: string | null;
  instructions: string | null;
  primary: boolean;
  geofenceRef: string | null;
  status: "Active" | "Inactive";
  contactName: string | null;
  contactPhone: string | null;
  createdAt: string;
  updatedAt: string;
};

// Mock data
const MOCK_LOCATIONS: Location[] = [
  {
    id: "LOC-001",
    name: "Charitos Hospital - Main Campus",
    type: "Delivery",
    address: "42 Medical Way",
    city: "Ikeja",
    state: "Lagos",
    country: "Nigeria",
    coordinates: "6.6018° N, 3.3515° E",
    hours: "24/7",
    instructions: "Deliver to loading bay at the back of the main building. Security checkpoint at gate.",
    primary: true,
    geofenceRef: "GF-001",
    status: "Active",
    contactName: "Dr. Adeola Ogunlesi",
    contactPhone: "+234 800 111 2222",
    createdAt: "2026-01-15",
    updatedAt: "2026-08-20",
  },
  {
    id: "LOC-002",
    name: "Charitos Hospital - Annex",
    type: "Pickup",
    address: "15 Hospital Road",
    city: "Ikeja",
    state: "Lagos",
    country: "Nigeria",
    coordinates: "6.6128° N, 3.3415° E",
    hours: "06:00 - 20:00",
    instructions: "Annex building. Use the side entrance for pickups.",
    primary: false,
    geofenceRef: "GF-002",
    status: "Active",
    contactName: "Mr. Femi Adeyemi",
    contactPhone: "+234 800 333 4444",
    createdAt: "2026-02-10",
    updatedAt: "2026-08-18",
  },
  {
    id: "LOC-003",
    name: "Charitos Hospital - Billing Office",
    type: "Billing",
    address: "3 Finance Plaza",
    city: "Ikeja",
    state: "Lagos",
    country: "Nigeria",
    coordinates: null,
    hours: "09:00 - 17:00",
    instructions: "Billing department on the 2nd floor.",
    primary: false,
    geofenceRef: null,
    status: "Active",
    contactName: "Mrs. Toyin Akinola",
    contactPhone: "+234 800 555 6666",
    createdAt: "2026-03-01",
    updatedAt: "2026-08-15",
  },
  {
    id: "LOC-004",
    name: "Charitos Hospital - Warehouse",
    type: "Operating",
    address: "22 Logistics Avenue",
    city: "Ikeja",
    state: "Lagos",
    country: "Nigeria",
    coordinates: "6.5988° N, 3.3615° E",
    hours: "24/7",
    instructions: "Main warehouse for medical supplies storage.",
    primary: false,
    geofenceRef: "GF-003",
    status: "Inactive",
    contactName: "Mr. Chidi Okonkwo",
    contactPhone: "+234 800 777 8888",
    createdAt: "2026-04-20",
    updatedAt: "2026-07-30",
  },
];

const TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pickup: { bg: "bg-blue-bg", text: "text-blue", icon: <Package className="h-3.5 w-3.5" /> },
  Delivery: { bg: "bg-green-bg", text: "text-green", icon: <Truck className="h-3.5 w-3.5" /> },
  Billing: { bg: "bg-amber-bg", text: "text-amber", icon: <Building2 className="h-3.5 w-3.5" /> },
  Operating: { bg: "bg-purple-bg", text: "text-purple", icon: <Home className="h-3.5 w-3.5" /> },
};

const TYPE_FILTERS = ["All", "Pickup", "Delivery", "Billing", "Operating"];
const STATUS_FILTERS = ["All", "Active", "Inactive"];

export default function CustomerLocationsPage() {
  const router = useRouter();
  const params = useParams();
  const customerId = params.customerId as string;

  const [search, setSearch] = useState("");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Location[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_LOCATIONS);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.address.toLowerCase().includes(search.toLowerCase()) ||
      item.city.toLowerCase().includes(search.toLowerCase()) ||
      item.state.toLowerCase().includes(search.toLowerCase());

    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesType && matchesStatus;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading locations...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedType !== "All" || selectedStatus !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedType("All");
    setSelectedStatus("All");
  };

  const goToLocationDetail = (locationId: string) => {
    router.push(`/crm/customers/${customerId}/locations/${locationId}`);
  };

  const goToAddLocation = () => {
    router.push(`/crm/customers/${customerId}/locations/create`);
  };

  // Stats
  const totalLocations = data.length;
  const primaryCount = data.filter((d) => d.primary).length;
  const activeCount = data.filter((d) => d.status === "Active").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Customer Locations
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Pickup, delivery, billing or operating sites, hours, instructions and coordinates/geofence reference.
          </p>
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
            onClick={goToAddLocation}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Location
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Locations</div>
          <div className="text-lg font-bold text-ink-950">{totalLocations}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Primary</div>
          <div className="text-lg font-bold text-primary">{primaryCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Active</div>
          <div className="text-lg font-bold text-green">{activeCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">With Geofence</div>
          <div className="text-lg font-bold text-ink-950">
            {data.filter((d) => d.geofenceRef).length}
          </div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, address, or city..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "Pickup", "Delivery", "Billing", "Operating"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Filter className="h-4 w-4 text-ink-400" />
          {selectedType}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Active", "Inactive"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <CheckCircle2 className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white/70 text-ink-600 shadow-sm hover:bg-white"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedType !== "All" ? 1 : 0) + (selectedStatus !== "All" ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-sm font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {/* ===== FILTER DRAWER ===== */}
      {showFilters && (
        <div className="rounded-xl border border-line bg-white/70 p-3.5 shadow-sm backdrop-blur-xl">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Type</div>
              <div className="flex gap-1.5">
                {TYPE_FILTERS.map((type) => (
                  <button
                    key={type}
                    onClick={() => setSelectedType(type)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedType === type
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
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
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ===== LOCATION CARDS ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <MapPin className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No locations found</h3>
          <p className="mt-1 text-sm text-ink-500">No locations match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {filteredData.map((item) => {
            const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.Delivery;

            return (
              <div
                key={item.id}
                onClick={() => goToLocationDetail(item.id)}
                className={`group relative cursor-pointer rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-1 ${
                  item.primary
                    ? "border-primary/30 bg-primary-bg/10 hover:border-primary/50"
                    : item.status === "Inactive"
                    ? "border-red/20 bg-red-bg/10 hover:border-red/40"
                    : "border-line bg-white hover:border-ink-300"
                }`}
              >
                {/* Glow effect */}
                <div className={`absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 ${
                  item.primary ? "bg-primary/5" :
                  item.status === "Inactive" ? "bg-red-500/5" :
                  "bg-primary/5"
                }`} />

                <div className="relative z-10 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-xl ${typeStyle.bg} ${typeStyle.text}`}
                    >
                      {typeStyle.icon}
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-[15px] font-semibold text-ink-950">
                          {item.name}
                        </span>
                        {item.primary && (
                          <span className="rounded-full bg-primary-bg px-2 py-0.5 text-[9px] font-semibold text-primary">
                            Primary
                          </span>
                        )}
                      </div>
                      <div className="flex flex-wrap items-center gap-1.5 mt-0.5">
                        <span className="text-[12px] font-medium text-ink-500">{item.type}</span>
                        <span className="text-ink-300">·</span>
                        <span className="text-[12px] text-ink-500">{item.city}, {item.state}</span>
                        <span className="text-ink-300">·</span>
                        <span className={`text-[12px] font-medium ${item.status === "Active" ? "text-green" : "text-red"}`}>
                          {item.status}
                        </span>
                      </div>
                    </div>
                  </div>
                  <button className="flex-shrink-0 text-ink-300 hover:text-ink-600 transition-colors">
                    <MoreHorizontal className="h-4 w-4" />
                  </button>
                </div>

                <div className="relative z-10 mt-3 space-y-1.5 text-sm">
                  <div className="flex items-center gap-2 text-ink-600">
                    <MapPin className="h-4 w-4 text-ink-400" />
                    <span>{item.address}</span>
                  </div>
                  {item.coordinates && (
                    <div className="flex items-center gap-2 text-ink-600">
                      <Navigation className="h-4 w-4 text-ink-400" />
                      <span className="font-mono text-[12px]">{item.coordinates}</span>
                    </div>
                  )}
                  {item.hours && (
                    <div className="flex items-center gap-2 text-ink-600">
                      <Clock className="h-4 w-4 text-ink-400" />
                      <span>{item.hours}</span>
                    </div>
                  )}
                </div>

                {item.instructions && (
                  <div className="relative z-10 mt-2 text-sm text-ink-500 bg-surface-2 rounded-lg px-3 py-1.5">
                    <span className="font-medium">Instructions:</span> {item.instructions}
                  </div>
                )}

                <div className="relative z-10 mt-3 flex flex-wrap items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-2 text-[11px] text-ink-400">
                    <User className="h-3.5 w-3.5" />
                    <span>{item.contactName || "—"}</span>
                    {item.contactPhone && (
                      <>
                        <span className="text-ink-300">·</span>
                        <span>{item.contactPhone}</span>
                      </>
                    )}
                  </div>
                  {item.geofenceRef && (
                    <span className="rounded-full bg-blue-bg px-2 py-0.5 text-[9px] font-medium text-blue">
                      Geofence: {item.geofenceRef}
                    </span>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <MapPin className="h-4 w-4" />
          CRM-05 · Customer Locations
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} locations shown
        </span>
      </div>
    </div>
  );
}