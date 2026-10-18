package main

import (
    "net/http"
    "net/http/httptest"
    "sync/atomic"
    "testing"
    "time"

    "cosmicforge/logistics/services/api-gateway/internal/config"
    "cosmicforge/logistics/services/api-gateway/internal/features/routing/models"
)

func TestRoutingOnlyGatewayPreservesPartnerAndRejectsAdmin(t *testing.T) {
    var calls atomic.Int64
    upstream:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        calls.Add(1)
        if r.URL.Path!="/api/v1/partner/v1/deliveries" || r.Header.Get("X-API-Key")!="partner-token" { t.Errorf("forwarded partner request %s %q",r.URL.Path,r.Header.Get("X-API-Key")) }
        w.Header().Set("X-Partner-Owner","true")
        w.WriteHeader(http.StatusAccepted)
    }))
    defer upstream.Close()
    router,err:=newRouter(config.Config{
        Origins:map[string]string{models.UpstreamPartnerWeb:upstream.URL},
        Timeout:time.Second,BreakerThreshold:2,BreakerCooldown:time.Second,
        AllowedOrigins:[]string{"http://localhost:3000"},
    })
    if err!=nil { t.Fatal(err) }
    req:=httptest.NewRequest(http.MethodPost,"/api/v1/partner/v1/deliveries",nil)
    req.Header.Set("X-API-Key","partner-token")
    resp:=httptest.NewRecorder()
    router.ServeHTTP(resp,req)
    if resp.Code!=http.StatusAccepted || resp.Header().Get("X-Partner-Owner")!="true" || calls.Load()!=1 {
        t.Fatalf("partner path was not transparently forwarded: code=%d count=%d",resp.Code,calls.Load())
    }
    for _,path:=range []string{"/api/v1/admin/users","/internal/karryam-admin/v1/capabilities","/api/v1/internal/callbacks/trip-started"} {
        resp=httptest.NewRecorder()
        router.ServeHTTP(resp,httptest.NewRequest(http.MethodGet,path,nil))
        if resp.Code!=http.StatusNotFound || calls.Load()!=1 { t.Fatalf("private route leaked: %s %d",path,resp.Code) }
    }
    resp=httptest.NewRecorder()
    router.ServeHTTP(resp,httptest.NewRequest(http.MethodGet,"/ready",nil))
    if resp.Code!=http.StatusOK { t.Fatalf("routing-only readiness unexpectedly depends on partner database: %d",resp.Code) }
}
