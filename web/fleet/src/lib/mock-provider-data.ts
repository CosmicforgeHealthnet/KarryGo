// src/lib/mock-provider-data.ts

export type Provider = {
  id: string;
  initials: string;
  name: string;
  phone: string;
  email: string;
  service: "Dispatch" | "Hauling";
  branch: string;
  membershipStatus: "Active" | "Inactive" | "Pending" | "Suspended";
  availability: "Online" | "Busy" | "Offline" | "Leave";
  eligibility: "Eligible" | "Warning" | "Blocked";
  asset: string;
  joinedAt: string;
  // Additional fields for detail page
  rating: number;
  completedJobs: number;
  acceptanceRate: number;
  onTimeRate: number;
  utilization: number;
  todayJobs: number;
  weekJobs: number;
  monthJobs: number;
  earnings: string;
  memberSince: string;
  lastActive: string;
};

export type WorkSetup = {
  roles: { label: string; enabled: boolean }[];
  branch: string;
  vehicleClasses: string[];
  companyWorkPolicy: string;
  visibilityPolicy: string;
};

export type AvailabilityBlocker = {
  id: string;
  kind: "Qualification" | "Training" | "Document" | "Safety";
  detail: string;
  severity: "critical" | "warning";
};

export type DeclineRecord = {
  id: string;
  reference: string;
  category: "Safety" | "Availability" | "Operational" | "Personal" | "Technical";
  note: string;
  safetyFlag: boolean;
  time: string;
  outcome: string;
};

export type MembershipEvent = {
  id: string;
  label: string;
  detail: string;
  time: string;
};

export type ComplianceRequirement = {
  id: string;
  name: string;
  status: "Verified" | "Expiring soon" | "Missing";
  expiry: string;
  source: "Company" | "External authority";
};

// ===== PROVIDER DETAIL =====
export const PROVIDER_DETAIL: Provider = {
  id: "prov_po",
  initials: "PO",
  name: "Peter Okafor",
  phone: "+234 803 555 0142",
  email: "peter.okafor@example.com",
  service: "Dispatch",
  branch: "Ikeja",
  membershipStatus: "Active",
  availability: "Online",
  eligibility: "Eligible",
  asset: "Bike BR-88",
  joinedAt: "Mar 14, 2025",
  rating: 4.8,
  completedJobs: 342,
  acceptanceRate: 94,
  onTimeRate: 97,
  utilization: 78,
  todayJobs: 3,
  weekJobs: 18,
  monthJobs: 72,
  earnings: "284,500",
  memberSince: "Jan 2024",
  lastActive: "12 sec ago",
};

// ===== PROVIDERS LIST =====
export const PROVIDERS: Provider[] = [
  {
    id: "prov_po",
    initials: "PO",
    name: "Peter Okafor",
    phone: "+234 803 555 0142",
    email: "peter.okafor@example.com",
    service: "Dispatch",
    branch: "Ikeja",
    membershipStatus: "Active",
    availability: "Online",
    eligibility: "Eligible",
    asset: "Bike BR-88",
    joinedAt: "Mar 14, 2025",
    rating: 4.8,
    completedJobs: 342,
    acceptanceRate: 94,
    onTimeRate: 97,
    utilization: 78,
    todayJobs: 3,
    weekJobs: 18,
    monthJobs: 72,
    earnings: "284,500",
    memberSince: "Jan 2024",
    lastActive: "12 sec ago",
  },
  {
    id: "prov_mb",
    initials: "MB",
    name: "Musa Bello",
    phone: "+234 803 456 7890",
    email: "musa.bello@example.com",
    service: "Hauling",
    branch: "Apapa",
    membershipStatus: "Active",
    availability: "Busy",
    eligibility: "Eligible",
    asset: "Van VN-04",
    joinedAt: "Mar 10, 2025",
    rating: 4.2,
    completedJobs: 189,
    acceptanceRate: 86,
    onTimeRate: 92,
    utilization: 82,
    todayJobs: 2,
    weekJobs: 12,
    monthJobs: 48,
    earnings: "192,300",
    memberSince: "Mar 2024",
    lastActive: "45 sec ago",
  },
  {
    id: "prov_ay",
    initials: "AY",
    name: "Amina Yusuf",
    phone: "+234 804 567 8901",
    email: "amina.yusuf@example.com",
    service: "Dispatch",
    branch: "Yaba",
    membershipStatus: "Active",
    availability: "Online",
    eligibility: "Warning",
    asset: "Bike BR-71",
    joinedAt: "Feb 28, 2025",
    rating: 4.6,
    completedJobs: 178,
    acceptanceRate: 88,
    onTimeRate: 94,
    utilization: 72,
    todayJobs: 1,
    weekJobs: 10,
    monthJobs: 42,
    earnings: "156,700",
    memberSince: "Feb 2024",
    lastActive: "2 min ago",
  },
  {
    id: "prov_cu",
    initials: "CU",
    name: "Chinedu Uche",
    phone: "+234 805 678 9012",
    email: "chinedu.uche@example.com",
    service: "Dispatch",
    branch: "Lekki",
    membershipStatus: "Active",
    availability: "Offline",
    eligibility: "Eligible",
    asset: "Bike BR-20",
    joinedAt: "Feb 15, 2025",
    rating: 4.9,
    completedJobs: 512,
    acceptanceRate: 97,
    onTimeRate: 99,
    utilization: 85,
    todayJobs: 0,
    weekJobs: 15,
    monthJobs: 65,
    earnings: "412,800",
    memberSince: "Dec 2023",
    lastActive: "4 min ago",
  },
  {
    id: "prov_ka",
    initials: "KA",
    name: "Kabiru Ahmed",
    phone: "+234 806 789 0123",
    email: "kabiru.ahmed@example.com",
    service: "Hauling",
    branch: "Lekki",
    membershipStatus: "Suspended",
    availability: "Leave",
    eligibility: "Blocked",
    asset: "Truck TR-12",
    joinedAt: "Jan 20, 2025",
    rating: 4.4,
    completedJobs: 234,
    acceptanceRate: 78,
    onTimeRate: 85,
    utilization: 60,
    todayJobs: 0,
    weekJobs: 5,
    monthJobs: 28,
    earnings: "312,500",
    memberSince: "Jan 2024",
    lastActive: "1 hr ago",
  },
  {
    id: "prov_eo",
    initials: "EO",
    name: "Esther Obi",
    phone: "+234 807 890 1234",
    email: "esther.obi@example.com",
    service: "Dispatch",
    branch: "Ikeja",
    membershipStatus: "Active",
    availability: "Online",
    eligibility: "Eligible",
    asset: "Bike BR-12",
    joinedAt: "Jan 5, 2025",
    rating: 4.7,
    completedJobs: 412,
    acceptanceRate: 92,
    onTimeRate: 96,
    utilization: 80,
    todayJobs: 2,
    weekJobs: 14,
    monthJobs: 58,
    earnings: "356,200",
    memberSince: "Jan 2024",
    lastActive: "3 min ago",
  },
];

