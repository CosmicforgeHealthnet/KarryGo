import apiClient from '@/lib/api/client';
import type {
  CompleteInspectionInput,
  CreateInspectionInput,
  Inspection,
  InspectionTemplate,
  SaveTemplateInput,
} from '@/lib/types/inspection';

export const inspectionService = {
  async listTemplates(): Promise<InspectionTemplate[]> {
    const res = await apiClient.get<{ data: InspectionTemplate[] }>(
      '/inspection-templates',
    );
    return res.data.data ?? [];
  },

  async getTemplate(templateId: string): Promise<InspectionTemplate> {
    const res = await apiClient.get<{ data: InspectionTemplate }>(
      `/inspection-templates/${encodeURIComponent(templateId)}`,
    );
    return res.data.data;
  },

  async createTemplate(
    input: SaveTemplateInput,
    idempotencyKey: string,
  ): Promise<InspectionTemplate> {
    const res = await apiClient.post<{ data: InspectionTemplate }>(
      '/inspection-templates',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async updateTemplate(
    templateId: string,
    input: SaveTemplateInput,
  ): Promise<InspectionTemplate> {
    const { version, ...body } = input;
    const res = await apiClient.patch<{ data: InspectionTemplate }>(
      `/inspection-templates/${encodeURIComponent(templateId)}`,
      body,
      { headers: { 'If-Match': String(version ?? 0) } },
    );
    return res.data.data;
  },

  async listInspections(): Promise<Inspection[]> {
    const res = await apiClient.get<{ data: Inspection[] }>('/inspections');
    return res.data.data ?? [];
  },

  async getInspection(inspectionId: string): Promise<Inspection> {
    const res = await apiClient.get<{ data: Inspection }>(
      `/inspections/${encodeURIComponent(inspectionId)}`,
    );
    return res.data.data;
  },

  async createInspection(
    input: CreateInspectionInput,
    idempotencyKey: string,
  ): Promise<Inspection> {
    const res = await apiClient.post<{ data: Inspection }>(
      '/inspections',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async completeInspection(
    inspectionId: string,
    input: CompleteInspectionInput,
    idempotencyKey: string,
  ): Promise<Inspection> {
    const { expectedVersion, ...body } = input;
    const res = await apiClient.post<{ data: Inspection }>(
      `/inspections/${encodeURIComponent(inspectionId)}/complete`,
      body,
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },
};

export function newInspectionIdempotencyKey(): string {
  if (
    typeof crypto !== 'undefined' &&
    typeof crypto.randomUUID === 'function'
  ) {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}