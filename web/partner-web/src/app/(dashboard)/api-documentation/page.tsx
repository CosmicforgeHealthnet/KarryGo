"use client";

import { useState } from "react";
import { cn } from "@/lib/utils";

const BASE_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api/v1/partner/v1";

const AUTH_SNIPPET = "Authorization: Bearer YOUR_API_KEY_OR_SESSION_TOKEN";

const EXAMPLE_REQUEST = `POST /deliveries
{
  "pickup_address": "24 Awawu str, Ikotun Lagos",
  "pickup_contact_name": "John Doe",
  "pickup_contact_phone": "+2348012345678",
  "dropoff_address": "23 Akeju, Victoria Island",
  "receiver_name": "Jane Smith",
  "receiver_phone": "+2348098765432",
  "package_description": "Medication package",
  "package_type": "medicine",
  "package_size": "small",
  "is_fragile": true,
  "service_tier": "express"
}`;

const EXAMPLE_RESPONSE = `{
  "id": "booking_123",
  "booking_ref": "KGP-2026-001",
  "status": "pending",
  "fare": {
    "kobo": 70000,
    "display": "₦700.00",
    "currency": "NGN"
  },
  "created_at": "2026-07-15T10:00:00Z"
}`;

const ENDPOINTS = [
  { endpoint: "/deliveries", method: "POST", description: "Create a new delivery" },
  { endpoint: "/deliveries", method: "GET", description: "List all deliveries (with pagination)" },
  { endpoint: "/deliveries/{id}", method: "GET", description: "Get delivery details by ID or booking_ref" },
  { endpoint: "/deliveries/{id}", method: "DELETE", description: "Cancel a delivery" },
  { endpoint: "/quotes", method: "POST", description: "Get a delivery quote" },
  { endpoint: "/webhooks", method: "POST", description: "Register a webhook endpoint" },
  { endpoint: "/webhooks", method: "GET", description: "List all webhooks" },
  { endpoint: "/webhooks/{id}", method: "DELETE", description: "Delete a webhook" },
  { endpoint: "/webhooks/{id}/test", method: "POST", description: "Test a webhook endpoint" },
  { endpoint: "/webhook-logs", method: "GET", description: "List all webhook delivery logs" },
  { endpoint: "/webhook-logs/{id}", method: "GET", description: "Get webhook delivery log details" },
  { endpoint: "/api-keys", method: "POST", description: "Generate a new API key" },
  { endpoint: "/api-keys", method: "GET", description: "List all API keys" },
  { endpoint: "/api-keys/{id}", method: "DELETE", description: "Revoke an API key" },
  { endpoint: "/profile", method: "PATCH", description: "Update partner profile" },
  { endpoint: "/profile/account", method: "DELETE", description: "Delete partner account" },
];

const ERROR_CODES = [
  { code: "200", meaning: "Success" },
  { code: "201", meaning: "Created" },
  { code: "400", meaning: "Bad Request - Invalid input" },
  { code: "401", meaning: "Unauthorized - Invalid or missing token" },
  { code: "403", meaning: "Forbidden - Insufficient permissions" },
  { code: "404", meaning: "Not Found - Resource doesn't exist" },
  { code: "409", meaning: "Conflict - Resource already exists" },
  { code: "422", meaning: "Unprocessable Entity - Validation error" },
  { code: "429", meaning: "Too Many Requests - Rate limited" },
  { code: "500", meaning: "Internal Server Error" },
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
        <p className="mt-1 text-sm text-ink-500">
          Complete reference for the Cosmicforge Logistics Partner API.
        </p>
      </div>

      <div className="space-y-6 rounded-2xl bg-white p-5">
        <div>
          <h2 className="text-base font-bold text-ink-900">Getting Started</h2>
          <p className="mt-1 text-sm text-ink-500">
            The Partner API allows you to create, track, and manage deliveries programmatically.
          </p>
        </div>

        <CodeBlock label="Base URL" content={BASE_URL} />

        <div>
          <h2 className="text-base font-bold text-ink-900">Authentication</h2>
          <p className="mt-1 text-sm text-ink-500">
            All API requests require authentication. You can authenticate using either:
          </p>
          <ul className="mt-2 list-disc pl-5 text-sm text-ink-500">
            <li>A <strong>Session Token</strong> (from dashboard login)</li>
            <li>An <strong>API Key</strong> (generated from Settings → API Keys)</li>
          </ul>
          <div className="mt-3">
            <CodeBlock
              label="Bearer Token Authentication"
              eyebrow="http"
              content={AUTH_SNIPPET}
            >
              <p className="font-mono text-sm text-ink-700">
                <span className="font-semibold text-ink-900">Authorization:</span>{" "}
                Bearer YOUR_API_KEY_OR_SESSION_TOKEN
              </p>
            </CodeBlock>
          </div>
        </div>

        <div>
          <h2 className="text-base font-bold text-ink-900">Example Request</h2>
          <p className="mt-1 text-sm text-ink-500">Create a new delivery with package details.</p>
          <div className="mt-3">
            <CodeBlock label="Request" eyebrow="http" content={EXAMPLE_REQUEST} />
          </div>
        </div>

        <div>
          <h2 className="text-base font-bold text-ink-900">Example Response</h2>
          <p className="mt-1 text-sm text-ink-500">Successful delivery creation response.</p>
          <div className="mt-3">
            <CodeBlock label="Response" eyebrow="json" content={EXAMPLE_RESPONSE} />
          </div>
        </div>
      </div>

      <div>
        <h2 className="mb-3 text-base font-bold text-ink-900">Endpoints</h2>
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
                <tr key={`${row.method}-${row.endpoint}`} className="border-b border-ink-100 last:border-0">
                  <td className="px-5 py-4 font-mono text-xs text-ink-700">{row.endpoint}</td>
                  <td className="px-5 py-4">
                    <span className={cn(
                      "rounded-full px-2.5 py-1 text-xs font-semibold",
                      row.method === "POST" && "bg-green-100 text-green-700",
                      row.method === "GET" && "bg-blue-100 text-blue-700",
                      row.method === "DELETE" && "bg-red-100 text-red-700",
                      row.method === "PATCH" && "bg-amber-100 text-amber-700"
                    )}>
                      {row.method}
                    </span>
                  </td>
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
                  <td className="px-5 py-4">
                    <span className={cn(
                      "font-semibold",
                      row.code.startsWith("2") && "text-green-600",
                      row.code.startsWith("4") && "text-amber-600",
                      row.code.startsWith("5") && "text-red-600"
                    )}>
                      {row.code}
                    </span>
                  </td>
                  <td className="px-5 py-4 text-ink-500">{row.meaning}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <div className="rounded-2xl bg-brand-50 p-5 border border-brand-200">
        <h3 className="text-sm font-bold text-brand-700">Need Help?</h3>
        <p className="mt-1 text-sm text-brand-600">
          For support, contact us at support@cosmicforgelogistics.com or visit our help center.
        </p>
      </div>
    </div>
  );
}