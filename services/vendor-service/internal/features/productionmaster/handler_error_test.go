package productionmaster

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/shared/go/apperrors"
)

func TestHandlerErrorMapsDuplicateCodeToConflict(t *testing.T) {
	err := handlerError(fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23505"}))
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Status != http.StatusConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}
