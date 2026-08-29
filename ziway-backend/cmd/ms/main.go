package main

import (
	"fmt"
	"os"

	"go.uber.org/zap"

	"ziway/backend/internal/mbs"
	"ziway/backend/internal/mbs/ams"
	"ziway/backend/internal/mbs/cms"
	"ziway/backend/internal/mbs/dms"
	"ziway/backend/internal/mbs/ems"
	"ziway/backend/internal/mbs/fms"
	"ziway/backend/internal/mbs/gms"
	"ziway/backend/internal/mbs/hms"
	"ziway/backend/internal/mbs/ims"
	"ziway/backend/internal/mbs/oms"
	"ziway/backend/internal/mbs/tms"
	"ziway/backend/internal/mbs/sms"
	"ziway/backend/internal/mbs/vms"
	"ziway/backend/pkg/config"
	"ziway/backend/pkg/db"
	"ziway/backend/pkg/eventbus"
	"ziway/backend/pkg/logger"
	"ziway/backend/pkg/middleware"
	"ziway/backend/pkg/response"

	"github.com/gin-gonic/gin"
)

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

	database, err := db.InitDB(v, log)
	if err != nil {
		log.Fatal("init db", zap.Error(err))
	}

	bus := eventbus.New()

	deps := mbs.Dependencies{
		DB:       database,
		Logger:   log,
		EventBus: bus,
	}

	// Register all 12 MBS modules
	registry := []mbs.Service{
		ams.New(deps),
		cms.New(deps),
		dms.New(deps),
		hms.New(deps),
		fms.New(deps),
		tms.New(deps),
		ems.New(deps),
		gms.New(deps),
		oms.New(deps),
		vms.New(deps),
		ims.New(deps),
		sms.New(deps),
	}

	// AutoMigrate all MBS schemas
	for _, svc := range registry {
		if err := svc.AutoMigrate(database); err != nil {
			log.Fatal("automigrate failed", zap.String("ms", svc.Name()), zap.Error(err))
		}
		log.Info("MBS registered", zap.String("name", svc.Name()), zap.String("schema", svc.Schema()))
	}

	r := gin.New()
	r.Use(middleware.CORS(), middleware.TraceID(), middleware.Recover(log))

	r.GET("/health", func(c *gin.Context) {
		names := make([]string, len(registry))
		for i, svc := range registry {
			names[i] = svc.Name()
		}
		response.OK(c, gin.H{
			"status":      "ok",
			"service":     "ziway-mbs",
			"mbs_count":   len(registry),
			"mbs_modules": names,
		})
	})

	// Register each MBS under /api/v1/{mbs_name}/
	api := r.Group("/api/v1")
	for _, svc := range registry {
		svc.RegisterRoutes(api.Group("/" + svc.Name()))
	}

	port := v.GetString("server.http_port")
	if port == "" {
		port = "8081"
	}
	log.Info("ziway-mbs starting", zap.String("port", port), zap.Int("mbs_count", len(registry)))
	if err := r.Run(":" + port); err != nil {
		log.Fatal("server failed", zap.Error(err))
	}
}
