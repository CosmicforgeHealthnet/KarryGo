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

	// Section 12. Managed settlement is posted only by Fleet and read back by
	// Fleet and the execution services; the usecase layer narrows the caller
	// further than this HMAC group does.
	internal.POST("/finance/v1/managed-settlements", handler.PostManagedSettlement)
	internal.GET("/finance/v1/managed-settlements/:instructionId/:instructionVersion", handler.GetManagedSettlement)
	internal.GET("/finance/v1/managed-settlements/by-key/:idempotencyKey", handler.GetManagedSettlementByKey)
	internal.GET("/finance/v1/managed-settlements/by-job/:sourceService/:sourceJobId", handler.ListManagedSettlementsForJob)
	internal.PUT("/finance/v1/managed-jobs/:sourceService/:sourceJobId", handler.MarkJobManaged)
	internal.POST("/finance/v1/managed-settlements/:instructionId/:instructionVersion/reverse", handler.ReverseManagedSettlement)
	internal.POST("/refunds", handler.RequestRefund)
	internal.GET("/payments/:reference", handler.GetInternalPayment)
	internal.PUT("/payments/:reference/provider", handler.AssignPaymentProvider)
	internal.POST("/vendor-bank-profiles/resolve", handler.ResolveVendorBankProfile)
	internal.PUT("/vendor-bank-profiles/:businessId", handler.PutVendorBankProfile)
	internal.GET("/vendor-bank-profiles/:businessId", handler.GetVendorBankProfile)
	internal.POST("/commerce/delivery-funding", handler.ApplyDeliveryFunding)
	internal.GET("/commerce/delivery-funding/:sourceReference", handler.GetDeliveryFunding)
	internal.POST("/commerce/payment-intents", handler.CreateCommerceIntent)
	internal.GET("/commerce/payment-intents/:parentReference", handler.GetCommerceIntent)
	internal.POST("/commerce/payment-intents/:parentReference/hold", handler.HoldCommerceIntent)
	internal.POST("/commerce/allocations/:vendorOrderId/vendor-release", handler.ReleaseVendorPayable)
	internal.POST("/commerce/allocations/:vendorOrderId/delivery-release", handler.ReleaseCommerceDelivery)
	internal.POST("/commerce/allocations/:vendorOrderId/delivery-responsibility", handler.TransitionDeliveryResponsibility)
	internal.GET("/commerce/delivery-responsibility-transitions/:operationRef", handler.GetDeliveryResponsibilityTransition)
	internal.POST("/commerce/allocations/:vendorOrderId/refunds", handler.RefundCommerceAllocation)
	internal.GET("/commerce/operations/:operationType/:idempotencyKey", handler.GetCommerceOperation)
	internal.GET("/vendor-finance/:businessId/summary", handler.VendorPayableSummary)
	internal.GET("/vendor-finance/:businessId/transactions", handler.VendorFinanceTransactions)
	internal.GET("/vendor-finance/:businessId/bank-accounts", handler.VendorFinanceBankAccounts)
	internal.POST("/vendor-finance/:businessId/withdrawals", handler.RequestVendorWithdrawal)
	internal.GET("/vendor-finance/:businessId/withdrawals", handler.VendorWithdrawals)
	internal.GET("/vendor-finance/:businessId/withdrawals/:reference", handler.VendorWithdrawal)

	group.POST("/webhooks/paystack", handler.PaystackWebhook)
}
