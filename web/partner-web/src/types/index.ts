export type OnboardingStep =
  | "business-info"
  | "business-logo"
  | "profile-photo"
  | "success";

export interface Partner {
  id: string;
  businessName: string;
  businessPhone: string;
  businessEmail: string;
  state: string;
  city: string;
  logoUrl?: string;
  photoUrl?: string;
  status: "pending_verification" | "verified";
}

export interface DeliveryStats {
  totalDeliveries: number;
  successRate: number;
  deliveriesToday: number;
}

export interface ActivityItem {
  id: string;
  message: string;
  timestamp: string;
  group: "Today" | "Yesterday" | string;
  read: boolean;
}

export interface Delivery {
  id: string;
  reference: string;
  recipient: string;
  status: "pending" | "in_transit" | "delivered" | "cancelled";
  createdAt: string;
}

export interface WebhookLog {
  id: string;
  event: string;
  url: string;
  statusCode: number;
  sentAt: string;
}
