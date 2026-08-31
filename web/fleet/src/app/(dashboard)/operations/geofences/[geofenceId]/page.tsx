// src/app/(dashboard)/operations/geofences/[geofenceId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  MapPin,
  Edit3,
  Trash2,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Clock,
  Grid3x3,
  Layers,
  Globe,
  Navigation,
  MoreVertical,
  User,
  Calendar,
  Copy,
  Download,
} from "lucide-react";

const GEOFENCE_DETAIL = {
  id: "GEOF-001",
  name: "Ikeja Mall Pickup Zone",
  type: "Pickup" as const,
  branch: "Ikeja",
  status: "Active" as const,
  geometry: "Polygon · 12 vertices",
  area: "0.8 km²",
  createdAt: "2026-08-15",
  lastUpdated: "2 min ago",
  createdBy: "T. Adeyemi",
  purpose: "Designated pickup area for mall orders",
  description: "This geofence covers the Ikeja Mall area including the main entrance, parking lot, and surrounding streets. All pickup orders from the mall must be initiated within this zone.",
  coordinates: [
    { lat: "6.5244", lng: "3.3792" },
    { lat: "6.5280", lng: "3.3835" },
    { lat: "6.5210", lng: "3.3880" },
    { lat: "6.5180", lng: "3.3820" },
    { lat: "6.5220", lng: "3.3760" },
  ],
};

const TYPE_BADGES = {
  Pickup: { bg: "bg-green-bg", text: "text-green", icon: MapPin },
  Delivery: { bg: "bg-primary-bg", text: "text-primary", icon: Navigation },
  Restricted: { bg: "bg-red-bg", text: "text-red", icon: AlertCircle },
  Parking: { bg: "bg-amber-bg", text: "text-amber", icon: Layers },
  Depot: { bg: "bg-purple-bg", text: "text-purple", icon: Globe },
};

const STATUS_BADGES = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Inactive: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Draft: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
};

