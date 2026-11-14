import apiClient from '@/lib/api/client';
import { commandHeaders, enc, query } from '@/lib/api/headers';
import type { Assignment } from '@/lib/types/asset';
import type {
  AssetHistoryEntry,
  CostEntry,
  CostHistory,
  CostInput,
  Defect,
  DefectAction,
  MeterReading,
  RegisterTyreInput,
  Tyre,
  TyreAction,
} from '@/lib/types/assetCare';

// Asset lifecycle reads and maintenance records. Every write is idempotent;
// defect and tyre transitions also carry the record version the user saw,
// and unassigning a provider carries the asset version.
export const assetCareService = {
  async history(assetId: string): Promise<AssetHistoryEntry[]> {
    return (await apiClient.get<{ data: AssetHistoryEntry[] }>(`/assets/${enc(assetId)}/history`)).data.data ?? [];
  },
  async costs(assetId: string): Promise<CostHistory> {
    const res = await apiClient.get<{ data: CostHistory }>(`/assets/${enc(assetId)}/costs`);
    return res.data.data ?? { assetId, entries: [], totals: [] };
  },
  async recordCost(assetId: string, input: CostInput, idempotencyKey: string): Promise<CostEntry> {
    const res = await apiClient.post<{ data: CostEntry }>(`/assets/${enc(assetId)}/costs`, input, commandHeaders({ idempotencyKey }));
    return res.data.data;
  },
  async meterReadings(assetId: string, meterType?: string): Promise<MeterReading[]> {
    const res = await apiClient.get<{ data: MeterReading[] }>(`/assets/${enc(assetId)}/meter-readings${query({ meterType })}`);
    return res.data.data ?? [];
  },
  async recordMeterReading(
    input: { assetId: string; meterType: string; reading: number; recordedAt: string },
    idempotencyKey: string,
  ): Promise<MeterReading> {
    return (await apiClient.post<{ data: MeterReading }>('/meter-readings', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async unassignProvider(
    assetId: string,
    input: { assignmentId?: string; reason: string },
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Assignment> {
    const res = await apiClient.post<{ data: Assignment }>(
      `/assets/${enc(assetId)}/unassign-provider`,
      input,
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },

  async listDefects(params: { assetId?: string; status?: string; severity?: string } = {}): Promise<Defect[]> {
    return (await apiClient.get<{ data: Defect[] }>(`/defects${query(params)}`)).data.data ?? [];
  },
  async getDefect(defectId: string): Promise<Defect> {
    return (await apiClient.get<{ data: Defect }>(`/defects/${enc(defectId)}`)).data.data;
  },
  async reportDefect(
    input: { assetId: string; description: string; severity: string; itemCode?: string },
    idempotencyKey: string,
  ): Promise<Defect> {
    return (await apiClient.post<{ data: Defect }>('/defects', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async defectAction(
    defectId: string,
    action: 'defer' | 'start-repair' | 'resolve',
    input: DefectAction,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Defect> {
    const res = await apiClient.post<{ data: Defect }>(
      `/defects/${enc(defectId)}/${action}`,
      input,
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },

  async listTyres(params: { assetId?: string; status?: string } = {}): Promise<Tyre[]> {
    return (await apiClient.get<{ data: Tyre[] }>(`/tyres${query(params)}`)).data.data ?? [];
  },
  async getTyre(tyreId: string): Promise<Tyre> {
    return (await apiClient.get<{ data: Tyre }>(`/tyres/${enc(tyreId)}`)).data.data;
  },
  async registerTyre(input: RegisterTyreInput, idempotencyKey: string): Promise<Tyre> {
    return (await apiClient.post<{ data: Tyre }>('/tyres', input, commandHeaders({ idempotencyKey }))).data.data;
  },
  async tyreAction(
    tyreId: string,
    action: 'install' | 'remove' | 'rotate' | 'tread-readings',
    input: TyreAction,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Tyre> {
    const res = await apiClient.post<{ data: Tyre }>(
      `/tyres/${enc(tyreId)}/${action}`,
      input,
      commandHeaders({ idempotencyKey, expectedVersion }),
    );
    return res.data.data;
  },
};
