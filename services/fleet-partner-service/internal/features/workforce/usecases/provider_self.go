package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Provider self-view (spec 46.2 "providers can view their own schedules,
// qualifications, training and explainable performance inputs",
// FLEET-17.15). The membership always comes from the internal handler's
// resolver, never from request input, so a provider reads only their own
// records. No fare, customer price, commission, margin, settlement amount or
// other organization financial data is read, and staff identifiers (who
// verified, who assigned) are not returned.

func selfScope(org, membership string) tenant.Context {
	return tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeSystem, ActorID: "provider:" + membership}
}

// ProviderOwnSchedule returns shift assignments (draft shifts excluded: a
// plan the company has not published is not yet the provider's schedule) and
// leave overlapping [from, to).
func (s *Service) ProviderOwnSchedule(ctx context.Context, org, membership string, from, to time.Time) (models.OwnSchedule, error) {
	if s.o.Flags.KillSwitch || (!s.o.Flags.Shifts && !s.o.Flags.Leave) {
		return models.OwnSchedule{}, s.gate(false, false)
	}
	out := models.OwnSchedule{MembershipID: membership, From: from, To: to, Shifts: []models.OwnShift{}, Leave: []models.OwnLeave{}}
	if s.o.Flags.Shifts {
		shifts, err := s.repo.ProviderOwnShifts(ctx, s.o.Pool, org, membership, from, to)
		if err != nil {
			return out, err
		}
		out.Shifts = shifts
	}
	if s.o.Flags.Leave {
		leave, err := s.repo.ProviderOwnLeave(ctx, s.o.Pool, org, membership, from, to)
		if err != nil {
			return out, err
		}
		out.Leave = leave
	}
	return out, nil
}

// ProviderOwnQualifications lists the provider's qualification records plus
// every ACTIVE qualification mandatory for their service category that they
// do not hold.
func (s *Service) ProviderOwnQualifications(ctx context.Context, org, membership string) ([]models.OwnQualification, error) {
	if e := s.gate(s.o.Flags.Qualifications, false); e != nil {
		return nil, e
	}
	now := s.o.Now().UTC()
	out, err := s.repo.ProviderOwnQualifications(ctx, s.o.Pool, org, membership)
	if err != nil {
		return nil, err
	}
	for i := range out {
		v := &out[i]
		v.Expired = v.Status == "EXPIRED" || (v.ExpiresAt != nil && !v.ExpiresAt.After(now))
	}
	return out, nil
}

// ProviderOwnTraining returns the provider's enrolments and completions.
func (s *Service) ProviderOwnTraining(ctx context.Context, org, membership string) (models.OwnTraining, error) {
	out := models.OwnTraining{MembershipID: membership, Enrollments: []models.OwnTrainingEnrollment{}, Completions: []models.OwnTrainingCompletion{}}
	if e := s.gate(s.o.Flags.Training, false); e != nil {
		return out, e
	}
	enrollments, err := s.repo.ProviderOwnEnrollments(ctx, s.o.Pool, org, membership)
	if err != nil {
		return out, err
	}
	out.Enrollments = enrollments
	completions, err := s.repo.ProviderOwnCompletions(ctx, s.o.Pool, org, membership)
	if err != nil {
		return out, err
	}
	out.Completions = completions
	return out, nil
}

// ProviderOwnPerformance returns the provider's performance snapshots with
// their full inputs, explanation and source versions. Snapshots contain only
// counts and assignment references; no money is ever recorded in them.
func (s *Service) ProviderOwnPerformance(ctx context.Context, org, membership string) ([]models.PerformanceSnapshot, error) {
	return s.ListPerformance(ctx, selfScope(org, membership), membership)
}
