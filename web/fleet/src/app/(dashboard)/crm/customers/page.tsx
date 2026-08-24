// src/app/(dashboard)/crm/customers/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Users,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  Building2,
  Plus,
  User,
  Mail,
  Phone,
  Briefcase,
} from "lucide-react";

type Customer = {
  id: string;
  name: string;
  initials: string;
  type: "Corporate" | "SME" | "Enterprise" | "Individual";
  status: "Active" | "Follow-up" | "At Risk" | "Inactive";
  risk: "Low" | "Medium" | "High";
  accountOwner: string;
  branch: string;
  lifetimeValue: number;
  orders: number;
  lastActivity: string;
  email: string;
  phone: string;
  location: string;
};

const CUSTOMERS: Customer[] = [
  {
    id: "CUST-001",
    name: "Charitos Hospital",
    initials: "CH",
    type: "Corporate",
    status: "Active",
    risk: "Low",
    accountOwner: "F. Daniel",
    branch: "Ikeja",
    lifetimeValue: 4200000,
    orders: 46,
    lastActivity: "2 days ago",
    email: "accounts@charitoshospital.com",
    phone: "+234 800 123 4567",
    location: "Ikeja, Lagos",
  },
  {
    id: "CUST-002",
    name: "Oakfield Stores",
    initials: "OS",
    type: "SME",
    status: "Follow-up",
    risk: "Medium",
    accountOwner: "A. Yusuf",
    branch: "Ikeja",
    lifetimeValue: 1800000,
    orders: 27,
    lastActivity: "6 days ago",
    email: "info@oakfieldstores.com",
    phone: "+234 800 234 5678",
    location: "Ikeja, Lagos",
  },
  {
    id: "CUST-003",
    name: "Greenline Foods",
    initials: "GL",
    type: "Enterprise",
    status: "Active",
    risk: "Low",
    accountOwner: "F. Daniel",
    branch: "Ikeja",
    lifetimeValue: 6900000,
    orders: 74,
    lastActivity: "Today",
    email: "procurement@greenlinefoods.com",
    phone: "+234 800 345 6789",
    location: "Ikeja, Lagos",
  },
  {
    id: "CUST-004",
    name: "Solaris Construction",
    initials: "SC",
    type: "Corporate",
    status: "At Risk",
    risk: "High",
    accountOwner: "M. Bello",
    branch: "Lekki",
    lifetimeValue: 3100000,
    orders: 18,
    lastActivity: "14 days ago",
    email: "accounts@solarisconstruction.com",
    phone: "+234 800 456 7890",
    location: "Lekki, Lagos",
  },
  {
    id: "CUST-005",
    name: "BlueMarket Retail",
    initials: "BR",
    type: "SME",
    status: "Active",
    risk: "Low",
    accountOwner: "A. Yusuf",
    branch: "Ikeja",
    lifetimeValue: 940000,
    orders: 12,
    lastActivity: "4 days ago",
    email: "info@bluemarketretail.com",
    phone: "+234 800 567 8901",
    location: "Ikeja, Lagos",
  },
  {
    id: "CUST-006",
    name: "Apex Logistics Internal",
    initials: "AL",
    type: "Corporate",
    status: "Active",
    risk: "Low",
    accountOwner: "F. Daniel",
    branch: "Ikeja",
    lifetimeValue: 2800000,
    orders: 8,
    lastActivity: "1 day ago",
    email: "internal@apexlogistics.com",
    phone: "+234 800 678 9012",
    location: "Ikeja, Lagos",
  },
  {
    id: "CUST-007",
    name: "Sunrise Industries",
    initials: "SI",
    type: "Corporate",
    status: "Inactive",
    risk: "Low",
    accountOwner: "M. Bello",
    branch: "Apapa",
    lifetimeValue: 520000,
    orders: 4,
    lastActivity: "1 month ago",
    email: "info@sunriseindustries.com",
    phone: "+234 800 789 0123",
    location: "Apapa, Lagos",
  },
  {
    id: "CUST-008",
    name: "GlobalTech Solutions",
    initials: "GS",
    type: "Enterprise",
    status: "Active",
    risk: "Medium",
    accountOwner: "F. Daniel",
    branch: "Lekki",
    lifetimeValue: 5600000,
    orders: 34,
    lastActivity: "3 days ago",
    email: "procurement@globaltech.com",
    phone: "+234 800 890 1234",
    location: "Lekki, Lagos",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  "Follow-up": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  "At Risk": { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Inactive: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

const TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Corporate: { bg: "bg-primary-bg", text: "text-primary" },
  SME: { bg: "bg-amber-bg", text: "text-amber" },
  Enterprise: { bg: "bg-purple-bg", text: "text-purple" },
  Individual: { bg: "bg-green-bg", text: "text-green" },
};

const RISK_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Low: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Medium: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  High: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const STATUS_FILTERS = ["All", "Active", "Follow-up", "At Risk", "Inactive"];
const TYPE_FILTERS = ["All", "Corporate", "SME", "Enterprise", "Individual"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function CustomerDirectoryPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Customer[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(CUSTOMERS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.email.toLowerCase().includes(search.toLowerCase()) ||
      item.accountOwner.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesType && matchesBranch;
  });

  const totalCustomers = data.length;
  const activeCount = data.filter((d) => d.status === "Active").length;
  const atRiskCount = data.filter((d) => d.status === "At Risk").length;
  const totalLifetimeValue = data.reduce((acc, d) => acc + d.lifetimeValue, 0);

  const hasActiveFilters = selectedStatus !== "All" || selectedType !== "All" || selectedBranch !== "All branches" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedType("All");
    setSelectedBranch("All branches");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading customers...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Customer Directory</h1>
          <p className="mt-1.5">
            Customer profiles, status, risk, account ownership, and recent activity.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={() => router.push("/crm/customers/create")}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Customer
          </button>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Users className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Total</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalCustomers}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{activeCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">At Risk</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{atRiskCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Users className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Lifetime Value</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalLifetimeValue)}
          </div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, ID, email, or account owner..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <Briefcase className="h-3.5 w-3.5 text-ink-400" />
          {selectedType}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Status:</span>
          {selectedStatus}
          <span className="text-ink-300">·</span>
          <Building2 className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
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
              {(selectedStatus !== "All" ? 1 : 0) + (selectedType !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
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
          {filteredData.length} customers
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

      {/* ── Customer Cards ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Users className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No customers found</h3>
          <p className="mt-1 text-sm text-ink-500">No customers match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;
            const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.Corporate;
            const riskStyle = RISK_STYLES[item.risk] || RISK_STYLES.Low;
            const formattedLTV = new Intl.NumberFormat("en-NG", {
              style: "currency",
              currency: "NGN",
              minimumFractionDigits: 0,
            }).format(item.lifetimeValue);

            return (
              <div
                key={item.id}
                onClick={() => router.push(`/crm/customers/${item.id}`)}
                className={`group rounded-lg border p-4 transition-colors cursor-pointer ${
                  item.status === "Active"
                    ? "border-green/20 bg-white hover:border-green/40 hover:bg-surface-2"
                    : item.status === "At Risk"
                    ? "border-red/30 bg-red-bg/10 hover:border-red/50 hover:bg-red-bg/20"
                    : item.status === "Follow-up"
                    ? "border-amber/30 bg-amber-bg/10 hover:border-amber/50 hover:bg-amber-bg/20"
                    : "border-line bg-white hover:border-ink-300 hover:bg-surface-2"
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full text-base font-bold text-white ${
                        item.status === "Active" ? "bg-primary" :
                        item.status === "At Risk" ? "bg-red" :
                        item.status === "Follow-up" ? "bg-amber" :
                        "bg-ink-400"
                      }`}
                    >
                      {item.initials}
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-ink-950">{item.name}</span>
                        <span className="font-mono text-[10px] text-ink-400">{item.id}</span>
                      </div>
                      <div className="flex flex-wrap items-center gap-1.5 mt-0.5">
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                        >
                          {statusStyle.icon}
                          {item.status}
                        </span>
                        <span
                          className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${typeStyle.bg} ${typeStyle.text}`}
                        >
                          {item.type}
                        </span>
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-medium ${riskStyle.bg} ${riskStyle.text}`}
                        >
                          <span className={`h-1 w-1 rounded-full ${riskStyle.dot}`} />
                          {item.risk}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Building2 className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.branch}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <User className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.accountOwner}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Mail className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{item.email}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Phone className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.phone}</span>
                  </div>
                </div>

                <div className="mt-3 border-t border-line pt-2.5">
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-ink-500">Lifetime Value</span>
                    <span className="font-semibold text-ink-950">{formattedLTV}</span>
                  </div>
                  <div className="flex items-center justify-between text-xs mt-0.5">
                    <span className="text-ink-500">Orders</span>
                    <span className="font-medium text-ink-950">{item.orders}</span>
                  </div>
                </div>

                <div className="mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[10px] text-ink-400">
                    <Clock className="h-3.5 w-3.5" />
                    Last activity: {item.lastActivity}
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