package security

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func sign(t *testing.T, c Claims, key ed25519.PrivateKey, kid string) string {
	t.Helper()
	h, _ := json.Marshal(map[string]string{"alg": "EdDSA", "typ": "JWT", "kid": kid})
	p, _ := json.Marshal(c)
	b := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	return b + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(b)))
}
func TestAssertionAndAuthorization(t *testing.T) {
	now := time.Now()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	trust := config.Trust{Issuer: "hq-test", Audience: "karryam-test", PolicyVersion: "test-v1", AssertionLifetimeSeconds: 60, SessionLifetimeSeconds: 300, AllowedACR: []string{"mfa"}, StrongACR: []string{"mfa"}, StaffKeys: map[string]string{"one": base64.RawURLEncoding.EncodeToString(pub)}, StaffKeyExpires: map[string]int64{"one": now.Add(time.Hour).Unix()}}
	valid := Claims{Issuer: trust.Issuer, Subject: "staff-1", Audience: trust.Audience, Product: "karryam", Roles: []string{"staff"}, Permissions: []string{"test.read"}, Markets: []string{"NG"}, Organizations: []string{"org-1"}, SessionID: "session-1", AuthTime: now.Add(-10 * time.Second).Unix(), ACR: "mfa", JTI: "jti-1", IssuedAt: now.Unix(), NotBefore: now.Unix(), Expires: now.Add(time.Minute).Unix(), RequestDigest: strings.Repeat("a", 64)}
	if _, err := Verify(sign(t, valid, key, "one"), trust, now); err != nil {
		t.Fatal("valid assertion denied", err)
	}
	tests := map[string]func(*Claims){"issuer": func(c *Claims) { c.Issuer = "evil" }, "audience": func(c *Claims) { c.Audience = "healthnet" }, "product": func(c *Claims) { c.Product = "healthnet" }, "subject": func(c *Claims) { c.Subject = "" }, "session": func(c *Claims) { c.SessionID = "" }, "session_expired": func(c *Claims) { c.AuthTime = now.Add(-time.Hour).Unix() }, "expiry": func(c *Claims) { c.Expires = now.Unix() }, "future": func(c *Claims) { c.NotBefore = now.Add(time.Second).Unix() }, "lifetime": func(c *Claims) { c.Expires = now.Add(time.Hour).Unix() }, "mfa": func(c *Claims) { c.ACR = "password" }, "permission_bound": func(c *Claims) {
		c.Permissions = make([]string, 129)
		for i := range c.Permissions {
			c.Permissions[i] = fmt.Sprintf("permission.%d", i)
		}
	}, "empty_permissions": func(c *Claims) { c.Permissions = nil }, "duplicate_scope": func(c *Claims) { c.Markets = []string{"NG", "NG"} }}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			c := valid
			change(&c)
			if _, err := Verify(sign(t, c, key, "one"), trust, now); err == nil {
				t.Fatal("invalid assertion accepted")
			}
		})
	}
	bad := sign(t, valid, key, "unknown")
	if _, err := Verify(bad, trust, now); err == nil {
		t.Fatal("unknown key accepted")
	}
	pub2, key2, _ := ed25519.GenerateKey(rand.Reader)
	trust.StaffKeys["two"] = base64.RawURLEncoding.EncodeToString(pub2)
	trust.StaffKeyExpires["two"] = now.Add(time.Hour).Unix()
	if _, err := Verify(sign(t, valid, key2, "two"), trust, now); err != nil {
		t.Fatal("rotation overlap rejected")
	}
	delete(trust.StaffKeys, "one")
	if _, err := Verify(sign(t, valid, key, "one"), trust, now); err == nil {
		t.Fatal("retired key accepted")
	}
	scope := Scope{Market: "NG", Organization: "org-1"}
	policy := Policy{Permission: "test.read", Risk: R1, Version: trust.PolicyVersion}
	if Authorize(valid, policy, scope, trust) != nil {
		t.Fatal("valid policy denied")
	}
	for _, s := range []Scope{{Market: "IT", Organization: "org-1"}, {Market: "NG", Organization: "org-2"}, {Market: "*", Organization: "org-1"}} {
		if Authorize(valid, policy, s, trust) == nil {
			t.Fatal("scope escaped", s)
		}
	}
	policy.Permission = "finance.write"
	if Authorize(valid, policy, scope, trust) == nil {
		t.Fatal("missing permission accepted")
	}
}

func TestApprovalSeparationAndBinding(t *testing.T) {
	now := time.Now()
	c := Claims{Subject: "operator"}
	p := Policy{Risk: R3, Version: "v1"}
	scope := Scope{Market: "NG", Organization: "org"}
	valid := Approval{ID: "approval-1", Operator: "operator", Approver: "independent-approver", Action: "test.action", Digest: strings.Repeat("a", 64), PolicyVersion: "v1", Scope: scope, Expires: now.Add(time.Minute)}
	if CheckApproval(c, p, valid, "test.action", valid.Digest, scope, now) != nil {
		t.Fatal("valid approval refused")
	}
	for name, change := range map[string]func(*Approval){"self": func(a *Approval) { a.Approver = "operator" }, "action": func(a *Approval) { a.Action = "other" }, "digest": func(a *Approval) { a.Digest = "tampered" }, "expired": func(a *Approval) { a.Expires = now }, "policy": func(a *Approval) { a.PolicyVersion = "old" }, "tenant": func(a *Approval) { a.Scope.Organization = "other" }, "revoked": func(a *Approval) { a.Revoked = true }} {
		t.Run(name, func(t *testing.T) {
			a := valid
			change(&a)
			if CheckApproval(c, p, a, "test.action", valid.Digest, scope, now) == nil {
				t.Fatal("approval bypass")
			}
		})
	}
	p.Risk = R4
	if CheckApproval(c, p, valid, "test.action", valid.Digest, scope, now) == nil {
		t.Fatal("break-glass without incident")
	}
}
func TestStrictJSON(t *testing.T) {
	var x struct {
		A string `json:"a"`
	}
	for _, s := range []string{`{"a":"first","a":"second"}`, `{"a":"x","performed_by":"admin"}`, `{"a":"x"} {"a":"y"}`, `[]`, `null`} {
		if Strict([]byte(s), &x) == nil {
			t.Errorf("ambiguous JSON accepted: %s", s)
		}
	}
}
