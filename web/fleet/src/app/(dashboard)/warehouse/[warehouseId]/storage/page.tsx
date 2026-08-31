// src/app/(dashboard)/warehouse/[warehouseId]/storage/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  Layers,
  Package,
  RefreshCw,
  Search,
  Plus,
  MoreHorizontal,
  ChevronDown,
  ChevronRight,
  CheckCircle2,
  AlertCircle,
  Clock,
  MapPin,
  Eye,
  ArrowRight,
  Sparkles,
  Building2,
  Boxes,
  Grid3X3,
  AlertTriangle,
  Edit3,
  Trash2,
  X,
  Filter,
  Lock,
} from "lucide-react";

type StorageNode = {
  id: string;
  name: string;
  type: "Zone" | "Aisle" | "Bay" | "Bin" | "Controlled";
  parentId: string | null;
  capacity: number;
  used: number;
  status: "Available" | "Full" | "Restricted" | "Maintenance";
  children: StorageNode[];
};

// Mock data
const MOCK_STORAGE: StorageNode = {
  id: "root",
  name: "Ikeja Central Warehouse",
  type: "Zone",
  parentId: null,
  capacity: 12300,
  used: 9800,
  status: "Available",
  children: [
    {
      id: "zone-1",
      name: "Aisle A - General Storage",
      type: "Zone",
      parentId: "root",
      capacity: 5000,
      used: 4200,
      status: "Available",
      children: [
        {
          id: "aisle-a1",
          name: "Aisle A1",
          type: "Aisle",
          parentId: "zone-1",
          capacity: 2500,
          used: 2100,
          status: "Available",
          children: [
            {
              id: "bay-a1-01",
              name: "Bay A1-01",
              type: "Bay",
              parentId: "aisle-a1",
              capacity: 500,
              used: 480,
              status: "Full",
              children: [
                {
                  id: "bin-a1-01-001",
                  name: "Bin A1-01-001",
                  type: "Bin",
                  parentId: "bay-a1-01",
                  capacity: 100,
                  used: 100,
                  status: "Full",
                  children: [],
                },
                {
                  id: "bin-a1-01-002",
                  name: "Bin A1-01-002",
                  type: "Bin",
                  parentId: "bay-a1-01",
                  capacity: 100,
                  used: 95,
                  status: "Full",
                  children: [],
                },
              ],
            },
            {
              id: "bay-a1-02",
              name: "Bay A1-02",
              type: "Bay",
              parentId: "aisle-a1",
              capacity: 500,
              used: 450,
              status: "Available",
              children: [
                {
                  id: "bin-a1-02-001",
                  name: "Bin A1-02-001",
                  type: "Bin",
                  parentId: "bay-a1-02",
                  capacity: 100,
                  used: 80,
                  status: "Available",
                  children: [],
                },
              ],
            },
          ],
        },
        {
          id: "aisle-a2",
          name: "Aisle A2",
          type: "Aisle",
          parentId: "zone-1",
          capacity: 2500,
          used: 2100,
          status: "Available",
          children: [
            {
              id: "bay-a2-01",
              name: "Bay A2-01",
              type: "Bay",
              parentId: "aisle-a2",
              capacity: 500,
              used: 420,
              status: "Available",
              children: [],
            },
            {
              id: "bay-a2-02",
              name: "Bay A2-02",
              type: "Bay",
              parentId: "aisle-a2",
              capacity: 500,
              used: 500,
              status: "Full",
              children: [],
            },
          ],
        },
      ],
    },
    {
      id: "zone-2",
      name: "Aisle B - Heavy Storage",
      type: "Zone",
      parentId: "root",
      capacity: 3000,
      used: 2800,
      status: "Full",
      children: [
        {
          id: "aisle-b1",
          name: "Aisle B1",
          type: "Aisle",
          parentId: "zone-2",
          capacity: 1500,
          used: 1400,
          status: "Full",
          children: [],
        },
        {
          id: "aisle-b2",
          name: "Aisle B2",
          type: "Aisle",
          parentId: "zone-2",
          capacity: 1500,
          used: 1400,
          status: "Full",
          children: [],
        },
      ],
    },
    {
      id: "zone-3",
      name: "Cold Storage",
      type: "Zone",
      parentId: "root",
      capacity: 1500,
      used: 800,
      status: "Available",
      children: [
        {
          id: "aisle-c1",
          name: "Cold Aisle C1",
          type: "Aisle",
          parentId: "zone-3",
          capacity: 800,
          used: 400,
          status: "Available",
          children: [],
        },
        {
          id: "aisle-c2",
          name: "Cold Aisle C2",
          type: "Aisle",
          parentId: "zone-3",
          capacity: 700,
          used: 400,
          status: "Available",
          children: [],
        },
      ],
    },
    {
      id: "zone-4",
      name: "Aisle C - Fragile Storage",
      type: "Zone",
      parentId: "root",
      capacity: 2000,
      used: 1200,
      status: "Available",
      children: [
        {
          id: "aisle-c3",
          name: "Aisle C3",
          type: "Aisle",
          parentId: "zone-4",
          capacity: 1000,
          used: 600,
          status: "Available",
          children: [],
        },
        {
          id: "aisle-c4",
          name: "Aisle C4",
          type: "Aisle",
          parentId: "zone-4",
          capacity: 1000,
          used: 600,
          status: "Available",
          children: [],
        },
      ],
    },
    {
      id: "zone-5",
      name: "Dock Zone",
      type: "Zone",
      parentId: "root",
      capacity: 800,
      used: 600,
      status: "Available",
      children: [],
    },
  ],
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Available: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Full: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Restricted: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertTriangle className="h-3.5 w-3.5" />,
  },
  Maintenance: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
};

