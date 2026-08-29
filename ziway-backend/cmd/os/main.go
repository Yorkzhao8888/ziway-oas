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
	"ziway/backend/pkg/jwt"
	"ziway/backend/pkg/logger"
	"ziway/backend/pkg/middleware"
	"ziway/backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type inProcessCaller struct{}

func (c *inProcessCaller) CallMBS(mbsName, method string, payload interface{}) (interface{}, error) {
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

	// ── Security trio: public_key + rbac_model + rbac_policy ──
	// fail-closed: any missing → refuse to start.
	pubKeyPath := v.GetString("jwt.public_key_path")
	rbacModelPath := v.GetString("rbac.model_path")
	rbacPolicyPath := v.GetString("rbac.policy_path")

	var missing []string
	if pubKeyPath == "" {
		missing = append(missing, "jwt.public_key_path")
	}
	if rbacModelPath == "" {
		missing = append(missing, "rbac.model_path")
	}
	if rbacPolicyPath == "" {
		missing = append(missing, "rbac.policy_path")
	}
	if len(missing) > 0 {
		log.Fatal("security config incomplete — refusing to start (fail-closed)",
			zap.Strings("missing_fields", missing),
		)
	}

	verifier, err := jwt.NewVerifier(pubKeyPath)
	if err != nil {
		log.Fatal("JWT public key load failed — refusing to start (fail-closed)",
			zap.String("path", pubKeyPath),
			zap.Error(err),
		)
	}
	log.Info("JWT ENABLED", zap.String("public_key", pubKeyPath))

	rbacEnforcer, err := middleware.NewFileRBACEnforcer(rbacModelPath, rbacPolicyPath)
	if err != nil {
		log.Fatal("RBAC policy load failed — refusing to start (fail-closed)",
			zap.String("model", rbacModelPath),
			zap.String("policy", rbacPolicyPath),
			zap.Error(err),
		)
	}
	log.Info("RBAC ENABLED",
		zap.String("model", rbacModelPath),
		zap.String("policy", rbacPolicyPath),
	)

	// ── BOS orchestrators ──
	deps := bos.Dependencies{
		Logger:    log,
		MBSCaller: &inProcessCaller{},
		MBSAddr:   v.GetString("mbs.addr"),
	}
	if deps.MBSAddr == "" {
		deps.MBSAddr = "localhost:8081"
	}

	registry := []bos.Orchestrator{
		cos.New(deps),
		dos.New(deps),
		ios.New(deps),
		vos.New(deps),
		tos.New(deps),
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

	// /health is public — no auth required
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

	// All BOS API routes require JWT + RBAC
	api := r.Group("/api/v1/bos")
	api.Use(middleware.JWTAuth(verifier, nil, log))
	api.Use(middleware.RBAC(rbacEnforcer, log))

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
		zap.Bool("jwt_enabled", true),
		zap.Bool("rbac_enabled", true),
	)
	if err := r.Run(":"+port); err != nil {
		log.Fatal("server failed", zap.Error(err))
	}
}
