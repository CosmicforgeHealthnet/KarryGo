"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { cn } from "@/lib/utils";
import { deliveryService } from "@/services/delivery-service";
import { useAuthStore } from "@/store/auth-store";
import { ApiError } from "@/lib/api-client";
import type { DeliveryDetail } from "@/types";

const STATUS_STYLES: Record<string, string> = {
  pending: "bg-ink-100 text-ink-500",
  dispatching: "bg-amber-100 text-warning",
  accepted: "bg-brand-100 text-brand-700",
  arrived: "bg-brand-100 text-brand-700",
  in_progress: "bg-amber-100 text-warning",
  completed: "bg-ink-100 text-ink-700",
  cancelled: "bg-red-100 text-danger",
  failed: "bg-red-100 text-danger",
  no_provider: "bg-red-100 text-danger",
};

function statusLabel(status: string): string {
  return status
    .split("_")
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-GB");
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-sm font-bold text-ink-900">{label}</p>
      <p className="mt-1 text-sm text-ink-700">{value}</p>
    </div>
  );
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h2 className="text-base font-bold text-brand-600">{children}</h2>;
}

export default function DeliveryDetailPage() {
  const router = useRouter();
  const params = useParams<{ id: string }>();
  const token = useAuthStore((s) => s.token);

  const [detail, setDetail] = useState<DeliveryDetail | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!token) {
      setIsLoading(false);
      return;
    }
    let cancelled = false;

    async function load() {
      setIsLoading(true);
      setError(null);
      try {
        const res = await deliveryService.get(token!, params.id);
        if (!cancelled) setDetail(res);
      } catch (err) {
        if (cancelled) return;
        setError(
          err instanceof ApiError
            ? err.message
            : "Couldn't load this delivery. Please try again."
        );
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    }

    load();
    return () => {
      cancelled = true;
    };
  }, [token, params.id]);

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={() => router.back()}
          aria-label="Go back"
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-white text-ink-900"
        >
          <ArrowLeft className="h-5 w-5" />
        </button>
        <h1 className="flex-1 rounded-2xl bg-white px-5 py-4 text-lg font-bold text-ink-900">
          Delivery Details
        </h1>
      </div>

      {isLoading ? (
        <div className="h-96 animate-pulse rounded-2xl bg-white/60" />
      ) : error ? (
        <div className="rounded-2xl border border-red-100 bg-red-50 p-5 text-sm text-red-600">
          {error}
        </div>
      ) : detail ? (
        <div className="space-y-6 rounded-2xl bg-white p-5">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label="Booking Ref" value={`#${detail.booking_ref}`} />
            <div>
              <p className="text-sm font-bold text-ink-900">Status</p>
              <span
                className={cn(
                  "mt-1 inline-block rounded-full px-3 py-1 text-xs font-semibold",
                  STATUS_STYLES[detail.status] ?? "bg-ink-100 text-ink-700"
                )}
              >
                {statusLabel(detail.status)}
              </span>
            </div>
            <Field label="Created Date" value={formatDate(detail.created_at)} />
          </div>

          <div className="space-y-4">
            <SectionTitle>Pickup Information</SectionTitle>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <Field
                label="Contact Name"
                value={detail.pickup.contact_name ?? "Not provided"}
              />
              <Field
                label="Phone Number"
                value={detail.pickup.contact_phone ?? "Not provided"}
              />
              <Field label="Address" value={detail.pickup.address} />
            </div>
          </div>

          <div className="space-y-4">
            <SectionTitle>Drop-off Information</SectionTitle>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <Field label="Contact Name" value={detail.receiver.name} />
              <Field label="Phone Number" value={detail.receiver.phone} />
              <Field label="Address" value={detail.dropoff.address} />
            </div>
          </div>

          <div className="space-y-4">
            <SectionTitle>Delivery Information</SectionTitle>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <Field label="Fare" value={detail.fare.display} />
              <Field
                label="Distance"
                value={
                  detail.distance_km != null
                    ? `${detail.distance_km.toFixed(1)} km`
                    : "Not available"
                }
              />
              <Field
                label="Package Description"
                value={detail.package.description ?? "Not provided"}
              />
            </div>
          </div>

          <div className="space-y-4">
            <SectionTitle>Assigned Rider</SectionTitle>
            {detail.rider.id ? (
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                <Field label="Rider Name" value={detail.rider.name ?? "—"} />
                <Field label="Rider Phone" value={detail.rider.phone ?? "—"} />
                <div>
                  <p className="text-sm font-bold text-ink-900">Vehicle</p>
                  <p className="mt-1 text-sm text-ink-700">
                    {detail.rider.vehicle_color ?? "—"} &middot;{" "}
                    {detail.rider.vehicle_model ?? "—"}{" "}
                    <span className="font-semibold text-brand-600">
                      {detail.rider.plate_number ?? ""}
                    </span>
                  </p>
                </div>
              </div>
            ) : (
              <p className="text-sm text-ink-500">
                No rider assigned yet.
              </p>
            )}
          </div>
        </div>
      ) : null}
    </div>
  );
}