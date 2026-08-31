// src/app/(dashboard)/operations/dispatch/plans/create/page.tsx

"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  Calendar,
  MapPin,
  Users,
  Clock,
  Save,
  X,
  ChevronDown,
} from "lucide-react";

const BRANCHES = ["Ikeja", "Lekki", "Yaba", "Apapa"];
const TIME_SLOTS = [
  "06:00 – 10:00",
  "09:00 – 12:00",
  "12:00 – 18:00",
  "16:00 – 20:00",
];

export default function CreatePlanPage() {
  const router = useRouter();
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [formData, setFormData] = useState({
    name: "",
    branch: "Ikeja",
    window: "06:00 – 10:00",
    owner: "",
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSubmitting(true);
    await new Promise((resolve) => setTimeout(resolve, 1200));
    setIsSubmitting(false);
    router.push("/operations/dispatch/plans");
  };

  const handleCancel = () => {
    router.push("/operations/dispatch/plans");
  };

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div>
        <h1>Create dispatch plan</h1>
        <p className="mt-1.5">
          Define a new planning window for your branch operations.
        </p>
      </div>

      {/* ── Form ── */}
      <form onSubmit={handleSubmit} className="space-y-4">
        {/* Plan Name */}
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-ink-700">
              Plan name
              <span className="text-red ml-0.5">*</span>
            </label>
            <input
              type="text"
              required
              value={formData.name}
              onChange={(e) =>
                setFormData((prev) => ({ ...prev, name: e.target.value }))
              }
              placeholder="e.g., Ikeja morning wave"
              className="w-full rounded-lg border border-line px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
            />
            <p className="text-xs text-ink-500">
              Give your plan a descriptive name that reflects the branch and time.
            </p>
          </div>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {/* Branch */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-ink-700">
                Branch
                <span className="text-red ml-0.5">*</span>
              </label>
              <div className="relative">
                <select
                  required
                  value={formData.branch}
                  onChange={(e) =>
                    setFormData((prev) => ({ ...prev, branch: e.target.value }))
                  }
                  className="w-full appearance-none rounded-lg border border-line px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                >
                  {BRANCHES.map((branch) => (
                    <option key={branch} value={branch}>
                      {branch}
                    </option>
                  ))}
                </select>
                <ChevronDown className="absolute right-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-400 pointer-events-none" />
              </div>
            </div>
          </div>

          {/* Time Window */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-ink-700">
                Time window
                <span className="text-red ml-0.5">*</span>
              </label>
              <div className="relative">
                <select
                  required
                  value={formData.window}
                  onChange={(e) =>
                    setFormData((prev) => ({ ...prev, window: e.target.value }))
                  }
                  className="w-full appearance-none rounded-lg border border-line px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                >
                  {TIME_SLOTS.map((slot) => (
                    <option key={slot} value={slot}>
                      {slot}
                    </option>
                  ))}
                </select>
                <ChevronDown className="absolute right-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-400 pointer-events-none" />
              </div>
            </div>
          </div>
        </div>

        {/* Owner */}
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-ink-700">
              Plan owner
              <span className="text-red ml-0.5">*</span>
            </label>
            <input
              type="text"
              required
              value={formData.owner}
              onChange={(e) =>
                setFormData((prev) => ({ ...prev, owner: e.target.value }))
              }
              placeholder="e.g., T. Adeyemi"
              className="w-full rounded-lg border border-line px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
            />
          </div>
        </div>

        {/* ── Actions ── */}
        <div className="flex flex-col-reverse gap-2.5 sm:flex-row sm:justify-end">
          <button
            type="button"
            onClick={handleCancel}
            className="flex items-center justify-center gap-2 rounded-lg border border-line bg-white px-5 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <X className="h-4 w-4" />
            Cancel
          </button>
          <button
            type="submit"
            disabled={isSubmitting}
            className="flex items-center justify-center gap-2 rounded-lg bg-primary px-5 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60 disabled:cursor-not-allowed"
          >
            {isSubmitting ? (
              <>
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                Creating...
              </>
            ) : (
              <>
                <Save className="h-4 w-4" />
                Create plan
              </>
            )}
          </button>
        </div>
      </form>
    </div>
  );
}