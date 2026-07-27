package clients

import (
	"context"
	"sync"
)

// FakeSender is an in-memory Sender for tests and local smoke runs.
//
// It is deliberately NOT selectable from configuration: `cmd` only ever wires
// NotificationSender, so a no-op adapter can never be configured as a
// production success path. Tests construct this type directly.
type FakeSender struct {
	mu sync.Mutex

	// FailWith, when set, makes every send fail with that error — used to
	// prove the API returns otp_delivery_unavailable rather than success.
	FailWith error

	SentOTPs        []OTPMessage
	SentInvitations []InvitationMessage
}

// NewFakeSender builds an empty fake.
func NewFakeSender() *FakeSender {
	return &FakeSender{}
}

// SendOTP records the message.
func (f *FakeSender) SendOTP(_ context.Context, message OTPMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailWith != nil {
		return "", f.FailWith
	}
	f.SentOTPs = append(f.SentOTPs, message)
	return "fake-notification-" + message.IdempotencyKey, nil
}

// SendInvitation records the message.
func (f *FakeSender) SendInvitation(_ context.Context, message InvitationMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailWith != nil {
		return "", f.FailWith
	}
	f.SentInvitations = append(f.SentInvitations, message)
	return "fake-notification-" + message.IdempotencyKey, nil
}

// LastOTP returns the most recent OTP message.
func (f *FakeSender) LastOTP() (OTPMessage, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.SentOTPs) == 0 {
		return OTPMessage{}, false
	}
	return f.SentOTPs[len(f.SentOTPs)-1], true
}

// LastInvitation returns the most recent invitation message.
func (f *FakeSender) LastInvitation() (InvitationMessage, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.SentInvitations) == 0 {
		return InvitationMessage{}, false
	}
	return f.SentInvitations[len(f.SentInvitations)-1], true
}

// OTPCount returns how many OTP messages were sent.
func (f *FakeSender) OTPCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.SentOTPs)
}

// Reset clears recorded messages.
func (f *FakeSender) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SentOTPs = nil
	f.SentInvitations = nil
	f.FailWith = nil
}
