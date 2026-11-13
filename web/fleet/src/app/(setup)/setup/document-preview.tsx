"use client";
import { useEffect, useRef, useState } from "react";
import { Download, Loader2, X } from "lucide-react";
import { organizationService as api } from "@/lib/services/organizationService";
import {
  previewErrorMessage,
  previewFilename,
  previewKind,
} from "@/lib/onboarding/onboardingData";
import { ErrorNotice, humanize } from "./onboarding-ui";

type Previewed = { id: string; documentType: string };
type View =
  | { state: "loading" }
  | { state: "ready"; url: string; kind: "pdf" | "image"; filename: string }
  | { state: "error"; error: unknown };

// A read-only viewer for one attached document. Content is fetched through the
// authenticated Fleet API and shown from a temporary in-memory object URL that
// is released when the viewer closes; nothing about the document changes.
export function DocumentPreview({
  organizationId,
  document,
  onClose,
}: {
  organizationId: string;
  document: Previewed;
  onClose: () => void;
}) {
  const [view, setView] = useState<View>({ state: "loading" });
  const [attempt, setAttempt] = useState(0);
  const closeRef = useRef<HTMLButtonElement>(null);
  const title = humanize(document.documentType);

  useEffect(() => {
    const abort = new AbortController();
    let url: string | null = null;
    api
      .previewDocument(organizationId, document.id, { signal: abort.signal })
      .then((blob) => {
        if (abort.signal.aborted) return;
        const kind = previewKind(blob.type);
        if (!kind) {
          setView({ state: "error", error: { response: { status: 415, data: {} } } });
          return;
        }
        // Re-wrap with the verified type so the browser renders exactly that.
        const type = blob.type.split(";")[0].trim().toLowerCase();
        url = URL.createObjectURL(new Blob([blob], { type }));
        setView({ state: "ready", url, kind, filename: previewFilename(document.documentType, type) });
      })
      .catch((error) => {
        if (!abort.signal.aborted) setView({ state: "error", error });
      });
    return () => {
      abort.abort();
      if (url) URL.revokeObjectURL(url);
    };
  }, [organizationId, document.id, document.documentType, attempt]);

  useEffect(() => {
    const opener = globalThis.document.activeElement as HTMLElement | null;
    closeRef.current?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    globalThis.document.addEventListener("keydown", onKey);
    return () => {
      globalThis.document.removeEventListener("keydown", onKey);
      opener?.focus?.();
    };
  }, [onClose]);

  const failure = view.state === "error" ? previewErrorMessage(view.error) : null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={onClose} aria-hidden="true" />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={`${title} preview`}
        className="relative flex h-[min(90dvh,900px)] w-full max-w-4xl flex-col rounded-2xl border border-line bg-surface p-4 sm:p-6"
      >
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <h2 style={{ fontSize: "15px" }} className="font-semibold text-ink-950">
            {title}
          </h2>
          <div className="flex items-center gap-2">
            {view.state === "ready" && (
              <a href={view.url} download={view.filename} className="btn btn-secondary btn-sm">
                <Download />
                Download
              </a>
            )}
            <button
              ref={closeRef}
              type="button"
              onClick={onClose}
              className="btn btn-secondary btn-sm"
              aria-label="Close preview"
            >
              <X />
              <span className="hidden sm:inline">Close</span>
            </button>
          </div>
        </div>
        <div className="min-h-0 flex-1 overflow-auto rounded-xl border border-line bg-surface-2">
          {view.state === "loading" && (
            <div role="status" className="flex h-full items-center justify-center gap-2 text-sm text-ink-500">
              <Loader2 className="h-5 w-5 animate-spin text-primary" />
              Loading document…
            </div>
          )}
          {view.state === "ready" && view.kind === "pdf" && (
            // The browser's own PDF viewer. Browsers without one (most phones)
            // show the fallback instead of an empty panel.
            <object
              data={view.url}
              type="application/pdf"
              aria-label={`${title} document`}
              className="h-full w-full rounded-xl"
            >
              <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center text-sm text-ink-600">
                <p>This browser can&apos;t display PDF files here.</p>
                <a href={view.url} download={view.filename} className="btn btn-secondary btn-sm">
                  <Download />
                  Download to view
                </a>
              </div>
            </object>
          )}
          {view.state === "ready" && view.kind === "image" && (
            <div className="flex min-h-full items-center justify-center p-4">
              {/* A local object URL of verified image bytes; next/image cannot optimize it. */}
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={view.url} alt={`${title} document`} className="max-h-full max-w-full object-contain" />
            </div>
          )}
          {failure && (
            <div className="p-4">
              <ErrorNotice
                error={{ response: { data: { error: { message: failure.message, requestId: failure.requestId } } } }}
                onRetry={() => {
                  setView({ state: "loading" });
                  setAttempt((n) => n + 1);
                }}
                retryLabel="Try again"
              />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
