// src/app/(dashboard)/settings/staff/invite/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { RefreshCw, Send, UserPlus, AlertCircle } from "lucide-react";
import { staffService, staffErrorMessage } from "@/lib/services/staffService";
import type { Role } from "@/lib/types/staff";

type InviteFormData = {
  email: string;
  name: string;
  phone: string;
  roleId: string;
};

// The owner role belongs to the company's founder and is never granted by
// invitation.
const INVITABLE = (role: Role) => role.status === "ACTIVE" && role.code !== "OWNER" && role.id !== "rol_system_owner";

export default function InviteStaffPage() {
  const router = useRouter();
  const [roles, setRoles] = useState<Role[]>([]);
  const [rolesError, setRolesError] = useState<string | null>(null);
  const [formData, setFormData] = useState<InviteFormData>({ email: "", name: "", phone: "", roleId: "" });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);

  useEffect(() => {
    staffService
      .roles()
      .then((all) => {
        const invitable = all.filter(INVITABLE);
        setRoles(invitable);
        setFormData((prev) => ({ ...prev, roleId: prev.roleId || invitable[0]?.id || "" }));
      })
      .catch((e) => setRolesError(staffErrorMessage(e, "Roles could not be loaded.")));
  }, []);

  const validateForm = () => {
    const newErrors: Record<string, string> = {};
    if (!formData.email.trim()) newErrors.email = "Email is required";
    else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email.trim())) newErrors.email = "Invalid email address";
    if (!formData.name.trim()) newErrors.name = "Name is required";
    if (!formData.phone.trim()) newErrors.phone = "Phone number is required";
    else if (formData.phone.replace(/\D/g, "").length < 10) newErrors.phone = "Enter a valid phone number";
    if (!formData.roleId) newErrors.role = "Role is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async () => {
    if (!validateForm()) return;
    setIsSubmitting(true);
    setSubmitError(null);
    try {
      await staffService.invite({
        fullName: formData.name.trim(),
        email: formData.email.trim(),
        phone: formData.phone.trim(),
        roleIds: [formData.roleId],
      });
      router.push("/settings/staff");
    } catch (e) {
      setSubmitError(staffErrorMessage(e, "The invitation could not be sent. Please try again."));
      setIsSubmitting(false);
    }
  };

  const selectedRole = roles.find((r) => r.id === formData.roleId);
  const inputClass = (field: string) =>
    `mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
      errors[field] ? "border-red focus:border-red" : "border-line focus:border-primary"
    }`;

  return (
    <div className="space-y-5 max-w-3xl mx-auto">
      {/* ===== HEADER ===== */}
      <div>
        <div className="flex items-center gap-2 text-sm text-ink-500">
          <button onClick={() => router.push("/settings/staff")} className="hover:text-primary transition-colors">
            Staff
          </button>
          <span>/</span>
          <span className="text-ink-700 font-medium">Invite Staff</span>
        </div>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-ink-950">Invite Staff</h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Send an invitation to a new staff member. They sign up with this email and phone number to join your company.
        </p>
      </div>

      {/* ===== FORM ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-6">
          {(submitError || rolesError) && (
            <div role="alert" className="mb-4 flex items-start gap-2 rounded-xl border border-red/30 bg-red-bg px-4 py-3 text-sm text-red">
              <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
              {submitError || rolesError}
            </div>
          )}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {/* Name */}
            <div className="sm:col-span-2">
              <label htmlFor="invite-name" className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Full Name <span className="text-red">*</span>
              </label>
              <input
                id="invite-name"
                type="text"
                value={formData.name}
                onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                placeholder="e.g. John Doe"
                className={inputClass("name")}
              />
              {errors.name && <p className="mt-1 text-sm text-red">{errors.name}</p>}
            </div>

            {/* Email */}
            <div>
              <label htmlFor="invite-email" className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Email Address <span className="text-red">*</span>
              </label>
              <input
                id="invite-email"
                type="email"
                value={formData.email}
                onChange={(e) => setFormData((prev) => ({ ...prev, email: e.target.value }))}
                placeholder="colleague@company.com"
                className={inputClass("email")}
              />
              {errors.email && <p className="mt-1 text-sm text-red">{errors.email}</p>}
            </div>

            {/* Phone */}
            <div>
              <label htmlFor="invite-phone" className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Phone Number <span className="text-red">*</span>
              </label>
              <input
                id="invite-phone"
                type="tel"
                value={formData.phone}
                onChange={(e) => setFormData((prev) => ({ ...prev, phone: e.target.value }))}
                placeholder="e.g. 08012345678"
                className={inputClass("phone")}
              />
              {errors.phone && <p className="mt-1 text-sm text-red">{errors.phone}</p>}
            </div>

            {/* Role */}
            <div className="sm:col-span-2">
              <label htmlFor="invite-role" className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Role <span className="text-red">*</span>
              </label>
              <select
                id="invite-role"
                value={formData.roleId}
                onChange={(e) => setFormData((prev) => ({ ...prev, roleId: e.target.value }))}
                className={inputClass("role")}
                disabled={roles.length === 0}
              >
                {roles.length === 0 && <option value="">Loading roles…</option>}
                {roles.map((r) => (
                  <option key={r.id} value={r.id}>{r.name}</option>
                ))}
              </select>
              {selectedRole?.description && <p className="mt-1 text-xs text-ink-500">{selectedRole.description}</p>}
              {errors.role && <p className="mt-1 text-sm text-red">{errors.role}</p>}
            </div>
          </div>

          {/* Submit Buttons */}
          <div className="mt-6 flex flex-wrap gap-3">
            <button
              onClick={handleSubmit}
              disabled={isSubmitting}
              className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
                isSubmitting ? "bg-ink-400 cursor-not-allowed" : "bg-primary shadow-primary/20 hover:bg-primary/90"
              }`}
            >
              {isSubmitting ? (
                <>
                  <RefreshCw className="h-4 w-4 animate-spin" />
                  Sending...
                </>
              ) : (
                <>
                  <Send className="h-4 w-4" />
                  Send Invitation
                </>
              )}
            </button>
            <button
              onClick={() => router.push("/settings/staff")}
              className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
            >
              Cancel
            </button>
          </div>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <UserPlus className="h-4 w-4" />
          The invitation is sent by email and expires after a few days. You can resend or cancel it from the staff list.
        </span>
      </div>
    </div>
  );
}
