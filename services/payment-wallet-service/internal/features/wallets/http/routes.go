package wallethttp

import (
	"time"

	"github.com/gin-gonic/gin"

	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func RegisterRoutes(group *gin.RouterGroup, service *walletusecases.WalletService, customerAccessSecret []byte, providerAccessSecrets map[string][]byte, serviceSecrets serviceauth.Secrets) {
	handler := NewHandler(service)

	customer := group.Group("")
	customer.Use(sharedauth.BearerMiddleware(sharedauth.NewTokenSigner(customerAccessSecret), "customer", "customer"))
	customer.GET("/wallets/me", handler.CustomerWallet)
	customer.GET("/wallets/me/transactions", handler.CustomerTransactions)
	customer.POST("/topups", handler.CreateTopUp)

	provider := group.Group("/provider")
	provider.Use(providerBearerMiddleware(providerAccessSecrets))
	provider.GET("/earnings", handler.ProviderEarnings)
	provider.POST("/bank-accounts/resolve", handler.ResolveBankAccount)
	provider.POST("/bank-accounts", handler.RegisterBankAccount)
	provider.POST("/withdrawals", handler.RequestWithdrawal)

	internal := group.Group("/internal")
	internal.Use(serviceauth.Middleware(serviceauth.NewVerifier(serviceSecrets, 5*time.Minute)))
	internal.POST("/payment-intents", handler.CreatePaymentIntent)
	internal.POST("/payment-intents/:id/pay-from-wallet", handler.PayFromWallet)
	internal.POST("/jobs/:source_service/:source_reference/complete", handler.CompleteJob)
	internal.POST("/refunds", handler.RequestRefund)
	internal.GET("/payments/:reference", handler.GetInternalPayment)

	group.POST("/webhooks/paystack", handler.PaystackWebhook)
}
