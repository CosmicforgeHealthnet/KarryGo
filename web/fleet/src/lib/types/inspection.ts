export interface ChecklistItem {
  code: string;
  label?: string;
  required?: boolean;
  evidenceRequired?: boolean;
  critical?: boolean;
}

export interface CriticalityRule {
  criticalFailure?: string;
  [key: string]: unknown;
}

export interface InspectionTemplate {
  id: string;
  organizationId: string;
  name: string;
  status: string;
  assetTypes: string[];
  serviceTypes: string[];
  checklistItems: ChecklistItem[];
  criticalityRules: CriticalityRule;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface SaveTemplateInput {
  name: string;
  assetTypes?: string[];
  serviceTypes?: string[];
  checklistItems: ChecklistItem[];
  criticalityRules: CriticalityRule;
  version?: number;
}

export interface Inspection {
  id: string;
  organizationId: string;
  assetId: string;
  templateId: string;
  inspectorMembershipId: string;
  assignmentId: string;
  tripReference: string;
  status: string;
  overallResult: string;
  defectSummary: unknown;
  evidenceMediaIds: string[];
  startedAt: string | null;
  completedAt: string | null;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface Answer {
  itemCode: string;
  passed: boolean;
  advisory: boolean;
  evidenceMediaIds: string[];
}

export interface CreateInspectionInput {
  assetId: string;
  templateId: string;
  inspectorMembershipId: string;
  assignmentId?: string;
  tripReference?: string;
}

export interface CompleteInspectionInput {
  expectedVersion: number;
  answers: Answer[];
  defectSummary?: unknown;
}