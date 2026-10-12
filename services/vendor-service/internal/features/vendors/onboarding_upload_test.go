package vendors

import (
	"context"
	"testing"
	"time"
)

type uploadAuthorizer struct{ allowed bool }

func (a uploadAuthorizer) Authorize(context.Context, string, string, *string) (bool, error) {
	return a.allowed, nil
}

type uploadAuthority struct {
	calls int
	grant OnboardingUploadGrant
}

func (a *uploadAuthority) GetObject(context.Context, string, string) (MediaObject, error) {
	return MediaObject{}, nil
}
func (a *uploadAuthority) AuthorizeOnboardingUpload(_ context.Context, business string, input OnboardingUploadInput, key string) (OnboardingUploadGrant, error) {
	a.calls++
	return a.grant, nil
}
func TestOnboardingUploadRejectsScopeAndUnsupportedContentBeforeAuthority(t *testing.T) {
	actor := Actor{StaffID: "staff-a", VendorID: "business-a"}
	valid := OnboardingUploadInput{Dimension: DimensionBusiness, SubjectType: "BUSINESS", SubjectRef: "business-a", MIMEType: "application/pdf", ByteSize: 100}
	authority := &uploadAuthority{grant: OnboardingUploadGrant{SessionID: "session-a", MediaRef: "object-a", URL: "https://media.example.com/content", Token: "one-use", ExpiresAt: time.Now().Add(time.Minute), MaxBytes: 100, AllowedMIMETypes: []string{"application/pdf"}}}
	service := NewService(nil, uploadAuthorizer{true}, nil, nil)
	service.media = authority
	tests := map[string]func(*OnboardingUploadInput){"foreign business": func(i *OnboardingUploadInput) { i.SubjectRef = "business-b" }, "mime": func(i *OnboardingUploadInput) { i.MIMEType = "text/html" }, "size": func(i *OnboardingUploadInput) { i.ByteSize = MaxOnboardingUploadBytes + 1 }, "empty": func(i *OnboardingUploadInput) { i.ByteSize = 0 }, "dimension": func(i *OnboardingUploadInput) { i.Dimension = "INVENTED" }, "subject": func(i *OnboardingUploadInput) { i.SubjectType = "CUSTOMER" }}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := valid
			mutate(&input)
			if _, err := service.AuthorizeOnboardingUpload(context.Background(), actor, input, "command-a"); err == nil {
				t.Fatal("unsafe upload accepted")
			}
		})
	}
	if authority.calls != 0 {
		t.Fatal("unsafe requests reached media authority")
	}
	grant, err := service.AuthorizeOnboardingUpload(context.Background(), actor, valid, "command-a")
	if err != nil || grant.MediaRef != "object-a" || authority.calls != 1 {
		t.Fatalf("valid upload failed: %v", err)
	}
	service.auth = uploadAuthorizer{false}
	if _, err = service.AuthorizeOnboardingUpload(context.Background(), actor, valid, "command-a"); err == nil || authority.calls != 1 {
		t.Fatal("denied staff reached media authority")
	}
	service.auth = uploadAuthorizer{true}
	authority.grant.ExpiresAt = time.Now().Add(-time.Minute)
	if _, err = service.AuthorizeOnboardingUpload(context.Background(), actor, valid, "command-a"); err == nil {
		t.Fatal("expired grant returned to phone")
	}
}
