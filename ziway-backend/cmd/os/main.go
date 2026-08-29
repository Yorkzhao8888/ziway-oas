package main

import (
	"fmt"
	"os"

	"go.uber.org/zap"

	"ziway/backend/internal/bos"
	"ziway/backend/internal/bos/aos"
	"ziway/backend/internal/bos/cos"
	"ziway/backend/internal/bos/dos"
	"ziway/backend/internal/bos/eos"
	"ziway/backend/internal/bos/fos"
	"ziway/backend/internal/bos/gos"
	"ziway/backend/internal/bos/hos"
	"ziway/backend/internal/bos/ios"
	"ziway/backend/internal/bos/oos"
	"ziway/backend/internal/bos/tos"
	"ziway/backend/internal/bos/sos"
	"ziway/backend/internal/bos/vos"
	"ziway/backend/pkg/config"
	"ziway/backend/pkg/logger"
	"ziway/backend/pkg/middleware"
	"ziway/backend/pkg/response"

	"github.com/gin-gonic/gin"
)

// inProcessCaller is the P0 implementation of bos.MBSCaller.
// In P1, this will be replaced by gRPC client.
type inProcessCaller struct{}

func (c *inProcessCaller) CallMBS(mbsName, method string, payload interface{}) (interface{}, error) {
	// P0: BOS and MBS run in different processes, call via HTTP.
	// This is a placeholder — actual implementation uses HTTP proxy.
	return nil, fmt.Errorf("MBS %s method %s: use HTTP proxy in P0", mbsName, method)
}

func main() {
	v, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	env := v.GetString("app.env")
	if env == "" {
		env = "dev"
	}
	log := logger.New(env)
	defer log.Sync()

	deps := bos.Dependencies{
		Logger:    log,
		MBSCaller: &inProcessCaller{},
		MBSAddr:   v.GetString("mbs.addr"),
	}
	if deps.MBSAddr == "" {
		deps.MBSAddr = "localhost:8081" // P0 default
	}

	// ZW-ARC-017: 12 BOS modules (5 orchestration + 7 passthrough)
	// Orchestration:  cos(cms+dms), dos(dms+hms+fms), ios(ims+fms), vos(gms+oms+vms), tos(tms)
	// Passthrough:    aos(ams), eos(ems), hos(hms), sos(sms), fos(fms), gos(gms), oos(oms)
	registry := []bos.Orchestrator{
		// Orchestration BOS (P0 priority)
		cos.New(deps),
		dos.New(deps),
		ios.New(deps),
		vos.New(deps),
		tos.New(deps),
		// Passthrough BOS
		aos.New(deps),
		eos.New(deps),
		hos.New(deps),
		sos.New(deps),
		fos.New(deps),
		gos.New(deps),
		oos.New(deps),
	}

	r := gin.New()
	r.Use(middleware.CORS(), middleware.TraceID(), middleware.Recover(log))

	r.GET("/health", func(c *gin.Context) {
		names := make([]string, len(registry))
		for i, o := range registry {
			names[i] = o.Name()
		}
		response.OK(c, gin.H{
			"status":      "ok",
			"service":     "ziway-bos",
			"bos_count":   len(registry),
			"bos_modules": names,
			"ref":         "ZW-ARC-017",
		})
	})

	// BOS routes under /api/v1/bos/{bos_name}/
	api := r.Group("/api/v1/bos")
	for _, o := range registry {
		o.RegisterRoutes(api.Group("/" + o.Name()))
		log.Info("BOS registered",
			zap.String("name", o.Name()),
			zap.Strings("mbs_deps", o.MBSDependencies()),
		)
	}

	port := v.GetString("server.http_port")
	if port == "" {
		port = "8082"
	}
	log.Info("ziway-bos starting",
		zap.String("port", port),
		zap.Int("bos_count", len(registry)),
	)
	if err := r.Run(":"+port); err != nil {
		log.Fatal("server failed", zap.Error(err))
	}
}
