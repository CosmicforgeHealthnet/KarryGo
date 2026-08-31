// src/lib/mock-alerts-data.ts

export type Alert = {
  id: string;
  title: string;
  description: string;
  severity: "Critical" | "High" | "Medium" | "Low";
  status: "Open" | "Acknowledged" | "Resolved" | "Dismissed";
  type: "Trip" | "Provider" | "Asset" | "Safety" | "System" | "Weather" | "Customer";
  source: string;
  branch: string;
  assignedTo?: string;
  sla: string;
  createdAt: string;
  age: string;
  actions: {
    label: string;
    timestamp: string;
  }[];
  tripId?: string;
  assignmentId?: string;
  providerId?: string;
  assetId?: string;
};

export const ALERTS_DATA: Alert[] = [
  {
    id: "ALT-001",
    title: "Trip KG-8491 is behind schedule",
    description: "Trip is 12 minutes behind schedule. ETA exceeded by 8 minutes. Provider is stuck in traffic.",
    severity: "Critical",
    status: "Open",
    type: "Trip",
    source: "Dispatch Service",
    branch: "Ikeja",
    sla: "Breached",
    createdAt: "2026-08-17T14:10:00Z",
    age: "6m ago",
    tripId: "TRP-8491",
    providerId: "P-044",
    actions: [
      { label: "Alert created", timestamp: "14:10" },
      { label: "Provider notified", timestamp: "14:12" },
    ],
  },
  {
    id: "ALT-002",
    title: "Vehicle BR-204 safety block active",
    description: "Vehicle BR-204 has an active safety block from failed inspection. Cannot be assigned until cleared.",
    severity: "High",
    status: "Acknowledged",
    type: "Asset",
    source: "Fleet Service",
    branch: "Ikeja",
    assignedTo: "T. Adeyemi",
    sla: "Warning",
    createdAt: "2026-08-17T13:55:00Z",
    age: "21m ago",
    assetId: "BR-204",
    actions: [
      { label: "Alert created", timestamp: "13:55" },
      { label: "Acknowledged by T. Adeyemi", timestamp: "14:05" },
    ],
  },
  {
    id: "ALT-003",
    title: "Provider P-052 offline - missed shift",
    description: "Provider Amina Yusuf has not checked in for assigned shift. Last seen 45 minutes ago.",
    severity: "Medium",
    status: "Open",
    type: "Provider",
    source: "Workforce Service",
    branch: "Yaba",
    sla: "At risk",
    createdAt: "2026-08-17T13:40:00Z",
    age: "36m ago",
    providerId: "P-052",
    assignmentId: "ASN-8479",
    actions: [
      { label: "Alert created", timestamp: "13:40" },
      { label: "Supervisor notified", timestamp: "13:45" },
    ],
  },
  {
    id: "ALT-004",
    title: "Safety incident reported at Lekki",
    description: "Provider reported unsafe road conditions near delivery location. Driver safety is priority.",
    severity: "High",
    status: "Resolved",
    type: "Safety",
    source: "Compliance Service",
    branch: "Lekki",
    assignedTo: "F. Daniel",
    sla: "Met",
    createdAt: "2026-08-17T13:25:00Z",
    age: "51m ago",
    assignmentId: "ASN-8472",
    providerId: "P-061",
    actions: [
      { label: "Alert created", timestamp: "13:25" },
      { label: "Safety team notified", timestamp: "13:30" },
      { label: "Resolved - alternative route used", timestamp: "14:05" },
    ],
  },
  {
    id: "ALT-005",
    title: "Heavy weather affecting Lekki operations",
    description: "Heavy rain causing delays in Lekki branch. Multiple trips affected.",
    severity: "Medium",
    status: "Acknowledged",
    type: "Weather",
    source: "Weather Service",
    branch: "Lekki",
    assignedTo: "K. Ahmed",
    sla: "Met",
    createdAt: "2026-08-17T12:30:00Z",
    age: "1.8h ago",
    actions: [
      { label: "Alert created", timestamp: "12:30" },
      { label: "Acknowledged by K. Ahmed", timestamp: "12:45" },
      { label: "All providers notified", timestamp: "13:00" },
    ],
  },
  {
    id: "ALT-006",
    title: "System maintenance scheduled",
    description: "Scheduled system maintenance tonight from 02:00-04:00. Operations may be affected.",
    severity: "Low",
    status: "Dismissed",
    type: "System",
    source: "System Service",
    branch: "All branches",
    sla: "Met",
    createdAt: "2026-08-17T11:00:00Z",
    age: "3.2h ago",
    actions: [
      { label: "Alert created", timestamp: "11:00" },
      { label: "Dismissed by admin", timestamp: "11:30" },
    ],
  },
];

export const ALERT_DETAIL = ALERTS_DATA[0];