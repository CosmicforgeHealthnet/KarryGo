// src/app/(dashboard)/settings/roles/[roleId]/page.tsx

"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Shield,
  RefreshCw,
  Edit3,
  ArrowLeft,
  Save,
  Clock,
  Archive,
  Search,
} from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS, FormField } from "@/components/features/FormField";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { staffService } from "@/lib/services/staffService";
import type { Permission, Role } from "@/lib/types/staff";

// "/settings/roles/new" opens this editor in create mode.
const NEW_ROLE_ID = "new";

type StatusStyle = {
  bg: string;
  text: string;
  dot: string;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Inactive: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Archived: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const statusOf = (status: string) =>
  status === "ACTIVE" ? "Active" : status === "ARCHIVED" ? "Archived" : "Inactive";

const titleCase = (value: string) =>
  value.replace(/[_.]+/g, " ").replace(/\b\w/g, (m) => m.toUpperCase()).trim();

export default function RoleEditorPage() {
  const params = useParams();
  const router = useRouter();
  const roleId = params.roleId as string;
  const creating = roleId === NEW_ROLE_ID;

  const [loading, setLoading] = useState<boolean>(true);
  const [role, setRole] = useState<Role | null>(null);
  const [catalogue, setCatalogue] = useState<Permission[]>([]);
  const [assignedUsers, setAssignedUsers] = useState<number>(0);
  const [loadError, setLoadError] = useState<ApiErrorInfo | null>(null);
  const [notFound, setNotFound] = useState<boolean>(false);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);

  const [isEditing, setIsEditing] = useState<boolean>(creating);
  const [isSaving, setIsSaving] = useState<boolean>(false);
  const [saveError, setSaveError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [filter, setFilter] = useState<string>("");

  const [code, setCode] = useState<string>("");
  const [name, setName] = useState<string>("");
  const [description, setDescription] = useState<string>("");
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const resetForm = (value: Role | null) => {
    setCode(value?.code ?? "");
    setName(value?.name ?? "");
    setDescription(value?.description ?? "");
    setSelected(new Set(value?.permissions ?? []));
  };

  const load = useCallback(async () => {
    try {
      const [permissions, loaded, members] = await Promise.all([
        staffService.permissions(),
        creating ? Promise.resolve(null) : staffService.getRole(roleId),
        creating ? Promise.resolve([]) : staffService.list().catch(() => []),
      ]);
      setCatalogue(permissions.filter((p) => p.isActive));
      setRole(loaded);
      resetForm(loaded);
      setAssignedUsers(members.filter((m) => m.roles.some((r) => r.id === roleId)).length);
      setLoadError(null);
      setSecondsAgo(0);
    } catch (e) {
      const status = (e as { response?: { status?: number } })?.response?.status;
      if (status === 404) setNotFound(true);
      else setLoadError(describeApiError(e, "The role could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [creating, roleId]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading role details...</p>
      </div>
    );
  }

  if (!creating && !role) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Shield className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">{notFound ? "Role not found" : "Role could not be loaded"}</h3>
        <p className="mt-1 text-sm text-ink-500">{notFound ? "No role found with this ID." : loadError?.message}</p>
        {loadError?.requestId && <p className="mt-1 text-xs text-ink-400">Reference: {loadError.requestId}</p>}
        <button
          onClick={() => router.push("/settings/roles")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Roles
        </button>
      </div>
    );
  }

  const isSystem = role?.isSystem ?? false;
  const status = role ? statusOf(role.status) : "Active";
  const statusStyle = STATUS_STYLES[status] || STATUS_STYLES.Active;
  const editable = creating || (!isSystem && role?.status !== "ARCHIVED");

  const formatDateTime = (date: string): string =>
    new Date(date).toLocaleString("en-GB", { day: "2-digit", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });

  const togglePermission = (permission: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(permission)) next.delete(permission);
      else next.add(permission);
      return next;
    });
  };

  const handleSave = async () => {
    setIsSaving(true);
    setSaveError(null);
    setNotice(null);
    const body = { name: name.trim(), description: description.trim(), permissions: [...selected].sort() };
    try {
      if (creating) {
        const created = await staffService.createRole({ ...body, code: code.trim().toUpperCase() });
        router.replace(`/settings/roles/${encodeURIComponent(created.id)}`);
        return;
      }
      if (!role) return;
      const saved = await staffService.updateRole(role.id, body, role.version);
      setRole(saved);
      resetForm(saved);
      setIsEditing(false);
      setNotice("Role updated.");
    } catch (e) {
      setSaveError(describeApiError(e, "The role could not be saved."));
    } finally {
      setIsSaving(false);
    }
  };

  const handleArchive = async () => {
    if (!role) return;
    if (!window.confirm(`Archive ${role.name}? Staff holding it lose its permissions.`)) return;
    setIsSaving(true);
    setSaveError(null);
    try {
      await staffService.archiveRole(role.id, role.version);
      await load();
      setNotice("Role archived.");
    } catch (e) {
      setSaveError(describeApiError(e, "The role could not be archived."));
    } finally {
      setIsSaving(false);
    }
  };

  // Viewing shows what the role grants; editing shows the whole catalogue.
  const q = filter.trim().toLowerCase();
  const rows = (isEditing ? catalogue : catalogue.filter((p) => selected.has(p.code)))
    .filter((p) => q === "" || `${p.code} ${p.description} ${p.category}`.toLowerCase().includes(q));
  // A granted permission no longer in the catalogue is still shown.
  const known = new Set(catalogue.map((p) => p.code));
  const unknownGranted = [...selected].filter((p) => !known.has(p));

  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/settings/roles")}
              className="hover:text-primary transition-colors"
            >
              Roles
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{creating ? "New role" : role?.code}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            {creating ? "New role" : role?.name}
          </h1>
          {role && (
            <div className="mt-1 flex flex-wrap items-center gap-2">
              <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                {status}
              </span>
              <span className="text-[10px] text-ink-500">{isSystem ? "System" : "Custom"}</span>
              <span className="text-[10px] text-ink-500 font-mono">v{role.version}</span>
            </div>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2 flex-shrink-0">
          {!creating && (
            <button
              onClick={() => load()}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
            >
              <RefreshCw className="h-3 w-3" />
              {secondsAgo}s
            </button>
          )}
          {!creating && editable && !isEditing && (
            <button
              onClick={() => {
                setIsEditing(true);
                setNotice(null);
              }}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-primary/20 bg-primary-bg px-3.5 text-[11.5px] font-medium text-primary shadow-sm transition hover:bg-primary/10"
            >
              <Edit3 className="h-3.5 w-3.5" />
              Edit Role
            </button>
          )}
          {!creating && editable && !isEditing && (
            <button
              onClick={handleArchive}
              disabled={isSaving}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-red/20 bg-red-bg px-3.5 text-[11.5px] font-medium text-red shadow-sm transition hover:bg-red/10 disabled:opacity-50"
            >
              <Archive className="h-3.5 w-3.5" />
              Archive
            </button>
          )}
          <button
            onClick={() => router.push("/settings/roles")}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
        </div>
      </div>

      <ApiErrorNotice error={loadError ?? saveError} />
      {notice && <div className="rounded-xl border border-green/30 bg-green-bg px-4 py-3 text-sm text-green">{notice}</div>}

      {/* ===== DESCRIPTION ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        {isEditing ? (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <FormField label="Code" required={creating} hint={creating ? "Upper-case letters, digits or underscores." : undefined}>
              <input
                required={creating}
                disabled={!creating}
                value={code}
                onChange={(e) => setCode(e.target.value)}
                placeholder="DISPATCH_LEAD"
                className={FORM_INPUT_CLASS}
              />
            </FormField>
            <FormField label="Name" required>
              <input required value={name} onChange={(e) => setName(e.target.value)} placeholder="Dispatch lead" className={FORM_INPUT_CLASS} />
            </FormField>
            <div className="sm:col-span-2">
              <FormField label="Description">
                <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={2} className={FORM_INPUT_CLASS} />
              </FormField>
            </div>
          </div>
        ) : (
          <>
            <p className="text-sm text-ink-700">{role?.description || "No description."}</p>
            {role && (
              <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-ink-500">
                <span>Created: {formatDateTime(role.createdAt)}</span>
                <span>·</span>
                <span>Updated: {formatDateTime(role.updatedAt)}</span>
                <span>·</span>
                <span>{assignedUsers} users assigned</span>
                {isSystem && (
                  <>
                    <span>·</span>
                    <span>System roles are managed by the platform and cannot be changed.</span>
                  </>
                )}
              </div>
            )}
          </>
        )}
      </div>

      {/* ===== PERMISSIONS ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="flex flex-wrap items-center justify-between gap-3 p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <Shield className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Permissions</span>
            <span className="text-xs text-ink-400">
              {selected.size}/{catalogue.length} enabled
            </span>
          </div>
          <div className="flex h-8 items-center gap-2 rounded-lg border border-line bg-white px-2.5">
            <Search className="h-3.5 w-3.5 text-ink-400" />
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Filter permissions..."
              className="w-40 bg-transparent text-xs text-ink-950 outline-none placeholder:text-ink-400"
            />
          </div>
        </div>
        {rows.length === 0 && unknownGranted.length === 0 ? (
          <p className="p-4 text-sm text-ink-500">{isEditing ? "No permissions match your filter." : "This role grants no permissions."}</p>
        ) : (
          <div className="divide-y divide-line">
            {rows.map((perm) => {
              const enabled = selected.has(perm.code);
              return (
                <div
                  key={perm.code}
                  className={`flex items-center gap-4 p-4 transition ${isSystem ? "opacity-60" : "hover:bg-surface-2"}`}
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium text-ink-950 text-sm">{titleCase(perm.code)}</span>
                      <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                        {titleCase(perm.category || perm.code.split(".")[0] || "general")}
                      </span>
                    </div>
                    <div className="mt-0.5 text-xs text-ink-500">{perm.description || perm.code}</div>
                  </div>
                  <div className="flex-shrink-0">
                    {isEditing ? (
                      <button
                        type="button"
                        role="switch"
                        aria-checked={enabled}
                        aria-label={perm.code}
                        onClick={() => togglePermission(perm.code)}
                        className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full transition-colors ${
                          enabled ? "bg-primary" : "bg-ink-300"
                        }`}
                      >
                        <span
                          className={`inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform ${
                            enabled ? "translate-x-5" : "translate-x-0.5"
                          } mt-0.5`}
                        />
                      </button>
                    ) : (
                      <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${
                        enabled ? "bg-green-bg text-green" : "bg-surface-2 text-ink-500"
                      }`}>
                        {enabled ? "Enabled" : "Disabled"}
                      </span>
                    )}
                  </div>
                </div>
              );
            })}
            {unknownGranted.map((code) => (
              <div key={code} className="flex items-center gap-4 p-4">
                <div className="flex-1 min-w-0">
                  <span className="font-mono text-sm text-ink-700">{code}</span>
                  <div className="mt-0.5 text-xs text-ink-500">No longer in the permission catalogue.</div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* ===== SAVE ===== */}
      {isEditing && (
        <div className="flex flex-wrap gap-3">
          <button
            onClick={handleSave}
            disabled={isSaving || name.trim() === "" || (creating && code.trim() === "")}
            className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
              isSaving
                ? "bg-ink-400 cursor-not-allowed"
                : "bg-primary shadow-primary/20 hover:bg-primary/90 disabled:opacity-60"
            }`}
          >
            {isSaving ? (
              <>
                <RefreshCw className="h-4 w-4 animate-spin" />
                Saving...
              </>
            ) : (
              <>
                <Save className="h-4 w-4" />
                {creating ? "Create Role" : "Save Changes"}
              </>
            )}
          </button>
          <button
            onClick={() => {
              if (creating) {
                router.push("/settings/roles");
                return;
              }
              resetForm(role);
              setIsEditing(false);
              setSaveError(null);
            }}
            className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
          >
            Cancel
          </button>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          IAM-05 · Role Editor{role ? ` · ${role.code}` : ""}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          {role ? `Updated ${secondsAgo}s ago · v${role.version}` : "Not saved yet"}
        </span>
      </div>
    </div>
  );
}
