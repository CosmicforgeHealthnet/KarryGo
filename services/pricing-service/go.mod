module cosmicforge/logistics/services/pricing-service

go 1.26.3

require (
	cosmicforge/logistics/shared/go v0.0.0
	github.com/gin-gonic/gin v1.12.0
	github.com/jackc/pgx/v5 v5.10.0
	github.com/joho/godotenv v1.5.1
)

require github.com/google/uuid v1.6.0 // indirect

replace cosmicforge/logistics/shared/go => ../../shared/go
