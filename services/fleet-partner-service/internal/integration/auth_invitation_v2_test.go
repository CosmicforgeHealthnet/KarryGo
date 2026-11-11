package integration_test

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/shared/go/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type authTestOrganizations struct{ pool *pgxpool.Pool }

func (r authTestOrganizations) OrganizationStatus(ctx context.Context, id string) (string, string, error) {
	var name, status string
	err := r.pool.QueryRow(ctx, `SELECT legal_name,status FROM fleet_partner_organizations WHERE id=$1`, id).Scan(&name, &status)
	return name, status, err
}

func TestAuthV2StaffInvitationAndMultipleOrganizations(t *testing.T) {
	ctx := context.Background()
	var invitations *staffrepos.InvitationRepository
	h := newAuthHarness(t, nil, func(opts *authusecases.Options) {
		invitations = staffrepos.NewInvitationRepository(opts.Pool)
		staff := staffusecases.NewService(staffusecases.Options{Pool: opts.Pool, Members: staffrepos.NewMemberRepository(opts.Pool), Invitations: invitations, Roles: staffrepos.NewRoleRepository(opts.Pool), Organizations: authTestOrganizations{opts.Pool}, Events: events.NewDomainPublisher(events.NewOutboxRepository(opts.Pool)), Metrics: observability.NewMetrics(), InvitationSecret: []byte("invite-secret")})
		opts.Invitations = staff
		opts.Memberships = staff
	})
	for _, token := range []string{"invalid-token"} {
		if _, err := h.service.Start(ctx, authusecases.StartInput{Email: "invitee@example.com", Phone: "+2348012345678", InvitationToken: token}); err == nil {
			t.Fatal("invalid invitation accepted")
		}
	}
	if h.sender.OTPCount() != 0 {
		t.Fatal("invalid invitation sent OTP")
	}
	var identityID string
	for i, token := range []string{"first-invitation", "second-invitation"} {
		org := createOrganization(t, h.pool, token, "RC-"+token)
		invitation := &staffmodels.StaffInvitation{FullName: "Invited Staff", EmailNormalized: "invitee@example.com", PhoneE164: "+2348012345678", InvitationTokenHash: auth.HashRefreshToken([]byte("invite-secret"), token), ExpiresAt: time.Now().Add(time.Hour), CreatedBy: "test-owner"}
		if err := invitations.Create(ctx, h.pool, org.ID, invitation); err != nil {
			t.Fatal(err)
		}
		if _, err := h.pool.Exec(ctx, `UPDATE fleet_partner_auth_otp_challenges SET resend_available_at=now()-interval '1 second'`); err != nil {
			t.Fatal(err)
		}
		challenge, err := h.service.Start(ctx, authusecases.StartInput{Email: "invitee@example.com", Phone: "+2348012345678", InvitationToken: token})
		if err != nil {
			t.Fatal(err)
		}
		message, _ := h.sender.LastOTP()
		result, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challenge.ChallengeID, Code: message.Code})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			identityID = result.IdentityID
			if result.NextStep != authusecases.NextStepReady {
				t.Fatal("first invitation did not issue organization session")
			}
		} else {
			if result.IdentityID != identityID || result.NextStep != authusecases.NextStepOrganizationSelection || len(result.Organizations) != 2 {
				t.Fatal("multiple memberships/identity not preserved")
			}
		}
		if _, err := h.service.Start(ctx, authusecases.StartInput{Email: "invitee@example.com", Phone: "+2348012345678", InvitationToken: token}); err == nil {
			t.Fatal("consumed invitation accepted")
		}
	}
	var members int
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_organization_users WHERE identity_id=$1`, identityID).Scan(&members); err != nil || members != 2 {
		t.Fatal("staff memberships not persisted", err)
	}
}
