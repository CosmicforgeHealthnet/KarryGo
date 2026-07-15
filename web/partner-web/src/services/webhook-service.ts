import { apiClient } from "@/lib/api-client";
import type { WebhookLog, WebhookLogDetail } from "@/types";

interface Webhook {
  id: string;
  url: string;
  events: string[];
  status: string;
  created_at: string;
}

interface WebhookLogListResponse {
  deliveries: WebhookLog[];
  meta: { page: number; limit: number; total: number };
}

export const webhookService = {
  // Webhook logs (for webhook-logs page)
  list(
    token: string,
    params: { page?: number; limit?: number } = {}
  ): Promise<WebhookLogListResponse> {
    const query = new URLSearchParams();
    query.set("page", String(params.page ?? 1));
    query.set("limit", String(params.limit ?? 20));
    return apiClient.get<WebhookLogListResponse>(
      `/api/v1/partner/v1/webhook-logs?${query.toString()}`,
      token
    );
  },

  get(token: string, id: string): Promise<WebhookLogDetail> {
    return apiClient.get<WebhookLogDetail>(
      `/api/v1/partner/v1/webhook-logs/${encodeURIComponent(id)}`,
      token
    );
  },

  // Webhook management (for Settings page)
  listWebhooks(token: string): Promise<{ webhooks: Webhook[] }> {
    return apiClient.get<{ webhooks: Webhook[] }>(
      `/api/v1/partner/v1/webhooks`,
      token
    );
  },

  createWebhook(token: string, data: { url: string; events: string[] }): Promise<Webhook & { signing_secret: string }> {
    return apiClient.post<Webhook & { signing_secret: string }>(
      `/api/v1/partner/v1/webhooks`,
      data,
      token
    );
  },

  deleteWebhook(token: string, id: string): Promise<void> {
    return apiClient.delete<void>(
      `/api/v1/partner/v1/webhooks/${encodeURIComponent(id)}`,
      token
    );
  },

  testWebhook(token: string, id: string): Promise<void> {
    return apiClient.post<void>(
      `/api/v1/partner/v1/webhooks/${encodeURIComponent(id)}/test`,
      {},
      token
    );
  },
};