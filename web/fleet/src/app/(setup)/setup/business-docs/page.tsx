"use client";
import { useRef, useState, type FormEvent } from "react";
import { useOnboarding } from "../onboarding-context";
import { organizationService as api } from "@/lib/services/organizationService";
import {
  Section,
  StepForm,
  SaveFooter,
  humanize,
  useStepNavigation,
} from "../onboarding-ui";
const allowed = ["application/pdf", "image/jpeg", "image/png", "image/webp"];
const localError = (message: string) => ({
  response: { data: { error: { message } } },
});
export default function Documents() {
  const { data, reload, reloadSaved, editable } = useOnboarding();
  const id = data.organization!.organizationId;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [confirm, setConfirm] = useState<string | null>(null);
  const [pending, setPending] = useState<
    Record<string, { fileId: string; replaceId?: string }>
  >({});
  const guard = useRef(false);
  const go = useStepNavigation("documents");
  async function attach(type: string, fileId: string, replaceId?: string) {
    await api.attachDocument(id, type, fileId, replaceId);
    setPending((p) => {
      const next = { ...p };
      delete next[type];
      return next;
    });
    await reload();
  }
  async function upload(type: string, file: File, replaceId?: string) {
    if (guard.current) return;
    guard.current = true;
    setBusy(true);
    setError(null);
    try {
      if (!allowed.includes(file.type))
        throw localError("Choose a PDF, JPEG, PNG or WebP file.");
      if (file.size > 25 * 1024 * 1024 || file.size === 0)
        throw localError("Choose a nonempty file up to 25 MB.");
      const grant = await api.requestUploadAuthorization(id, {
        documentType: type,
        idempotencyKey: crypto.randomUUID(),
      });
      if (
        file.size > grant.maxSizeBytes ||
        !grant.allowedMimeTypes.includes(file.type)
      )
        throw localError(
          "This file does not meet the upload authorization's type or size limits.",
        );
      const hash = await crypto.subtle.digest(
        "SHA-256",
        await file.arrayBuffer(),
      );
      const checksum = Array.from(new Uint8Array(hash), (b) =>
        b.toString(16).padStart(2, "0"),
      ).join("");
      await api.completeDocumentUpload(id, {
        ...grant,
        declaredMime: file.type,
        declaredSize: file.size,
        declaredChecksum: checksum,
        file,
      });
      setPending((p) => ({
        ...p,
        [type]: { fileId: grant.mediaObjectId, replaceId },
      }));
      await attach(type, grant.mediaObjectId, replaceId);
    } catch (err) {
      setError(err);
    } finally {
      guard.current = false;
      setBusy(false);
    }
  }
  async function retryAttachment(type: string) {
    if (guard.current) return;
    guard.current = true;
    setBusy(true);
    setError(null);
    try {
      await attach(type, pending[type].fileId, pending[type].replaceId);
    } catch (err) {
      setError(err);
    } finally {
      guard.current = false;
      setBusy(false);
    }
  }
  async function remove(documentId: string) {
    if (guard.current) return;
    guard.current = true;
    setBusy(true);
    setError(null);
    try {
      await api.removeDocument(id, documentId);
      await reload();
      setConfirm(null);
    } catch (err) {
      setError(err);
    } finally {
      guard.current = false;
      setBusy(false);
    }
  }
  async function proceed(e: FormEvent) {
    e.preventDefault();
    if (guard.current) return;
    setBusy(true);
    try {
      const current = await reload();
      if (
        current.status?.stages.find((s) => s.stage === "BUSINESS_DOCUMENTS")
          ?.status !== "COMPLETE"
      )
        throw localError(
          "Upload every required document and wait for the server to confirm each file is ready.",
        );
      go();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  return (
    <StepForm
      title="Business documents"
      description="Upload the documents required for company review. Files must finish upload and safety checks before they count as ready."
      onSubmit={proceed}
      error={error}
      onReload={() => {
        void reloadSaved().catch(setError);
      }}
    >
      {data.status!.requiredDocumentTypes.map((type) => {
        const docs = data.documents.filter((d) => d.documentType === type);
        return (
          <Section key={type} title={humanize(type)}>
            <div className="space-y-4">
              {docs.length === 0 && (
                <p className="text-sm text-ink-500">No document attached.</p>
              )}
              {docs.map((doc) => (
                <div
                  key={doc.id}
                  className="rounded-xl border border-line p-3 text-sm"
                >
                  <p className="font-semibold">
                    Status: {humanize(doc.status)}
                  </p>
                  {doc.status === "READY" && (
                    <p className="mt-1 text-xs text-ink-500">
                      Ready for company review. Company approval is still
                      required.
                    </p>
                  )}
                  <p className="mt-2 break-all text-xs text-ink-500">
                    File reference: {doc.fileId}
                  </p>
                  {doc.createdAt && (
                    <p className="mt-1 text-xs text-ink-500">
                      Attached {new Date(doc.createdAt).toLocaleDateString()}
                    </p>
                  )}
                  {doc.expiresAt && (
                    <p className="text-xs text-ink-500">
                      Expires {new Date(doc.expiresAt).toLocaleDateString()}
                    </p>
                  )}
                  {editable && (
                    <div className="mt-3 flex flex-wrap items-center gap-3">
                      <label className="btn btn-secondary cursor-pointer">
                        Replace
                        <span className="sr-only"> {humanize(type)}</span>
                        <input
                          type="file"
                          className="sr-only"
                          accept={allowed.join(",")}
                          disabled={busy}
                          onChange={(e) => {
                            const file = e.target.files?.[0];
                            if (file) void upload(type, file, doc.id);
                            e.target.value = "";
                          }}
                        />
                      </label>
                      {confirm === doc.id ? (
                        <>
                          <span>Remove this attachment?</span>
                          <button
                            type="button"
                            disabled={busy}
                            onClick={() => remove(doc.id)}
                            className="font-semibold text-red"
                          >
                            Confirm removal
                          </button>
                          <button
                            type="button"
                            onClick={() => setConfirm(null)}
                          >
                            Cancel
                          </button>
                        </>
                      ) : (
                        <button
                          type="button"
                          disabled={busy}
                          onClick={() => setConfirm(doc.id)}
                          className="text-red"
                        >
                          Remove
                        </button>
                      )}
                    </div>
                  )}
                </div>
              ))}
              {editable && docs.length === 0 && (
                <label className="btn btn-secondary cursor-pointer">
                  Upload {humanize(type)}
                  <input
                    type="file"
                    className="sr-only"
                    accept={allowed.join(",")}
                    disabled={busy}
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) void upload(type, file);
                      e.target.value = "";
                    }}
                  />
                </label>
              )}
              {pending[type] && (
                <div
                  role="status"
                  className="rounded-xl bg-surface-2 p-3 text-sm"
                >
                  The file was uploaded. Its attachment has not yet been
                  confirmed.{" "}
                  <button
                    type="button"
                    className="font-semibold text-label-accent"
                    disabled={busy}
                    onClick={() => retryAttachment(type)}
                  >
                    Check readiness and attach
                  </button>
                </div>
              )}
            </div>
          </Section>
        );
      })}
      <p className="text-xs text-ink-500">
        PDF, JPEG, PNG or WebP · Up to 25 MB per file. Removing an attachment
        retains the underlying records for audit.
      </p>
      <SaveFooter step="documents" busy={busy} editable={editable} />
    </StepForm>
  );
}
