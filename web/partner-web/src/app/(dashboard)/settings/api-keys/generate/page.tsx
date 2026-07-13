"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { cn } from "@/lib/utils";

type Environment = "Sandbox" | "Live";

const PERMISSIONS = [
  "Create Deliveries",
  "Read Deliveries",
  "Track Deliveries",
  "Manage Webhooks",
] as const;

const EXPIRATION_OPTIONS = ["30 days", "90 days", "1 year", "No expiration"];

function CheckboxRow({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: () => void;
  label: string;
}) {
  return (
    <button
      type="button"
      onClick={onChange}
      className="flex w-full items-center gap-3 py-1 text-left"
    >
      <span
        className={cn(
          "flex h-6 w-6 shrink-0 items-center justify-center rounded-md border-2",
          checked ? "border-brand-500 bg-brand-500" : "border-brand-500 bg-white"
        )}
      >
        {checked && (
          <svg viewBox="0 0 16 16" className="h-3.5 w-3.5 fill-none stroke-white stroke-[2.5]">
            <path d="M3 8.5l3 3 7-7" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        )}
      </span>
      <span className="text-sm text-ink-900">{label}</span>
    </button>
  );
}

export default function GenerateApiKeyPage() {
  const router = useRouter();
  const [environment, setEnvironment] = useState<Environment | null>(null);
  const [label, setLabel] = useState("");
  const [placeholder, setPlaceholder] = useState("");
  const [description, setDescription] = useState("");
  const [expiration, setExpiration] = useState("");
  const [permissions, setPermissions] = useState<string[]>([]);
  const [attemptedSubmit, setAttemptedSubmit] = useState(false);

  const togglePermission = (permission: string) => {
    setPermissions((prev) =>
      prev.includes(permission)
        ? prev.filter((p) => p !== permission)
        : [...prev, permission]
    );
  };

  // Permissions must be non-empty: a key generated with zero permissions gets
  // rejected by every permission-gated backend route (403 insufficient_scope),
  // so it would be created but instantly useless. Requiring at least one here
  // stops that trap before it happens instead of letting someone discover it
  // only when their key mysteriously fails every request.
  const canGenerate =
    environment !== null && label.trim().length > 0 && permissions.length > 0;

  const handleGenerate = () => {
    setAttemptedSubmit(true);
    if (!canGenerate) return;
    // Mock only - no backend call yet.
    router.push("/settings?tab=api-keys");
  };

  return (
    <div className="space-y-6 pb-4">
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={() => router.back()}
          aria-label="Go back"
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-white text-ink-900"
        >
          <ArrowLeft className="h-5 w-5" />
        </button>
      </div>

      <div className="rounded-2xl bg-white p-5">
        <h1 className="text-lg font-bold text-ink-900">Generate API Key</h1>
        <p className="mt-1 text-sm text-ink-500">
          Generate a new API key for authenticating requests to the cosmicforge Logistics
          API
        </p>
      </div>

      <div className="space-y-6 rounded-2xl bg-white p-5">
        <div>
          <p className="text-sm font-bold text-ink-900">
            Environment <span className="text-danger">*</span>
          </p>
          <p className="mt-1 text-sm text-ink-500">Choose which environment the key is for.</p>
          <div className="mt-3 space-y-1">
            <CheckboxRow
              checked={environment === "Sandbox"}
              onChange={() => setEnvironment("Sandbox")}
              label="Sandbox"
            />
            <CheckboxRow
              checked={environment === "Live"}
              onChange={() => setEnvironment("Live")}
              label="Live"
            />
          </div>
          <div className="mt-4 rounded-xl bg-ink-100/60 p-3">
            <p className="text-sm font-bold text-ink-900">Note:</p>
            <p className="mt-1 text-xs text-ink-500">
              Sandbox keys are for testing. Live keys are used for real deliveries.
            </p>
          </div>
        </div>

        <div>
          <p className="text-sm font-bold text-ink-900">
            Key Name <span className="text-danger">*</span>
          </p>
          <p className="mt-1 text-sm text-ink-500">
            This is an internal label to know what the key is used for.
          </p>

          <div className="mt-4 space-y-4">
            <div>
              <label className="mb-2 block text-sm font-medium text-ink-900">Label</label>
              <input
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                placeholder="API Key name"
                className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand-500 focus:outline-none"
              />
            </div>
            <div>
              <label className="mb-2 block text-sm font-medium text-ink-900">Placeholder</label>
              <input
                value={placeholder}
                onChange={(e) => setPlaceholder(e.target.value)}
                placeholder="Website"
                className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand-500 focus:outline-none"
              />
            </div>
            <div>
              <label className="mb-2 block text-sm font-medium text-ink-900">Description</label>
              <textarea
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Description"
                rows={3}
                className="w-full resize-none rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand-500 focus:outline-none"
              />
            </div>
          </div>
        </div>

        <div>
          <label className="mb-2 block text-sm font-bold text-ink-900">Expiration</label>
          <select
            value={expiration}
            onChange={(e) => setExpiration(e.target.value)}
            className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 focus:border-brand-500 focus:outline-none"
          >
            <option value="">Select Option</option>
            {EXPIRATION_OPTIONS.map((opt) => (
              <option key={opt} value={opt}>
                {opt}
              </option>
            ))}
          </select>
        </div>

        <div>
          <p className="text-sm font-bold text-ink-900">
            Permissions <span className="text-danger">*</span>
          </p>
          <p className="mt-1 text-sm text-ink-500">
            Choose which applies. At least one is required - a key with no
            permissions can&apos;t be used for anything.
          </p>
          <div className="mt-3 space-y-1">
            {PERMISSIONS.map((permission) => (
              <CheckboxRow
                key={permission}
                checked={permissions.includes(permission)}
                onChange={() => togglePermission(permission)}
                label={permission}
              />
            ))}
          </div>
          {attemptedSubmit && permissions.length === 0 && (
            <p className="mt-2 text-sm font-medium text-danger">
              Select at least one permission.
            </p>
          )}
        </div>
      </div>

      <button
        type="button"
        onClick={handleGenerate}
        className={cn(
          "w-full rounded-xl px-6 py-4 text-base font-semibold transition-colors",
          canGenerate
            ? "bg-brand-500 text-white hover:bg-brand-600"
            : "cursor-not-allowed bg-brand-200 text-white"
        )}
      >
        Generate API Key
      </button>
    </div>
  );
}