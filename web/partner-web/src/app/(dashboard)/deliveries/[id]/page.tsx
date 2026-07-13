"use client";

import { useParams, useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { cn } from "@/lib/utils";

interface DeliveryDetail {
  status: string;
  createdDate: string;
  pickupContact: string;
  pickupPhone: string;
  pickupAddress: string;
  dropoffContact: string;
  dropoffPhone: string;
  dropoffAddress: string;
  fare: string;
  distance: string;
  packageDescription: string;
  riderName: string;
  riderPhone: string;
  vehicleColor: string;
  vehicleModel: string;
  plateNumber: string;
}

const MOCK_DELIVERY_DETAILS: Record<string, DeliveryDetail> = {
  "1234445": {
    status: "Upcoming",
    createdDate: "21/06/2026",
    pickupContact: "Dema",
    pickupPhone: "+2343434567890",
    pickupAddress: "No 34, Adetokunbo street, Ikeja Lagos",
    dropoffContact: "Dema",
    dropoffPhone: "+2343434567890",
    dropoffAddress: "No 34, Adetokunbo street, Ikeja Lagos",
    fare: "$70.0",
    distance: "21 km",
    packageDescription: "Medication package",
    riderName: "Dema",
    riderPhone: "+2343434567890",
    vehicleColor: "Red",
    vehicleModel: "Toyota Camry",
    plateNumber: "EKY232GY",
  },
};

const FALLBACK_DETAIL: DeliveryDetail = MOCK_DELIVERY_DETAILS["1234445"];

const STATUS_STYLES: Record<string, string> = {
  Upcoming: "bg-brand-100 text-brand-700",
  "In Transit": "bg-amber-100 text-warning",
  Completed: "bg-ink-100 text-ink-700",
  Cancelled: "bg-red-100 text-danger",
};

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
  const detail = MOCK_DELIVERY_DETAILS[params.id] ?? FALLBACK_DETAIL;

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

      <div className="space-y-6 rounded-2xl bg-white p-5">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <Field label="Delivery ID" value={`#${params.id}`} />
          <div>
            <p className="text-sm font-bold text-ink-900">Status</p>
            <span
              className={cn(
                "mt-1 inline-block rounded-full px-3 py-1 text-xs font-semibold",
                STATUS_STYLES[detail.status] ?? STATUS_STYLES.Upcoming
              )}
            >
              {detail.status}
            </span>
          </div>
          <Field label="Created Date" value={detail.createdDate} />
        </div>

        <div className="space-y-4">
          <SectionTitle>Pickup Information</SectionTitle>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label="Contact Name" value={detail.pickupContact} />
            <Field label="Phone Number" value={detail.pickupPhone} />
            <Field label="Address" value={detail.pickupAddress} />
          </div>
        </div>

        <div className="space-y-4">
          <SectionTitle>Drop-off Information</SectionTitle>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label="Contact Name" value={detail.dropoffContact} />
            <Field label="Phone Number" value={detail.dropoffPhone} />
            <Field label="Address" value={detail.dropoffAddress} />
          </div>
        </div>

        <div className="space-y-4">
          <SectionTitle>Delivery Information</SectionTitle>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label="Fare" value={detail.fare} />
            <Field label="Distance" value={detail.distance} />
            <Field label="Package Description" value={detail.packageDescription} />
          </div>
        </div>

        <div className="space-y-4">
          <SectionTitle>Assigned Rider</SectionTitle>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label="Rider Name" value={detail.riderName} />
            <Field label="Rider Phone" value={detail.riderPhone} />
            <div>
              <p className="text-sm font-bold text-ink-900">Vehicle</p>
              <p className="mt-1 text-sm text-ink-700">
                {detail.vehicleColor} &middot; {detail.vehicleModel}{" "}
                <span className="font-semibold text-brand-600">{detail.plateNumber}</span>
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}