package walletrepositories

import "testing"

func validFunding() ApplyDeliveryFundingInput {
	return ApplyDeliveryFundingInput{SourceService: "vendor-service", SourceReference: "funding:order_1", VendorOrderID: "order_1", SponsoringVendorID: "biz_1", ProviderType: "dispatch", ProviderID: "provider_1", Currency: "NGN", AuthoritativeDeliveryMinor: 2300, CustomerContributionMinor: 1500, VendorSubsidyMinor: 300, PlatformSubsidyMinor: 500, PlatformApprovalRef: "platform_approval_1", CustomerPaymentReference: "pay_1", IdempotencyKey: "funding:order_1"}
}

func TestPlatformFundingRequiresApprovalEvidence(t *testing.T) {
	input := validFunding()
	input.PlatformApprovalRef = ""
	if validateDeliveryFunding(input) == nil {
		t.Fatal("platform subsidy without approval was accepted")
	}
}
func TestDeliveryFundingInvariant(t *testing.T) {
	input := validFunding()
	if err := validateDeliveryFunding(input); err != nil {
		t.Fatal(err)
	}
	input.PlatformSubsidyMinor = 499
	if err := validateDeliveryFunding(input); err == nil {
		t.Fatal("underfunded delivery accepted")
	}
	input = validFunding()
	input.VendorSubsidyMinor = -1
	if err := validateDeliveryFunding(input); err == nil {
		t.Fatal("negative subsidy accepted")
	}
}
func TestDeliveryFundingFingerprintBindsSponsorAndAmounts(t *testing.T) {
	a := validFunding()
	b := a
	b.SponsoringVendorID = "biz_2"
	first, _ := fundingFingerprint(a)
	second, _ := fundingFingerprint(b)
	if first == second {
		t.Fatal("foreign sponsor produced same fingerprint")
	}
	b = a
	b.PlatformSubsidyMinor++
	b.CustomerContributionMinor--
	second, _ = fundingFingerprint(b)
	if first == second {
		t.Fatal("changed split produced same fingerprint")
	}
}
