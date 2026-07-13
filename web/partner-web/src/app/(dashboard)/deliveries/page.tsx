"use client";

import { useState } from "react";
import Link from "next/link";
import { MoreVertical, Search } from "lucide-react";
import { cn } from "@/lib/utils";

type DeliveryStatus = "Upcoming" | "In Transit" | "Completed" | "Cancelled";

interface Delivery {
  id: string;
  date: string;
  customer: string;
  pickup: string;
  dropoff: string;
  driver: string;
  driverTenure: string;
  driverTrips: string;
  fare: string;
  status: DeliveryStatus;
}

const MOCK_DELIVERIES: Delivery[] = [
  {
    id: "1234445",
    date: "06 Mar 2026, 11:59",
    customer: "Anthonia Dipson",
    pickup: "24 Awawu str, Abaranje Ikotun lagos.",
    dropoff: "23 Akeju Victoria Island",
    driver: "Jenny Wilson",
    driverTenure: "1 year",
    driverTrips: "1,000+",
    fare: "\u20A67.00",
    status: "Upcoming",
  },
  {
    id: "1234446",
    date: "06 Mar 2026, 09:15",
    customer: "Michael Osei",
    pickup: "12 Allen Avenue, Ikeja Lagos.",
    dropoff: "5 Admiralty Way, Lekki Phase 1",
    driver: "David Okon",
    driverTenure: "2 years",
    driverTrips: "2,300+",
    fare: "\u20A612.50",
    status: "In Transit",
  },
  {
    id: "1234447",
    date: "05 Mar 2026, 18:40",
    customer: "Grace Nnamdi",
    pickup: "3 Bode Thomas str, Surulere Lagos.",
    dropoff: "14 Chevron Drive, Lekki",
    driver: "Jenny Wilson",
    driverTenure: "1 year",
    driverTrips: "1,000+",
    fare: "\u20A69.00",
    status: "Completed",
  },
  {
    id: "1234448",
    date: "05 Mar 2026, 14:05",
    customer: "Tunde Bakare",
    pickup: "7 Marina str, Lagos Island.",
    dropoff: "21 Ozumba Mbadiwe, Victoria Island",
    driver: "David Okon",
    driverTenure: "2 years",
    driverTrips: "2,300+",
    fare: "\u20A66.20",
    status: "Cancelled",
  },
];

const STATUS_STYLES: Record<DeliveryStatus, string> = {
  Upcoming: "bg-brand-100 text-brand-700",
  "In Transit": "bg-amber-100 text-warning",
  Completed: "bg-ink-100 text-ink-700",
  Cancelled: "bg-red-100 text-danger",
};

