"use client";

import { useParams, useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";

interface WebhookLogDetail {
  requestUrl: string;
  payload: string;
  response: string;
  errorMessage: string;
}

const MOCK_LOG_DETAILS: Record<string, WebhookLogDetail> = {
  wh_1001: {
    requestUrl: "https://api.karrygo.com/webhooks/delivery-status",
    payload: '{ "event": "delivery.created", "delivery_id": "#1234445" }',
    response: '{ "received": true }',
    errorMessage: "NIL",
  },
  wh_1003: {
    requestUrl: "https://api.karrygo.com/webhooks/delivery-status",
    payload: '{ "event": "delivery.failed", "delivery_id": "#1234447" }',
    response: '{ "error": "Endpoint unreachable" }',
    errorMessage: "Connection timed out after 3 retries.",
  },
};

const FALLBACK_DETAIL: WebhookLogDetail = {
  requestUrl: "https://api.karrygo.com/webhooks/delivery-status",
  payload: '{ "event": "delivery.updated" }',
  response: '{ "received": true }',
  errorMessage: "NIL",
};

export default function WebhookLogDetailPage() {
  const router = useRouter();
  const params = useParams<{ id: string }>();
  const detail = MOCK_LOG_DETAILS[params.id] ?? FALLBACK_DETAIL;

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
              onClick={() => alert("Retrying delivery...")}
              className="w-full rounded-xl bg-brand-500 px-4 py-2.5 text-sm font-semibold text-white hover:bg-brand-600"
            >
              Retry
            </button>
          </div>
        </div>

        <dl className="mt-4 space-y-5">
          <div>
            <dt className="text-sm font-bold text-ink-900">Request URL</dt>
            <dd className="mt-2 truncate border-b border-dashed border-ink-300 pb-2 text-sm text-ink-500">
              {detail.requestUrl}
            </dd>
          </div>
          <div>
            <dt className="text-sm font-bold text-ink-900">Payload</dt>
            <dd className="mt-2 break-all text-sm text-ink-500">{detail.payload}</dd>
          </div>
          <div>
            <dt className="text-sm font-bold text-ink-900">Response</dt>
            <dd className="mt-2 break-all text-sm text-ink-500">{detail.response}</dd>
          </div>
          <div>
            <dt className="text-sm font-bold text-ink-900">Error Message</dt>
            <dd className="mt-2 text-sm text-ink-500">{detail.errorMessage}</dd>
          </div>
        </dl>
      </div>
    </div>
  );
}