const TYPE_ICONS: Record<string, React.ReactNode> = {
  Zone: <Building2 className="h-4 w-4" />,
  Aisle: <Grid3X3 className="h-4 w-4" />,
  Bay: <Boxes className="h-4 w-4" />,
  Bin: <Package className="h-4 w-4" />,
  Controlled: <Lock className="h-4 w-4" />,
};

export default function StorageHierarchyPage() {
  const router = useRouter();
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<StorageNode | null>(null);
  const [search, setSearch] = useState("");
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(new Set(["root"]));
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_STORAGE);
      setLoading(false);
    };
    loadData();
  }, [warehouseId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const toggleNode = (nodeId: string) => {
    setExpandedNodes((prev) => {
      const next = new Set(prev);
      if (next.has(nodeId)) {
        next.delete(nodeId);
      } else {
        next.add(nodeId);
      }
      return next;
    });
  };

  const goBack = () => {
    router.push(`/warehouse/${warehouseId}`);
  };

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading storage hierarchy...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Layers className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Storage hierarchy not found</h3>
        <p className="mt-1 text-sm text-ink-500">No storage structure exists for this warehouse.</p>
        <button
          onClick={goBack}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to warehouse
        </button>
      </div>
    );
  }

  const totalCapacity = data.capacity;
  const totalUsed = data.used;
  const utilization = Math.round((totalUsed / totalCapacity) * 100);
  const fullZones = data.children.filter((c) => c.status === "Full").length;
  const restrictedZones = data.children.filter((c) => c.status === "Restricted").length;

  const renderNode = (node: StorageNode, depth: number = 0) => {
    const isExpanded = expandedNodes.has(node.id);
    const hasChildren = node.children.length > 0;
    const statusStyle = STATUS_STYLES[node.status] || STATUS_STYLES.Available;
    const TypeIcon = TYPE_ICONS[node.type] || <Package className="h-4 w-4" />;
    const usagePercent = Math.round((node.used / node.capacity) * 100);
    const isFull = node.status === "Full";

    const matchesSearch =
      search.trim() === "" ||
      node.name.toLowerCase().includes(search.toLowerCase()) ||
      node.id.toLowerCase().includes(search.toLowerCase());

    if (search.trim() !== "" && !matchesSearch && !hasChildren) return null;
    if (search.trim() !== "" && !matchesSearch && hasChildren) {
      const hasMatchingChild = node.children.some((c) =>
        c.name.toLowerCase().includes(search.toLowerCase()) ||
        c.id.toLowerCase().includes(search.toLowerCase())
      );
      if (!hasMatchingChild) return null;
    }

    return (
      <div key={node.id}>
        <div
          className={`flex items-center gap-3 rounded-xl border border-line p-3 transition hover:bg-surface-2 ${
            depth === 0 ? "bg-primary-bg/30 border-primary/20" : ""
          }`}
          style={{ marginLeft: `${depth * 20}px` }}
        >
          <button
            onClick={() => toggleNode(node.id)}
            className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-lg hover:bg-surface-3 transition-colors"
          >
            {hasChildren ? (
              isExpanded ? (
                <ChevronDown className="h-4 w-4 text-ink-500" />
              ) : (
                <ChevronRight className="h-4 w-4 text-ink-500" />
              )
            ) : (
              <div className="h-4 w-4" />
            )}
          </button>

          <div className={`flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl ${statusStyle.bg} ${statusStyle.text}`}>
            {TypeIcon}
          </div>

          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-[14px] font-semibold text-ink-950">{node.name}</span>
              <span className="font-mono text-[10px] text-ink-400">{node.id}</span>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                {node.type}
              </span>
            </div>
            <div className="mt-0.5 flex flex-wrap items-center gap-3 text-[12px] text-ink-500">
              <span>{node.used.toLocaleString()} / {node.capacity.toLocaleString()} units</span>
              <span className="text-ink-300">·</span>
              <span className={usagePercent >= 80 ? "text-green" : usagePercent >= 50 ? "text-amber" : "text-ink-500"}>
                {usagePercent}% used
              </span>
              {node.children.length > 0 && (
                <>
                  <span className="text-ink-300">·</span>
                  <span>{node.children.length} sub-locations</span>
                </>
              )}
            </div>
          </div>

          <div className="flex items-center gap-2">
            <div className="w-20 h-1.5 rounded-full bg-surface-3 overflow-hidden">
              <div
                className={`h-full rounded-full ${isFull ? "bg-red" : usagePercent >= 80 ? "bg-green" : usagePercent >= 50 ? "bg-amber" : "bg-ink-300"}`}
                style={{ width: `${usagePercent}%` }}
              />
            </div>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
              {node.status}
            </span>
          </div>
        </div>

        {isExpanded && hasChildren && (
          <div className="mt-1 space-y-1">
            {node.children.map((child) => renderNode(child, depth + 1))}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={goBack}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Storage Hierarchy
              </h1>
              <span className="rounded-full bg-primary-bg px-2.5 py-0.5 text-[11px] font-semibold text-primary">
                {data.children.length} zones
              </span>
              <div className="flex items-center gap-1 rounded-full bg-green-bg px-2.5 py-0.5 text-[10px] font-semibold text-green">
                <span className="relative flex h-1.5 w-1.5">
                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                  <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green" />
                </span>
                {utilization}% utilized
              </div>
            </div>
            <p className="mt-1 text-[13.5px] text-ink-600">
              Zones, aisles, bays, bins and controlled locations with capacity and status.
            </p>
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <Plus className="h-3.5 w-3.5" />
            Add Location
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-primary-bg p-1.5">
              <Layers className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Total Locations</div>
              <div className="font-display text-[18px] font-bold text-ink-950">
                {data.children.reduce((acc, c) => acc + 1 + c.children.length, 0)}
              </div>
            </div>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-blue-bg p-1.5">
              <Boxes className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Capacity</div>
              <div className="font-display text-[18px] font-bold text-ink-950">
                {totalCapacity.toLocaleString()}
              </div>
            </div>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-green-bg p-1.5">
              <Package className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Used</div>
              <div className="font-display text-[18px] font-bold text-ink-950">
                {totalUsed.toLocaleString()}
              </div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-ink-400">
            <span>{utilization}% utilization</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-amber-bg p-1.5">
              <AlertCircle className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Full / Restricted</div>
              <div className="font-display text-[18px] font-bold text-ink-950">
                {fullZones + restrictedZones}
              </div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-ink-400">
            <span>{fullZones} full, {restrictedZones} restricted</span>
          </div>
        </div>
      </div>

      {/* ===== SEARCH ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[240px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name or ID..."
            className="w-full bg-transparent text-[13px] text-ink-950 outline-none placeholder:text-ink-400 py-2"
          />
        </div>
        {search && (
          <button
            onClick={() => setSearch("")}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-[12px] font-medium text-ink-500 transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
        <button className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[12px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
          <Filter className="h-4 w-4" />
          Filter
        </button>
      </div>

      {/* ===== TREE VIEW ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <div className="space-y-1">
          {renderNode(data)}
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Layers className="h-4 w-4" />
          WH-03 · Storage Hierarchy
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}