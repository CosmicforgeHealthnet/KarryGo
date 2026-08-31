// src/app/(dashboard)/providers/add/page.tsx

"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";

type Step = "form" | "resolving" | "resolved";
type ResolutionResult = "not_found" | "matched" | "conflict";

export default function AddProviderPage() {
  const router = useRouter();
  const [step, setStep] = useState<Step>("form");
  const [resolution, setResolution] = useState<ResolutionResult | null>(null);
  const [form, setForm] = useState({
    legalName: "",
    phone: "",
    email: "",
    service: "Dispatch",
    branch: "Ikeja",
  });

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setStep("resolving");

    await new Promise((resolve) => setTimeout(resolve, 900));

    const mockResult: ResolutionResult = form.phone.endsWith("0")
      ? "matched"
      : form.phone.endsWith("1")
        ? "conflict"
        : "not_found";

    setResolution(mockResult);
    setStep("resolved");
  }

  function handleConfirm() {
    router.push("/providers");
  }

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      {/* ── Header ── */}
      <div>
        <h1>Add or link provider</h1>
        <p className="mt-1.5">
          We'll check for an existing platform identity before creating a new one.
        </p>
      </div>

      {step === "form" && (
        <form onSubmit={handleSubmit} className="space-y-4">
          <FormField label="Legal name">
            <input
              required
              value={form.legalName}
              onChange={(e) => setForm({ ...form, legalName: e.target.value })}
              className="w-full rounded-lg border border-line px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </FormField>

          <div className="grid grid-cols-2 gap-4">
            <FormField label="Phone">
              <input
                required
                value={form.phone}
                onChange={(e) => setForm({ ...form, phone: e.target.value })}
                placeholder="+234..."
                className="w-full rounded-lg border border-line px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </FormField>
            <FormField label="Email">
              <input
                type="email"
                value={form.email}
                onChange={(e) => setForm({ ...form, email: e.target.value })}
                className="w-full rounded-lg border border-line px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </FormField>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <FormField label="Service category">
              <select
                value={form.service}
                onChange={(e) => setForm({ ...form, service: e.target.value })}
                className="w-full rounded-lg border border-line bg-white px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              >
                <option>Dispatch</option>
                <option>Hauling</option>
              </select>
            </FormField>
            <FormField label="Branch">
              <select
                value={form.branch}
                onChange={(e) => setForm({ ...form, branch: e.target.value })}
                className="w-full rounded-lg border border-line bg-white px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              >
                {["Ikeja", "Lekki", "Yaba", "Apapa"].map((b) => (
                  <option key={b}>{b}</option>
                ))}
              </select>
            </FormField>
          </div>

          <button
            type="submit"
            className="rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            Check identity and continue
          </button>
        </form>
      )}

      {step === "resolving" && (
        <div className="flex flex-col items-center gap-3 py-10 text-center">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-line border-t-primary" />
          <p className="text-sm text-ink-600">
            Checking for an existing platform identity…
          </p>
        </div>
      )}

      {step === "resolved" && resolution && (
        <div>
          {resolution === "not_found" && (
            <ResolutionPanel
              tone="green"
              title="No existing identity found"
              detail={`A new provider identity will be created for ${form.legalName || "this provider"}.`}
              action="Create and invite"
              onConfirm={handleConfirm}
              onBack={() => setStep("form")}
            />
          )}
          {resolution === "matched" && (
            <ResolutionPanel
              tone="blue"
              title="Existing identity matched"
              detail={`${form.legalName || "This provider"} already has a KarryGo platform identity. Linking will send a membership invitation — it will not create a duplicate account.`}
              action="Link and invite"
              onConfirm={handleConfirm}
              onBack={() => setStep("form")}
            />
          )}
          {resolution === "conflict" && (
            <ResolutionPanel
              tone="red"
              title="Conflicting identity found"
              detail="The phone number matches an existing identity with different name details. This can't be auto-merged — review manually before proceeding."
              action={null}
              onConfirm={handleConfirm}
              onBack={() => setStep("form")}
            />
          )}
        </div>
      )}
    </div>
  );
}

function FormField({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
        {label}
      </label>
      {children}
    </div>
  );
}

function ResolutionPanel({
  tone,
  title,
  detail,
  action,
  onConfirm,
  onBack,
}: {
  tone: "green" | "blue" | "red";
  title: string;
  detail: string;
  action: string | null;
  onConfirm: () => void;
  onBack: () => void;
}) {
  const toneStyle = {
    green: "border-green/20 bg-green-bg text-green",
    blue: "border-primary/20 bg-primary-bg text-primary",
    red: "border-red/20 bg-red-bg text-red",
  }[tone];

  return (
    <div className={`rounded-lg border p-5 ${toneStyle}`}>
      <div className="text-sm font-bold">{title}</div>
      <p className="mt-2 text-sm leading-relaxed text-ink-800">
        {detail}
      </p>
      <div className="mt-5 flex gap-2.5">
        <button
          onClick={onBack}
          className="rounded-md border border-line bg-white px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          Back
        </button>
        {action && (
          <button
            onClick={onConfirm}
            className="rounded-md bg-primary px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            {action}
          </button>
        )}
      </div>
    </div>
  );
}