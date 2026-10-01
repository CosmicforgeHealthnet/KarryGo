package goods

import (
	"context"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/handover"
)

// ReadReceiverCredential reconstructs the active credential only after the
// bearer account matches the mission's explicit frozen receiver account.
func (s *Service) ReadReceiverCredential(ctx context.Context, accountID, missionRef, challengeRef string) (Credential, error) {
	if !s.Enabled() || len(s.secret) == 0 || s.receiverReadLimiter == nil {
		return Credential{}, ErrUnavailable
	}
	accountID, missionRef, challengeRef = strings.TrimSpace(accountID), strings.TrimSpace(missionRef), strings.TrimSpace(challengeRef)
	if accountID == "" || missionRef == "" || challengeRef == "" {
		return Credential{}, ErrInvalid
	}
	allowed, err := s.receiverReadLimiter.Allow(ctx, "taxi-goods:receiver-read:"+accountID+":"+missionRef, 12, time.Minute)
	if err != nil {
		return Credential{}, ErrUnavailable
	}
	if !allowed {
		return Credential{}, ErrRateLimited
	}
	mission, err := s.repo.ReadMissionForOperations(ctx, missionRef)
	if err != nil {
		return Credential{}, err
	}
	if mission.ReceiverRef == "" {
		return Credential{}, ErrReceiverSecureChannelRequired
	}
	if mission.ReceiverRef != accountID {
		return Credential{}, ErrNotFound
	}
	if mission.Status != StatusPickedUp && mission.Status != StatusInTransit && mission.Status != StatusAtDropoff && mission.Status != StatusDeliveryHandoverRequired {
		return Credential{}, ErrNotFound
	}
	record, err := s.repo.LatestChallenge(ctx, mission.ID, StageDelivery, false)
	if err != nil {
		return Credential{}, err
	}
	if record.ChallengeRef != challengeRef || record.Status != "ACTIVE" || record.ProviderID != mission.ProviderID || record.ReceiverRef != accountID || !record.ExpiresAt.After(s.now().UTC()) {
		return Credential{}, ErrNotFound
	}
	secure := handover.CodeRecord{ChallengeRef: record.ChallengeRef, Version: record.Version, Service: "taxi-goods", Stage: handover.StepDelivery,
		BookingID: mission.ID, ProviderID: mission.ProviderID, OwnerID: accountID, Hash: record.OTPHash, QRHash: record.QRHash, ExpiresAt: record.ExpiresAt}
	secrets, ok := handover.RecoverSecureCredentials(s.secret, secure)
	if !ok {
		return Credential{}, ErrUnavailable
	}
	return Credential{ChallengeRef: record.ChallengeRef, Stage: StageDelivery, OTP: secrets.Code, QRToken: secrets.QRToken, Version: record.Version, ExpiresAt: record.ExpiresAt}.WithDirection(mission.MissionRef), nil
}
