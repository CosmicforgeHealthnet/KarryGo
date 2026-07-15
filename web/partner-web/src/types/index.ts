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
  event_type: string;
  status: "pending" | "delivered" | "failed_permanent";
  response_status: number | null;
  attempts: number;
  payload: any;
  last_attempt_at: string | null;
  next_attempt_at: string | null;
  created_at: string;
}

export interface WebhookLogDetail extends WebhookLog {
  webhook_id: string;
}

export interface AuthAccount {
  id: string;
  business_name: string | null;
  business_email: string;
  business_phone: string | null;
  state: string | null;
  city: string | null;
  business_logo_url: string | null;
  profile_photo_url: string | null;
  status: string;
  plan: string;
  created_at: string;
}

export interface AuthSession {
  token: string;
  expires_at: string;
  account: AuthAccount;
}

export interface DashboardSummary {
  total_deliveries: number;
  deliveries_today: number;
  success_rate: number;
  success_rate_display: string;
}

export interface DeliverySummary {
  id: string;
  booking_ref: string;
  status: string;
  service_tier: string;
  fare_amount: { kobo: number; display: string };
  pickup_address: string;
  dropoff_address: string;
  receiver_name: string;
  external_ref: string | null;
  created_at: string;
}

export interface DeliveryDetail {
  id: string;
  booking_ref: string;
  external_ref: string | null;
  status: string;
  service_tier: string;
  distance_km: number | null;
  pickup: {
    address: string;
    lat: number;
    lng: number;
    contact_name: string | null;
    contact_phone: string | null;
  };
  dropoff: { address: string; lat: number; lng: number };
  receiver: { name: string; phone: string };
  package: {
    type: string | null;
    size: string | null;
    description: string | null;
    weight_kg: number | null;
    is_fragile: boolean;
  };
  fare: { kobo: number; display: string; currency: string };
  rider: {
    id: string | null;
    name: string | null;
    phone: string | null;
    rating: number | null;
    vehicle_color: string | null;
    vehicle_model: string | null;
    plate_number: string | null;
  };
  tracking_url: string | null;
  timeline: { status: string; at: string }[];
  created_at: string;
}
// API Key types
export interface APIKey {
  id: string;
  prefix: string;
  environment: "sandbox" | "live";
  label: string;
  description: string;
  expires_at: string | null;
  permissions: string[];
  status: "active" | "revoked";
  last_used_at: string | null;
  created_at: string;
  revoked_at: string | null;
}

export interface CreateAPIKeyInput {
  label: string;
  environment: "sandbox" | "live";
  description: string;
  expiration: string; // "30 days", "90 days", "1 year", "No expiration"
  permissions: string[];
}

export interface IssuedAPIKey {
  id: string;
  key: string;
  prefix: string;
  environment: string;
  description: string;
  expires_at: string | null;
  permissions: string[];
  warning: string;
}

export interface RevokeAPIKeyResult {
  id: string;
  revoked: boolean;
}
// Billing types
export interface BillingOverview {
  current_balance: { kobo: number; display: string };
  current_month_charges: { kobo: number; display: string };
  total_billed: { kobo: number; display: string };
}

export interface Invoice {
  id: string;
  invoice_number: string;
  period_start: string;
  period_end: string;
  due_date: string;
  amount: { kobo: number; display: string };
  status: "paid" | "pending" | "overdue";
  pdf_url: string | null;
  created_at: string;
}

export interface BillingUsageSummary {
  deliveries_this_month: number;
  billable_deliveries: number;
  rate_per_delivery: { kobo: number; display: string };
  total_cost: { kobo: number; display: string };
}

export interface PaymentMethod {
  id: string;
  type: string;
  last_four: string;
  expiry_month: string;
  expiry_year: string;
  is_default: boolean;
  created_at: string;
}