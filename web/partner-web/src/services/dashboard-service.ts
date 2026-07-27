import { apiClient } from "@/lib/api-client";
import type { ActivityItem, DashboardSummary, DeliverySummary } from "@/types";

interface DeliveryListResponse {
  deliveries: DeliverySummary[];
  meta: { page: number; limit: number; total: number };
}

function groupLabel(createdAt: string): string {
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);

  const date = new Date(createdAt);
  date.setHours(0, 0, 0, 0);

  if (date.getTime() === today.getTime()) return "Today";
  if (date.getTime() === yesterday.getTime()) return "Yesterday";
  return new Date(createdAt).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
  });
}

function formatTime(createdAt: string): string {
  return new Date(createdAt).toLocaleTimeString("en-US", {
    hour: "numeric",
    minute: "2-digit",
  });
}

function toActivityItem(delivery: DeliverySummary): ActivityItem {
  const status = delivery.status.replace(/_/g, " ");
  const label = status.charAt(0).toUpperCase() + status.slice(1);
  return {
    id: delivery.id,
    message: `${label} — delivery to ${delivery.receiver_name} (${delivery.booking_ref})`,
    timestamp: formatTime(delivery.created_at),
    group: groupLabel(delivery.created_at),
    read: true,
  };
}

export const dashboardService = {
  getSummary(token: string): Promise<DashboardSummary> {
    return apiClient.get<DashboardSummary>(
      "/api/v1/partner/v1/billing/dashboard-summary",
      token
    );
  },

 async getRecentActivity(token: string, limit = 10): Promise<ActivityItem[]> {
    const res = await apiClient.get<DeliveryListResponse>(
      `/api/v1/partner/v1/deliveries?page=1&limit=${limit}`,
      token
    );
    return (res.deliveries ?? []).map(toActivityItem);
  },
};