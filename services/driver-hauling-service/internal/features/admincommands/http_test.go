package admincommands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestPostgresSignedCommandBoundaryOnTheRealRegistration(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	w.allow("trucks", w.truck, w.scope)
	gin.SetMode(gin.TestMode)
	write, read := bytes.Repeat([]byte("w"), 32), bytes.Repeat([]byte("r"), 32)
	router := gin.New()
	(&admincommand.Handler{Service: w.svc, WriteSecret: write, ReadSecret: read}).Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	client, err := admincommand.NewClient("driver-hauling-service", server.URL, write, read)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	q := adminread.Query{Scope: adminread.Scope{Market: w.scope.Market, Zone: w.scope.Zone, Organization: w.scope.Organization, Tenant: w.scope.Tenant}, Staff: "staff-a", Purpose: purpose, Start: time.Now().Add(-time.Hour), End: time.Now(), Limit: 1, Role: "staff", PolicyID: w.svc.Policy.ID(), PolicyVersion: w.svc.Policy.Version(), PolicyChecksum: w.svc.Policy.Checksum()}
	inspection := "insp-" + uuid.NewString()
	record := w.command("trucks", w.truck, "RECORD_INSPECTION", inspection, 0, map[string]string{"inspection_kind": "REFRIGERATION", "performed_on": "2026-10-01"})
	receipt, err := client.Execute(ctx, q, record)
	if err != nil || receipt.ResultingState != "RECORDED" {
		t.Fatal("signed command", err)
	}
	back, err := client.Readback(ctx, q, "trucks", w.truck, record.IdempotencyKey)
	if err != nil || back != receipt {
		t.Fatal("signed readback", err)
	}
	if _, err := client.Readback(ctx, q, "trucks", w.truck, uuid.NewString()); !errors.Is(err, admincommand.ErrAbsent) {
		t.Fatal("absence", err)
	}
	history, err := client.History(ctx, q, "trucks", w.truck)
	if err != nil || len(history.Decisions) != 1 || history.CurrentState != "ACTIVE" {
		t.Fatal("signed history", err, history)
	}
	stale := w.command("trucks", w.truck, "ATTACH_INSPECTION_EVIDENCE", inspection, 5, map[string]string{"media_object_id": uuid.NewString()})
	if _, err := client.Execute(ctx, q, stale); !errors.Is(err, admincommand.ErrRefused) {
		t.Fatal("stale version over the wire", err)
	}
	wrong, _ := admincommand.NewClient("driver-hauling-service", server.URL, bytes.Repeat([]byte("x"), 32), read)
	if _, err := wrong.Execute(ctx, q, w.command("trucks", w.truck, "ATTACH_INSPECTION_EVIDENCE", inspection, 1, map[string]string{"media_object_id": uuid.NewString()})); err == nil {
		t.Fatal("wrong key accepted")
	}
	// A captured signed request replayed byte-for-byte is refused by nonce.
	body, _ := json.Marshal(w.command("trucks", w.truck, "ATTACH_INSPECTION_EVIDENCE", inspection, 1, map[string]string{"media_object_id": uuid.NewString()}))
	signed, _ := http.NewRequest(http.MethodPost, server.URL+w.spec.Path()+"?"+q.Values().Encode(), bytes.NewReader(body))
	signed.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(signed, adminread.Caller, write, body, time.Now()); err != nil {
		t.Fatal(err)
	}
	send := func() int {
		r := signed.Clone(ctx)
		r.Body = io.NopCloser(bytes.NewReader(body))
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := send(); code != 200 {
		t.Fatal("first signed attach", code)
	}
	if code := send(); code != 409 {
		t.Fatal("replayed nonce accepted", code)
	}
	if w.scalar(`SELECT evidence_count::text FROM hauling_admin_inspections WHERE id=$1`, inspection) != "1" {
		t.Fatal("replay executed twice")
	}
	raw, _ := json.Marshal(history)
	if strings.Contains(string(raw), "staff-a") {
		t.Fatal("history discloses staff identity")
	}
}