// ===== WORK SETUP =====
export const WORK_SETUP: WorkSetup = {
  roles: [
    { label: "Standard delivery", enabled: true },
    { label: "Express / priority delivery", enabled: true },
    { label: "Fragile / high-value handling", enabled: false },
  ],
  branch: "Ikeja",
  vehicleClasses: ["Motorcycle — Standard", "Motorcycle — Cargo box"],
  companyWorkPolicy: "Company assignments + open marketplace",
  visibilityPolicy: "Visible to all branches",
};

// ===== AVAILABILITY BLOCKERS =====
export const AVAILABILITY_BLOCKERS: AvailabilityBlocker[] = [
  {
    id: "blk_1",
    kind: "Training",
    detail: "Defensive riding refresher expires in 6 days",
    severity: "warning",
  },
  {
    id: "blk_2",
    kind: "Document",
    detail: "Vehicle roadworthiness certificate expires in 12 days",
    severity: "warning",
  },
  {
    id: "blk_3",
    kind: "Safety",
    detail: "Outstanding vehicle inspection required",
    severity: "critical",
  },
];

// ===== DECLINES =====
export const DECLINES: DeclineRecord[] = [
  {
    id: "dec_1",
    reference: "AS-1187",
    category: "Safety",
    note: "Reported unsafe road condition on assigned route",
    safetyFlag: true,
    time: "2 days ago",
    outcome: "Reassigned",
  },
  {
    id: "dec_2",
    reference: "AS-1142",
    category: "Availability",
    note: "Already committed to another active trip",
    safetyFlag: false,
    time: "5 days ago",
    outcome: "Reassigned",
  },
  {
    id: "dec_3",
    reference: "AS-1098",
    category: "Operational",
    note: "Package exceeded stated size for vehicle class",
    safetyFlag: false,
    time: "1 week ago",
    outcome: "Reassigned",
  },
  {
    id: "dec_4",
    reference: "AS-1054",
    category: "Personal",
    note: "Personal emergency - requested time off",
    safetyFlag: false,
    time: "2 weeks ago",
    outcome: "Reassigned",
  },
  {
    id: "dec_5",
    reference: "AS-1021",
    category: "Technical",
    note: "Vehicle navigation system malfunction",
    safetyFlag: false,
    time: "3 weeks ago",
    outcome: "Reassigned",
  },
];

// ===== MEMBERSHIP EVENTS =====
export const MEMBERSHIP_EVENTS: MembershipEvent[] = [
  {
    id: "ev_1",
    label: "Membership activated",
    detail: "Consent confirmed by provider",
    time: "Mar 14, 2025",
  },
  {
    id: "ev_2",
    label: "Invitation accepted",
    detail: "Provider confirmed via SMS link",
    time: "Mar 12, 2025",
  },
  {
    id: "ev_3",
    label: "Invitation sent",
    detail: "Sent by Operations Manager",
    time: "Mar 10, 2025",
  },
  {
    id: "ev_4",
    label: "Provider profile created",
    detail: "Initial profile setup completed",
    time: "Mar 8, 2025",
  },
  {
    id: "ev_5",
    label: "Invitation generated",
    detail: "System generated invitation link",
    time: "Mar 7, 2025",
  },
];

// ===== COMPLIANCE REQUIREMENTS =====
export const COMPLIANCE_REQUIREMENTS: ComplianceRequirement[] = [
  {
    id: "cmp_1",
    name: "Rider identity verification",
    status: "Verified",
    expiry: "—",
    source: "External authority",
  },
  {
    id: "cmp_2",
    name: "Vehicle roadworthiness certificate",
    status: "Expiring soon",
    expiry: "In 12 days",
    source: "Company",
  },
  {
    id: "cmp_3",
    name: "Defensive riding training",
    status: "Expiring soon",
    expiry: "In 6 days",
    source: "Company",
  },
  {
    id: "cmp_4",
    name: "Background check",
    status: "Verified",
    expiry: "—",
    source: "External authority",
  },
  {
    id: "cmp_5",
    name: "Vehicle insurance",
    status: "Missing",
    expiry: "—",
    source: "Company",
  },
  {
    id: "cmp_6",
    name: "Medical fitness certificate",
    status: "Verified",
    expiry: "—",
    source: "External authority",
  },
];