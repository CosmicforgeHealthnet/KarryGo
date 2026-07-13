"use client";

import { useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { User, KeyRound, Webhook as WebhookIcon, X, LogOut, ChevronRight, Key } from "lucide-react";
import { cn } from "@/lib/utils";

type Tab = "profile" | "api-keys" | "webhooks";

// NOTE: swap this for your existing src/lib/nigerian-states.ts export if the
// shape matches (e.g. `import { NIGERIAN_STATES } from "@/lib/nigerian-states"`).
// Kept inline here since I couldn't see that file's export name/shape.
const STATES = ["Abuja (FCT)", "Lagos", "Rivers", "Kano", "Oyo", "Kaduna", "Enugu"];

interface ApiKey {
  id: string;
  name: string;
  environment: "Sandbox" | "Live";
  createdDate: string;
}

const MOCK_API_KEYS: ApiKey[] = [];

interface ProfileData {
  businessName: string;
  phone: string;
  email: string;
  state: string;
  city: string;
  logoFileName: string | null;
}

const INITIAL_PROFILE: ProfileData = {
  businessName: "Dema UI",
  phone: "13456781234",
  email: "",
  state: "Abuja (FCT)",
  city: "",
  logoFileName: "IMG_20250720_174356.jpg",
};

function Checkerboard() {
  return (
    <div
      className="mx-auto h-28 w-28 shrink-0 rounded-full"
      style={{
        backgroundImage:
          "linear-gradient(45deg, #d8dcdd 25%, transparent 25%), linear-gradient(-45deg, #d8dcdd 25%, transparent 25%), linear-gradient(45deg, transparent 75%, #d8dcdd 75%), linear-gradient(-45deg, transparent 75%, #d8dcdd 75%)",
        backgroundSize: "16px 16px",
        backgroundPosition: "0 0, 0 8px, 8px -8px, -8px 0px",
        backgroundColor: "#eef1f2",
      }}
    />
  );
}

function ProfileTab() {
  const [editing, setEditing] = useState(false);
  const [profile, setProfile] = useState<ProfileData>(INITIAL_PROFILE);
  const [draft, setDraft] = useState<ProfileData>(INITIAL_PROFILE);

  const startEditing = () => {
    setDraft(profile);
    setEditing(true);
  };

  const save = () => {
    setProfile(draft);
    setEditing(false);
  };

  if (!editing) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-lg font-bold text-ink-900">Profile Information</h1>
          <p className="mt-1 text-sm text-ink-500">Update your account details.</p>
        </div>

        <Checkerboard />

        <div className="space-y-4">
          <div>
            <p className="text-sm font-bold text-ink-900">Business Name</p>
            <p className="mt-1 text-sm text-ink-700">{profile.businessName}</p>
          </div>
          <div>
            <p className="text-sm font-bold text-ink-900">Business Phone Number</p>
            <p className="mt-1 text-sm text-ink-700">{profile.phone}</p>
          </div>
          <div>
            <p className="text-sm font-bold text-ink-900">Business Email</p>
            <p className="mt-1 text-sm text-ink-700">{profile.email || "Not set"}</p>
          </div>
          <div>
            <p className="text-sm font-bold text-ink-900">Location</p>
            <p className="mt-1 text-sm text-ink-700">
              {profile.state}
              {profile.city ? `, ${profile.city}` : ""}
            </p>
          </div>
          {profile.logoFileName && (
            <div>
              <p className="text-sm font-bold text-ink-900">Business Logo</p>
              <div className="mt-2 flex items-center gap-3">
                <span className="h-9 w-9 shrink-0 rounded-full bg-ink-100" />
                <p className="text-sm text-ink-700">{profile.logoFileName}</p>
              </div>
            </div>
          )}
        </div>

        <button
          type="button"
          className="flex w-full items-center justify-between rounded-xl border border-danger/30 bg-white px-5 py-4 text-sm font-semibold text-danger"
        >
          <span className="flex items-center gap-2">
            <LogOut className="h-4 w-4" />
            Log Out
          </span>
          <ChevronRight className="h-4 w-4" />
        </button>

        <div className="rounded-xl bg-danger/10 p-4">
          <p className="text-sm font-bold text-danger">Delete Account</p>
          <p className="mt-1 text-xs text-danger/80">
            When you delete your account, you lose complete access.
          </p>
          <button
            type="button"
            className="mt-3 rounded-lg bg-danger px-4 py-2 text-sm font-semibold text-white hover:bg-danger/90"
          >
            Delete Account
          </button>
        </div>

        <button
          type="button"
          onClick={startEditing}
          className="w-full rounded-xl bg-brand-200 px-6 py-4 text-base font-semibold text-white hover:bg-brand-300"
        >
          Edit Profile
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-bold text-ink-900">Profile Information</h1>
        <p className="mt-1 text-sm text-ink-500">Edit your profile details.</p>
      </div>

      <Checkerboard />

      <div className="flex items-center justify-center gap-3">
        <button
          type="button"
          className="rounded-xl border border-brand-300 bg-brand-100 px-5 py-2.5 text-sm font-semibold text-brand-700 hover:bg-brand-200"
        >
          Change Photo
        </button>
        <button
          type="button"
          onClick={() => setDraft((d) => ({ ...d, logoFileName: null }))}
          className="rounded-xl bg-brand-600 px-5 py-2.5 text-sm font-semibold text-white hover:bg-brand-700"
        >
          Remove Photo
        </button>
      </div>

      <div>
        <label className="mb-2 block text-sm font-medium text-ink-900">Business Name</label>
        <input
          value={draft.businessName}
          onChange={(e) => setDraft((d) => ({ ...d, businessName: e.target.value }))}
          placeholder="Business name"
          className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand-500 focus:outline-none"
        />
      </div>

      <div>
        <label className="mb-2 block text-sm font-medium text-ink-900">Business Phone Number</label>
        <input
          value={draft.phone}
          onChange={(e) => setDraft((d) => ({ ...d, phone: e.target.value }))}
          placeholder="Phone number"
          className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand-500 focus:outline-none"
        />
      </div>

      <div>
        <label className="mb-2 block text-sm font-medium text-ink-900">Business Email</label>
        <input
          value={draft.email}
          onChange={(e) => setDraft((d) => ({ ...d, email: e.target.value }))}
          placeholder="Email here"
          className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand-500 focus:outline-none"
        />
      </div>

      <div className="space-y-3">
        <label className="block text-sm font-medium text-ink-900">Location</label>
        <select
          value={draft.state}
          onChange={(e) => setDraft((d) => ({ ...d, state: e.target.value }))}
          className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 focus:border-brand-500 focus:outline-none"
        >
          {STATES.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
        <input
          value={draft.city}
          onChange={(e) => setDraft((d) => ({ ...d, city: e.target.value }))}
          placeholder="City"
          className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand-500 focus:outline-none"
        />
      </div>

      <div>
        <label className="mb-2 block text-sm font-medium text-ink-900">Business Logo</label>
        {draft.logoFileName ? (
          <div className="flex items-center justify-between rounded-xl border border-ink-100 px-4 py-3">
            <div className="flex items-center gap-3">
              <span className="h-8 w-8 shrink-0 rounded-full bg-ink-100" />
              <p className="text-sm text-ink-700">{draft.logoFileName}</p>
            </div>
            <button
              type="button"
              onClick={() => setDraft((d) => ({ ...d, logoFileName: null }))}
              aria-label="Remove logo"
              className="text-ink-500 hover:text-danger"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        ) : (
          <button
            type="button"
            onClick={() => setDraft((d) => ({ ...d, logoFileName: "logo.jpg" }))}
            className="w-full rounded-xl border border-dashed border-ink-100 px-4 py-4 text-sm text-ink-500"
          >
            Upload a logo
          </button>
        )}
      </div>

      <button
        type="button"
        onClick={save}
        className="w-full rounded-xl bg-brand-200 px-6 py-4 text-base font-semibold text-white hover:bg-brand-500"
      >
        Save
      </button>
    </div>
  );
}

function ApiKeysTab() {
  return (
    <div className="space-y-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-lg font-bold text-ink-900">API Keys</h1>
          <p className="mt-1 text-sm text-ink-500">
            Manage your API keys for third-party integrations.
          </p>
        </div>
      </div>

      <div className="flex justify-end">
        <Link
          href="/settings/api-keys/generate"
          className="inline-block rounded-xl bg-brand-600 px-5 py-3 text-sm font-semibold text-white hover:bg-brand-700"
        >
          Generate API Key
        </Link>
      </div>

      {MOCK_API_KEYS.length === 0 ? (
        <div className="rounded-2xl border border-ink-100 bg-ink-100/60 px-6 py-10 text-center">
          <Key className="mx-auto h-6 w-6 text-ink-500" />
          <p className="mt-3 text-base font-bold text-ink-900">No API keys found</p>
          <p className="mt-1 text-sm text-ink-500">
            Generate your first API Key to start integrating the Cosmicforge Logistics API
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {MOCK_API_KEYS.map((key) => (
            <div key={key.id} className="rounded-2xl bg-white p-4">
              <p className="text-sm font-semibold text-ink-900">{key.name}</p>
              <p className="mt-1 text-xs text-ink-500">
                {key.environment} &middot; created {key.createdDate}
              </p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function WebhooksTab() {
  // No design reference was provided for this tab - built to match the
  // empty-state pattern used on API Keys until a real spec shows up.
  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-lg font-bold text-ink-900">Webhooks</h1>
        <p className="mt-1 text-sm text-ink-500">
          Manage endpoints that receive delivery event notifications.
        </p>
      </div>

      <div className="flex justify-end">
        <button
          type="button"
          className="rounded-xl bg-brand-600 px-5 py-3 text-sm font-semibold text-white hover:bg-brand-700"
        >
          Add Webhook Endpoint
        </button>
      </div>

      <div className="rounded-2xl border border-ink-100 bg-ink-100/60 px-6 py-10 text-center">
        <WebhookIcon className="mx-auto h-6 w-6 text-ink-500" />
        <p className="mt-3 text-base font-bold text-ink-900">No webhooks configured</p>
        <p className="mt-1 text-sm text-ink-500">
          Add an endpoint to start receiving delivery event notifications.
        </p>
      </div>
    </div>
  );
}

export default function SettingsPage() {
  const searchParams = useSearchParams();
  const initialTab = (searchParams.get("tab") as Tab) || "profile";
  const [tab, setTab] = useState<Tab>(initialTab);

  const tabs: { id: Tab; label: string; icon: typeof User }[] = [
    { id: "profile", label: "Profile", icon: User },
    { id: "api-keys", label: "API Keys", icon: KeyRound },
    { id: "webhooks", label: "Webhooks", icon: WebhookIcon },
  ];

  return (
    <div className="space-y-6">
      <div className="rounded-2xl bg-white p-5">
        <h1 className="text-lg font-bold text-ink-900">Settings</h1>
        <p className="mt-1 text-sm text-ink-500">
          Manage your account settings and preferences.
        </p>
      </div>

      <div className="flex gap-6 border-b border-ink-100">
        {tabs.map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            type="button"
            onClick={() => setTab(id)}
            className={cn(
              "flex items-center gap-1.5 border-b-2 pb-3 text-sm font-semibold transition-colors",
              tab === id
                ? "border-brand-600 text-brand-600"
                : "border-transparent text-ink-500 hover:text-ink-700"
            )}
          >
            <Icon className="h-4 w-4" />
            {label}
          </button>
        ))}
      </div>

      <div className="rounded-2xl bg-white p-5">
        {tab === "profile" && <ProfileTab />}
        {tab === "api-keys" && <ApiKeysTab />}
        {tab === "webhooks" && <WebhooksTab />}
      </div>
    </div>
  );
}