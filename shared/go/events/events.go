package events

const (
	CustomerCreated        = "customer.created"
	ProviderCreated        = "provider.created"
	ProviderVerified       = "provider.verified"
	ProviderAuthOTPRequested     = "provider.auth.otp_requested"
	ProviderAuthSessionCreated   = "provider.auth.session_created"
	TaxiRideCreated        = "taxi.ride.created"
	TaxiRideMatched        = "taxi.ride.matched"
	DeliveryCreated        = "dispatch.delivery.created"
	DeliveryMatched        = "dispatch.delivery.matched"
	HaulageCreated         = "hauling.job.created"
	HaulageMatched         = "hauling.job.matched"
	ServiceCompleted       = "service.completed"
	PaymentCaptured        = "payment.captured"
	WalletDebited          = "wallet.debited"
	ProviderEarningCreated = "provider.earning.created"
	PayoutRequested        = "payout.requested"
	NotificationSend       = "notification.send"
	SupportTicketCreated   = "support.ticket.created"
	VerificationRequested  = "verification.requested"
	MediaAssetUploaded     = "media.asset.uploaded"
)
