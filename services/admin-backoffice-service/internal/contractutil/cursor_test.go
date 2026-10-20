package contractutil

import (
	"bytes"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"testing"
	"time"
)

func TestCursorScopeExpiryAndBounds(t *testing.T) {
	now := time.Now()
	c := Cursor{Product: "karryam", Staff: "staff-1", Permission: "operations.read", Scope: security.Scope{Market: "NG", Organization: "org-1"}, Sort: "created_at,id", AfterTime: now, AfterID: "operation-1", Expires: now.Add(time.Minute).Unix()}
	key := bytes.Repeat([]byte{43}, 32)
	token, err := Encode(c, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Decode(token, key, c, now); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Cursor){func(c *Cursor) { c.Scope.Organization = "org-2" }, func(c *Cursor) { c.Scope.Market = "IT" }, func(c *Cursor) { c.Staff = "staff-2" }, func(c *Cursor) { c.Permission = "other" }} {
		expected := c
		change(&expected)
		if _, err = Decode(token, key, expected, now); err == nil {
			t.Fatal("cursor escaped scope")
		}
	}
	if _, err = Decode(token+"x", key, c, now); err == nil {
		t.Fatal("cursor tampering accepted")
	}
	if _, err = Decode(token, key, c, now.Add(time.Hour)); err == nil {
		t.Fatal("expired cursor accepted")
	}
	if Bounds(100, now, now.Add(time.Hour)) != nil || Bounds(101, now, now) == nil || Bounds(10, now, now.Add(32*24*time.Hour)) == nil {
		t.Fatal("unbounded query")
	}
}
