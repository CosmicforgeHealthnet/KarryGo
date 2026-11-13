// src/app/(dashboard)/settings/staff/page.tsx

"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import {
  Users,
  UserPlus,
  UserCheck,
  Clock,
  UserX,
  Search,
  SlidersHorizontal,
  X,
  Eye,
  Send,
  XCircle,
  AlertCircle,
} from "lucide-react";
import { staffService, staffErrorMessage, INVITATION_STATUS_LABEL } from "@/lib/services/staffService";
import type { Role } from "@/lib/types/staff";

type Row = {
  key: string;
  kind: "member" | "invitation";
  id: string;
  name: string;
  email: string;
  phone: string;
  role: string;
  scope: string;
  status: string;
  invitationState: string;
  date: string | null;
  owner: boolean;
};

type StatusStyle = {
  bg: string;
  text: string;
  dot: string;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Suspended: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Invited: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  Inactive: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

type InvitationStyle = {
  bg: string;
  text: string;
};

const INVITATION_STYLES: Record<string, InvitationStyle> = {
  Sent: { bg: "bg-primary-bg", text: "text-primary" },
  Accepted: { bg: "bg-green-bg", text: "text-green" },
  Expired: { bg: "bg-red-bg", text: "text-red" },
  Pending: { bg: "bg-amber-bg", text: "text-amber" },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500" },
  None: { bg: "bg-surface-2", text: "text-ink-500" },
};

const STATUS_FILTERS = ["All", "Active", "Invited", "Suspended", "Inactive"];

const scopeLabel = (branchIds: string[]) =>
  branchIds.length === 0 ? "Company-wide" : `${branchIds.length} branch${branchIds.length === 1 ? "" : "es"}`;

const memberStatus = (status: string) =>
  status === "ACTIVE" ? "Active" : status === "SUSPENDED" ? "Suspended" : status === "INVITED" ? "Invited" : "Inactive";

export default function StaffDirectoryPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [data, setData] = useState<Row[]>([]);
  const [search, setSearch] = useState<string>("");
  const [selectedStatus, setSelectedStatus] = useState<string>("All");
  const [showFilters, setShowFilters] = useState<boolean>(false);

  const load = useCallback(async () => {
    try {
      const [members, invitations, roles] = await Promise.all([
        staffService.list(),
        staffService.invitations(),
        staffService.roles().catch(() => [] as Role[]),
      ]);
      setError(null);
      const roleNames = new Map(roles.map((r) => [r.id, r.name]));
      const rows: Row[] = [
        ...members.map((m) => ({
          key: `m-${m.id}`,
          kind: "member" as const,
          id: m.id,
          name: m.fullName || m.email || "Unnamed",
          email: m.email,
          phone: m.phone,
          role: m.roles.map((r) => r.name).join(", ") || (m.isOwner ? "Owner" : "No role"),
          scope: scopeLabel(m.branchIds),
          status: memberStatus(m.status),
          invitationState: m.invitedAt ? (m.activatedAt ? "Accepted" : "Sent") : "None",
          date: m.activatedAt ?? m.createdAt,
          owner: m.isOwner,
        })),
        ...invitations
          .filter((i) => i.status === "PENDING" || i.status === "EXPIRED")
          .map((i) => ({
            key: `i-${i.id}`,
            kind: "invitation" as const,
            id: i.id,
            name: i.fullName || i.email,
            email: i.email,
            phone: i.phone,
            role: i.roleIds.map((r) => roleNames.get(r) ?? r).join(", ") || "No role",
            scope: scopeLabel(i.branchIds),
            status: "Invited",
            invitationState: INVITATION_STATUS_LABEL[i.status] ?? i.status,
            date: i.createdAt,
            owner: false,
          })),
      ];
      setData(rows);
    } catch (e) {
      setError(staffErrorMessage(e, "We couldn't load your staff. Please try again."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    // Started from a promise callback so no state is set synchronously
    // inside the effect.
    Promise.resolve().then(load);
  }, [load]);

  const invitationAction = async (row: Row, action: "resend" | "cancel") => {
    setBusy(row.key);
    setNotice(null);
    try {
      if (action === "resend") {
        await staffService.resendInvitation(row.id);
        setNotice(`Invitation resent to ${row.email}.`);
      } else {
        await staffService.cancelInvitation(row.id);
        setNotice(`Invitation to ${row.email} cancelled.`);
      }
      await load();
    } catch (e) {
      setNotice(staffErrorMessage(e));
    } finally {
      setBusy(null);
    }
  };

  const filteredData = data.filter((item) => {
    const q = search.trim().toLowerCase();
    const matchesSearch =
      q === "" ||
      item.name.toLowerCase().includes(q) ||
      item.email.toLowerCase().includes(q) ||
      item.phone.toLowerCase().includes(q) ||
      item.role.toLowerCase().includes(q);

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesStatus;
  });

  const totalActive = data.filter((d) => d.status === "Active").length;
  const totalPending = data.filter((d) => d.status === "Invited").length;
  const totalSuspended = data.filter((d) => d.status === "Suspended").length;

  const hasActiveFilters = selectedStatus !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
  };

  const filterCount = selectedStatus !== "All" ? 1 : 0;

  const formatDate = (date: string | null): string => {
    if (!date) return "—";
    return new Date(date).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading staff directory...</span>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
        <div className="rounded-full bg-red-bg p-4">
          <AlertCircle className="h-8 w-8 text-red" />
        </div>
        <h3 className="mt-3 text-sm font-semibold text-ink-950">Staff could not be loaded</h3>
        <p className="mt-1 text-sm text-ink-500">{error}</p>
        <button onClick={() => { setLoading(true); load(); }} className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors">
          Try again
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">Staff Directory</h1>
          <p className="mt-1.5 text-sm text-ink-600">
            Your company&apos;s office staff: who they are, their roles, scope and invitation state. Drivers and riders are managed under Providers.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={() => router.push("/settings/staff/invite")}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <UserPlus className="h-3.5 w-3.5" />
            Invite Staff
          </button>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Users className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Total</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <UserCheck className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalActive}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Pending</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalPending}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <UserX className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Suspended</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalSuspended}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, email, role..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <UserCheck className="h-3.5 w-3.5 text-ink-400" />
          {selectedStatus}
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {filterCount}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filteredData.length} staff
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</span>
              <div className="flex flex-wrap gap-1.5">
                {STATUS_FILTERS.map((status) => (
                  <button
                    key={status}
                    onClick={() => setSelectedStatus(status)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedStatus === status
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {status}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {notice && (
        <div role="status" className="rounded-lg border border-line bg-white px-4 py-3 text-sm text-ink-700">
          {notice}
        </div>
      )}

      {/* ── Staff Table ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Users className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No staff found</h3>
          <p className="mt-1 text-sm text-ink-500">
            {data.length === 0 ? "Invite your first staff member to get started." : "No staff members match your current filters."}
          </p>
          <button onClick={data.length === 0 ? () => router.push("/settings/staff/invite") : clearFilters} className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors">
            {data.length === 0 ? "Invite staff" : "Clear filters"}
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-line bg-white">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[800px]">
              <thead>
                <tr className="bg-surface-1">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Staff</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Role</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Scope</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Invitation</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Since</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;
                  const invitationStyle = INVITATION_STYLES[item.invitationState] || INVITATION_STYLES.None;
                  const isMember = item.kind === "member";

                  return (
                    <tr
                      key={item.key}
                      onClick={isMember ? () => router.push(`/settings/staff/${item.id}`) : undefined}
                      className={`border-b border-line hover:bg-surface-2 transition-colors group ${isMember ? "cursor-pointer" : ""}`}
                    >
                      <td className="px-4 py-3 whitespace-nowrap">
                        <div className="font-medium text-ink-950 text-xs">
                          {item.name}
                          {item.owner && <span className="ml-1.5 rounded-full bg-primary-bg px-1.5 py-0.5 text-[9px] font-semibold text-primary">Owner</span>}
                        </div>
                        <div className="text-[10px] text-ink-500">{item.email}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs whitespace-nowrap">{item.role}</td>
                      <td className="px-4 py-3 text-ink-600 text-xs whitespace-nowrap">{item.scope}</td>
                      <td className="px-4 py-3 whitespace-nowrap">
                        <span className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${invitationStyle.bg} ${invitationStyle.text}`}>
                          {item.invitationState}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-xs whitespace-nowrap">{formatDate(item.date)}</td>
                      <td className="px-4 py-3 text-right whitespace-nowrap">
                        {isMember ? (
                          <button aria-label={`View ${item.name}`} className="text-ink-400 group-hover:text-primary transition-colors">
                            <Eye className="h-4 w-4" />
                          </button>
                        ) : (
                          <div className="flex items-center justify-end gap-2">
                            <button
                              onClick={() => invitationAction(item, "resend")}
                              disabled={busy === item.key}
                              className="flex items-center gap-1 rounded-md border border-line px-2 py-1 text-[10px] font-semibold text-ink-700 hover:bg-surface-2 disabled:opacity-50"
                            >
                              <Send className="h-3 w-3" />
                              Resend
                            </button>
                            <button
                              onClick={() => invitationAction(item, "cancel")}
                              disabled={busy === item.key}
                              className="flex items-center gap-1 rounded-md border border-line px-2 py-1 text-[10px] font-semibold text-red hover:bg-red-bg disabled:opacity-50"
                            >
                              <XCircle className="h-3 w-3" />
                              Cancel
                            </button>
                          </div>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}