export default function DeliveriesPage() {
  const [query, setQuery] = useState("");
  const [date, setDate] = useState("");
  const [status, setStatus] = useState("");

  const filtered = MOCK_DELIVERIES.filter((delivery) => {
    const matchesQuery = [delivery.customer, delivery.id, delivery.driver].some(
      (field) => field.toLowerCase().includes(query.toLowerCase())
    );
    const matchesStatus = status ? delivery.status === status : true;
    return matchesQuery && matchesStatus;
  });

  return (
    <div className="space-y-6">
      <div className="rounded-2xl bg-white p-5">
        <h1 className="text-lg font-bold text-ink-900">Delivery List</h1>
        <p className="mt-1 text-sm text-ink-500">Here&apos;s what&apos;s on your counter today.</p>
      </div>

      <div className="flex items-center gap-2 rounded-xl bg-ink-100 px-4 py-3">
        <Search className="h-4 w-4 shrink-0 text-ink-500" />
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          type="text"
          placeholder="Search"
          className="w-full bg-transparent text-sm text-ink-900 placeholder:text-ink-500 focus:outline-none"
        />
      </div>

      <div className="rounded-2xl bg-brand-100 p-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 sm:items-end content:grid-cols-[1fr_1fr_auto_auto]">
          <div>
            <label className="mb-2 block text-sm font-medium text-ink-900">Date</label>
            <input
              type="date"
              value={date}
              onChange={(e) => setDate(e.target.value)}
              className="w-full rounded-xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 focus:border-brand-500 focus:outline-none"
            />
          </div>
          <div>
            <label className="mb-2 block text-sm font-medium text-ink-900">Status</label>
            <select
              value={status}
              onChange={(e) => setStatus(e.target.value)}
              className="w-full rounded-xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 focus:border-brand-500 focus:outline-none"
            >
              <option value="">All</option>
              <option value="Upcoming">Upcoming</option>
              <option value="In Transit">In Transit</option>
              <option value="Completed">Completed</option>
              <option value="Cancelled">Cancelled</option>
            </select>
          </div>
          <button
            type="button"
            className="h-fit rounded-xl bg-brand-600 px-5 py-3 text-sm font-semibold text-white hover:bg-brand-700"
          >
            Apply Filter
          </button>
          <button
            type="button"
            onClick={() => {
              setDate("");
              setStatus("");
            }}
            className="h-fit rounded-xl bg-brand-200 px-5 py-3 text-sm font-semibold text-brand-700 hover:bg-brand-300"
          >
            Reset
          </button>
        </div>
      </div>

      <div className="flex items-center justify-between">
        <h2 className="text-base font-bold text-ink-900">Delivery List</h2>
        <Link href="#" className="text-sm font-semibold text-brand-600 hover:text-brand-700">
          View All
        </Link>
      </div>

      <div className="space-y-4">
        {filtered.map((delivery, i) => (
          <Link
            key={`${delivery.id}-${i}`}
            href={`/deliveries/${delivery.id}`}
            className="block rounded-2xl bg-white p-4 transition-shadow hover:shadow-md"
          >
            <div className="flex items-start justify-between">
              <div>
                <p className="text-sm font-bold text-ink-900">#{delivery.id}</p>
                <p className="mt-0.5 text-xs text-ink-500">{delivery.date}</p>
              </div>
              <button
                type="button"
                aria-label="More options"
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                }}
                className="text-ink-500 hover:text-ink-700"
              >
                <MoreVertical className="h-5 w-5" />
              </button>
            </div>

            <p className="mt-3 text-xs font-semibold text-brand-600">Customer</p>
            <p className="text-sm font-semibold text-ink-900">{delivery.customer}</p>

            <div className="mt-3 flex gap-3">
              <div className="flex flex-col items-center pt-1">
                <span className="h-2.5 w-2.5 shrink-0 rounded-full bg-brand-500 ring-4 ring-brand-100" />
                <span className="my-1 w-px flex-1 bg-brand-200" />
                <span className="h-2.5 w-2.5 shrink-0 rounded-full bg-brand-500 ring-4 ring-brand-100" />
              </div>
              <div className="flex-1 space-y-3 text-sm">
                <div>
                  <p className="text-xs text-ink-500">Pick-up</p>
                  <p className="font-medium text-ink-900">{delivery.pickup}</p>
                </div>
                <div>
                  <p className="text-xs text-ink-500">Drop off</p>
                  <p className="font-medium text-ink-900">{delivery.dropoff}</p>
                </div>
              </div>
            </div>

            <p className="mt-3 text-xs font-semibold text-brand-600">Assigned Driver</p>
            <div className="mt-1 flex items-center gap-3">
              <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-brand-400 text-sm font-semibold text-white">
                {delivery.driver.charAt(0)}
              </span>
              <div>
                <p className="text-sm font-semibold text-ink-900">{delivery.driver}</p>
                <p className="text-xs text-ink-500">
                  Driving with KarryGo &middot; {delivery.driverTenure} &nbsp;&middot;&nbsp; Completed Trips: {delivery.driverTrips}
                </p>
              </div>
            </div>

            <div className="mt-4 flex items-center justify-between">
              <p className="text-sm font-bold text-ink-900">{delivery.fare}</p>
              <span className={cn("rounded-full px-3 py-1 text-xs font-semibold", STATUS_STYLES[delivery.status])}>
                {delivery.status}
              </span>
            </div>
          </Link>
        ))}

        {filtered.length === 0 && (
          <p className="rounded-2xl bg-white p-6 text-center text-sm text-ink-500">
            No deliveries match your filters.
          </p>
        )}
      </div>
    </div>
  );
}