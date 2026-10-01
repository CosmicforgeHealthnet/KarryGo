// lib/services/financeService.ts

import apiClient from "@/lib/api/client";
import type {
  BudgetInput,
  BudgetRow,
  ClaimInput,
  ClaimRow,
  CommandBody,
  CompensationRuleInput,
  CompensationRuleRow,
  CreateInvoiceInput,
  CreditNoteInput,
  CreditNoteRow,
  ExpenseInput,
  ExpenseRow,
  InvoiceView,
  Operation,
  OperationAcceptedResponse,
  OperationAuditEntry,
  OperationHistoryEntry,
  PaginatedResponse,
  PayrollPeriodInput,
  PayrollPeriodRow,
  PayrollPeriodView,
  Projection,
  ProviderStatement,
  ReceivableRow,
  RequestRefundInput,
  RequestReversalInput,
  RequestSettlementInput,
  SettlementInput,
  SubcontractorSettlementRow,
  SubcontractorSettlementView,
  UpdateInvoiceInput,
} from "@/lib/types/finance";

const BASE = "";

const newIdempotencyKey = (): string => {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2)}`;
};

const unwrapList = <T>(envelope: { data: T[]; nextCursor?: string }): PaginatedResponse<T> => ({
  data: envelope.data ?? [],
  nextCursor: envelope.nextCursor,
});

// The backend uses `SELECT row_to_json(...)` so a single record arrives under
// `{ data: {...}, meta: { requestId } }`. This unwraps to the row.
const unwrapOne = <T>(envelope: { data: T }): T => envelope.data;

// ── financeops ────────────────────────────────────────────────────────────

const invoices = {
  async list(): Promise<PaginatedResponse<InvoiceView>> {
    const r = await apiClient.get<{ data: InvoiceView[] }>(`${BASE}/invoices`);
    return unwrapList(r.data);
  },

  async get(invoiceId: string): Promise<InvoiceView> {
    const r = await apiClient.get<{ data: InvoiceView }>(
      `${BASE}/invoices/${invoiceId}`
    );
    return unwrapOne(r.data);
  },

  async create(input: CreateInvoiceInput): Promise<InvoiceView> {
    const r = await apiClient.post<{ data: InvoiceView }>(
      `${BASE}/invoices`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },

  async update(
    invoiceId: string,
    input: UpdateInvoiceInput,
    expectedVersion: number
  ): Promise<InvoiceView> {
    const r = await apiClient.patch<{ data: InvoiceView }>(
      `${BASE}/invoices/${invoiceId}`,
      { ...input, expectedVersion },
      {
        headers: {
          "Idempotency-Key": newIdempotencyKey(),
          "If-Match": String(expectedVersion),
        },
      }
    );
    return unwrapOne(r.data);
  },

  async approve(invoiceId: string, expectedVersion: number): Promise<InvoiceView> {
    const r = await apiClient.post<{ data: InvoiceView }>(
      `${BASE}/invoices/${invoiceId}/approve`,
      { expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },

  async issue(invoiceId: string, expectedVersion: number): Promise<InvoiceView> {
    const r = await apiClient.post<{ data: InvoiceView }>(
      `${BASE}/invoices/${invoiceId}/issue`,
      { expectedVersion },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },

  async cancel(
    invoiceId: string,
    expectedVersion: number,
    reason?: string
  ): Promise<InvoiceView> {
    const r = await apiClient.post<{ data: InvoiceView }>(
      `${BASE}/invoices/${invoiceId}/cancel`,
      { expectedVersion, reason },
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },

  async credit(
    invoiceId: string,
    input: CreditNoteInput,
    expectedVersion: number
  ): Promise<CreditNoteRow> {
    const r = await apiClient.post<{ data: CreditNoteRow }>(
      `${BASE}/invoices/${invoiceId}/credit-note`,
      { ...input, expectedVersion },
      {
        headers: {
          "Idempotency-Key": newIdempotencyKey(),
          "If-Match": String(expectedVersion),
        },
      }
    );
    return unwrapOne(r.data);
  },
};

const receivables = {
  async list(): Promise<PaginatedResponse<ReceivableRow>> {
    const r = await apiClient.get<{ data: ReceivableRow[] }>(
      `${BASE}/receivables`
    );
    return unwrapList(r.data);
  },

  async get(receivableId: string): Promise<ReceivableRow> {
    const r = await apiClient.get<{ data: ReceivableRow }>(
      `${BASE}/receivables/${receivableId}`
    );
    return unwrapOne(r.data);
  },
};

const expenses = {
  async list(): Promise<PaginatedResponse<ExpenseRow>> {
    const r = await apiClient.get<{ data: ExpenseRow[] }>(`${BASE}/expenses`);
    return unwrapList(r.data);
  },

  async get(expenseId: string): Promise<ExpenseRow> {
    const r = await apiClient.get<{ data: ExpenseRow }>(
      `${BASE}/expenses/${expenseId}`
    );
    return unwrapOne(r.data);
  },

  async create(input: ExpenseInput): Promise<ExpenseRow> {
    const r = await apiClient.post<{ data: ExpenseRow }>(
      `${BASE}/expenses`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },

  async update(
    expenseId: string,
    input: ExpenseInput,
    expectedVersion: number
  ): Promise<ExpenseRow> {
    const r = await apiClient.patch<{ data: ExpenseRow }>(
      `${BASE}/expenses/${expenseId}`,
      { ...input, expectedVersion },
      {
        headers: {
          "Idempotency-Key": newIdempotencyKey(),
          "If-Match": String(expectedVersion),
        },
      }
    );
    return unwrapOne(r.data);
  },

  async submit(expenseId: string, expectedVersion: number): Promise<ExpenseRow> {
    return runCommand(`/expenses/${expenseId}/submit`, expectedVersion);
  },

  async approve(expenseId: string, expectedVersion: number): Promise<ExpenseRow> {
    return runCommand(`/expenses/${expenseId}/approve`, expectedVersion);
  },

  async reject(
    expenseId: string,
    expectedVersion: number,
    reason: string
  ): Promise<ExpenseRow> {
    return runCommand(`/expenses/${expenseId}/reject`, expectedVersion, reason);
  },

  async reverse(
    expenseId: string,
    expectedVersion: number,
    reason: string
  ): Promise<ExpenseRow> {
    return runCommand(`/expenses/${expenseId}/reverse`, expectedVersion, reason);
  },
};

const budgets = {
  async list(): Promise<PaginatedResponse<BudgetRow>> {
    const r = await apiClient.get<{ data: BudgetRow[] }>(`${BASE}/budgets`);
    return unwrapList(r.data);
  },

  async get(budgetId: string): Promise<BudgetRow> {
    const r = await apiClient.get<{ data: BudgetRow }>(
      `${BASE}/budgets/${budgetId}`
    );
    return unwrapOne(r.data);
  },

  async create(input: BudgetInput): Promise<BudgetRow> {
    const r = await apiClient.post<{ data: BudgetRow }>(
      `${BASE}/budgets`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },

  async update(
    budgetId: string,
    input: BudgetInput,
    expectedVersion: number
  ): Promise<BudgetRow> {
    const r = await apiClient.patch<{ data: BudgetRow }>(
      `${BASE}/budgets/${budgetId}`,
      { ...input, expectedVersion },
      {
        headers: {
          "Idempotency-Key": newIdempotencyKey(),
          "If-Match": String(expectedVersion),
        },
      }
    );
    return unwrapOne(r.data);
  },

  async approve(budgetId: string, expectedVersion: number): Promise<BudgetRow> {
    return runCommand(`/budgets/${budgetId}/approve`, expectedVersion);
  },

  async close(budgetId: string, expectedVersion: number): Promise<BudgetRow> {
    return runCommand(`/budgets/${budgetId}/close`, expectedVersion);
  },
};

const claims = {
  async list(): Promise<PaginatedResponse<ClaimRow>> {
    const r = await apiClient.get<{ data: ClaimRow[] }>(`${BASE}/claims`);
    return unwrapList(r.data);
  },

  async get(claimId: string): Promise<ClaimRow> {
    const r = await apiClient.get<{ data: ClaimRow }>(
      `${BASE}/claims/${claimId}`
    );
    return unwrapOne(r.data);
  },

  async create(input: ClaimInput): Promise<ClaimRow> {
    const r = await apiClient.post<{ data: ClaimRow }>(
      `${BASE}/claims`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },

  async update(
    claimId: string,
    input: ClaimInput,
    expectedVersion: number
  ): Promise<ClaimRow> {
    const r = await apiClient.patch<{ data: ClaimRow }>(
      `${BASE}/claims/${claimId}`,
      { ...input, expectedVersion },
      {
        headers: {
          "Idempotency-Key": newIdempotencyKey(),
          "If-Match": String(expectedVersion),
        },
      }
    );
    return unwrapOne(r.data);
  },
};

const compensationRules = {
  async list(): Promise<PaginatedResponse<CompensationRuleRow>> {
    const r = await apiClient.get<{ data: CompensationRuleRow[] }>(
      `${BASE}/compensation-rules`
    );
    return unwrapList(r.data);
  },

  async get(ruleId: string): Promise<CompensationRuleRow> {
    const r = await apiClient.get<{ data: CompensationRuleRow }>(
      `${BASE}/compensation-rules/${ruleId}`
    );
    return unwrapOne(r.data);
  },

  async create(input: CompensationRuleInput): Promise<CompensationRuleRow> {
    const r = await apiClient.post<{ data: CompensationRuleRow }>(
      `${BASE}/compensation-rules`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },

  async update(
    ruleId: string,
    input: CompensationRuleInput,
    expectedVersion: number
  ): Promise<CompensationRuleRow> {
    const r = await apiClient.patch<{ data: CompensationRuleRow }>(
      `${BASE}/compensation-rules/${ruleId}`,
      { ...input, expectedVersion },
      {
        headers: {
          "Idempotency-Key": newIdempotencyKey(),
          "If-Match": String(expectedVersion),
        },
      }
    );
    return unwrapOne(r.data);
  },

  async approve(ruleId: string, expectedVersion: number): Promise<CompensationRuleRow> {
    return runCommand(`/compensation-rules/${ruleId}/approve`, expectedVersion);
  },

  async deactivate(ruleId: string, expectedVersion: number): Promise<CompensationRuleRow> {
    return runCommand(`/compensation-rules/${ruleId}/deactivate`, expectedVersion);
  },
};

const payrollPeriods = {
  async list(): Promise<PaginatedResponse<PayrollPeriodRow>> {
    const r = await apiClient.get<{ data: PayrollPeriodRow[] }>(
      `${BASE}/payroll-periods`
    );
    return unwrapList(r.data);
  },

  async get(periodId: string): Promise<PayrollPeriodView> {
    const r = await apiClient.get<{ data: PayrollPeriodView }>(
      `${BASE}/payroll-periods/${periodId}`
    );
    return unwrapOne(r.data);
  },

  async create(input: PayrollPeriodInput): Promise<PayrollPeriodView> {
    const r = await apiClient.post<{ data: PayrollPeriodView }>(
      `${BASE}/payroll-periods`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },

  async calculate(periodId: string, expectedVersion: number): Promise<PayrollPeriodView> {
    return runCommand(`/payroll-periods/${periodId}/calculate`, expectedVersion);
  },

  async approve(periodId: string, expectedVersion: number): Promise<PayrollPeriodView> {
    return runCommand(`/payroll-periods/${periodId}/approve`, expectedVersion);
  },

  async post(periodId: string, expectedVersion: number): Promise<PayrollPeriodView> {
    return runCommand(`/payroll-periods/${periodId}/post`, expectedVersion);
  },

  async reverse(
    periodId: string,
    expectedVersion: number,
    reason: string
  ): Promise<PayrollPeriodView> {
    return runCommand(`/payroll-periods/${periodId}/reverse`, expectedVersion, reason);
  },
};

const providerStatements = {
  async forMembership(membershipId: string): Promise<ProviderStatement> {
    const r = await apiClient.get<{ data: ProviderStatement }>(
      `${BASE}/providers/${membershipId}/statements`
    );
    return unwrapOne(r.data);
  },
};

const settlements = {
  async list(): Promise<PaginatedResponse<SubcontractorSettlementRow>> {
    const r = await apiClient.get<{ data: SubcontractorSettlementRow[] }>(
      `${BASE}/subcontractor-settlements`
    );
    return unwrapList(r.data);
  },

  async get(settlementId: string): Promise<SubcontractorSettlementView> {
    const r = await apiClient.get<{ data: SubcontractorSettlementView }>(
      `${BASE}/subcontractor-settlements/${settlementId}`
    );
    return unwrapOne(r.data);
  },

  async create(input: SettlementInput): Promise<SubcontractorSettlementView> {
    const r = await apiClient.post<{ data: SubcontractorSettlementView }>(
      `${BASE}/subcontractor-settlements`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },

  async approve(
    settlementId: string,
    expectedVersion: number
  ): Promise<SubcontractorSettlementView> {
    return runCommand(
      `/subcontractor-settlements/${settlementId}/approve`,
      expectedVersion
    );
  },

  async post(
    settlementId: string,
    expectedVersion: number
  ): Promise<SubcontractorSettlementView> {
    return runCommand(
      `/subcontractor-settlements/${settlementId}/post`,
      expectedVersion
    );
  },
};

// ── operator ──────────────────────────────────────────────────────────────

const operations = {
  async list(
    filter: { status?: string; operationType?: string; subjectId?: string; limit?: number } = {}
  ): Promise<PaginatedResponse<Operation>> {
    const r = await apiClient.get<{ data: { operations: Operation[] } }>(
      `${BASE}/finance/operations`,
      { params: filter }
    );
    return { data: r.data.data.operations ?? [] };
  },

  async get(operationId: string): Promise<Operation> {
    const r = await apiClient.get<{ data: Operation }>(
      `${BASE}/finance/operations/${operationId}`
    );
    return unwrapOne(r.data);
  },

  async history(operationId: string): Promise<OperationHistoryEntry[]> {
    const r = await apiClient.get<{ data: { history: OperationHistoryEntry[] } }>(
      `${BASE}/finance/operations/${operationId}/history`
    );
    return r.data.data.history ?? [];
  },

  async audit(operationId: string): Promise<OperationAuditEntry[]> {
    const r = await apiClient.get<{ data: { audit: OperationAuditEntry[] } }>(
      `${BASE}/finance/operations/${operationId}/audit`
    );
    return r.data.data.audit ?? [];
  },

  async replay(operationId: string): Promise<{ status: string }> {
    const r = await apiClient.post<{ data: { status: string } }>(
      `${BASE}/finance/operations/${operationId}/replay`,
      {}
    );
    return r.data.data;
  },
};

const projections = {
  async list(
    filter: { projectionType?: string; freshness?: string; subjectId?: string; limit?: number } = {}
  ): Promise<PaginatedResponse<Projection>> {
    const r = await apiClient.get<{ data: { projections: Projection[] } }>(
      `${BASE}/finance/projections`,
      { params: filter }
    );
    return { data: r.data.data.projections ?? [] };
  },

  async get(projectionId: string): Promise<Projection> {
    const r = await apiClient.get<{ data: Projection }>(
      `${BASE}/finance/projections/${projectionId}`
    );
    return unwrapOne(r.data);
  },
};

const reconciliation = {
  async listCases(
    filter: { projectionType?: string; subjectId?: string; limit?: number } = {}
  ): Promise<PaginatedResponse<Projection>> {
    const r = await apiClient.get<{ data: { cases: Projection[] } }>(
      `${BASE}/finance/reconciliation-cases`,
      { params: filter }
    );
    return { data: r.data.data.cases ?? [] };
  },

  async getCase(projectionId: string): Promise<Projection> {
    const r = await apiClient.get<{ data: Projection }>(
      `${BASE}/finance/reconciliation-cases/${projectionId}`
    );
    return unwrapOne(r.data);
  },

  async replayCase(projectionId: string): Promise<{ status: string }> {
    const r = await apiClient.post<{ data: { status: string } }>(
      `${BASE}/finance/reconciliation-cases/${projectionId}/replay`,
      {}
    );
    return r.data.data;
  },

  async requestReadBack(projectionId: string): Promise<{ status: string }> {
    const r = await apiClient.post<{ data: { status: string } }>(
      `${BASE}/finance/reconciliation-cases/${projectionId}/read-back`,
      {}
    );
    return r.data.data;
  },
};

const refunds = {
  async request(input: RequestRefundInput): Promise<OperationAcceptedResponse> {
    const r = await apiClient.post<{ data: OperationAcceptedResponse }>(
      `${BASE}/finance/refunds`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },
};

const reversals = {
  async request(input: RequestReversalInput): Promise<OperationAcceptedResponse> {
    const r = await apiClient.post<{ data: OperationAcceptedResponse }>(
      `${BASE}/finance/reversals`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },
};

const operatorSettlements = {
  async request(input: RequestSettlementInput): Promise<OperationAcceptedResponse> {
    const r = await apiClient.post<{ data: OperationAcceptedResponse }>(
      `${BASE}/finance/settlements`,
      input,
      { headers: { "Idempotency-Key": newIdempotencyKey() } }
    );
    return unwrapOne(r.data);
  },
};

// ── Command helper ────────────────────────────────────────────────────────

async function runCommand<T>(
  path: string,
  expectedVersion: number,
  reason?: string
): Promise<T> {
  const body: CommandBody = { expectedVersion };
  if (reason) body.reason = reason;
  const r = await apiClient.post<{ data: T }>(`${BASE}${path}`, body, {
    headers: { "Idempotency-Key": newIdempotencyKey() },
  });
  return unwrapOne(r.data);
}

// ── Export ────────────────────────────────────────────────────────────────

export const financeService = {
  invoices,
  receivables,
  expenses,
  budgets,
  claims,
  compensationRules,
  payrollPeriods,
  providerStatements,
  settlements,
  operations,
  projections,
  reconciliation,
  refunds,
  reversals,
  operatorSettlements,
};

export type FinanceService = typeof financeService;