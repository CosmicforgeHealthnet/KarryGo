// lib/types/controlTower.ts

// Types mirroring dispatch.Board (internal/features/dispatch/domain.go) plus
// the dispatch types referenced inside it. Field names match the Go JSON
// tags exactly so the payload from GET /control-tower deserializes cleanly.

export interface RoutePlan {
  id: string;
  organizationId: string;
  branchId: string;
  dispatchPlanId?: string;
  replacementForRouteId?: string;
  routeReference: string;
  routeType: string;
  providerMembershipId?: string;
  providerId?: string;
  vehicleOrTruckReference?: string;
  executionService: string;
  status: string;
  plannedDistance: number;
  distanceUnit: string;
  plannedDurationSeconds: number;
  optimizationReference?: string;
  optimizationVersion: number;
  constraintSnapshot: unknown;
  approvedBy?: string;
  approvedAt?: string;
  publishedAt?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface TrackingProjection {
  organizationId: string;
  sourceService: string;
  canonicalTripReference: string;
  assignmentId?: string;
  routePlanId?: string;
  providerId?: string;
  vehicleOrTruckReference?: string;
  latitude?: number;
  longitude?: number;
  heading?: number;
  speed?: number;
  accuracy?: number;
  operationalStatus: string;
  sourceTimestamp: string;
  receivedAt: string;
  sourceVersion: number;
  freshnessStatus: string;
  gapStatus: string;
  lastReconciledAt?: string;
}

export interface OperationalAlert {
  id: string;
  organizationId: string;
  branchId?: string;
  alertType: string;
  severity: string;
  sourceType: string;
  sourceId: string;
  routePlanId?: string;
  assignmentId?: string;
  shipmentId?: string;
  canonicalTripReference?: string;
  ownerUserId?: string;
  status: string;
  slaDueAt: string;
  acknowledgedAt?: string;
  resolvedAt?: string;
  resolutionCode?: string;
  resolutionReason?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

// Row types for the four []map[string]any arrays in Board. The keys are
// exactly the ones built in dispatch/control.go.

export interface UnassignedWork {
  id: string;
  sourceType: string;      // "ORDER" | "SHIPMENT" | "ROUTE"
  branchId: string;
  status: string;
  actionReason: string;    // e.g. "CONFIRMED_WITHOUT_ROUTE"
}

export interface PendingAssignmentRow {
  id: string;
  branchId: string;
  workOrigin: string;
  status: string;
  providerId: string;
  vehicleOrTruckReference: string;
  executionService: string;
  responseDeadline: string;
}

export interface ProviderAvailabilityRow {
  providerId: string;
  availability: string;    // eligibility status from candidate projection
  source: string;          // "candidate_projection"
  generatedAt: string;
  expiresAt: string;
}

export interface VehicleAvailabilityRow {
  vehicleOrTruckReference: string;
  availability: string;
  source: string;
  generatedAt: string;
  expiresAt: string;
}

export interface ProjectionGapRow {
  sourceService: string;
  canonicalTripReference: string;
  sourceVersion: number;
  gapStatus: string;
  receivedAt: string;
}

// Board — the top-level response. Fields mirror dispatch.Board exactly.

export interface Board {
  unassigned: UnassignedWork[];
  pendingAssignments: PendingAssignmentRow[];
  acceptedAssignments: PendingAssignmentRow[];
  activeRoutes: RoutePlan[];
  activeTrips: TrackingProjection[];
  alerts: OperationalAlert[];
  trackingSummary: Record<string, number>;
  capacitySummary: Record<string, number>;
  providerAvailability: ProviderAvailabilityRow[];
  vehicleAvailability: VehicleAvailabilityRow[];
  projectionGaps: ProjectionGapRow[];
  projectionIncomplete: boolean;
  reconciliationRequired: boolean;
}

export interface ControlTowerFilter {
  branchId?: string;
  status?: string;
  service?: string;
  providerId?: string;
  vehicleOrTruckReference?: string;
  limit?: number;
  offset?: number;
}