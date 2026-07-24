"use client";

import { useState, useEffect, useRef } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { User, KeyRound, Webhook as WebhookIcon, X, LogOut, ChevronRight, Key, Copy, Check, Trash2, Plus, Upload, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth-store";
import { apiKeyService } from "@/services/api-key-service";
import { authService } from "@/services/auth-service";
import { profileService } from "@/services/profile-service";
import { webhookService } from "@/services/webhook-service";
import type { APIKey, AuthAccount } from "@/types";

type Tab = "profile" | "api-keys" | "webhooks";

const STATES = ["Abuja (FCT)", "Lagos", "Rivers", "Kano", "Oyo", "Kaduna", "Enugu", "Kogi"];

const getFullImageUrl = (url: string | null) => {
  if (!url) return '';
  
  // Extract the ID from any URL or just use the string
  let id = '';
  
  // Try to extract ID from URL
  const match = url.match(/([a-f0-9-]{8,}-[a-f0-9-]{4,}-[a-f0-9-]{4,}-[a-f0-9-]{4,}-[a-f0-9-]{12,})/);
  if (match) {
    id = match[1];
  } else {
    // If no ID found, return empty
    return '';
  }
  
  return `http://localhost:8109/api/v1/media-files/${id}/download`;
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
  const router = useRouter();
  const { token, clearSession } = useAuthStore();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const logoInputRef = useRef<HTMLInputElement>(null);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState(false);
  const [profile, setProfile] = useState<AuthAccount | null>(null);
  const [draft, setDraft] = useState<AuthAccount | null>(null);
  const [saveLoading, setSaveLoading] = useState(false);
  const [uploadLoading, setUploadLoading] = useState(false);
  const [logoUploadLoading, setLogoUploadLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [logoutLoading, setLogoutLoading] = useState(false);

  useEffect(() => {
    if (!token) return;
    loadProfile();
  }, [token]);

  const loadProfile = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await authService.me(token!);
      setProfile(data.account);
      setDraft(data.account);
    } catch (err) {
      setError("Failed to load profile");
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const startEditing = () => {
    setDraft(profile);
    setEditing(true);
    setError(null);
  };

  const handleSave = async () => {
    if (!token || !draft) return;
    setSaveLoading(true);
    setError(null);
    try {
      const payload: any = {};
      if (draft.business_name !== profile?.business_name) {
        payload.business_name = draft.business_name;
      }
      if (draft.business_phone !== profile?.business_phone) {
        payload.business_phone = draft.business_phone;
      }
      if (draft.state !== profile?.state) {
        payload.state = draft.state;
      }
      if (draft.city !== profile?.city) {
        payload.city = draft.city;
      }
      if (draft.business_logo_url !== profile?.business_logo_url) {
        payload.business_logo_url = draft.business_logo_url;
      }
      if (draft.profile_photo_url !== profile?.profile_photo_url) {
        payload.profile_photo_url = draft.profile_photo_url;
      }

      const response = await profileService.updateProfile(payload, token);
      setProfile(response.account);
      setEditing(false);
    } catch (err) {
      setError("Failed to save profile");
      console.error(err);
    } finally {
      setSaveLoading(false);
    }
  };

  const handlePhotoUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file || !token) {
      if (fileInputRef.current) fileInputRef.current.value = "";
      return;
    }

    if (!file.type.startsWith('image/')) {
      setError("Please upload an image file");
      if (fileInputRef.current) fileInputRef.current.value = "";
      return;
    }

    if (file.size > 5 * 1024 * 1024) {
      setError("Image must be less than 5MB");
      if (fileInputRef.current) fileInputRef.current.value = "";
      return;
    }

    setUploadLoading(true);
    setError(null);

    try {
      const formData = new FormData();
      formData.append("file", file);
      formData.append("purpose", "profile_photo");
      formData.append("owner_service", "partner-service");

      const response = await fetch(`http://localhost:8109/api/v1/media-files/uploads`, {
        method: "POST",
        headers: {
          "Authorization": `Bearer ${token}`,
        },
        body: formData,
      });

      if (!response.ok) {
        throw new Error("Upload failed");
      }

      const result = await response.json();
      const uploadedUrl = result.data.url || `http://localhost:8109/${result.data.id}/download`;
      
      setDraft((d) => d ? { ...d, profile_photo_url: uploadedUrl } : null);
      
    } catch (err) {
      setError("Failed to upload photo");
      console.error(err);
    } finally {
      setUploadLoading(false);
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
      }
    }
  };

  const handleLogoUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file || !token) {
      if (logoInputRef.current) logoInputRef.current.value = "";
      return;
    }

    if (!file.type.startsWith('image/')) {
      setError("Please upload an image file");
      if (logoInputRef.current) logoInputRef.current.value = "";
      return;
    }

    if (file.size > 5 * 1024 * 1024) {
      setError("Image must be less than 5MB");
      if (logoInputRef.current) logoInputRef.current.value = "";
      return;
    }

    setLogoUploadLoading(true);
    setError(null);

    try {
      const formData = new FormData();
      formData.append("file", file);
      formData.append("purpose", "business_logo");
      formData.append("owner_service", "partner-service");

      const response = await fetch(`http://localhost:8109/api/v1/media-files/uploads`, {
        method: "POST",
        headers: {
          "Authorization": `Bearer ${token}`,
        },
        body: formData,
      });

      if (!response.ok) {
        throw new Error("Upload failed");
      }

      const result = await response.json();
      const uploadedUrl = result.data.url || `http://localhost:8109/${result.data.id}/download`;
      
      setDraft((d) => d ? { ...d, business_logo_url: uploadedUrl } : null);
      
    } catch (err) {
      setError("Failed to upload logo");
      console.error(err);
    } finally {
      setLogoUploadLoading(false);
      if (logoInputRef.current) {
        logoInputRef.current.value = "";
      }
    }
  };

  const handleLogout = async () => {
    if (!token) return;
    setLogoutLoading(true);
    try {
      await authService.logout(token);
      clearSession();
      router.push("/login");
    } catch (err) {
      clearSession();
      router.push("/login");
    } finally {
      setLogoutLoading(false);
    }
  };

  const handleDeleteAccount = async () => {
    if (!token) return;
    if (!confirm("Are you sure you want to delete your account? This action cannot be undone. All your data will be permanently removed.")) {
      return;
    }
    setError(null);
    try {
      await profileService.deleteAccount(token);
      clearSession();
      router.push("/login");
    } catch (err) {
      setError("Failed to delete account");
      console.error(err);
    }
  };

  if (loading) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-lg font-bold text-ink-900">Profile Information</h1>
          <p className="mt-1 text-sm text-ink-500">Loading your profile...</p>
        </div>
        <div className="flex items-center justify-center min-h-[200px]">
          <div className="text-ink-500">Loading...</div>
        </div>
      </div>
    );
  }

  if (error || !profile) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-lg font-bold text-ink-900">Profile Information</h1>
          <p className="mt-1 text-sm text-ink-500">Update your account details.</p>
        </div>
        <div className="rounded-xl bg-danger/10 p-4 text-sm text-danger">
          {error || "No profile data found"}
          <button onClick={loadProfile} className="ml-3 font-semibold underline">
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (!editing) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-lg font-bold text-ink-900">Profile Information</h1>
          <p className="mt-1 text-sm text-ink-500">Update your account details.</p>
        </div>

        {profile.profile_photo_url ? (
          <img
            src={getFullImageUrl(profile.profile_photo_url)!}
            alt="Profile"
            className="mx-auto h-28 w-28 shrink-0 rounded-full object-cover"
          />
        ) : (
          <Checkerboard />
        )}

        <div className="space-y-4">
          <div>
            <p className="text-sm font-bold text-ink-900">Business Name</p>
            <p className="mt-1 text-sm text-ink-700">{profile.business_name || "Not set"}</p>
          </div>
          <div>
            <p className="text-sm font-bold text-ink-900">Business Phone Number</p>
            <p className="mt-1 text-sm text-ink-700">{profile.business_phone || "Not set"}</p>
          </div>
          <div>
            <p className="text-sm font-bold text-ink-900">Business Email</p>
            <p className="mt-1 text-sm text-ink-700">{profile.business_email || "Not set"}</p>
          </div>
          <div>
            <p className="text-sm font-bold text-ink-900">Location</p>
            <p className="mt-1 text-sm text-ink-700">
              {profile.state || "Not set"}
              {profile.city ? `, ${profile.city}` : ""}
            </p>
          </div>
          <div>
            <p className="text-sm font-bold text-ink-900">Plan</p>
            <p className="mt-1 text-sm text-ink-700 capitalize">{profile.plan || "Not set"}</p>
          </div>
          <div>
            <p className="text-sm font-bold text-ink-900">Account Status</p>
            <p className="mt-1 text-sm text-ink-700 capitalize">{profile.status || "Not set"}</p>
          </div>
          {profile.business_logo_url && (
            <div>
              <p className="text-sm font-bold text-ink-900">Business Logo</p>
              <div className="mt-2 flex items-center gap-3">
                <img 
                  src={getFullImageUrl(profile.business_logo_url)!} 
                  alt="Logo" 
                  className="h-9 w-9 shrink-0 rounded-full object-cover" 
                />
                <p className="text-sm text-ink-700">Logo uploaded</p>
              </div>
            </div>
          )}
        </div>

        <button
          type="button"
          onClick={handleLogout}
          disabled={logoutLoading}
          className="flex w-full items-center justify-between rounded-xl border border-danger/30 bg-white px-5 py-4 text-sm font-semibold text-danger hover:bg-danger/5 disabled:opacity-50"
        >
          <span className="flex items-center gap-2">
            <LogOut className="h-4 w-4" />
            {logoutLoading ? "Logging out..." : "Log Out"}
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
            onClick={handleDeleteAccount}
            className="mt-3 rounded-lg bg-danger px-4 py-2 text-sm font-semibold text-white hover:bg-danger/90"
          >
            Delete Account
          </button>
        </div>

        <button
          type="button"
          onClick={startEditing}
          className="w-full rounded-xl bg-brand-500 px-6 py-4 text-base font-semibold text-white hover:bg-brand-600"
        >
          Edit Profile
        </button>
      </div>
    );
  }

  // EDIT MODE
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-bold text-ink-900">Edit Profile</h1>
        <p className="mt-1 text-sm text-ink-500">Update your account details.</p>
      </div>

      {/* Profile Photo Upload Section */}
      <div className="flex flex-col items-center">
        {draft?.profile_photo_url ? (
          <img
            src={getFullImageUrl(draft.profile_photo_url)!}
            alt="Profile"
            className="mx-auto h-28 w-28 shrink-0 rounded-full object-cover border-2 border-ink-100"
          />
        ) : (
          <Checkerboard />
        )}
        
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          onChange={handlePhotoUpload}
          className="hidden"
          id="profile-photo-upload"
        />
        
        <div className="mt-3 flex items-center gap-3">
          <label
            htmlFor="profile-photo-upload"
            className={cn(
              "cursor-pointer rounded-xl border border-brand-300 bg-brand-100 px-5 py-2.5 text-sm font-semibold text-brand-700 hover:bg-brand-200 inline-flex items-center gap-2 transition-colors",
              uploadLoading && "opacity-50 cursor-not-allowed"
            )}
          >
            {uploadLoading ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Uploading...
              </>
            ) : (
              <>
                <Upload className="h-4 w-4" />
                Change Photo
              </>
            )}
          </label>
          <button
            type="button"
            onClick={() => setDraft((d) => d ? { ...d, profile_photo_url: null } : null)}
            className="rounded-xl bg-danger/10 px-5 py-2.5 text-sm font-semibold text-danger hover:bg-danger/20 transition-colors"
          >
            Remove Photo
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-xl bg-danger/10 p-3 text-sm text-danger">
          {error}
        </div>
      )}

      <div>
        <label className="mb-2 block text-sm font-medium text-ink-900">Business Name</label>
        <input
          value={draft?.business_name || ""}
          onChange={(e) => setDraft((d) => d ? { ...d, business_name: e.target.value } : null)}
          placeholder="Business name"
          className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand-500 focus:outline-none"
        />
      </div>

      <div>
        <label className="mb-2 block text-sm font-medium text-ink-900">Business Phone Number</label>
        <input
          value={draft?.business_phone || ""}
          onChange={(e) => setDraft((d) => d ? { ...d, business_phone: e.target.value } : null)}
          placeholder="Phone number"
          className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand-500 focus:outline-none"
        />
      </div>

      <div>
        <label className="mb-2 block text-sm font-medium text-ink-900">Business Email</label>
        <input
          value={draft?.business_email || ""}
          className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 bg-ink-50 cursor-not-allowed"
          disabled
        />
        <p className="mt-1 text-xs text-ink-500">Email cannot be changed</p>
      </div>

      <div className="space-y-3">
        <label className="block text-sm font-medium text-ink-900">Location</label>
        <select
          value={draft?.state || ""}
          onChange={(e) => setDraft((d) => d ? { ...d, state: e.target.value } : null)}
          className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 focus:border-brand-500 focus:outline-none"
        >
          <option value="">Select state</option>
          {STATES.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
        <input
          value={draft?.city || ""}
          onChange={(e) => setDraft((d) => d ? { ...d, city: e.target.value } : null)}
          placeholder="City"
          className="w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand-500 focus:outline-none"
        />
      </div>

      {/* Business Logo Upload Section */}
      <div>
        <label className="mb-2 block text-sm font-medium text-ink-900">Business Logo</label>
        
        <input
          ref={logoInputRef}
          type="file"
          accept="image/*"
          onChange={handleLogoUpload}
          className="hidden"
          id="business-logo-upload"
        />
        
        {draft?.business_logo_url ? (
          <div className="flex items-center justify-between rounded-xl border border-ink-100 px-4 py-3">
            <div className="flex items-center gap-3">
              <img 
                src={getFullImageUrl(draft.business_logo_url)!} 
                alt="Logo" 
                className="h-8 w-8 shrink-0 rounded-full object-cover" 
              />
              <p className="text-sm text-ink-700">Logo uploaded</p>
            </div>
            <div className="flex gap-2">
              <label
                htmlFor="business-logo-upload"
                className={cn(
                  "cursor-pointer rounded-lg px-3 py-1.5 text-xs font-semibold text-brand-600 hover:bg-brand-50 transition-colors",
                  logoUploadLoading && "opacity-50 cursor-not-allowed"
                )}
              >
                {logoUploadLoading ? "Uploading..." : "Change"}
              </label>
              <button
                type="button"
                onClick={() => setDraft((d) => d ? { ...d, business_logo_url: null } : null)}
                className="rounded-lg px-3 py-1.5 text-xs font-semibold text-danger hover:bg-danger/10 transition-colors"
              >
                Remove
              </button>
            </div>
          </div>
        ) : (
          <label
            htmlFor="business-logo-upload"
            className={cn(
              "flex w-full cursor-pointer items-center justify-center rounded-xl border border-dashed border-ink-200 px-4 py-6 text-sm text-ink-500 hover:bg-ink-50 transition-colors",
              logoUploadLoading && "opacity-50 cursor-not-allowed"
            )}
          >
            {logoUploadLoading ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Uploading...
              </>
            ) : (
              <>
                <Upload className="h-4 w-4 mr-2" />
                Upload a logo
              </>
            )}
          </label>
        )}
      </div>

      <button
        type="button"
        onClick={handleSave}
        disabled={saveLoading}
        className="w-full rounded-xl bg-brand-500 px-6 py-4 text-base font-semibold text-white hover:bg-brand-600 disabled:opacity-50 transition-colors"
      >
        {saveLoading ? (
          <>
            <Loader2 className="h-4 w-4 inline mr-2 animate-spin" />
            Saving...
          </>
        ) : (
          "Save Changes"
        )}
      </button>
    </div>
  );
}

