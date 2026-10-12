package usecases

import (
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
	"testing"
)

func TestVendorUploadAuthorizationTenantAndPurpose(t *testing.T) {
	valid := models.UploadAuthorization{CallerService: "vendor-service", OwnerService: "vendor-service", OwnerType: "VENDOR_BUSINESS", OwnerReference: "business-a", OrganizationID: "business-a", Visibility: models.VisibilityPrivate, UploadPurpose: models.PurposeComplianceDocument}
	if !vendorUploadAuthorization(&valid) {
		t.Fatal("vendor-owned private compliance grant rejected")
	}
	cases := map[string]func(*models.UploadAuthorization){"caller": func(a *models.UploadAuthorization) { a.CallerService = "customer-service" }, "owner service": func(a *models.UploadAuthorization) { a.OwnerService = "customer-service" }, "owner type": func(a *models.UploadAuthorization) { a.OwnerType = "CUSTOMER" }, "tenant": func(a *models.UploadAuthorization) { a.OrganizationID = "business-b" }, "owner": func(a *models.UploadAuthorization) { a.OwnerReference = "" }, "visibility": func(a *models.UploadAuthorization) { a.Visibility = models.VisibilityPublic }, "purpose": func(a *models.UploadAuthorization) { a.UploadPurpose = models.PurposeDisputeAttachment }}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			a := valid
			change(&a)
			if vendorUploadAuthorization(&a) {
				t.Fatal("foreign or unsafe grant accepted")
			}
		})
	}
	if vendorUploadAuthorization(nil) {
		t.Fatal("nil accepted")
	}
}

func TestVendorUploadAssetPurposes(t *testing.T) {
	for _, purpose := range []string{models.PurposeProductImage, models.PurposeBrandImage, models.PurposePastryReference} {
		grant := models.UploadAuthorization{CallerService: "vendor-service", OwnerService: "vendor-service", OwnerType: "VENDOR_BUSINESS", OwnerReference: "business-a", OrganizationID: "business-a", Visibility: models.VisibilityPrivate, UploadPurpose: purpose}
		if !vendorUploadAuthorization(&grant) {
			t.Fatalf("scoped asset rejected: %s", purpose)
		}
		grant.Visibility = models.VisibilityPublic
		if vendorUploadAuthorization(&grant) {
			t.Fatal("public asset accepted")
		}
	}
}
func TestVendorUploadAssetGrantBoundaries(t *testing.T) {
	s := NewService(Options{Policy: Policy{AllowedMIMETypes: []string{"image/jpeg", "application/pdf"}}})
	input := AuthorizeUploadInput{Caller: Caller{Service: "vendor-service", OrganizationID: "business-a"}, OwnerType: "VENDOR_BUSINESS", OwnerReference: "business-a", UploadPurpose: models.PurposeProductImage, Visibility: models.VisibilityPrivate, BindingType: models.BindingAsset, BindingID: "FOOD_ITEM:item-a", MaxSizeBytes: 100, AllowedMIMETypes: []string{"image/jpeg"}, FileName: "meal.jpg", IdempotencyKey: "key", RequestFingerprint: "fingerprint"}
	if err := s.validateAuthorizeInput(&input); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*AuthorizeUploadInput){"foreign caller": func(r *AuthorizeUploadInput) { r.Caller.Service = "customer-service" }, "foreign business": func(r *AuthorizeUploadInput) { r.OwnerReference = "business-b" }, "unbound": func(r *AuthorizeUploadInput) { r.BindingID = "" }, "public": func(r *AuthorizeUploadInput) { r.Visibility = models.VisibilityPublic }, "oversize": func(r *AuthorizeUploadInput) { r.MaxSizeBytes = 10<<20 + 1 }, "product PDF": func(r *AuthorizeUploadInput) { r.AllowedMIMETypes = []string{"application/pdf"} }} {
		t.Run(name, func(t *testing.T) {
			r := input
			change(&r)
			if s.validateAuthorizeInput(&r) == nil {
				t.Fatal("invalid asset grant accepted")
			}
		})
	}
}
