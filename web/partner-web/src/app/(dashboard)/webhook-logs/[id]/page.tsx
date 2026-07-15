"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { useAuthStore } from "@/store/auth-store";
import { webhookService } from "@/services/webhook-service";
import type { WebhookLogDetail } from "@/types";

export default function WebhookLogDetailPage() {
  const router = useRouter();
  const params = useParams<{ id: string }>();
  const token = useAuthStore((s) => s.token);
  const [detail, setDetail] = useState<WebhookLogDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!token || !params.id) return;
    fetchDetail();
  }, [token, params.id]);

  const fetchDetail = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await webhookService.get(token!, params.id);
      setDetail(data);
    } catch (err) {
      setError("Failed to load webhook log detail");
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const handleRetry = async () => {
    // TODO: Implement retry endpoint when available
    alert("Retry functionality coming soon");
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <div className="text-ink-500">Loading webhook log details...</div>
      </div>
    );
  }

  if (error || !detail) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <div className="text-danger">{error || "Webhook log not found"}</div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={() => router.back()}
          aria-label="Go back"
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-white text-ink-900"
        >
          <ArrowLeft className="h-5 w-5" />
        </button>
        <h1 className="flex-1 rounded-2xl bg-white px-5 py-4 text-lg font-bold text-ink-900">
          View Delivery Log
        </h1>
      </div>

      <div className="rounded-2xl bg-white p-5">
        <div className="flex justify-end">
          <div className="w-32">
            <button
              type="button"
              onClick={handleRetry}
              disabled={detail.status === "delivered"}
              className="w-full rounded-xl bg-brand-500 px-4 py-2.5 text-sm font-semibold text-white hover:bg-brand-600 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Retry
            </button>
          </div>
        </div>

        <dl className="mt-4 space-y-5">
          <div>
            <dt className="text-sm font-bold text-ink-900">Delivery ID</dt>
            <dd className="mt-2 border-b border-dashed border-ink-300 pb-2 text-sm text-ink-500">
              {detail.id}
            </dd>
          </div>
          <div>
            <dt className="text-sm font-bold text-ink-900">Event Type</dt>
            <dd className="mt-2 border-b border-dashed border-ink-300 pb-2 text-sm text-ink-500">
              {detail.event_type}
            </dd>
          </div>
          <div>
            <dt className="text-sm font-bold text-ink-900">Status</dt>
            <dd className="mt-2 border-b border-dashed border-ink-300 pb-2 text-sm text-ink-500">
              {detail.status}
            </dd>
          </div>
          <div>
            <dt className="text-sm font-bold text-ink-900">HTTP Response</dt>
            <dd className="mt-2 border-b border-dashed border-ink-300 pb-2 text-sm text-ink-500">
              {detail.response_status ?? "—"}
            </dd>
          </div>
          <div>
            <dt className="text-sm font-bold text-ink-900">Attempts</dt>
            <dd className="mt-2 border-b border-dashed border-ink-300 pb-2 text-sm text-ink-500">
              {detail.attempts}
            </dd>
          </div>
          <div>
            <dt className="text-sm font-bold text-ink-900">Created At</dt>
            <dd className="mt-2 border-b border-dashed border-ink-300 pb-2 text-sm text-ink-500">
              {new Date(detail.created_at).toLocaleString()}
            </dd>
          </div>
          <div>
            <dt className="text-sm font-bold text-ink-900">Last Attempt</dt>
            <dd className="mt-2 border-b border-dashed border-ink-300 pb-2 text-sm text-ink-500">
              {detail.last_attempt_at ? new Date(detail.last_attempt_at).toLocaleString() : "—"}
            </dd>
          </div>
          <div>
            <dt className="text-sm font-bold text-ink-900">Next Attempt</dt>
            <dd className="mt-2 border-b border-dashed border-ink-300 pb-2 text-sm text-ink-500">
              {detail.next_attempt_at ? new Date(detail.next_attempt_at).toLocaleString() : "—"}
            </dd>
          </div>
          <div>
            <dt className="text-sm font-bold text-ink-900">Payload</dt>
            <dd className="mt-2 break-all rounded-lg bg-ink-100 p-3 text-sm font-mono text-ink-700">
              <pre className="whitespace-pre-wrap">
                {JSON.stringify(detail.payload, null, 2)}
              </pre>
            </dd>
          </div>
        </dl>
      </div>
    </div>
  );
}