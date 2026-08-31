// src/lib/mock-control-tower-data.ts

export type ActiveWork = {
  id: string;
  type: "Trip" | "Assignment" | "Route";
  provider: string;
  asset: string;
  service: "Dispatch" | "Hauling";
  branch: string;
  status: "Active" | "Idle" | "Exception" | "Completed";
  location: string;
  lastUpdated: string;
  progress: number;
  eta: string;
  isLive: boolean;
  signal: "strong" | "medium" | "weak";
};

export type Exception = {
  id: string;
  type: "Late" | "Off-Route" | "Stale Location" | "Rejected Assignment" | "Expired Assignment" | "Safety" | "Capacity";
  severity: "Critical" | "High" | "Medium" | "Low";
  subject: string;
  subjectType: "Trip" | "Provider" | "Asset" | "Assignment";
  description: string;
  branch: string;
  status: "Open" | "Acknowledged" | "In Progress" | "Resolved";
  createdAt: string;
  age: string;
  assignedTo?: string;
};

export const CONTROL_TOWER_DATA: ActiveWork[] = [
  {
    id: "TRP-8491",
    type: "Trip",
    provider: "Peter Okafor",
    asset: "Bike BR-88",
    service: "Dispatch",
    branch: "Ikeja",
    status: "Active",
    location: "Ikeja → Lekki",
    lastUpdated: "12 sec ago",
    progress: 66,
    eta: "10:45",
    isLive: true,
    signal: "strong",
  },
  {
    id: "ASN-8487",
    type: "Assignment",
    provider: "Musa Bello",
    asset: "Van VN-04",
    service: "Hauling",
    branch: "Apapa",
    status: "Active",
    location: "Apapa Port",
    lastUpdated: "45 sec ago",
    progress: 33,
    eta: "11:20",
    isLive: true,
    signal: "medium",
  },
  {
    id: "TRP-8472",
    type: "Trip",
    provider: "Kabiru Ahmed",
    asset: "Truck TR-12",
    service: "Hauling",
    branch: "Lekki",
    status: "Exception",
    location: "Off-route",
    lastUpdated: "5 min ago",
    progress: 33,
    eta: "N/A",
    isLive: false,
    signal: "weak",
  },
  {
    id: "RTE-0142",
    type: "Route",
    provider: "Amina Yusuf",
    asset: "Bike BR-71",
    service: "Dispatch",
    branch: "Yaba",
    status: "Idle",
    location: "Yaba Depot",
    lastUpdated: "2 min ago",
    progress: 0,
    eta: "Not started",
    isLive: false,
    signal: "strong",
  },
  {
    id: "TRP-8465",
    type: "Trip",
    provider: "Chinedu Uche",
    asset: "Bike BR-20",
    service: "Dispatch",
    branch: "Yaba",
    status: "Completed",
    location: "Yaba → Surulere",
    lastUpdated: "12 min ago",
    progress: 100,
    eta: "Completed",
    isLive: false,
    signal: "strong",
  },
  {
    id: "ASN-8479",
    type: "Assignment",
    provider: "Tunde Adeyemi",
    asset: "Bike BR-12",
    service: "Dispatch",
    branch: "Ikeja",
    status: "Exception",
    location: "Ikeja",
    lastUpdated: "3 min ago",
    progress: 0,
    eta: "N/A",
    isLive: false,
    signal: "weak",
  },
];

export const EXCEPTIONS_DATA: Exception[] = [
  {
    id: "EXC-001",
    type: "Late",
    severity: "Critical",
    subject: "Trip KG-8491",
    subjectType: "Trip",
    description: "Trip is 12 minutes behind schedule. ETA exceeded by 8 minutes.",
    branch: "Ikeja",
    status: "Open",
    createdAt: "2026-08-17T14:10:00Z",
    age: "6m ago",
    assignedTo: "T. Adeyemi",
  },
  {
    id: "EXC-002",
    type: "Off-Route",
    severity: "High",
    subject: "Trip KG-8472",
    subjectType: "Trip",
    description: "Vehicle BR-204 is off-route. Last known location: 2.4km from planned route.",
    branch: "Lekki",
    status: "Acknowledged",
    createdAt: "2026-08-17T14:05:00Z",
    age: "11m ago",
    assignedTo: "M. Bello",
  },
  {
    id: "EXC-003",
    type: "Stale Location",
    severity: "Medium",
    subject: "Provider P-044",
    subjectType: "Provider",
    description: "Provider location signal is stale. Last update 4 minutes ago.",
    branch: "Ikeja",
    status: "Open",
    createdAt: "2026-08-17T14:12:00Z",
    age: "4m ago",
  },
  {
    id: "EXC-004",
    type: "Rejected Assignment",
    severity: "High",
    subject: "Assignment AS-1187",
    subjectType: "Assignment",
    description: "Provider declined assignment due to unsafe vehicle report.",
    branch: "Apapa",
    status: "In Progress",
    createdAt: "2026-08-17T14:00:00Z",
    age: "16m ago",
    assignedTo: "K. Ahmed",
  },
  {
    id: "EXC-005",
    type: "Safety",
    severity: "Critical",
    subject: "Vehicle BR-204",
    subjectType: "Asset",
    description: "Safety block active - inspection required before next assignment.",
    branch: "Ikeja",
    status: "Open",
    createdAt: "2026-08-17T13:55:00Z",
    age: "21m ago",
  },
  {
    id: "EXC-006",
    type: "Capacity",
    severity: "Medium",
    subject: "Provider M. Bello",
    subjectType: "Provider",
    description: "Provider has reached daily capacity. No additional assignments can be offered.",
    branch: "Lekki",
    status: "Resolved",
    createdAt: "2026-08-17T13:45:00Z",
    age: "31m ago",
    assignedTo: "F. Daniel",
  },
  {
    id: "EXC-007",
    type: "Expired Assignment",
    severity: "Low",
    subject: "Assignment AS-8458",
    subjectType: "Assignment",
    description: "Assignment response deadline expired. Provider did not respond in time.",
    branch: "Ikeja",
    status: "Resolved",
    createdAt: "2026-08-17T13:30:00Z",
    age: "46m ago",
  },
];