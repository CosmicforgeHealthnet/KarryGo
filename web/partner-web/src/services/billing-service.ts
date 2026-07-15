import { apiClient } from "@/lib/api-client";
import type { BillingOverview, Invoice, BillingUsageSummary, PaymentMethod } from "@/types";

export const billingService = {
  getOverview(token: string): Promise<BillingOverview> {
    return apiClient.get<BillingOverview>(
      `/api/v1/partner/v1/billing/overview`,
      token
    );
  },

  getDashboardSummary(token: string): Promise<BillingUsageSummary> {
    return apiClient.get<BillingUsageSummary>(
      `/api/v1/partner/v1/billing/dashboard-summary`,
      token
    );
  },

  listInvoices(token: string): Promise<Invoice[]> {
    return apiClient.get<{ invoices: Invoice[] }>(
      `/api/v1/partner/v1/billing/invoices`,
      token
    ).then(response => response.invoices || []);
  },

  attachPaymentMethod(token: string, data: { payment_method_id: string }): Promise<PaymentMethod> {
    return apiClient.post<PaymentMethod>(
      `/api/v1/partner/v1/billing/payment-methods`,
      data,
      token
    );
  },
};