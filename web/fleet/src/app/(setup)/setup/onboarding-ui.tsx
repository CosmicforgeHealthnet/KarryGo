"use client";
import {
  Suspense,
  useEffect,
  useRef,
  type ReactNode,
  type InputHTMLAttributes,
  type SelectHTMLAttributes,
  type FormEvent,
} from "react";
import Link from "next/link";
import { ArrowLeft, ArrowRight, Loader2 } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { previousPath, nextPath } from "./steps";

export const control =
  "w-full min-w-0 rounded-xl border border-field-border bg-surface px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-label-accent focus:ring-4 focus:ring-primary/15 disabled:opacity-60";
export function humanize(code: string) {
  return code
    .toLowerCase()
    .replaceAll("_", " ")
    .replace(/(^|\s)\S/g, (c) => c.toUpperCase());
}
type SafeError = {
  response?: {
    status?: number;
    data?: {
      error?: {
        message?: string;
        requestId?: string;
        request_id?: string;
        details?: { field?: string; message?: string }[];
      };
    };
  };
  message?: string;
};
export function errorMessage(error: unknown) {
  const e = error as SafeError;
  const status = e?.response?.status;
  const body = e?.response?.data?.error;
  const fallback =
    status === 401
      ? "Your session expired. Sign in again to continue."
      : status === 403
        ? "You do not have permission to change this company's information."
        : status === 409 || status === 412
          ? "This record changed elsewhere. Your entries are retained. Reload the saved record before retrying."
          : status === 503
            ? "The service is unavailable. Keep your entries and try again shortly."
            : !e?.response
              ? "We couldn't connect. Check your connection and try again."
              : "An unexpected error occurred. Please retry or contact support.";
  const message =
    status && status >= 500 && status !== 503
      ? fallback
      : body?.message || fallback;
  return {
    message,
    requestId: body?.requestId ?? body?.request_id,
    fields: body?.details ?? [],
  };
}
export function ErrorNotice({
  error,
  onRetry,
  retryLabel = "Reload saved information",
}: {
  error: unknown;
  onRetry?: () => void;
  retryLabel?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.focus();
  }, [error]);
  const safe = errorMessage(error);
  return (
    <div
      ref={ref}
      tabIndex={-1}
      role="alert"
      className="rounded-xl border border-red/30 bg-red-bg p-4 text-sm text-ink-950 outline-none"
    >
      <p>{safe.message}</p>
      {safe.fields.length > 0 && (
        <ul className="mt-2 list-disc pl-5">
          {safe.fields.map((f, i) => (
            <li key={i}>
              {f.field ? `${humanize(f.field)}: ` : ""}
              {f.message}
            </li>
          ))}
        </ul>
      )}
      {safe.requestId && (
        <p className="mt-2 break-all text-xs">Reference: {safe.requestId}</p>
      )}
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="btn btn-secondary mt-3"
        >
          {retryLabel}
        </button>
      )}
    </div>
  );
}
export function Loading() {
  return (
    <div
      role="status"
      aria-live="polite"
      className="animate-pulse space-y-4 p-6"
    >
      <p>Loading your saved company information…</p>
      <div className="h-10 rounded-xl bg-surface-2" />
      <div className="h-40 rounded-xl bg-surface-2" />
    </div>
  );
}
export function Field({
  label,
  name,
  hint,
  ...props
}: InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  name: string;
  hint?: string;
}) {
  return (
    <div className="min-w-0">
      <label
        htmlFor={name}
        className="mb-1 block text-[13px] font-semibold text-ink-900"
      >
        {label}
        {props.required && <span aria-hidden="true"> *</span>}
      </label>
      <input
        {...props}
        id={name}
        name={name}
        className={control}
        aria-describedby={hint ? `${name}-hint` : undefined}
      />
      {hint && (
        <p id={`${name}-hint`} className="mt-1 text-xs text-ink-500">
          {hint}
        </p>
      )}
    </div>
  );
}
export function SelectField({
  label,
  name,
  children,
  ...props
}: SelectHTMLAttributes<HTMLSelectElement> & { label: string; name: string }) {
  return (
    <div className="min-w-0">
      <label
        htmlFor={name}
        className="mb-1 block text-[13px] font-semibold text-ink-900"
      >
        {label}
        {props.required && <span aria-hidden="true"> *</span>}
      </label>
      <select {...props} id={name} name={name} className={control}>
        {children}
      </select>
    </div>
  );
}
export function Section({
  title,
  children,
  edit,
}: {
  title: string;
  children: ReactNode;
  edit?: string;
}) {
  return (
    <section className="rounded-2xl border border-line bg-surface p-4 sm:p-6">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <h2 style={{ fontSize: "15px" }} className="font-semibold text-ink-950">{title}</h2>
        {edit && (
          <Link
            href={edit}
            className="shrink-0 text-sm font-semibold text-label-accent"
          >
            Edit {title.toLowerCase()}
          </Link>
        )}
      </div>
      {children}
    </section>
  );
}
export function StepForm({
  title,
  description,
  children,
  onSubmit,
  error,
  onReload,
}: {
  title: string;
  description: string;
  children: ReactNode;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  error?: unknown;
  onReload?: () => void;
}) {
  return (
    <form onSubmit={onSubmit} className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
      <div>
        <h1 className="text-ink-950">{title}</h1>
        <p className="mt-1.5 text-sm text-ink-600">{description}</p>
      </div>
      {error != null && (
        <ErrorNotice
          error={error}
          onRetry={onReload}
          retryLabel="Reload saved record and discard unsaved edits"
        />
      )}
      <fieldset className="space-y-6">{children}</fieldset>
    </form>
  );
}
function SaveFooterContent({
  step,
  busy,
  disabled,
  editable = true,
  label = "Save & continue",
}: {
  step: string;
  busy: boolean;
  disabled?: boolean;
  editable?: boolean;
  label?: string;
}) {
  const pathname = usePathname();
  const search = useSearchParams();
  const returnToReview = step !== "review" && search.get("return") === "review";
  const actionLabel = returnToReview ? "Save & return to review" : label;
  return (
    // Matches the original two-button Fleet footer: primary action first on
    // mobile, Back beneath it; only the two actions appear at this boundary.
    <div className="flex flex-col-reverse gap-3 border-t border-line pt-6 pb-[max(1rem,env(safe-area-inset-bottom))] sm:flex-row sm:items-center sm:justify-between">
      <Link href={previousPath(step)} className="btn btn-secondary btn-lg w-full sm:w-auto">
        <ArrowLeft />
        Back
      </Link>
      {editable ? (
        <button
          type="submit"
          disabled={busy || disabled}
          aria-busy={busy}
          className="btn btn-primary btn-lg w-full sm:w-auto sm:min-w-[200px]"
        >
          {busy ? <Loader2 className="animate-spin" /> : null}
          {busy ? "Saving…" : actionLabel}
          {!busy && <ArrowRight />}
        </button>
      ) : (
        <Link
          className="btn btn-primary btn-lg w-full sm:w-auto sm:min-w-[200px]"
          href={pathname === "/setup/submit" ? "/setup" : nextPath(step)}
        >
          Continue
          <ArrowRight />
        </Link>
      )}
    </div>
  );
}
export function useStepNavigation(step: string) {
  const router = useRouter();
  return () =>
    router.push(
      new URLSearchParams(window.location.search).get("return") === "review"
        ? "/setup/submit"
        : nextPath(step),
    );
}
export function TextValue({ label, value }: { label: string; value: unknown }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-ink-500">{label}</dt>
      <dd className="mt-1 break-words text-sm font-medium text-ink-950">
        {typeof value === "string" && value ? value : "Not provided"}
      </dd>
    </div>
  );
}

export function SaveFooter(props: Parameters<typeof SaveFooterContent>[0]) {
  return (
    <Suspense
      fallback={
        <p role="status" className="py-5 text-sm">
          Loading navigation…
        </p>
      }
    >
      <SaveFooterContent {...props} />
    </Suspense>
  );
}