export default function GeofenceDetailPage() {
  const params = useParams();
  const router = useRouter();
  const geofenceId = params.geofenceId as string;

  const [loading, setLoading] = useState(true);
  const [geofence, setGeofence] = useState(GEOFENCE_DETAIL);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
      setGeofence(GEOFENCE_DETAIL);
      setLoading(false);
    };
    loadData();
  }, [geofenceId]);

  const handleDelete = () => {
    if (window.confirm(`Delete geofence "${geofence.name}"? This action cannot be undone.`)) {
      router.push("/operations/geofences");
    }
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading geofence...</span>
        </div>
      </div>
    );
  }

  const typeBadge = TYPE_BADGES[geofence.type as keyof typeof TYPE_BADGES] || TYPE_BADGES.Pickup;
  const statusBadge = STATUS_BADGES[geofence.status as keyof typeof STATUS_BADGES] || STATUS_BADGES.Draft;
  const isActive = geofence.status === "Active";
  const TypeIcon = typeBadge.icon;

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2.5 flex-wrap">
            <h1>{geofence.name}</h1>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusBadge.bg} ${statusBadge.text}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${statusBadge.dot}`} />
              {geofence.status}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${typeBadge.bg} ${typeBadge.text}`}>
              <TypeIcon className="h-3 w-3" />
              {geofence.type}
            </span>
          </div>
          <p className="mt-0.5 text-sm text-ink-500">
            {geofence.branch} branch · Created by {geofence.createdBy}
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
          <button
            onClick={handleDelete}
            className="flex h-10 items-center gap-1.5 rounded-lg border border-red/30 bg-red-bg px-3.5 text-sm font-medium text-red transition-colors hover:bg-red-bg/70"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {/* ── Main Grid ── */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.4fr_1fr]">
        {/* ── Left Column ── */}
        <div className="space-y-5">
          {/* Map */}
          <div className="rounded-lg border border-line bg-white overflow-hidden">
            <div className="border-b border-line px-4 py-3">
              <span className="text-sm font-medium text-ink-700">Geofence Map</span>
            </div>
            <div className="relative h-[280px] sm:h-[340px] bg-[#e8ecf1]">
              <div className="absolute inset-0" style={{
                backgroundImage: `
                  linear-gradient(rgba(200,200,210,0.3) 1px, transparent 1px),
                  linear-gradient(90deg, rgba(200,200,210,0.3) 1px, transparent 1px)
                `,
                backgroundSize: '30px 30px'
              }} />
              <div className="absolute inset-0 flex items-center justify-center">
                <div className="rounded-full border-4 border-primary/30 bg-primary/5 p-8">
                  <MapPin className="h-12 w-12 text-primary/40" />
                </div>
              </div>
              <svg className="absolute inset-0 w-full h-full" viewBox="0 0 100 100" preserveAspectRatio="none">
                <polygon
                  points="25,30 35,25 45,35 40,50 30,55 20,45"
                  fill="rgba(43,44,170,0.15)"
                  stroke="#2B2CAA"
                  strokeWidth="2"
                />
              </svg>
            </div>
            <div className="border-t border-line px-4 py-2.5">
              <span className="text-xs text-ink-500">
                {geofence.geometry} · Area: {geofence.area}
              </span>
            </div>
          </div>

          {/* Purpose */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Purpose</div>
            <p className="mt-1.5 text-sm text-ink-800">{geofence.purpose}</p>

            {geofence.description && (
              <>
                <div className="mt-4 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Description</div>
                <p className="mt-1.5 text-sm text-ink-700">{geofence.description}</p>
              </>
            )}
          </div>
        </div>

        {/* ── Right Column ── */}
        <div className="space-y-5">
          {/* Details */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="flex items-center gap-2.5 mb-4">
              <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
                <Grid3x3 className="h-4 w-4" />
              </div>
              <span className="text-sm font-semibold text-ink-950">Details</span>
            </div>

            <div className="space-y-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">ID</div>
                <div className="mt-0.5 font-mono text-sm text-ink-950">{geofence.id}</div>
              </div>
              <div className="border-t border-line pt-3">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Geometry</div>
                <div className="mt-0.5 text-sm text-ink-950">{geofence.geometry}</div>
              </div>
              <div className="border-t border-line pt-3">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Area</div>
                <div className="mt-0.5 text-sm text-ink-950">{geofence.area}</div>
              </div>
              <div className="border-t border-line pt-3">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Created</div>
                <div className="mt-0.5 flex items-center gap-2 text-sm text-ink-950">
                  <Calendar className="h-4 w-4 text-ink-400" />
                  {geofence.createdAt}
                </div>
              </div>
              <div className="border-t border-line pt-3">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Last Updated</div>
                <div className="mt-0.5 flex items-center gap-2 text-sm text-ink-950">
                  <Clock className="h-4 w-4 text-ink-400" />
                  {geofence.lastUpdated}
                </div>
              </div>
            </div>
          </div>

          {/* Actions */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="text-sm font-semibold text-ink-950 mb-3">Actions</div>
            <div className="flex flex-col gap-2">
              <button className="flex items-center gap-2 rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
                <Edit3 className="h-4 w-4" />
                Edit Geofence
              </button>
              <button className="flex items-center gap-2 rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
                <Copy className="h-4 w-4" />
                Duplicate
              </button>
              <button className="flex items-center gap-2 rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
                <Download className="h-4 w-4" />
                Export GeoJSON
              </button>
              {isActive ? (
                <button className="flex items-center gap-2 rounded-md border border-amber/30 bg-amber-bg/30 px-4 py-2.5 text-sm font-medium text-amber transition-colors hover:bg-amber-bg/50">
                  <XCircle className="h-4 w-4" />
                  Deactivate
                </button>
              ) : (
                <button className="flex items-center gap-2 rounded-md border border-green/30 bg-green-bg/30 px-4 py-2.5 text-sm font-medium text-green transition-colors hover:bg-green-bg/50">
                  <CheckCircle2 className="h-4 w-4" />
                  Activate
                </button>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}