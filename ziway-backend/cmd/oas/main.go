package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"os"
	"time"
	"ziway/backend/internal/oas"
	"ziway/backend/internal/oas/handlers"
	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/config"
	"ziway/backend/pkg/db"
	"ziway/backend/pkg/envpolicy"
	"ziway/backend/pkg/jwt"
	"ziway/backend/pkg/logger"
	"ziway/backend/pkg/model"
	"ziway/backend/pkg/ratelimit"
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

	// ===== OAS Environment (四环境系统) =====
	// OAS_ENV controls convenience capability availability (DEV/BETA/RC/PROD)
	oasEnv := envpolicy.GetOASEnv()
	log.Info("OAS environment initialized", zap.String("oas_env", oasEnv.String()))

	database, err := db.InitDB(v, log)
	if err != nil {
		log.Fatal("init db", zap.Error(err))
	}
	database.AutoMigrate(
		&oasmodel.SystemConfig{}, &oasmodel.AuditLog{}, &oasmodel.DomainRegistry{},
		&oasmodel.GovernancePolicy{}, &oasmodel.ServiceRegistry{}, &oasmodel.APIKey{},
		&oasmodel.FederationNode{}, &oasmodel.RBACPolicy{}, &oasmodel.OASUser{}, &oasmodel.OASRole{}, &oasmodel.OASUserRole{},
		&model.Organization{}, &model.UserOrganization{},
		&oasmodel.ApprovalRequest{},
		&oasmodel.OAuthClient{}, &oasmodel.OAuthAuthorizationCode{},
	)

	// 迁移现有 admin 账号的 role_code（如果为空）
	database.Model(&oasmodel.OASUser{}).Where("username = ? AND (role_code = '' OR role_code IS NULL)", "oas-ou-admin").Update("role_code", "SU")
	database.Model(&oasmodel.OASUser{}).Where("username = ? AND (role_code = '' OR role_code IS NULL)", "oas-au-admin").Update("role_code", "AU")
	database.Model(&oasmodel.OASUser{}).Where("username = ? AND (role_code = '' OR role_code IS NULL)", "oas-oam-admin").Update("role_code", "OAM")

	// Login rate limiter: 5 failures = 15 min lockout
	loginLimiter := ratelimit.NewLoginLimiter(5, 15*time.Minute)

	handlers.H = &handlers.Handlers{
		DB:                  database,
		Log:                 log,
		Cfg:                 v,
		OASEnv:              oasEnv,
		LoginLimiter:        loginLimiter,
		RegeneratePolicyCSV: oas.RegeneratePolicyCSV,
	}

	r := gin.New()
	var jwtIssuer *jwt.Issuer
	var jwtVerifier *jwt.Verifier
	var jwtPublicKey *rsa.PublicKey
	if pkPath := v.GetString("jwt.private_key_path"); pkPath != "" {
		accessTTL := v.GetDuration("jwt.access_ttl")
		if accessTTL == 0 {
			accessTTL = 15 * time.Minute
		}
		issuer, err := jwt.NewIssuer(pkPath, accessTTL, 7*24*time.Hour, v.GetString("jwt.issuer"))
		if err != nil {
			log.Fatal("init jwt issuer", zap.Error(err))
		}
		jwtIssuer = issuer
		log.Info("JWT issuer initialized", zap.String("private_key", pkPath))

		// Load public key for verification endpoints
		pubKeyPath := v.GetString("jwt.public_key_path")
		if pubKeyPath != "" {
			pubData, err := os.ReadFile(pubKeyPath)
			if err == nil {
				block, _ := pem.Decode(pubData)
				if block != nil {
					pub, err := x509.ParsePKIXPublicKey(block.Bytes)
					if err == nil {
						if rsaPub, ok := pub.(*rsa.PublicKey); ok {
							jwtPublicKey = rsaPub
							log.Info("JWT public key loaded", zap.String("path", pubKeyPath))
						}
					}
				}
			}
			// Init verifier for admin auth middleware
			verifier, err := jwt.NewVerifier(pubKeyPath)
			if err != nil {
				log.Error("failed to init JWT verifier", zap.Error(err))
			} else {
				jwtVerifier = verifier
				log.Info("JWT verifier initialized")
			}
		}
	}

	handlers.H.JWTIssuer = jwtIssuer
	handlers.H.JWTVerifier = jwtVerifier
	handlers.H.JWTPublicKey = jwtPublicKey

	oas.Register(r)

	// ===== Beta Edition: Quick Login API (一键登录) =====
	// Only available in beta edition for testing purposes
	edition := v.GetString("app.edition")
	if edition == "" {
		edition = "production"
	}

	log.Info("product edition", zap.String("edition", edition))

	oas.SeedDefaultOAuthClients(database, log, oasEnv)

	// Seed default RBAC policies if empty
	oas.SeedRBACPolicies(database, log)
	// Seed test users based on product edition
	edition = v.GetString("app.edition")
	if edition == "" {
		edition = "production"
	}
	oas.SeedTestUsers(database, log, edition)

	port := v.GetString("server.http_port")
	if port == "" {
		port = "8080"
	}
	log.Info("ziway-oas starting", zap.String("port", port))
	if err := r.Run(":" + port); err != nil {
		log.Fatal("server failed", zap.Error(err))
	}
}
