import apiClient from '@/lib/api/client';
import type {
  AddProviderQualificationInput,
  AssignShiftInput,
  CompleteCourseInput,
  Completion,
  Course,
  CreateCourseInput,
  CreateLeaveInput,
  CreateProfileInput,
  CreateQualificationInput,
  CreateShiftInput,
  Leave,
  Page,
  PerformanceSnapshot,
  Profile,
  ProviderQualification,
  Qualification,
  Shift,
  ShiftAssignment,
  UpdateCourseInput,
  UpdateProfileInput,
  UpdateQualificationInput,
  UpdateShiftInput,
} from '@/lib/types/workforce';

function buildQuery(params?: Page): string {
  if (!params) return '';
  const q = new URLSearchParams();
  if (params.branchId) q.set('branchId', params.branchId);
  if (params.status) q.set('status', params.status);
  if (params.limit != null) q.set('limit', String(params.limit));
  if (params.offset != null) q.set('offset', String(params.offset));
  const qs = q.toString();
  return qs ? `?${qs}` : '';
}

export const workforceService = {
  // -----------------------------------------------------------------------
  // Profiles
  // -----------------------------------------------------------------------

  async listProfiles(params?: Page): Promise<Profile[]> {
    const res = await apiClient.get<{ data: Profile[] }>(
      `/workforce/profiles${buildQuery(params)}`,
    );
    return res.data.data ?? [];
  },

  async getProfile(profileId: string): Promise<Profile> {
    const res = await apiClient.get<{ data: Profile }>(
      `/workforce/profiles/${encodeURIComponent(profileId)}`,
    );
    return res.data.data;
  },

  async createProfile(
    input: CreateProfileInput,
    idempotencyKey: string,
  ): Promise<Profile> {
    const res = await apiClient.post<{ data: Profile }>(
      '/workforce/profiles',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async updateProfile(
    profileId: string,
    input: UpdateProfileInput,
  ): Promise<Profile> {
    const { expectedVersion, ...body } = input;
    const res = await apiClient.patch<{ data: Profile }>(
      `/workforce/profiles/${encodeURIComponent(profileId)}`,
      body,
      { headers: { 'If-Match': String(expectedVersion) } },
    );
    return res.data.data;
  },

  async archiveProfile(
    profileId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Profile> {
    const res = await apiClient.post<{ data: Profile }>(
      `/workforce/profiles/${encodeURIComponent(profileId)}/archive`,
      { expectedVersion },
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  // -----------------------------------------------------------------------
  // Shifts
  // -----------------------------------------------------------------------

  async listShifts(params?: Page): Promise<Shift[]> {
    const res = await apiClient.get<{ data: Shift[] }>(
      `/shifts${buildQuery(params)}`,
    );
    return res.data.data ?? [];
  },

  async getShift(shiftId: string): Promise<Shift> {
    const res = await apiClient.get<{ data: Shift }>(
      `/shifts/${encodeURIComponent(shiftId)}`,
    );
    return res.data.data;
  },

  async createShift(
    input: CreateShiftInput,
    idempotencyKey: string,
  ): Promise<Shift> {
    const res = await apiClient.post<{ data: Shift }>('/shifts', input, {
      headers: { 'Idempotency-Key': idempotencyKey },
    });
    return res.data.data;
  },

  async updateShift(
    shiftId: string,
    input: UpdateShiftInput,
  ): Promise<Shift> {
    const { expectedVersion, ...body } = input;
    const res = await apiClient.patch<{ data: Shift }>(
      `/shifts/${encodeURIComponent(shiftId)}`,
      body,
      { headers: { 'If-Match': String(expectedVersion) } },
    );
    return res.data.data;
  },

  async publishShift(
    shiftId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Shift> {
    const res = await apiClient.post<{ data: Shift }>(
      `/shifts/${encodeURIComponent(shiftId)}/publish`,
      { expectedVersion },
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async cancelShift(
    shiftId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Shift> {
    const res = await apiClient.post<{ data: Shift }>(
      `/shifts/${encodeURIComponent(shiftId)}/cancel`,
      { expectedVersion },
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async assignShift(
    shiftId: string,
    input: AssignShiftInput,
    idempotencyKey: string,
  ): Promise<ShiftAssignment> {
    const res = await apiClient.post<{ data: ShiftAssignment }>(
      `/shifts/${encodeURIComponent(shiftId)}/assignments`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  // -----------------------------------------------------------------------
  // Shift assignments
  // -----------------------------------------------------------------------

  async getAssignment(assignmentId: string): Promise<ShiftAssignment> {
    const res = await apiClient.get<{ data: ShiftAssignment }>(
      `/shift-assignments/${encodeURIComponent(assignmentId)}`,
    );
    return res.data.data;
  },

  async checkInAssignment(
    assignmentId: string,
    expectedVersion: number,
    source: string,
    idempotencyKey: string,
  ): Promise<ShiftAssignment> {
    const res = await apiClient.post<{ data: ShiftAssignment }>(
      `/shift-assignments/${encodeURIComponent(assignmentId)}/check-in`,
      { expectedVersion, source },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async checkOutAssignment(
    assignmentId: string,
    expectedVersion: number,
    source: string,
    idempotencyKey: string,
  ): Promise<ShiftAssignment> {
    const res = await apiClient.post<{ data: ShiftAssignment }>(
      `/shift-assignments/${encodeURIComponent(assignmentId)}/check-out`,
      { expectedVersion, source },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async cancelAssignment(
    assignmentId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<ShiftAssignment> {
    const res = await apiClient.post<{ data: ShiftAssignment }>(
      `/shift-assignments/${encodeURIComponent(assignmentId)}/cancel`,
      { expectedVersion },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  // -----------------------------------------------------------------------
  // Leave
  // -----------------------------------------------------------------------

  async listLeave(params?: Page): Promise<Leave[]> {
    const res = await apiClient.get<{ data: Leave[] }>(
      `/leave${buildQuery(params)}`,
    );
    return res.data.data ?? [];
  },

  async getLeave(leaveId: string): Promise<Leave> {
    const res = await apiClient.get<{ data: Leave }>(
      `/leave/${encodeURIComponent(leaveId)}`,
    );
    return res.data.data;
  },

  async createLeave(
    input: CreateLeaveInput,
    idempotencyKey: string,
  ): Promise<Leave> {
    const res = await apiClient.post<{ data: Leave }>('/leave', input, {
      headers: { 'Idempotency-Key': idempotencyKey },
    });
    return res.data.data;
  },

  async approveLeave(
    leaveId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Leave> {
    const res = await apiClient.post<{ data: Leave }>(
      `/leave/${encodeURIComponent(leaveId)}/approve`,
      { expectedVersion },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async rejectLeave(
    leaveId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Leave> {
    const res = await apiClient.post<{ data: Leave }>(
      `/leave/${encodeURIComponent(leaveId)}/reject`,
      { expectedVersion },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async cancelLeave(
    leaveId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ): Promise<Leave> {
    const res = await apiClient.post<{ data: Leave }>(
      `/leave/${encodeURIComponent(leaveId)}/cancel`,
      { expectedVersion },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  // -----------------------------------------------------------------------
  // Qualifications
  // -----------------------------------------------------------------------

  async listQualifications(): Promise<Qualification[]> {
    const res = await apiClient.get<{ data: Qualification[] }>(
      '/qualifications',
    );
    return res.data.data ?? [];
  },

  async getQualification(qualificationId: string): Promise<Qualification> {
    const res = await apiClient.get<{ data: Qualification }>(
      `/qualifications/${encodeURIComponent(qualificationId)}`,
    );
    return res.data.data;
  },

  async createQualification(
    input: CreateQualificationInput,
    idempotencyKey: string,
  ): Promise<Qualification> {
    const res = await apiClient.post<{ data: Qualification }>(
      '/qualifications',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async updateQualification(
    qualificationId: string,
    input: UpdateQualificationInput,
  ): Promise<Qualification> {
    const { expectedVersion, ...body } = input;
    const res = await apiClient.patch<{ data: Qualification }>(
      `/qualifications/${encodeURIComponent(qualificationId)}`,
      body,
      { headers: { 'If-Match': String(expectedVersion) } },
    );
    return res.data.data;
  },

  async listProviderQualifications(
    membershipId: string,
  ): Promise<ProviderQualification[]> {
    const res = await apiClient.get<{ data: ProviderQualification[] }>(
      `/providers/${encodeURIComponent(membershipId)}/qualifications`,
    );
    return res.data.data ?? [];
  },

  async addProviderQualification(
    membershipId: string,
    input: AddProviderQualificationInput,
    idempotencyKey: string,
  ): Promise<ProviderQualification> {
    const res = await apiClient.post<{ data: ProviderQualification }>(
      `/providers/${encodeURIComponent(membershipId)}/qualifications`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async verifyProviderQualification(
    providerQualificationId: string,
    expectedVersion: number,
    reference: string,
    idempotencyKey: string,
  ): Promise<ProviderQualification> {
    const res = await apiClient.post<{ data: ProviderQualification }>(
      `/provider-qualifications/${encodeURIComponent(providerQualificationId)}/verify`,
      { expectedVersion, reference },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async rejectProviderQualification(
    providerQualificationId: string,
    expectedVersion: number,
    reference: string,
    idempotencyKey: string,
  ): Promise<ProviderQualification> {
    const res = await apiClient.post<{ data: ProviderQualification }>(
      `/provider-qualifications/${encodeURIComponent(providerQualificationId)}/reject`,
      { expectedVersion, reference },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  async revokeProviderQualification(
    providerQualificationId: string,
    expectedVersion: number,
    reference: string,
    idempotencyKey: string,
  ): Promise<ProviderQualification> {
    const res = await apiClient.post<{ data: ProviderQualification }>(
      `/provider-qualifications/${encodeURIComponent(providerQualificationId)}/revoke`,
      { expectedVersion, reference },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
          'If-Match': String(expectedVersion),
        },
      },
    );
    return res.data.data;
  },

  // -----------------------------------------------------------------------
  // Training
  // -----------------------------------------------------------------------

  async listCourses(): Promise<Course[]> {
    const res = await apiClient.get<{ data: Course[] }>(
      '/training/courses',
    );
    return res.data.data ?? [];
  },

  async getCourse(courseId: string): Promise<Course> {
    const res = await apiClient.get<{ data: Course }>(
      `/training/courses/${encodeURIComponent(courseId)}`,
    );
    return res.data.data;
  },

  async createCourse(
    input: CreateCourseInput,
    idempotencyKey: string,
  ): Promise<Course> {
    const res = await apiClient.post<{ data: Course }>(
      '/training/courses',
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  async updateCourse(
    courseId: string,
    input: UpdateCourseInput,
  ): Promise<Course> {
    const { expectedVersion, ...body } = input;
    const res = await apiClient.patch<{ data: Course }>(
      `/training/courses/${encodeURIComponent(courseId)}`,
      body,
      { headers: { 'If-Match': String(expectedVersion) } },
    );
    return res.data.data;
  },

  async completeCourse(
    courseId: string,
    input: CompleteCourseInput,
    idempotencyKey: string,
  ): Promise<Completion> {
    const res = await apiClient.post<{ data: Completion }>(
      `/training/courses/${encodeURIComponent(courseId)}/complete`,
      input,
      { headers: { 'Idempotency-Key': idempotencyKey } },
    );
    return res.data.data;
  },

  // -----------------------------------------------------------------------
  // Performance
  // -----------------------------------------------------------------------

  async listPerformance(
    membershipId: string,
  ): Promise<PerformanceSnapshot[]> {
    const res = await apiClient.get<{ data: PerformanceSnapshot[] }>(
      `/performance/providers/${encodeURIComponent(membershipId)}`,
    );
    return res.data.data ?? [];
  },
};

export function newWorkforceIdempotencyKey(): string {
  if (
    typeof crypto !== 'undefined' &&
    typeof crypto.randomUUID === 'function'
  ) {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}