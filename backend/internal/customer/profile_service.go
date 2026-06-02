package customer

import (
	"context"
	"net/http"

	"karrygo/backend/internal/platform/apperrors"
)

type ProfileService struct {
	repository *ProfileRepository
}

func NewProfileService(repository *ProfileRepository) *ProfileService {
	return &ProfileService{repository: repository}
}

func (service *ProfileService) GetMe(_ context.Context, _ CustomerIdentity) (interface{}, error) {
	return nil, apperrors.New(
		http.StatusNotImplemented,
		apperrors.Code("not_implemented"),
		"Customer profile storage is not implemented yet.",
		nil,
	)
}
