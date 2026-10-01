// src/app/(dashboard)/providers/[providerId]/work-setup/page.tsx

"use client";

import { useState } from "react";
import { PROVIDER_DETAIL } from "@/lib/mock-provider-data";
import {
  Truck,
  Package,
  MapPin,
  Clock,
  CheckCircle2,
  AlertCircle,
  Edit3,
  Plus,
  X,
  Save,
  ChevronDown,
  Users,
  Briefcase,
  Settings,
  Shield,
  ArrowRight,
} from "lucide-react";

export default function WorkSetupTab() {
  const provider = PROVIDER_DETAIL;
  const [isEditing, setIsEditing] = useState(false);

  return (
    <div className="space-y-5">
      {/* ── Actions ── */}
      <div className="flex justify-end">
        <button
          onClick={() => setIsEditing(!isEditing)}
          className={`flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-semibold transition-colors ${
            isEditing
              ? "bg-primary text-white hover:bg-primary-hover"
              : "border border-line bg-white text-ink-700 hover:bg-surface-2"
          }`}
        >
          {isEditing ? (
            <>
              <Save className="h-4 w-4" />
              Save Changes
            </>
          ) : (
            <>
              <Edit3 className="h-4 w-4" />
              Edit Setup
            </>
          )}
        </button>
      </div>

      {/* ── Grid ── */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* Service Roles */}
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="flex items-center gap-2.5 mb-4">
            <Briefcase className="h-5 w-5 text-primary" />
            <h3 className="text-sm font-semibold text-ink-950">Service Roles</h3>
          </div>
          <div className="space-y-2.5">
            <div className="flex items-center justify-between rounded-lg border border-line bg-surface-2/50 px-4 py-3">
              <div className="flex items-center gap-3">
                <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary-bg text-primary">
                  <Package className="h-4 w-4" />
                </div>
                <div>
                  <div className="text-sm font-semibold text-ink-950">Dispatch</div>
                  <div className="text-xs text-ink-500">Primary role</div>
                </div>
              </div>
              <span className="rounded-full bg-green-bg px-2.5 py-0.5 text-[10px] font-semibold text-green">
                Active
              </span>
            </div>
            <div className="flex items-center justify-between rounded-lg border border-line bg-white px-4 py-3">
              <div className="flex items-center gap-3">
                <div className="flex h-8 w-8 items-center justify-center rounded-full bg-amber-bg text-amber">
                  <Truck className="h-4 w-4" />
                </div>
                <div>
                  <div className="text-sm font-semibold text-ink-950">Hauling</div>
                  <div className="text-xs text-ink-500">Secondary role</div>
                </div>
              </div>
              <span className="rounded-full bg-surface-2 px-2.5 py-0.5 text-[10px] font-medium text-ink-500">
                Inactive
              </span>
            </div>
            {isEditing && (
              <button className="flex w-full items-center justify-center gap-2 rounded-lg border-2 border-dashed border-line py-3 text-xs font-medium text-ink-500 transition-colors hover:border-primary hover:text-primary">
                <Plus className="h-4 w-4" />
                Add Role
              </button>
            )}
          </div>
        </div>

        {/* Branch Assignment */}
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="flex items-center gap-2.5 mb-4">
            <MapPin className="h-5 w-5 text-primary" />
            <h3 className="text-sm font-semibold text-ink-950">Branch Assignment</h3>
          </div>
          <div className="space-y-2.5">
            <div className="flex items-center justify-between rounded-lg border border-line bg-surface-2/50 px-4 py-3">
              <div>
                <div className="text-sm font-semibold text-ink-950">Ikeja</div>
                <div className="text-xs text-ink-500">Primary branch</div>
              </div>
              <span className="rounded-full bg-green-bg px-2.5 py-0.5 text-[10px] font-semibold text-green">
                Active
              </span>
            </div>
            <div className="flex items-center justify-between rounded-lg border border-line bg-white px-4 py-3">
              <div>
                <div className="text-sm font-semibold text-ink-950">Lekki</div>
                <div className="text-xs text-ink-500">Secondary branch</div>
              </div>
              <span className="rounded-full bg-surface-2 px-2.5 py-0.5 text-[10px] font-medium text-ink-500">
                Pending
              </span>
            </div>
            {isEditing && (
              <button className="flex w-full items-center justify-center gap-2 rounded-lg border-2 border-dashed border-line py-3 text-xs font-medium text-ink-500 transition-colors hover:border-primary hover:text-primary">
                <Plus className="h-4 w-4" />
                Assign Branch
              </button>
            )}
          </div>
        </div>

        {/* Vehicle Classes */}
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="flex items-center gap-2.5 mb-4">
            <Truck className="h-5 w-5 text-primary" />
            <h3 className="text-sm font-semibold text-ink-950">Vehicle Classes</h3>
          </div>
          <div className="flex flex-wrap gap-2">
            <span className="rounded-full bg-green-bg px-3 py-1.5 text-xs font-semibold text-green">
              Motorcycle
            </span>
            <span className="rounded-full bg-surface-2 px-3 py-1.5 text-xs font-medium text-ink-600">
              Van
            </span>
            <span className="rounded-full bg-surface-2 px-3 py-1.5 text-xs font-medium text-ink-600">
              Truck
            </span>
            {isEditing && (
              <button className="rounded-full border-2 border-dashed border-line px-3 py-1.5 text-xs font-medium text-ink-400 transition-colors hover:border-primary hover:text-primary">
                + Add
              </button>
            )}
          </div>
        </div>

        {/* Work Policy */}
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="flex items-center gap-2.5 mb-4">
            <Shield className="h-5 w-5 text-primary" />
            <h3 className="text-sm font-semibold text-ink-950">Work Policy</h3>
          </div>
          <div className="space-y-3">
            <div className="flex items-center gap-3 text-sm">
              <CheckCircle2 className="h-4 w-4 text-green" />
              <span className="text-ink-700">Available for assignments</span>
            </div>
            <div className="flex items-center gap-3 text-sm">
              <CheckCircle2 className="h-4 w-4 text-green" />
              <span className="text-ink-700">Can accept marketplace offers</span>
            </div>
            <div className="flex items-center gap-3 text-sm">
              <AlertCircle className="h-4 w-4 text-amber" />
              <span className="text-ink-700">Restricted to branch scope</span>
            </div>
            {isEditing && (
              <button className="flex items-center gap-2 text-xs font-medium text-primary transition-colors hover:text-primary-hover">
                <Settings className="h-4 w-4" />
                Configure Policy
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}