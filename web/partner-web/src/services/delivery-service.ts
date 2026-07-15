import { apiClient } from "@/lib/api-client";
import type { DeliveryDetail, DeliverySummary } from "@/types";

interface DeliveryListResponse {
  deliveries: DeliverySummary[];
  meta: { page: number; limit: number; total: number };
}

interface DeliveryDetailResponse {
  // GetDelivery returns the DeliveryDetail object directly as `data`.
  // Kept as a type alias for clarity at call sites.
}

export const deliveryService = {
  list(
    token: string,
    params: { page?: number; limit?: number; status?: string } = {}
  ): Promise<DeliveryListResponse> {
    const query = new URLSearchParams();
    query.set("page", String(params.page ?? 1));
    query.set("limit", String(params.limit ?? 20));
    if (params.status) query.set("status", params.status);
    return apiClient.get<DeliveryListResponse>(
      `/api/v1/partner/v1/deliveries?${query.toString()}`,
      token
    );
  },

  get(token: string, idOrRef: string): Promise<DeliveryDetail> {
    return apiClient.get<DeliveryDetail>(
      `/api/v1/partner/v1/deliveries/${encodeURIComponent(idOrRef)}`,
      token
    );
  },
};