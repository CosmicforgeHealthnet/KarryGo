package goods

import (
	"context"
	"errors"
	"net/http"

	"cosmicforge/logistics/shared/go/commercemission"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// HandoverObservation deliberately excludes hashes, credentials and authority
// lifecycle internals. The authority's observation revision orders updates.
type HandoverObservation struct {
	MissionRef    string `json:"mission_ref"`
	HandoverRef   string `json:"handover_ref"`
	Stage         string `json:"stage"`
	Status        string `json:"status"`
	SourceVersion int64  `json:"source_version"`
}

func (s *Service) ReadHandover(ctx context.Context, caller, missionRef, stage string) (HandoverObservation, error) {
	if caller != commercemission.SignatureCaller || (stage != StagePickup && stage != StageDelivery) {
		return HandoverObservation{}, ErrInvalid
	}
	// Reuse source authorization and feature gating before reading a challenge.
	if _, err := s.ReadMission(ctx, caller, missionRef); err != nil {
		return HandoverObservation{}, err
	}
	var o HandoverObservation
	err := s.repo.db.QueryRow(ctx, `SELECT m.mission_ref,h.challenge_ref,h.stage,h.status,h.observation_version
	 FROM taxi_goods_missions m JOIN taxi_goods_handover_challenges h ON h.mission_id=m.id
	 WHERE m.mission_ref=$1 AND h.stage=$2 AND m.source_service='vendor-service' ORDER BY h.version DESC LIMIT 1`, missionRef, stage).Scan(&o.MissionRef, &o.HandoverRef, &o.Stage, &o.Status, &o.SourceVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return HandoverObservation{}, ErrNotFound
	}
	if err != nil {
		return HandoverObservation{}, ErrUnavailable
	}
	return o, nil
}

func (h *Handler) readHandover(c *gin.Context) {
	o, err := h.service.ReadHandover(c.Request.Context(), caller(c), c.Param("missionRef"), c.Param("stage"))
	if err != nil {
		hError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": o})
}