function ApiKeysTab() {
  const token = useAuthStore((s) => s.token);
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [revoking, setRevoking] = useState<string | null>(null);

  useEffect(() => {
    if (!token) return;
    fetchKeys();
  }, [token]);

const fetchKeys = async () => {
  setLoading(true);
  setError(null);
  try {
    const data = await apiKeyService.list(token!);
    setKeys(data || []);
  } catch (err) {
    setError("Failed to load API keys");
    console.error(err);
    setKeys([]);
  } finally {
    setLoading(false);
  }
};
  const handleRevoke = async (id: string) => {
    if (!confirm("Are you sure you want to revoke this API key? This action cannot be undone.")) {
      return;
    }
    setRevoking(id);
    setError(null);
    try {
      await apiKeyService.revoke(token!, id);
      setKeys(keys.filter(k => k.id !== id));
    } catch (err) {
      setError("Failed to revoke API key");
      console.error(err);
    } finally {
      setRevoking(null);
    }
  };

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  if (loading) {
    return (
      <div className="space-y-5">
        <div>
          <h1 className="text-lg font-bold text-ink-900">API Keys</h1>
          <p className="mt-1 text-sm text-ink-500">Loading your API keys...</p>
        </div>
        <div className="flex items-center justify-center min-h-[200px]">
          <div className="text-ink-500">Loading...</div>
        </div>
      </div>
    );
  }

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
          className="inline-flex items-center gap-2 rounded-xl bg-brand-600 px-5 py-3 text-sm font-semibold text-white hover:bg-brand-700"
        >
          <Plus className="h-4 w-4" />
          Generate API Key
        </Link>
      </div>

      {error && (
        <div className="rounded-xl bg-danger/10 p-4 text-sm text-danger">
          {error}
          <button onClick={fetchKeys} className="ml-3 font-semibold underline">
            Retry
          </button>
        </div>
      )}

      {keys.length === 0 ? (
        <div className="rounded-2xl border border-ink-100 bg-ink-100/60 px-6 py-10 text-center">
          <Key className="mx-auto h-6 w-6 text-ink-500" />
          <p className="mt-3 text-base font-bold text-ink-900">No API keys found</p>
          <p className="mt-1 text-sm text-ink-500">
            Generate your first API Key to start integrating the Cosmicforge Logistics API
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {keys.map((key) => (
            <div key={key.id} className="rounded-2xl bg-white p-4 border border-ink-100">
              <div className="flex items-start justify-between">
                <div className="flex-1">
                  <p className="text-sm font-semibold text-ink-900">{key.label}</p>
                  <p className="mt-0.5 text-xs text-ink-500">
                    {key.environment} &middot; {key.status} &middot; created{" "}
                    {new Date(key.created_at).toLocaleDateString()}
                  </p>
                  {key.description && (
                    <p className="mt-1 text-xs text-ink-500">{key.description}</p>
                  )}
                  <div className="mt-2 flex flex-wrap gap-1">
                    <span className="rounded-full bg-ink-100 px-2 py-0.5 text-xs text-ink-700">
                      Prefix: {key.prefix}
                    </span>
                    {key.permissions.map((p) => (
                      <span key={p} className="rounded-full bg-brand-100 px-2 py-0.5 text-xs text-brand-700">
                        {p.replace(/_/g, " ")}
                      </span>
                    ))}
                  </div>
                  {key.last_used_at && (
                    <p className="mt-1 text-xs text-ink-500">
                      Last used: {new Date(key.last_used_at).toLocaleDateString()}
                    </p>
                  )}
                </div>
                <div className="flex gap-2 ml-4">
                  <button
                    onClick={() => copyToClipboard(key.prefix, key.id)}
                    className="rounded-lg p-2 text-ink-500 hover:bg-ink-100"
                    title="Copy prefix"
                  >
                    {copiedId === key.id ? (
                      <Check className="h-4 w-4 text-green-500" />
                    ) : (
                      <Copy className="h-4 w-4" />
                    )}
                  </button>
                  {key.status === "active" && (
                    <button
                      onClick={() => handleRevoke(key.id)}
                      disabled={revoking === key.id}
                      className="rounded-lg p-2 text-danger hover:bg-danger/10 disabled:opacity-50"
                      title="Revoke key"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  )}
                </div>
              </div>
              {key.expires_at && (
                <p className="mt-2 text-xs text-amber-600">
                  Expires: {new Date(key.expires_at).toLocaleDateString()}
                </p>
              )}
              {key.revoked_at && (
                <p className="mt-2 text-xs text-danger">
                  Revoked: {new Date(key.revoked_at).toLocaleDateString()}
                </p>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function WebhooksTab() {
  const token = useAuthStore((s) => s.token);
  const [webhooks, setWebhooks] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showAddForm, setShowAddForm] = useState(false);
  const [url, setUrl] = useState("");
  const [events, setEvents] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);

  const EVENT_OPTIONS = [
    "delivery.accepted",
    "delivery.in_progress",
    "delivery.completed",
    "delivery.cancelled",
    "delivery.no_provider",
    "delivery.failed",
  ];

  useEffect(() => {
    if (!token) return;
    fetchWebhooks();
  }, [token]);

  const fetchWebhooks = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await webhookService.listWebhooks(token!);
      setWebhooks(response.webhooks || []);
    } catch (err) {
      setError("Failed to load webhooks");
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const handleAdd = async () => {
    if (!token || !url.trim() || events.length === 0) return;
    setSubmitting(true);
    setError(null);
    try {
      await webhookService.createWebhook(token, { url: url.trim(), events });
      setUrl("");
      setEvents([]);
      setShowAddForm(false);
      await fetchWebhooks();
    } catch (err) {
      setError("Failed to create webhook");
      console.error(err);
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!token) return;
    if (!confirm("Are you sure you want to delete this webhook?")) return;
    try {
      await webhookService.deleteWebhook(token, id);
      await fetchWebhooks();
    } catch (err) {
      setError("Failed to delete webhook");
      console.error(err);
    }
  };

  const toggleEvent = (event: string) => {
    setEvents(prev =>
      prev.includes(event)
        ? prev.filter(e => e !== event)
        : [...prev, event]
    );
  };

  if (loading) {
    return (
      <div className="space-y-5">
        <div>
          <h1 className="text-lg font-bold text-ink-900">Webhooks</h1>
          <p className="mt-1 text-sm text-ink-500">Loading your webhooks...</p>
        </div>
        <div className="flex items-center justify-center min-h-[200px]">
          <div className="text-ink-500">Loading...</div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-lg font-bold text-ink-900">Webhooks</h1>
          <p className="mt-1 text-sm text-ink-500">
            Manage endpoints that receive delivery event notifications.
          </p>
        </div>
      </div>

      <div className="flex justify-end">
        <button
          type="button"
          onClick={() => setShowAddForm(!showAddForm)}
          className="inline-flex items-center gap-2 rounded-xl bg-brand-600 px-5 py-3 text-sm font-semibold text-white hover:bg-brand-700"
        >
          <Plus className="h-4 w-4" />
          {showAddForm ? "Cancel" : "Add Webhook Endpoint"}
        </button>
      </div>

      {error && (
        <div className="rounded-xl bg-danger/10 p-4 text-sm text-danger">
          {error}
          <button onClick={fetchWebhooks} className="ml-3 font-semibold underline">
            Retry
          </button>
        </div>
      )}

      {showAddForm && (
        <div className="rounded-2xl bg-white p-5 border border-ink-100 space-y-4">
          <div>
            <label className="mb-2 block text-sm font-medium text-ink-900">Endpoint URL</label>
            <input
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://your-api.com/webhook"
              className="w-full rounded-xl border border-ink-100 px-4 py-3 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand-500 focus:outline-none"
            />
          </div>
          <div>
            <label className="mb-2 block text-sm font-medium text-ink-900">Events</label>
            <div className="flex flex-wrap gap-2">
              {EVENT_OPTIONS.map((event) => (
                <button
                  key={event}
                  type="button"
                  onClick={() => toggleEvent(event)}
                  className={cn(
                    "rounded-full px-3 py-1.5 text-xs font-medium transition-colors",
                    events.includes(event)
                      ? "bg-brand-500 text-white"
                      : "bg-ink-100 text-ink-700 hover:bg-ink-200"
                  )}
                >
                  {event}
                </button>
              ))}
            </div>
            {events.length === 0 && (
              <p className="mt-1 text-xs text-amber-600">Select at least one event</p>
            )}
          </div>
          <button
            type="button"
            onClick={handleAdd}
            disabled={submitting || !url.trim() || events.length === 0}
            className="w-full rounded-xl bg-brand-500 px-6 py-3 text-sm font-semibold text-white hover:bg-brand-600 disabled:opacity-50"
          >
            {submitting ? "Creating..." : "Create Webhook"}
          </button>
        </div>
      )}

      {webhooks.length === 0 && !showAddForm ? (
        <div className="rounded-2xl border border-ink-100 bg-ink-100/60 px-6 py-10 text-center">
          <WebhookIcon className="mx-auto h-6 w-6 text-ink-500" />
          <p className="mt-3 text-base font-bold text-ink-900">No webhooks configured</p>
          <p className="mt-1 text-sm text-ink-500">
            Add an endpoint to start receiving delivery event notifications.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {webhooks.map((webhook) => (
            <div key={webhook.id} className="rounded-2xl bg-white p-4 border border-ink-100">
              <div className="flex items-start justify-between">
                <div className="flex-1">
                  <p className="text-sm font-semibold text-ink-900">{webhook.url}</p>
                  <p className="mt-0.5 text-xs text-ink-500">
                    Status: {webhook.status} &middot; Created: {new Date(webhook.created_at).toLocaleDateString()}
                  </p>
                  <div className="mt-2 flex flex-wrap gap-1">
                    {webhook.events.map((event: string) => (
                      <span key={event} className="rounded-full bg-brand-100 px-2 py-0.5 text-xs text-brand-700">
                        {event}
                      </span>
                    ))}
                  </div>
                </div>
                <button
                  onClick={() => handleDelete(webhook.id)}
                  className="rounded-lg p-2 text-danger hover:bg-danger/10"
                  title="Delete webhook"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
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