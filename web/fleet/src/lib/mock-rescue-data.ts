// src/lib/mock-rescue-data.ts

export type RescueCase = {
  id: string;
  title: string;
  description: string;
  severity: "Critical" | "High" | "Medium" | "Low";
  status: "Open" | "Assigned" | "In Progress" | "Resolved" | "Closed";
  type: "Vehicle Breakdown" | "Trip Delay" | "Provider Issue" | "Safety Incident" | "Customer Issue" | "Weather" | "Other";
  assignmentId?: string;
  tripId?: string;
  providerId?: string;
  branch: string;
  assignedTo?: string;
  createdAt: string;
  age: string;
  updatedAt: string;
  location: string;
  actions: {
    label: string;
    timestamp: string;
  }[];
};

export const RESCUE_CASES: RescueCase[] = [
  {
    id: "RSC-001",
    title: "Vehicle breakdown on route",
    description: "Bike BR-204 experienced mechanical failure on Ikeja-Lekki route. Provider is stranded.",
    severity: "Critical",
    status: "Open",
    type: "Vehicle Breakdown",
    tripId: "TRP-8491",
    providerId: "P-044",
    branch: "Ikeja",
    age: "6 min ago",
    createdAt: "2026-08-17T14:10:00Z",
    updatedAt: "2026-08-17T14:10:00Z",
    location: "Ikeja-Lekki Expressway, near Lekki Phase 1",
    actions: [
      { label: "Rescue created", timestamp: "14:10" },
      { label: "Towing service notified", timestamp: "14:12" },
    ],
  },
  {
    id: "RSC-002",
    title: "Provider unavailable",
    description: "Provider reported illness and cannot complete assigned trip.",
    severity: "High",
    status: "Assigned",
    type: "Provider Issue",
    assignmentId: "ASN-8479",
    providerId: "P-052",
    branch: "Yaba",
    age: "18 min ago",
    createdAt: "2026-08-17T13:55:00Z",
    updatedAt: "2026-08-17T14:05:00Z",
    location: "Yaba Depot",
    assignedTo: "T. Adeyemi",
    actions: [
      { label: "Rescue created", timestamp: "13:55" },
      { label: "Assigned to T. Adeyemi", timestamp: "14:05" },
    ],
  },
  {
    id: "RSC-003",
    title: "Trip delay due to traffic",
    description: "Heavy traffic on Oshodi-Apapa Expressway causing 25 min delay.",
    severity: "Medium",
    status: "In Progress",
    type: "Trip Delay",
    tripId: "TRP-8472",
    providerId: "P-029",
    branch: "Apapa",
    age: "32 min ago",
    createdAt: "2026-08-17T13:40:00Z",
    updatedAt: "2026-08-17T14:00:00Z",
    location: "Oshodi-Apapa Expressway",
    assignedTo: "M. Bello",
    actions: [
      { label: "Rescue created", timestamp: "13:40" },
      { label: "Alternative route suggested", timestamp: "13:50" },
      { label: "Provider notified", timestamp: "14:00" },
    ],
  },
  {
    id: "RSC-004",
    title: "Safety incident reported",
    description: "Provider reported unsafe road conditions near destination.",
    severity: "High",
    status: "Resolved",
    type: "Safety Incident",
    assignmentId: "ASN-8472",
    providerId: "P-061",
    branch: "Lekki",
    age: "45 min ago",
    createdAt: "2026-08-17T13:25:00Z",
    updatedAt: "2026-08-17T13:55:00Z",
    location: "Lekki Phase 1",
    assignedTo: "F. Daniel",
    actions: [
      { label: "Rescue created", timestamp: "13:25" },
      { label: "Safety team notified", timestamp: "13:30" },
      { label: "Resolved - alternative route used", timestamp: "13:55" },
    ],
  },
  {
    id: "RSC-005",
    title: "Customer issue - failed delivery",
    description: "Recipient unavailable at delivery location. Package needs rescheduling.",
    severity: "Low",
    status: "Open",
    type: "Customer Issue",
    tripId: "TRP-8465",
    providerId: "P-061",
    branch: "Yaba",
    age: "25 min ago",
    createdAt: "2026-08-17T13:45:00Z",
    updatedAt: "2026-08-17T13:45:00Z",
    location: "Surulere",
    actions: [
      { label: "Rescue created", timestamp: "13:45" },
    ],
  },
  {
    id: "RSC-006",
    title: "Weather disruption",
    description: "Heavy rain affecting operations in Lekki branch. Trip delays expected.",
    severity: "Medium",
    status: "Closed",
    type: "Weather",
    branch: "Lekki",
    age: "1.5 hr ago",
    createdAt: "2026-08-17T12:30:00Z",
    updatedAt: "2026-08-17T13:00:00Z",
    location: "Lekki area",
    assignedTo: "K. Ahmed",
    actions: [
      { label: "Rescue created", timestamp: "12:30" },
      { label: "All providers notified", timestamp: "12:40" },
      { label: "Operations adjusted", timestamp: "13:00" },
    ],
  },
];