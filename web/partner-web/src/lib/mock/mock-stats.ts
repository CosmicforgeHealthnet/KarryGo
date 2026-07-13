import type { DeliveryStats } from "@/types";

export const mockStats: DeliveryStats = {
  totalDeliveries: 3,
  successRate: 3,
  deliveriesToday: 1,
};

export const emptyStats: DeliveryStats = {
  totalDeliveries: 0,
  successRate: 0,
  deliveriesToday: 0,
};