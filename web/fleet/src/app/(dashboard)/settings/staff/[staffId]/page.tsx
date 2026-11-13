// src/app/(dashboard)/settings/staff/[staffId]/page.tsx

"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import { User, UserX, UserCheck, ArrowLeft, AlertCircle, ShieldCheck } from "lucide-react";
import { staffService, staffErrorMessage, STAFF_STATUS_LABEL } from "@/lib/services/staffService";
import { structureService } from "@/lib/services/structureService";
import type { Permission, Role, StaffMember } from "@/lib/types/staff";
import type { Branch, Department, RoleBinding } from "@/lib/types/structure";
import { StaffDetailsEditor, StaffRoleBindingsEditor } from "./staff-editors";

type StatusStyle = {
  bg: string;
  text: string;
  dot: string;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Suspended: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Invited: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Inactive: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const formatDateTime = (date: string | null): string => {
  if (!date) return "—";
  return new Date(date).toLocaleString("en-GB", { day: "2-digit", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });
};

export default function StaffAccessDetailPage() {
  const params = useParams();
  const router = useRouter();
  const staffId = params.staffId as string;
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<StaffMember | null>(null);
  const [roles, setRoles] = useState<Role[]>([]);
  const [permissions, setPermissions] = useState<Permission[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState<boolean>(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState<boolean>(false);
  const [showAll, setShowAll] = useState<boolean>(false);
  const [bindings, setBindings] = useState<RoleBinding[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [departments, setDepartments] = useState<Department[]>([]);

  const load = useCallback(async () => {
    try {
      const [member, allRoles, allPermissions, memberBindings, allBranches, allDepartments] = await Promise.all([
        staffService.get(staffId),
        staffService.roles().catch(() => [] as Role[]),
        staffService.permissions().catch(() => [] as Permission[]),
        staffService.roleBindings(staffId).catch(() => [] as RoleBinding[]),
        structureService.listBranches().catch(() => [] as Branch[]),
        structureService.listDepartments().catch(() => [] as Department[]),
      ]);
      setError(null);
      setData(member);
      setRoles(allRoles);
      setPermissions(allPermissions);
      setBindings(memberBindings);
      setBranches(allBranches);
      setDepartments(allDepartments);
    } catch (e) {
      const status = (e as { response?: { status?: number } })?.response?.status;
      if (status === 404) setNotFound(true);
      else setError(staffErrorMessage(e, "Staff details could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [staffId]);

  useEffect(() => {
    // Started from a promise callback so no state is set synchronously
    // inside the effect.
    Promise.resolve().then(load);
  }, [load]);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading staff details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          {notFound ? <User className="h-8 w-8 text-ink-400" /> : <AlertCircle className="h-8 w-8 text-red" />}
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">{notFound ? "Staff not found" : "Staff could not be loaded"}</h3>
        <p className="mt-1 text-sm text-ink-500">{notFound ? "No staff member found with this ID." : error}</p>
        <button
          onClick={() => router.push("/settings/staff")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Staff
        </button>
      </div>
    );
  }

  const statusLabel = STAFF_STATUS_LABEL[data.status] ?? data.status;
  const statusStyle = STATUS_STYLES[statusLabel] || STATUS_STYLES.Inactive;
  const roleIds = new Set(data.roles.map((r) => r.id));
  const granted = new Set(roles.filter((r) => roleIds.has(r.id)).flatMap((r) => r.permissions ?? []));
  const permissionLabel = new Map(permissions.map((p) => [p.code, p.description || p.code]));
  const effective = [...granted].sort();

  const changeStatus = async (action: "suspend" | "reactivate") => {
    setBusy(true);
    setActionError(null);
    try {
      if (action === "suspend") await staffService.suspend(data.id, data.version);
      else await staffService.reactivate(data.id, data.version);
      await load();
    } catch (e) {
      setActionError(staffErrorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button onClick={() => router.push("/settings/staff")} className="hover:text-primary transition-colors">
              Staff
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.fullName || data.email}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">{data.fullName || data.email}</h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
              {statusLabel}
            </span>
            {data.isOwner && (
              <span className="rounded-full bg-primary-bg px-2 py-0.5 text-[9px] font-semibold text-primary">Owner</span>
            )}
            {data.jobTitle && <span className="text-[10px] text-ink-500">{data.jobTitle}</span>}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2 flex-shrink-0">
          {!data.isOwner && data.status === "ACTIVE" && (
            <button
              onClick={() => changeStatus("suspend")}
              disabled={busy}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-red/20 bg-red-bg px-3.5 text-[11.5px] font-medium text-red shadow-sm transition hover:bg-red/10 disabled:opacity-50"
            >
              <UserX className="h-3.5 w-3.5" />
              Suspend
            </button>
          )}
          {!data.isOwner && data.status === "SUSPENDED" && (
            <button
              onClick={() => changeStatus("reactivate")}
              disabled={busy}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-green/20 bg-green-bg px-3.5 text-[11.5px] font-medium text-green shadow-sm transition hover:bg-green/10 disabled:opacity-50"
            >
              <UserCheck className="h-3.5 w-3.5" />
              Reactivate
            </button>
          )}
          <button
            onClick={() => router.push("/settings/staff")}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
        </div>
      </div>

      {actionError && (
        <div role="alert" className="flex items-start gap-2 rounded-xl border border-red/30 bg-red-bg px-4 py-3 text-sm text-red">
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          {actionError}
        </div>
      )}

      {/* ===== OVERVIEW GRID ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Contact</div>
          <div className="mt-1 text-sm font-medium text-ink-950 break-all">{data.email || "—"}</div>
          <div className="mt-0.5 text-xs text-ink-500">{data.phone || "—"}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Role</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.roles.map((r) => r.name).join(", ") || "No role"}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Scope</div>
          <div className="mt-1 text-sm font-medium text-ink-950">
            {data.branchIds.length === 0 ? "Company-wide" : `${data.branchIds.length} branch${data.branchIds.length === 1 ? "" : "es"}`}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Membership</div>
          <div className="mt-1 text-sm font-medium text-ink-950">Joined {formatDateTime(data.activatedAt ?? data.createdAt)}</div>
          {data.suspendedAt && <div className="mt-0.5 text-xs text-red">Suspended {formatDateTime(data.suspendedAt)}</div>}
        </div>
      </div>

      {/* ===== DETAILS AND ROLES ===== */}
      {data.status !== "ENDED" && (
        <>
          <StaffDetailsEditor member={data} branches={branches} departments={departments} onSaved={load} />
          <StaffRoleBindingsEditor
            staffId={data.id}
            bindings={bindings}
            roles={roles}
            branches={branches}
            departments={departments}
            onSaved={load}
          />
        </>
      )}

      {/* ===== PERMISSIONS ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <h3 className="flex items-center gap-2 text-[15px] font-semibold text-ink-950">
          <ShieldCheck className="h-4 w-4 text-primary" />
          Effective Permissions
          <span className="text-xs font-normal text-ink-400">{effective.length}</span>
        </h3>
        {effective.length === 0 ? (
          <p className="mt-3 text-sm text-ink-500">This staff member&apos;s roles grant no permissions.</p>
        ) : (
          <div className="mt-3 flex flex-wrap gap-2">
            {(showAll ? effective : effective.slice(0, 30)).map((perm) => (
              <span key={perm} title={perm} className="rounded-full bg-primary-bg px-3 py-1 text-xs font-medium text-primary">
                {permissionLabel.get(perm) ?? perm}
              </span>
            ))}
            {effective.length > 30 && (
              <button
                onClick={() => setShowAll((v) => !v)}
                className="rounded-full border border-line px-3 py-1 text-xs font-semibold text-ink-600 hover:bg-surface-2"
              >
                {showAll ? "Show fewer" : `Show all ${effective.length}`}
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
