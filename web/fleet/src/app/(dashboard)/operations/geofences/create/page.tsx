// src/app/(dashboard)/operations/geofences/create/page.tsx

"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import {
  MapPin,
  Save,
  ChevronDown,
  Globe,
  Navigation,
  AlertCircle,
  Layers,
} from "lucide-react";

const TYPE_OPTIONS = [
  { value: "Pickup", label: "Pickup", icon: MapPin },
  { value: "Delivery", label: "Delivery", icon: Navigation },
  { value: "Restricted", label: "Restricted", icon: AlertCircle },
  { value: "Parking", label: "Parking", icon: Layers },
  { value: "Depot", label: "Depot", icon: Globe },
];

const BRANCHES = ["Ikeja", "Lekki", "Yaba", "Apapa"];

export default function CreateGeofencePage() {
  const router = useRouter();
  const [submitting, setSubmitting] = useState(false);
  const [formData, setFormData] = useState({
    name: "",
    type: "Pickup",
    branch: "Ikeja",
    purpose: "",
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!formData.name.trim() || !formData.purpose.trim()) return;

    setSubmitting(true);
    await new Promise((resolve) => setTimeout(resolve, 800));
    setSubmitting(false);

    router.push("/operations/geofences");
  };

  const isFormValid = formData.name.trim() && formData.purpose.trim();

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div>
        <h1>Create Geofence</h1>
        <p className="mt-1.5">Define a new geographic boundary for operations</p>
      </div>

      {/* ── Form ── */}
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="space-y-4">
            {/* Name */}
            <div>
              <label className="mb-1.5 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Geofence Name <span className="text-red">*</span>
              </label>
              <input
                type="text"
                value={formData.name}
                onChange={(e) =>
                  setFormData((prev) => ({ ...prev, name: e.target.value }))
                }
                placeholder="e.g., Ikeja Mall Pickup Zone"
                className="w-full rounded-lg border border-line px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
              />
            </div>

            {/* Type + Branch */}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label className="mb-1.5 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Type <span className="text-red">*</span>
                </label>
                <div className="relative">
                  <select
                    value={formData.type}
                    onChange={(e) =>
                      setFormData((prev) => ({ ...prev, type: e.target.value }))
                    }
                    className="w-full appearance-none rounded-lg border border-line px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {TYPE_OPTIONS.map((opt) => (
                      <option key={opt.value} value={opt.value}>
                        {opt.label}
                      </option>
                    ))}
                  </select>
                  <ChevronDown className="absolute right-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-400 pointer-events-none" />
                </div>
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Branch <span className="text-red">*</span>
                </label>
                <div className="relative">
                  <select
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

            {/* Purpose */}
            <div>
              <label className="mb-1.5 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Purpose <span className="text-red">*</span>
              </label>
              <textarea
                value={formData.purpose}
                onChange={(e) =>
                  setFormData((prev) => ({ ...prev, purpose: e.target.value }))
                }
                rows={3}
                placeholder="Describe the purpose of this geofence..."
                className="w-full rounded-lg border border-line px-3.5 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400 resize-none"
              />
            </div>
          </div>
        </div>

        {/* ── Info Note ── */}
        <div className="rounded-lg border border-primary/20 bg-primary-bg/20 p-4">
          <div className="flex items-start gap-3">
            <MapPin className="h-5 w-5 text-primary flex-shrink-0 mt-0.5" />
            <div>
              <div className="text-sm font-semibold text-ink-950">Geometry Setup</div>
              <p className="mt-0.5 text-sm leading-relaxed text-ink-600">
                Geofence geometry will be defined on the map after creation. You can draw polygons, circles, or import coordinates.
              </p>
            </div>
          </div>
        </div>

        {/* ── Actions ── */}
        <div className="flex justify-end">
          <button
            type="submit"
            disabled={!isFormValid || submitting}
            className="flex items-center gap-2 rounded-lg bg-primary px-6 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60 disabled:cursor-not-allowed"
          >
            {submitting ? (
              <>
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                Creating...
              </>
            ) : (
              <>
                <Save className="h-4 w-4" />
                Create Geofence
              </>
            )}
          </button>
        </div>
      </form>
    </div>
  );
}