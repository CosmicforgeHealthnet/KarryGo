"use client";

import { useState } from "react";

const BASE_URL = "https://api.cosmicforgelogistics.com/";

const AUTH_SNIPPET = "Authorization: Bearer YOUR_API_KEY";

const EXAMPLE_REQUEST = `POST /deliveries
{
  "pickup_address": "24 Awawu str, Ikotun Lagos",
  "dropoff_address": "23 Akeju, Victoria Island",
  "package_description": "Medication package"
}`;

const EXAMPLE_RESPONSE = `{
  "id": "1234445",
  "status": "pending",
  "fare": "7.00",
  "created_at": "2026-07-11T10:00:00Z"
}`;

const ENDPOINTS = [
  { endpoint: "/deliveries", method: "POST", description: "Create Delivery" },
  { endpoint: "/deliveries/{id}", method: "GET", description: "Track Delivery" },
  { endpoint: "/deliveries/{id}/cancel", method: "POST", description: "Cancel Delivery" },
];

const ERROR_CODES = [
  { code: "200", meaning: "Success" },
  { code: "400", meaning: "Bad Request" },
  { code: "401", meaning: "Unauthorized" },
  { code: "404", meaning: "Not Found" },
  { code: "500", meaning: "Server Error" },
];

function CodeBlock({
  label,
  eyebrow,
  content,
  children,
}: {
  label?: string;
  eyebrow?: string;
  content: string;
  children?: React.ReactNode;
}) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(content);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard access can fail (permissions/insecure context) - fail quietly.
    }
  };

  return (
    <div>
      {label && <p className="mb-2 text-sm font-bold text-ink-900">{label}</p>}
      <div className="rounded-xl border border-brand-200 bg-ink-100/50 p-4">
        {eyebrow && (
          <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink-500">
            {eyebrow}
          </p>
        )}
        {children ?? (
          <pre className="overflow-x-auto whitespace-pre-wrap break-all font-mono text-sm text-ink-700">
            {content}
          </pre>
        )}
      </div>
      <div className="mt-2 flex justify-end">
        <button
          type="button"
          onClick={handleCopy}
          className="rounded-lg bg-brand-600 px-4 py-2 text-sm font-semibold text-white hover:bg-brand-700"
        >
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
    </div>
  );
}

export default function ApiDocumentationPage() {
  return (
    <div className="space-y-8">
      <div className="rounded-2xl bg-white p-5">
        <h1 className="text-lg font-bold text-ink-900">API Documentation</h1>
        <p className="mt-1 text-sm text-ink-500">Here&apos;s what&apos;s on your counter today.</p>
      </div>

      <div className="space-y-6 rounded-2xl bg-white p-5">
        <div>
          <h2 className="text-base font-bold text-ink-900">Getting Started</h2>
          <p className="mt-1 text-sm text-ink-500">
            Learn how to integrate your application with the Cosmicforge Logistics API.
          </p>
        </div>

        <CodeBlock label="Base URL" content={BASE_URL} />

        <div>
          <h2 className="text-base font-bold text-ink-900">Authentication</h2>
          <div className="mt-3">
            <CodeBlock
              label="Bearer Token Authentication"
              eyebrow="http"
              content={AUTH_SNIPPET}
            >
              <p className="font-mono text-sm text-ink-700">
                <span className="font-semibold text-ink-900">Authorization:</span>{" "}
                Bearer YOUR_API_KEY
              </p>
            </CodeBlock>
          </div>
        </div>

        <CodeBlock label="Example Request" eyebrow="http" content={EXAMPLE_REQUEST} />

        <CodeBlock label="Example Response" eyebrow="JSON" content={EXAMPLE_RESPONSE} />
      </div>

      <div>
        <h2 className="mb-3 text-base font-bold text-ink-900">Endpoints Table</h2>
        <div className="overflow-x-auto rounded-2xl bg-white">
          <table className="w-full min-w-[480px] text-left text-sm">
            <thead>
              <tr className="border-b border-ink-100 text-xs font-semibold uppercase tracking-wide text-ink-500">
                <th className="px-5 py-4">Endpoint</th>
                <th className="px-5 py-4">Method</th>
                <th className="px-5 py-4">Description</th>
              </tr>
            </thead>
            <tbody>
              {ENDPOINTS.map((row) => (
                <tr key={row.endpoint} className="border-b border-ink-100 last:border-0">
                  <td className="px-5 py-4 font-mono text-xs text-ink-700">{row.endpoint}</td>
                  <td className="px-5 py-4 text-ink-500">{row.method}</td>
                  <td className="px-5 py-4 text-ink-500">{row.description}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <div>
        <h2 className="mb-3 text-base font-bold text-ink-900">Error Codes</h2>
        <div className="overflow-x-auto rounded-2xl bg-white">
          <table className="w-full min-w-[320px] text-left text-sm">
            <thead>
              <tr className="border-b border-ink-100 text-xs font-semibold uppercase tracking-wide text-ink-500">
                <th className="px-5 py-4">Code</th>
                <th className="px-5 py-4">Meaning</th>
              </tr>
            </thead>
            <tbody>
              {ERROR_CODES.map((row) => (
                <tr key={row.code} className="border-b border-ink-100 last:border-0">
                  <td className="px-5 py-4 text-ink-500">{row.code}</td>
                  <td className="px-5 py-4 text-ink-500">{row.meaning}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}