// src/app/(dashboard)/operations/trips/[tripId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import {
  MapPin,
  Truck,
  Package,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  User,
  Calendar,
  Phone,
  Mail,
  Flag,
  Navigation,
} from "lucide-react";

type TripStop = {
  id: string;
  type: "Pickup" | "Dropoff";
  location: string;
  address: string;
  status: "Pending" | "Completed" | "Skipped";
  arrivalTime?: string;
  departureTime?: string;
  proof?: string;
};

type Trip = {
  id: string;
  assignmentId: string;
  provider: {
    id: string;
    name: string;
    phone: string;
    email: string;
    rating: number;
  };
  asset: {
    id: string;
    name: string;
    type: string;
  };
  service: "Dispatch" | "Hauling";
  branch: string;
  status: "Scheduled" | "En Route" | "At Pickup" | "In Transit" | "At Dropoff" | "Completed" | "Exception" | "Cancelled";
  pickup: string;
  dropoff: string;
  stops: TripStop[];
  eta: string;
  distance: string;
  duration: string;
  lastUpdated: string;
  createdAt: string;
  instructions: string;
  exceptions?: {
    type: string;
    description: string;
    severity: "Low" | "Medium" | "High";
  }[];
};

// Mock data
const MOCK_TRIP: Trip = {
  id: "TRP-8491",
  assignmentId: "ASN-8491",
  provider: {
    id: "P-044",
    name: "Peter Okafor",
    phone: "+234 802 345 6789",
    email: "peter.okafor@example.com",
    rating: 4.8,
  },
  asset: {
    id: "BR-88",
    name: "Honda ACE 110",
    type: "Motorcycle",
  },
  service: "Dispatch",
  branch: "Ikeja",
  status: "In Transit",
  pickup: "Ikeja",
  dropoff: "Lekki",
  stops: [
    {
      id: "STOP-001",
      type: "Pickup",
      location: "Ikeja Mall",
      address: "42 Awolowo Way, Ikeja",
      status: "Completed",
      arrivalTime: "14:15",
      departureTime: "14:20",
      proof: "Photo captured",
    },
    {
      id: "STOP-002",
      type: "Dropoff",
      location: "Mushin",
      address: "15 Ogunlana Drive, Mushin",
      status: "Completed",
      arrivalTime: "14:35",
      departureTime: "14:38",
      proof: "Signature collected",
    },
    {
      id: "STOP-003",
      type: "Pickup",
      location: "Oshodi",
      address: "8 Oshodi-Apapa Expressway",
      status: "Pending",
    },
    {
      id: "STOP-004",
      type: "Dropoff",
      location: "Lekki Phase 1",
      address: "3 Admiralty Way, Lekki",
      status: "Pending",
    },
  ],
  eta: "18 min",
  distance: "8.4 km",
  duration: "24 min",
  lastUpdated: "12 sec ago",
  createdAt: "2026-08-17T14:10:00Z",
  instructions: "Priority delivery - handle with care. Contact recipient 15 min before arrival.",
  exceptions: [
    {
      type: "Traffic delay",
      description: "Heavy traffic on Oshodi-Apapa Expressway",
      severity: "Medium",
    },
  ],
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Scheduled: {
    bg: "bg-primary-bg",
    text: "text-primary",
    dot: "bg-primary",
    icon: <Calendar className="h-3.5 w-3.5" />,
  },
  "En Route": {
    bg: "bg-primary-bg",
    text: "text-primary",
    dot: "bg-primary",
    icon: <Navigation className="h-3.5 w-3.5" />,
  },
  "At Pickup": {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <MapPin className="h-3.5 w-3.5" />,
  },
  "In Transit": {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  "At Dropoff": {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <MapPin className="h-3.5 w-3.5" />,
  },
  Completed: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Exception: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Cancelled: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
};

const STOP_STATUS_STYLES = {
  Completed: { bg: "bg-green-bg", text: "text-green", icon: CheckCircle2 },
  Pending: { bg: "bg-surface-2", text: "text-ink-500", icon: Clock },
  Skipped: { bg: "bg-amber-bg", text: "text-amber", icon: XCircle },
};

export default function TripDetailPage() {
  const params = useParams();
  const router = useRouter();
  const tripId = params.tripId as string;

  const [loading, setLoading] = useState(true);
  const [trip, setTrip] = useState<Trip | null>(null);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setTrip(MOCK_TRIP);
      setLoading(false);
    };
    loadData();
  }, [tripId]);

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading trip details...</span>
        </div>
      </div>
    );
  }

  if (!trip) {
    return (
      <div className="py-12">
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <AlertCircle className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">Trip not found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            The trip you're looking for doesn't exist or has been removed.
          </p>
          <Link
            href="/operations/trips"
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            Back to trips
          </Link>
        </div>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[trip.status] || STATUS_STYLES.Scheduled;
  const isActive = ["Scheduled", "En Route", "At Pickup", "In Transit", "At Dropoff"].includes(trip.status);
  const isException = trip.status === "Exception";
  const completedStops = trip.stops.filter((s) => s.status === "Completed").length;
  const totalStops = trip.stops.length;
  const progress = Math.round((completedStops / totalStops) * 100);

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2.5 flex-wrap">
            <h1>{trip.id}</h1>
            <span
              className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusStyle.bg} ${statusStyle.text}`}
            >
              {statusStyle.icon}
              {trip.status}
            </span>
            {isActive && (
              <span className="flex items-center gap-1 rounded-full bg-green-bg px-2 py-0.5 text-[10px] font-semibold text-green">
                <span className="relative flex h-1.5 w-1.5">
                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                  <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green" />
                </span>
                Live
              </span>
            )}
            {isException && (
              <span className="flex items-center gap-1 rounded-full bg-red-bg px-2 py-0.5 text-[10px] font-semibold text-red">
                <AlertCircle className="h-3 w-3" />
                Exception
              </span>
            )}
          </div>
          <p className="mt-0.5 text-sm text-ink-500">
            {trip.service} trip · {trip.branch} branch · Assignment{" "}
            <Link
              href={`/operations/assignments/${trip.assignmentId}`}
              className="font-mono text-primary hover:underline"
            >
              {trip.assignmentId}
            </Link>
          </p>
        </div>
        {isActive && (
          <Link
            href={`/operations/control-tower`}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Navigation className="h-3.5 w-3.5" />
            Track live
          </Link>
        )}
      </div>

      {/* ── Progress bar ── */}
      <div className="rounded-lg border border-line bg-white p-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-xs font-medium text-ink-600">Progress</span>
            <span className="text-base font-semibold text-ink-950">{progress}%</span>
          </div>
          <span className="text-xs text-ink-500">
            {completedStops} of {totalStops} stops completed
          </span>
        </div>
        <div className="mt-2 h-2 rounded-full bg-surface-3 overflow-hidden">
          <div
            className={`h-full rounded-full transition-all ${
              progress === 100 ? "bg-green" : isException ? "bg-red" : "bg-primary"
            }`}
            style={{ width: `${progress}%` }}
          />
        </div>
      </div>

      {/* ── Main content ── */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        {/* Left column - 2/3 */}
        <div className="space-y-5 lg:col-span-2">
          {/* Route & Stops */}
          <div className="rounded-lg border border-line bg-white">
            <div className="flex items-center gap-2 border-b border-line px-4 py-3">
              <MapPin className="h-4 w-4 text-ink-400" />
              <span className="text-sm font-semibold text-ink-950">Route & stops</span>
            </div>

            <div className="divide-y divide-line">
              {trip.stops.map((stop, index) => {
                const stopStyle = STOP_STATUS_STYLES[stop.status] || STOP_STATUS_STYLES.Pending;
                const StopIcon = stopStyle.icon;
                const isLast = index === trip.stops.length - 1;

                return (
                  <div key={stop.id} className="px-4 py-3">
                    <div className="flex gap-3">
                      <div className="flex flex-col items-center">
                        <div
                          className={`flex h-7 w-7 items-center justify-center rounded-full text-[10px] font-bold ${
                            stop.type === "Pickup"
                              ? "bg-primary-bg text-primary"
                              : "bg-green-bg text-green"
                          }`}
                        >
                          {index + 1}
                        </div>
                        {!isLast && <div className="mt-1 h-6 w-px bg-line" />}
                      </div>
                      <div className="flex-1">
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className="text-sm font-semibold text-ink-950">
                            {stop.type}: {stop.location}
                          </span>
                          <span
                            className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${stopStyle.bg} ${stopStyle.text}`}
                          >
                            <StopIcon className="h-3 w-3" />
                            {stop.status}
                          </span>
                        </div>
                        <div className="mt-0.5 text-xs text-ink-500">{stop.address}</div>
                        {(stop.arrivalTime || stop.departureTime) && (
                          <div className="mt-1 flex flex-wrap items-center gap-3 text-[10px] text-ink-500">
                            {stop.arrivalTime && (
                              <span>Arrived: {stop.arrivalTime}</span>
                            )}
                            {stop.departureTime && (
                              <span>Departed: {stop.departureTime}</span>
                            )}
                            {stop.proof && (
                              <span className="text-primary">✓ {stop.proof}</span>
                            )}
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>

            <div className="flex flex-wrap items-center gap-4 border-t border-line px-4 py-3 text-xs text-ink-500">
              <span><span className="font-medium text-ink-700">Distance:</span> {trip.distance}</span>
              <span><span className="font-medium text-ink-700">Est. duration:</span> {trip.duration}</span>
              <span><span className="font-medium text-ink-700">ETA:</span> {trip.eta}</span>
            </div>
          </div>

          {/* Exceptions */}
          {trip.exceptions && trip.exceptions.length > 0 && (
            <div className="rounded-lg border border-red/20 bg-red-bg/20 p-4">
              <div className="flex items-center gap-2">
                <AlertCircle className="h-4 w-4 text-red" />
                <span className="text-sm font-semibold text-red">Exceptions</span>
              </div>
              <div className="mt-2 space-y-2">
                {trip.exceptions.map((exc, index) => (
                  <div
                    key={index}
                    className={`rounded-lg border p-2.5 ${
                      exc.severity === "High"
                        ? "border-red/30 bg-red-bg/30"
                        : exc.severity === "Medium"
                        ? "border-amber/30 bg-amber-bg/20"
                        : "border-primary/30 bg-primary-bg/20"
                    }`}
                  >
                    <div className="flex items-center gap-2">
                      <span
                        className={`text-[10px] font-bold uppercase ${
                          exc.severity === "High"
                            ? "text-red"
                            : exc.severity === "Medium"
                            ? "text-amber"
                            : "text-primary"
                        }`}
                      >
                        {exc.severity}
                      </span>
                      <span className="text-xs font-semibold text-ink-950">{exc.type}</span>
                    </div>
                    <p className="mt-0.5 text-xs text-ink-600">{exc.description}</p>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Instructions */}
          {trip.instructions && (
            <div className="rounded-lg border border-line bg-white p-4">
              <span className="text-xs font-medium uppercase tracking-wide text-ink-500">Instructions</span>
              <p className="mt-1 text-sm text-ink-700">{trip.instructions}</p>
            </div>
          )}
        </div>

        {/* Right column - 1/3 */}
        <div className="space-y-5">
          {/* Provider card */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="flex items-center gap-2 mb-3">
              <User className="h-4 w-4 text-ink-400" />
              <span className="text-sm font-semibold text-ink-950">Provider</span>
            </div>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary-bg text-sm font-bold text-primary">
                {trip.provider.name.split(" ").map((n) => n[0]).join("")}
              </div>
              <div>
                <div className="text-sm font-semibold text-ink-950">{trip.provider.name}</div>
                <div className="flex items-center gap-2 text-xs text-ink-500">
                  <span className="flex items-center gap-0.5">
                    <span className="text-amber">★</span>
                    {trip.provider.rating}
                  </span>
                  <span className="text-ink-300">·</span>
                  <span className="font-mono text-[10px]">{trip.provider.id}</span>
                </div>
              </div>
            </div>
            <div className="mt-3 space-y-1.5 border-t border-line pt-3 text-xs text-ink-600">
              <div className="flex items-center gap-2">
                <Phone className="h-3.5 w-3.5 text-ink-400" />
                {trip.provider.phone}
              </div>
              <div className="flex items-center gap-2">
                <Mail className="h-3.5 w-3.5 text-ink-400" />
                {trip.provider.email}
              </div>
            </div>
            <div className="mt-3 flex gap-1.5">
              <button className="flex-1 rounded-md border border-line bg-white px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors">
                <Phone className="inline h-3.5 w-3.5 mr-1" />
                Call
              </button>
              <button className="flex-1 rounded-md border border-line bg-white px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors">
                <Package className="inline h-3.5 w-3.5 mr-1" />
                View
              </button>
            </div>
          </div>

          {/* Asset card */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="flex items-center gap-2 mb-3">
              <Truck className="h-4 w-4 text-ink-400" />
              <span className="text-sm font-semibold text-ink-950">Asset</span>
            </div>
            <div>
              <div className="text-sm font-semibold text-ink-950">{trip.asset.name}</div>
              <div className="flex items-center gap-2 text-xs text-ink-500">
                <span className="font-mono text-[10px]">{trip.asset.id}</span>
                <span className="text-ink-300">·</span>
                <span>{trip.asset.type}</span>
              </div>
            </div>
          </div>

          {/* Trip metadata */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="space-y-2 text-xs">
              <div className="flex justify-between">
                <span className="text-ink-500">Created</span>
                <span className="font-medium text-ink-950">
                  {new Date(trip.createdAt).toLocaleString()}
                </span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Last updated</span>
                <span className="font-medium text-ink-950">{trip.lastUpdated}</span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Distance</span>
                <span className="font-medium text-ink-950">{trip.distance}</span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Est. duration</span>
                <span className="font-medium text-ink-950">{trip.duration}</span>
              </div>
            </div>
          </div>

          {/* Actions */}
          {isActive && (
            <div className="rounded-lg border border-line bg-white p-4">
              <span className="text-sm font-semibold text-ink-950 mb-3 block">Actions</span>
              <div className="space-y-2">
                <button className="w-full rounded-md bg-primary px-3 py-2 text-xs font-semibold text-white hover:bg-primary-hover transition-colors">
                  <Navigation className="inline h-3.5 w-3.5 mr-1.5" />
                  Live track
                </button>
                <button className="w-full rounded-md border border-line bg-white px-3 py-2 text-xs font-medium text-ink-700 hover:bg-surface-2 transition-colors">
                  <Flag className="inline h-3.5 w-3.5 mr-1.5" />
                  Report exception
                </button>
                <button className="w-full rounded-md border border-red/30 bg-red-bg px-3 py-2 text-xs font-semibold text-red hover:bg-red-bg/70 transition-colors">
                  <XCircle className="inline h-3.5 w-3.5 mr-1.5" />
                  Cancel trip
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}