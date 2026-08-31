// src/lib/mock-dispatch-data.ts

export type DispatchPlanStatus = "Draft" | "Locked" | "Published";

export type DispatchJob = {
  id: string;
  title: string;
  provider: string;
  status: "Pending" | "In Progress" | "Completed";
  location: string;
};

export type DispatchPlan = {
  id: string;
  name: string;
  branch: string;
  window: string;
  jobCount: number;
  owner: string;
  status: DispatchPlanStatus;
  jobs: DispatchJob[];
};

// Unassigned Work Types
export type UnassignedWork = {
  id: string;
  title: string;
  route: string;
  serviceType: "Dispatch" | "Hauling" | "Private";
  urgency: "High" | "Medium" | "Low";
  blocker: string | null;
  pickupTime: string;
  deliveryWindow: string;
  distance: string;
  weight: string;
  parcelCount: number;
  fragile: boolean;
  coldChain: boolean;
  branch: string;
  createdAt: string;
};

// MOCK: replace once the DSP-03 dispatch-plans endpoint
// (fleet-partner-service) is wired in.
export const DISPATCH_PLANS: DispatchPlan[] = [
  {
    id: "PLN-0142",
    name: "Ikeja morning wave",
    branch: "Ikeja",
    window: "06:00 – 10:00",
    jobCount: 14,
    owner: "T. Adeyemi",
    status: "Published",
    jobs: [
      { id: "JOB-001", title: "Pickup: Ikeja Mall → Mushin", provider: "A. Balogun", status: "Completed", location: "Ikeja" },
      { id: "JOB-002", title: "Delivery: Mushin → Oshodi", provider: "K. Ibrahim", status: "In Progress", location: "Mushin" },
      { id: "JOB-003", title: "Pickup: Oshodi → Ikeja", provider: "S. Adeyemi", status: "Pending", location: "Oshodi" },
      { id: "JOB-004", title: "Delivery: Surulere → Yaba", provider: "M. Okafor", status: "Completed", location: "Surulere" },
    ],
  },
  {
    id: "PLN-0141",
    name: "Lekki express window",
    branch: "Lekki",
    window: "09:00 – 12:00",
    jobCount: 9,
    owner: "B. Chukwu",
    status: "Locked",
    jobs: [
      { id: "JOB-005", title: "Pickup: Lekki Phase 1 → VI", provider: "C. Obi", status: "In Progress", location: "Lekki" },
      { id: "JOB-006", title: "Delivery: VI → Ajah", provider: "F. Okonkwo", status: "Pending", location: "Victoria Island" },
    ],
  },
  {
    id: "PLN-0140",
    name: "Apapa hauling block",
    branch: "Apapa",
    window: "12:00 – 18:00",
    jobCount: 6,
    owner: "M. Bello",
    status: "Draft",
    jobs: [
      { id: "JOB-007", title: "Pickup: Apapa Port → Tin Can", provider: "R. Yusuf", status: "Pending", location: "Apapa" },
      { id: "JOB-008", title: "Delivery: Tin Can → Mile 2", provider: "H. Mohammed", status: "Pending", location: "Tin Can" },
    ],
  },
  {
    id: "PLN-0139",
    name: "Yaba evening wave",
    branch: "Yaba",
    window: "16:00 – 20:00",
    jobCount: 11,
    owner: "P. Okafor",
    status: "Draft",
    jobs: [
      { id: "JOB-009", title: "Pickup: Yaba Tech → UNILAG", provider: "D. Eze", status: "Pending", location: "Yaba" },
      { id: "JOB-010", title: "Delivery: UNILAG → Surulere", provider: "T. Nwachukwu", status: "Pending", location: "UNILAG" },
    ],
  },
];

// Mock unassigned work data
export const UNASSIGNED_WORK: UnassignedWork[] = [
  {
    id: "KG-8532",
    title: "Priority delivery",
    route: "Ikeja → Lekki",
    serviceType: "Dispatch",
    urgency: "High",
    blocker: "No eligible provider available",
    pickupTime: "14:10",
    deliveryWindow: "15:30 - 16:30",
    distance: "8.4 km",
    weight: "2.3 kg",
    parcelCount: 2,
    fragile: true,
    coldChain: false,
    branch: "Ikeja",
    createdAt: "2026-08-17T14:10:00Z",
  },
  {
    id: "KG-8528",
    title: "Retail replenishment",
    route: "Yaba → Surulere",
    serviceType: "Dispatch",
    urgency: "Medium",
    blocker: "Vehicle maintenance",
    pickupTime: "14:25",
    deliveryWindow: "15:45 - 16:45",
    distance: "5.2 km",
    weight: "7.2 kg",
    parcelCount: 4,
    fragile: false,
    coldChain: false,
    branch: "Yaba",
    createdAt: "2026-08-17T14:25:00Z",
  },
  {
    id: "KG-8521",
    title: "Private shipment",
    route: "Ikeja → VI",
    serviceType: "Private",
    urgency: "High",
    blocker: null,
    pickupTime: "14:40",
    deliveryWindow: "15:30 - 16:00",
    distance: "12.1 km",
    weight: "3.8 kg",
    parcelCount: 3,
    fragile: false,
    coldChain: true,
    branch: "Ikeja",
    createdAt: "2026-08-17T14:40:00Z",
  },
  {
    id: "KG-8519",
    title: "Hauling job",
    route: "Apapa → Ikeja",
    serviceType: "Hauling",
    urgency: "Low",
    blocker: "Asset unavailable",
    pickupTime: "15:05",
    deliveryWindow: "17:00 - 19:00",
    distance: "15.4 km",
    weight: "220 kg",
    parcelCount: 6,
    fragile: false,
    coldChain: false,
    branch: "Apapa",
    createdAt: "2026-08-17T15:05:00Z",
  },
  {
    id: "KG-8514",
    title: "Express courier",
    route: "Lekki → Ikoyi",
    serviceType: "Dispatch",
    urgency: "High",
    blocker: "Provider assigned but not confirmed",
    pickupTime: "15:20",
    deliveryWindow: "16:00 - 16:30",
    distance: "6.7 km",
    weight: "1.5 kg",
    parcelCount: 1,
    fragile: true,
    coldChain: false,
    branch: "Lekki",
    createdAt: "2026-08-17T15:20:00Z",
  },
  {
    id: "KG-8508",
    title: "Bulk goods transfer",
    route: "Yaba → Marina",
    serviceType: "Hauling",
    urgency: "Medium",
    blocker: null,
    pickupTime: "15:45",
    deliveryWindow: "17:30 - 19:30",
    distance: "18.2 km",
    weight: "450 kg",
    parcelCount: 12,
    fragile: false,
    coldChain: false,
    branch: "Yaba",
    createdAt: "2026-08-17T15:45:00Z",
  },
];

// Helper function to get unassigned work by ID
export const getUnassignedWorkById = (id: string): UnassignedWork | undefined => {
  return UNASSIGNED_WORK.find(work => work.id === id);
};

// Helper function to get unassigned work by branch
export const getUnassignedWorkByBranch = (branch: string): UnassignedWork[] => {
  return UNASSIGNED_WORK.filter(work => work.branch === branch);
};

// Helper function to get a plan by ID
export const getPlanById = (id: string): DispatchPlan | undefined => {
  return DISPATCH_PLANS.find(plan => plan.id === id);
};

// Helper function to get jobs for a plan
export const getPlanJobs = (planId: string): DispatchJob[] => {
  const plan = getPlanById(planId);
  return plan?.jobs || [];
};