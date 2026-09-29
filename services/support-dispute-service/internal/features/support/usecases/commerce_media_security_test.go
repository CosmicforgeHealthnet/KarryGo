package supportusecases

import (
	"context"
	"errors"
	"testing"

	supportclients "cosmicforge/logistics/services/support-dispute-service/internal/features/support/clients"
	"cosmicforge/logistics/shared/go/supportcase"
)

type commerceMediaVerdict struct {
	bindingType, bindingID string
	usable                 bool
	err                    error
}

func (v *commerceMediaVerdict) Validate(_ context.Context, objectID, bindingType, bindingID string) (supportclients.EvidenceVerdict, error) {
	v.bindingType, v.bindingID = bindingType, bindingID
	return supportclients.EvidenceVerdict{Usable: v.usable}, v.err
}

func TestCommerceCaseMediaRefsRequireOrderBoundMediaVerdict(t *testing.T) {
	in := supportcase.CreateRequest{SchemaVersion: 1, OperationRef: "op-1", Category: supportcase.QualityIssue,
		Subject: "Damaged product", Description: "Evidence available", CustomerRef: "customer-1",
		Context: supportcase.Context{CommerceOrderRef: "order-1", VendorOrderRef: "vendor-order-1", VendorRef: "vendor-1", MediaRefs: []string{"media-1"}}}
	if _, _, err := NewSupportService(newMemRepo(), Options{}).CreateCommerceCase(context.Background(), in); err == nil {
		t.Fatal("missing Media authority accepted")
	}
	v := &commerceMediaVerdict{}
	svc := NewSupportService(newMemRepo(), Options{Media: v})
	if _, _, err := svc.CreateCommerceCase(context.Background(), in); err == nil {
		t.Fatal("unusable MediaRef accepted")
	}
	if v.bindingType != "ORDER" || v.bindingID != "order-1" {
		t.Fatalf("validated binding %s/%s", v.bindingType, v.bindingID)
	}
	v.usable = true
	v.err = errors.New("media unavailable")
	if _, _, err := svc.CreateCommerceCase(context.Background(), in); err == nil {
		t.Fatal("Media outage accepted")
	}
}
