package main

import (
    "context"
    "errors"
    "log"
    "net/http"
    "os/signal"
    "strings"
    "syscall"
    "time"

    "cosmicforge/logistics/services/api-gateway/internal/config"
    routinghttp "cosmicforge/logistics/services/api-gateway/internal/features/routing/http"
    routingmodels "cosmicforge/logistics/services/api-gateway/internal/features/routing/models"
    "cosmicforge/logistics/shared/go/httpx"
    "github.com/gin-contrib/cors"
    "github.com/gin-gonic/gin"
)

func newRouter(cfg config.Config) (*gin.Engine,error) {
    edge,err:=routinghttp.NewEdge(routinghttp.Options{
        Routes:routingmodels.DefaultRoutes(), Origins:cfg.Origins, Timeout:cfg.Timeout,
        BreakerThreshold:cfg.BreakerThreshold, BreakerCooldown:cfg.BreakerCooldown,
    })
    if err!=nil { return nil,err }
    router:=gin.New()
    commonCORS:=cors.New(cors.Config{
        AllowOrigins:cfg.AllowedOrigins,
        AllowMethods:[]string{"GET","POST","PATCH","PUT","DELETE","OPTIONS"},
        AllowHeaders:[]string{"Origin","Content-Type","Authorization","X-API-Key","Idempotency-Key"},
        AllowCredentials:true,
    })
    router.Use(func(c *gin.Context) {
        // Partner service owns its entire partner-web CORS, cookies and sessions.
        // Applying CORS twice through ReverseProxy emits ambiguous browser headers.
        if strings.HasPrefix(c.Request.URL.Path,"/api/v1/partner/v1") {
            c.Next()
            return
        }
        commonCORS(c)
    })
    router.Use(httpx.RequestID(),httpx.Logger(),httpx.Recovery(),httpx.ErrorHandler())
    router.GET("/health",func(c *gin.Context){ c.JSON(http.StatusOK,gin.H{"success":true,"service":"api-gateway","status":"ok"}) })
    router.GET("/ready",func(c *gin.Context){ c.JSON(http.StatusOK,gin.H{"success":true,"service":"api-gateway","status":"ready"}) })
    router.GET("/api/v1/gateway/meta",func(c *gin.Context) {
        c.JSON(http.StatusOK,gin.H{"success":true,"service":"api-gateway","api_base":"/api/v1/gateway",
            "capabilities":[]string{"routes app traffic to canonical services","correlation and upstream resilience"},
            "partner_upstream_configured":cfg.Origins[routingmodels.UpstreamPartnerWeb]!="",
        })
    })
    router.GET("/api/v1/gateway/customer/overview",edge.Aggregate(routinghttp.CustomerOverviewParts,nil))
    router.NoRoute(edge.Handle)
    return router,nil
}
func main() {
    cfg,err:=config.Load()
    if err!=nil { log.Fatal(err) }
    if cfg.AppEnv=="production" { gin.SetMode(gin.ReleaseMode) }
    router,err:=newRouter(cfg)
    if err!=nil { log.Fatal(err) }
    root,stop:=signal.NotifyContext(context.Background(),syscall.SIGINT,syscall.SIGTERM)
    defer stop()
    srv:=&http.Server{Addr:cfg.HTTPAddr,Handler:router,ReadHeaderTimeout:10*time.Second}
    go func(){
        if err:=srv.ListenAndServe();err!=nil&&!errors.Is(err,http.ErrServerClosed) { log.Printf("gateway listen failure: %v",err);stop() }
    }()
    <-root.Done()
    ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
    defer cancel()
    if err:=srv.Shutdown(ctx);err!=nil { log.Printf("gateway shutdown: %v",err) }
}
