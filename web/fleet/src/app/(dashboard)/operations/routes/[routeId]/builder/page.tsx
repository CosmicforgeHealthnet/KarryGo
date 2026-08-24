// src/app/(dashboard)/operations/routes/[routeId]/builder/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  MapPin,
  Plus,
  X,
  GripVertical,
  Clock,
  Save,
  Trash2,
  MoveUp,
  MoveDown,
  Package,
  Truck,
  AlertCircle,
} from "lucide-react";
import { ROUTE_DETAIL } from "@/lib/mock-routes-data";

type Stop = {
  id: string;
  type: "Pickup" | "Dropoff" | "Waypoint";
  location: string;
  address: string;
  status: "Pending" | "Completed" | "Skipped" | "Exception";
  timeWindow?: string;
  assignedTo?: string;
};

const TYPE_STYLES = {
  Pickup: { bg: "bg-primary-bg", text: "text-primary", icon: Package },
  Dropoff: { bg: "bg-green-bg", text: "text-green", icon: Truck },
  Waypoint: { bg: "bg-surface-2", text: "text-ink-500", icon: MapPin },
};

export default function RouteBuilderPage() {
  const params = useParams();
  const router = useRouter();
  const routeId = params.routeId as string;

  const [loading, setLoading] = useState(true);
  const [route, setRoute] = useState(ROUTE_DETAIL);
  const [stops, setStops] = useState<Stop[]>(ROUTE_DETAIL.stops);
  const [isAddingStop, setIsAddingStop] = useState(false);

  const [newStopType, setNewStopType] = useState<"Pickup" | "Dropoff" | "Waypoint">("Pickup");
  const [newStopLocation, setNewStopLocation] = useState("");
  const [newStopAddress, setNewStopAddress] = useState("");
  const [newStopTimeWindow, setNewStopTimeWindow] = useState("");

  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 400);
    return () => clearTimeout(timer);
  }, []);

  const handleAddStop = () => {
    if (!newStopLocation.trim()) return;

    const newStop: Stop = {
      id: `STOP-${String(stops.length + 1).padStart(3, "0")}`,
      type: newStopType,
      location: newStopLocation.trim(),
      address: newStopAddress.trim() || "Address not set",
      status: "Pending",
      timeWindow: newStopTimeWindow || undefined,
    };

    setStops([...stops, newStop]);
    setNewStopLocation("");
    setNewStopAddress("");
    setNewStopTimeWindow("");
    setIsAddingStop(false);
  };

  const handleRemoveStop = (index: number) => {
    if (window.confirm("Remove this stop from the route?")) {
      const newStops = stops.filter((_, i) => i !== index);
      setStops(newStops);
    }
  };

  const handleMoveStop = (index: number, direction: "up" | "down") => {
    const newStops = [...stops];
    const targetIndex = direction === "up" ? index - 1 : index + 1;
    if (targetIndex < 0 || targetIndex >= newStops.length) return;
    [newStops[index], newStops[targetIndex]] = [newStops[targetIndex], newStops[index]];
    setStops(newStops);
  };

  const handleSave = async () => {
    setLoading(true);
    await new Promise((resolve) => setTimeout(resolve, 800));
    setLoading(false);
    router.push(`/operations/routes/${routeId}`);
  };

  const handleCancel = () => {
    router.push(`/operations/routes/${routeId}`);
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading builder...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div>
          <p className="text-sm text-ink-500">
            Add, reorder, or remove stops for this route
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={handleCancel}
            className="flex items-center gap-2 rounded-lg border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <X className="h-4 w-4" />
            Cancel
          </button>
          <button
            onClick={handleSave}
            className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Save className="h-4 w-4" />
            Save route
          </button>
        </div>
      </div>

      {/* ── Route Summary ── */}
      <div className="rounded-lg border border-line bg-white p-4">
        <div className="flex flex-wrap items-center gap-4 text-sm">
          <span className="flex items-center gap-2">
            <span className="font-medium text-ink-600">Route:</span>
            <span className="font-semibold text-ink-950">{route.name}</span>
          </span>
          <span className="hidden sm:block w-px h-5 bg-line" />
          <span className="flex items-center gap-2">
            <span className="font-medium text-ink-600">Stops:</span>
            <span className="font-semibold text-ink-950">{stops.length}</span>
          </span>
          <span className="hidden sm:block w-px h-5 bg-line" />
          <span className="flex items-center gap-2">
            <span className="font-medium text-ink-600">Service:</span>
            <span className="font-semibold text-ink-950">{route.service}</span>
          </span>
          <span className="hidden sm:block w-px h-5 bg-line" />
          <span className="flex items-center gap-2">
            <span className="font-medium text-ink-600">Branch:</span>
            <span className="font-semibold text-ink-950">{route.branch}</span>
          </span>
        </div>
      </div>

      {/* ── Stops List ── */}
      <div className="rounded-lg border border-line bg-white">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <div className="flex items-center gap-2">
            <MapPin className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Stops</span>
            <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
              {stops.length}
            </span>
          </div>
          <button
            onClick={() => setIsAddingStop(true)}
            className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Plus className="h-3.5 w-3.5" />
            Add stop
          </button>
        </div>

        {/* Add stop form */}
        {isAddingStop && (
          <div className="border-b border-line p-4 bg-primary-bg/10">
            <div className="flex items-center justify-between mb-3">
              <span className="text-sm font-semibold text-ink-950">Add new stop</span>
              <button
                onClick={() => setIsAddingStop(false)}
                className="text-ink-400 hover:text-ink-600 transition-colors"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label className="text-xs font-medium text-ink-600">Type</label>
                <select
                  value={newStopType}
                  onChange={(e) => setNewStopType(e.target.value as "Pickup" | "Dropoff" | "Waypoint")}
                  className="mt-1 w-full rounded-lg border border-line px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
                >
                  <option value="Pickup">Pickup</option>
                  <option value="Dropoff">Dropoff</option>
                  <option value="Waypoint">Waypoint</option>
                </select>
              </div>
              <div>
                <label className="text-xs font-medium text-ink-600">Location</label>
                <input
                  value={newStopLocation}
                  onChange={(e) => setNewStopLocation(e.target.value)}
                  placeholder="e.g., Ikeja Mall"
                  className="mt-1 w-full rounded-lg border border-line px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
                />
              </div>
              <div className="sm:col-span-2">
                <label className="text-xs font-medium text-ink-600">Address</label>
                <input
                  value={newStopAddress}
                  onChange={(e) => setNewStopAddress(e.target.value)}
                  placeholder="e.g., 42 Awolowo Way, Ikeja"
                  className="mt-1 w-full rounded-lg border border-line px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
                />
              </div>
              <div>
                <label className="text-xs font-medium text-ink-600">Time window (optional)</label>
                <input
                  value={newStopTimeWindow}
                  onChange={(e) => setNewStopTimeWindow(e.target.value)}
                  placeholder="e.g., 10:00 - 11:00"
                  className="mt-1 w-full rounded-lg border border-line px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
                />
              </div>
            </div>
            <div className="mt-3 flex gap-2.5">
              <button
                onClick={handleAddStop}
                className="flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
              >
                <Plus className="h-4 w-4" />
                Add stop
              </button>
              <button
                onClick={() => setIsAddingStop(false)}
                className="rounded-md border border-line bg-white px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {/* Stops list */}
        {stops.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <div className="rounded-full bg-surface-2 p-4 mb-3">
              <MapPin className="h-8 w-8 text-ink-400" />
            </div>
            <p className="text-sm font-medium text-ink-950">No stops added yet</p>
            <p className="text-sm text-ink-500">Click "Add stop" to start building your route</p>
          </div>
        ) : (
          <div className="divide-y divide-line">
            {stops.map((stop, index) => {
              const isFirst = index === 0;
              const isLast = index === stops.length - 1;
              const style = TYPE_STYLES[stop.type];
              const Icon = style.icon;

              return (
                <div
                  key={stop.id}
                  className="flex items-center gap-4 px-4 py-3 transition-colors hover:bg-surface-2 group"
                >
                  {/* Drag + Number */}
                  <div className="flex items-center gap-2 flex-shrink-0">
                    <div className="text-ink-300 cursor-grab opacity-50 group-hover:opacity-100 transition-opacity">
                      <GripVertical className="h-4 w-4" />
                    </div>
                    <div
                      className={`flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold ${style.bg} ${style.text}`}
                    >
                      {index + 1}
                    </div>
                  </div>

                  {/* Stop details */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-semibold text-ink-950">
                        {stop.type}: {stop.location}
                      </span>
                      <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                        {stop.status}
                      </span>
                    </div>
                    <div className="text-xs text-ink-500">{stop.address}</div>
                    {stop.timeWindow && (
                      <div className="flex items-center gap-1 text-[10px] text-ink-400">
                        <Clock className="h-3 w-3" />
                        {stop.timeWindow}
                      </div>
                    )}
                  </div>

                  {/* Actions */}
                  <div className="flex items-center gap-0.5 opacity-60 group-hover:opacity-100 transition-opacity">
                    <button
                      onClick={() => handleMoveStop(index, "up")}
                      disabled={isFirst}
                      className={`p-1.5 rounded-md transition-colors ${
                        isFirst
                          ? "text-ink-200 cursor-not-allowed"
                          : "text-ink-400 hover:bg-surface-3 hover:text-ink-600"
                      }`}
                    >
                      <MoveUp className="h-4 w-4" />
                    </button>
                    <button
                      onClick={() => handleMoveStop(index, "down")}
                      disabled={isLast}
                      className={`p-1.5 rounded-md transition-colors ${
                        isLast
                          ? "text-ink-200 cursor-not-allowed"
                          : "text-ink-400 hover:bg-surface-3 hover:text-ink-600"
                      }`}
                    >
                      <MoveDown className="h-4 w-4" />
                    </button>
                    <button
                      onClick={() => handleRemoveStop(index)}
                      className="p-1.5 rounded-md text-ink-400 transition-colors hover:bg-red-bg hover:text-red"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {/* Summary footer */}
        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-line px-4 py-3 text-xs text-ink-500">
          <span className="font-medium">{stops.length} stops in route</span>
          <div className="flex items-center gap-4">
            <span className="flex items-center gap-1.5">
              <span className="h-2 w-2 rounded-full bg-primary" />
              Pickup
            </span>
            <span className="flex items-center gap-1.5">
              <span className="h-2 w-2 rounded-full bg-green" />
              Dropoff
            </span>
            <span className="flex items-center gap-1.5">
              <span className="h-2 w-2 rounded-full bg-ink-400" />
              Waypoint
            </span>
          </div>
        </div>
      </div>

      {/* ── Route Info Cards ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="text-xs font-medium uppercase tracking-wide text-ink-500">Total distance</div>
          <div className="mt-1 text-base font-semibold text-ink-950">{route.distance}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="text-xs font-medium uppercase tracking-wide text-ink-500">Est. duration</div>
          <div className="mt-1 text-base font-semibold text-ink-950">{route.duration}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="text-xs font-medium uppercase tracking-wide text-ink-500">ETA</div>
          <div className="mt-1 text-base font-semibold text-ink-950">{route.eta}</div>
        </div>
      </div>
    </div>
  );
}