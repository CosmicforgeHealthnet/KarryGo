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
	customer.POST("/topups/:reference/verify", handler.VerifyTopUp)

	provider := group.Group("/provider")
	provider.Use(providerBearerMiddleware(providerAccessSecrets))
	provider.GET("/earnings", handler.ProviderEarnings)
	provider.GET("/bank-accounts", handler.ListBankAccounts)
	provider.POST("/bank-accounts/resolve", handler.ResolveBankAccount)
	provider.POST("/bank-accounts", handler.RegisterBankAccount)
	provider.DELETE("/bank-accounts/:id", handler.DeleteBankAccount)
	provider.PUT("/bank-accounts/:id/default", handler.SetDefaultBankAccount)
	provider.POST("/withdrawals", handler.RequestWithdrawal)
	provider.GET("/pin/status", handler.TransactionPinStatus)
	provider.POST("/pin", handler.SetTransactionPin)
	provider.PUT("/pin", handler.ChangeTransactionPin)

	internal := group.Group("/internal")
	internal.Use(serviceauth.Middleware(serviceauth.NewVerifier(serviceSecrets, 5*time.Minute)))
	internal.POST("/payment-intents", handler.CreatePaymentIntent)
	internal.POST("/payment-intents/:id/pay-from-wallet", handler.PayFromWallet)
	internal.POST("/jobs/:source_service/:source_reference/complete", handler.CompleteJob)
	internal.POST("/refunds", handler.RequestRefund)
	internal.GET("/payments/:reference", handler.GetInternalPayment)
	internal.PUT("/payments/:reference/provider", handler.AssignPaymentProvider)

	group.POST("/webhooks/paystack", handler.PaystackWebhook)
}
