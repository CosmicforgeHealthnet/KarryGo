"use client";
import { ApiError } from "@/lib/api-client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, Copy, Check } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth-store";
import { apiKeyService } from "@/services/api-key-service";
import type { CreateAPIKeyInput, IssuedAPIKey } from "@/types";

type Environment = "sandbox" | "live";

const PERMISSIONS = [
  { value: "create_deliveries", label: "Create Deliveries" },
  { value: "read_deliveries", label: "Read Deliveries" },
  { value: "track_deliveries", label: "Track Deliveries" },
  { value: "manage_webhooks", label: "Manage Webhooks" },
] as const;

const EXPIRATION_OPTIONS = [
  { value: "30 days", label: "30 days" },
  { value: "90 days", label: "90 days" },
  { value: "1 year", label: "1 year" },
  { value: "No expiration", label: "No expiration" },
];

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
  const token = useAuthStore((s) => s.token);
  const [environment, setEnvironment] = useState<Environment | null>(null);
  const [label, setLabel] = useState("");
  const [description, setDescription] = useState("");
  const [expiration, setExpiration] = useState("");
  const [permissions, setPermissions] = useState<string[]>([]);
  const [attemptedSubmit, setAttemptedSubmit] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [generatedKey, setGeneratedKey] = useState<IssuedAPIKey | null>(null);
  const [copied, setCopied] = useState(false);

  const togglePermission = (permission: string) => {
    setPermissions((prev) =>
      prev.includes(permission)
        ? prev.filter((p) => p !== permission)
        : [...prev, permission]
    );
  };

  const canGenerate =
    environment !== null &&
    label.trim().length > 0 &&
    permissions.length > 0;

 const handleGenerate = async () => {
    setAttemptedSubmit(true);
    if (!canGenerate || !token) return;

    setLoading(true);
    setError(null);

    const input: CreateAPIKeyInput = {
      label: label.trim(),
      environment: environment,
      description: description.trim(),
      expiration: expiration || "No expiration",
      permissions: permissions,
    };

    try {
      const result = await apiKeyService.create(token, input);
      setGeneratedKey(result);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Failed to generate API key. Please try again.");
      }
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 3000);
  };

  // If key is generated, show the key display
  if (generatedKey) {
    return (
      <div className="space-y-6 pb-4">
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={() => router.push("/settings?tab=api-keys")}
            aria-label="Go back"
            className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-white text-ink-900"
          >
            <ArrowLeft className="h-5 w-5" />
          </button>
        </div>

        <div className="rounded-2xl bg-white p-5">
          <h1 className="text-lg font-bold text-ink-900">API Key Generated</h1>
          <p className="mt-1 text-sm text-ink-500">
            Copy your new API key. It will not be shown again.
          </p>
        </div>

        <div className="rounded-2xl bg-white p-5 space-y-4">
          <div className="rounded-xl bg-amber-50 border border-amber-200 p-4">
            <p className="text-sm font-semibold text-amber-800">⚠️ {generatedKey.warning}</p>
          </div>

          <div>
            <label className="block text-sm font-medium text-ink-900">API Key</label>
            <div className="mt-2 flex items-center gap-3 rounded-xl border border-ink-100 bg-ink-50 p-4">
              <code className="flex-1 break-all text-sm font-mono text-ink-900">
                {generatedKey.key}
              </code>
              <button
                onClick={() => copyToClipboard(generatedKey.key)}
                className="rounded-lg p-2 text-ink-500 hover:bg-ink-100"
              >
                {copied ? <Check className="h-5 w-5 text-green-500" /> : <Copy className="h-5 w-5" />}
              </button>
            </div>
            {copied && <p className="mt-1 text-sm text-green-600">Copied to clipboard!</p>}
          </div>

          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <p className="text-ink-500">Environment</p>
              <p className="font-medium text-ink-900 capitalize">{generatedKey.environment}</p>
            </div>
            <div>
              <p className="text-ink-500">Prefix</p>
              <p className="font-medium text-ink-900">{generatedKey.prefix}</p>
            </div>
            {generatedKey.expires_at && (
              <div className="col-span-2">
                <p className="text-ink-500">Expires</p>
                <p className="font-medium text-ink-900">
                  {new Date(generatedKey.expires_at).toLocaleDateString()}
                </p>
              </div>
            )}
            <div className="col-span-2">
              <p className="text-ink-500">Permissions</p>
              <div className="mt-1 flex flex-wrap gap-1">
                {generatedKey.permissions.map((p) => (
                  <span key={p} className="rounded-full bg-brand-100 px-2 py-0.5 text-xs text-brand-700">
                    {p.replace(/_/g, " ")}
                  </span>
                ))}
              </div>
            </div>
            {generatedKey.description && (
              <div className="col-span-2">
                <p className="text-ink-500">Description</p>
                <p className="font-medium text-ink-900">{generatedKey.description}</p>
              </div>
            )}
          </div>

          <button
            type="button"
            onClick={() => router.push("/settings?tab=api-keys")}
            className="w-full rounded-xl bg-brand-500 px-6 py-4 text-base font-semibold text-white hover:bg-brand-600"
          >
            Done
          </button>
        </div>
      </div>
    );
  }

  // Generate form
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
          Generate a new API key for authenticating requests to the Cosmicforge Logistics API
        </p>
      </div>

      {error && (
        <div className="rounded-xl bg-danger/10 p-4 text-sm text-danger">
          {error}
        </div>
      )}

      <div className="space-y-6 rounded-2xl bg-white p-5">
        <div>
          <p className="text-sm font-bold text-ink-900">
            Environment <span className="text-danger">*</span>
          </p>
          <p className="mt-1 text-sm text-ink-500">Choose which environment the key is for.</p>
          <div className="mt-3 space-y-1">
            <CheckboxRow
              checked={environment === "sandbox"}
              onChange={() => setEnvironment("sandbox")}
              label="Sandbox"
            />
            <CheckboxRow
              checked={environment === "live"}
              onChange={() => setEnvironment("live")}
              label="Live"
            />
          </div>
          {attemptedSubmit && !environment && (
            <p className="mt-2 text-sm font-medium text-danger">
              Select an environment.
            </p>
          )}
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
              {attemptedSubmit && !label.trim() && (
                <p className="mt-1 text-sm font-medium text-danger">Label is required.</p>
              )}
            </div>
            <div>
              <label className="mb-2 block text-sm font-medium text-ink-900">Description</label>
              <textarea
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Description (optional)"
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
              <option key={opt.value} value={opt.value}>
                {opt.label}
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
            {PERMISSIONS.map(({ value, label }) => (
              <CheckboxRow
                key={value}
                checked={permissions.includes(value)}
                onChange={() => togglePermission(value)}
                label={label}
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
        disabled={loading}
        className={cn(
          "w-full rounded-xl px-6 py-4 text-base font-semibold transition-colors",
          canGenerate && !loading
            ? "bg-brand-500 text-white hover:bg-brand-600"
            : "cursor-not-allowed bg-brand-200 text-white"
        )}
      >
        {loading ? "Generating..." : "Generate API Key"}
      </button>
    </div>
  );
}