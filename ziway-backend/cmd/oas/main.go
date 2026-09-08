package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
	"ziway/backend/internal/oas"
	"ziway/backend/internal/oas/authz"
	"ziway/backend/internal/oas/handlers"
	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/envpolicy"
	"ziway/backend/pkg/model"
	"ziway/backend/pkg/password"
	"ziway/backend/pkg/ratelimit"

	"ziway/backend/pkg/config"
	"ziway/backend/pkg/db"
	"ziway/backend/pkg/jwt"
	"ziway/backend/pkg/logger"
	"ziway/backend/pkg/middleware"
	"ziway/backend/pkg/response"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// ========== OAS Models (Owner + Admin shared) ==========

// oasmodel.SystemConfig 系统配置项

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

	// Migrate existing admin accounts to have proper role_code
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
	r.Use(middleware.CORS(), middleware.TraceID(), middleware.Recover(log))

	r.GET("/health", handlers.H.Health)

	api := r.Group("/api/v1")

	// ===== Auth (public, no JWT required) =====
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

	// GET /api/v1/auth/public-key — return PEM for RS256 verification
	api.GET("/auth/public-key", handlers.H.PublicKey)

	// GET /.well-known/jwks.json — standard JWK Set endpoint
	r.GET("/.well-known/jwks.json", handlers.H.WellKnownJWKS)

	// POST /api/v1/os/:os/proxy/ams/auth/login — unified login, returns real JWT
	api.POST("/os/:os/proxy/ams/auth/login", handlers.H.ProxyAMSLogin)

	// ===== Beta Edition: Quick Login API (一键登录) =====
	// Only available in beta edition for testing purposes
	edition := v.GetString("app.edition")
	if edition == "" {
		edition = "production"
	}
	log.Info("product edition", zap.String("edition", edition))

	// ===== Quick Login & Test Accounts (DEV/BETA only) =====
	// OAS_ENV controls availability: DEV and BETA allow, RC and PROD return 404 (fail-closed)
	if envpolicy.IsQuickLoginEnabled(oasEnv) {
		// POST /api/v1/auth/quick-login — one-click login for testing
		// Request: {"role": "SU"} or {"username": "admin"}
		// Returns JWT without password verification
		api.POST("/auth/quick-login", handlers.H.QuickLogin)

		// GET /api/v1/auth/test-accounts — list available test accounts (beta only)
		api.GET("/auth/test-accounts", handlers.H.TestAccounts)
	}

	// ===== POST /api/v1/auth/dev-token — temporary token for development =====
	// Environment-aware: OAS_ENV controls availability (DEV/BETA allow, RC/PROD disabled)
	// Single source of truth: OAS_ENV (not COZE_PROJECT_ENV which may be auto-injected)
	devTokenEnv := os.Getenv("ZIWAY_DEV_TOKEN_ENABLED")
	// DEV/BETA: enabled unless explicitly set to "false"; RC/PROD: always disabled
	devTokenEnabled := envpolicy.IsDevTokenEnabled(oasEnv) && devTokenEnv != "false"
	if devTokenEnabled {
		api.POST("/auth/dev-token", handlers.H.DevToken)
		log.Info("DEV token endpoint enabled (DEV/BETA environment)", zap.String("oas_env", oasEnv.String()), zap.String("ZIWAY_DEV_TOKEN_ENABLED", devTokenEnv))
	}

	// POST /api/v1/oauth/authorize-code — Generate authorization code (called after login in OAuth flow)
	if jwtVerifier != nil {
		api.POST("/oauth/authorize-code", middleware.JWTAuth(jwtVerifier, nil, log), handlers.H.AuthorizeCode)
	}

	// ===== Owner Plane (/owner/*) — OU 权限 =====
	owner := api.Group("/owner")
	{
		// 事业场生命周期
		owner.GET("/domains", func(c *gin.Context) {
			var items []oasmodel.DomainRegistry
			database.Order("created_at DESC").Find(&items)
			response.OK(c, items)
		})
		owner.POST("/domains", func(c *gin.Context) {
			var d oasmodel.DomainRegistry
			if err := c.ShouldBindJSON(&d); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}
			database.Create(&d)
			response.Created(c, d)
		})
		owner.PUT("/domains/:id/status", func(c *gin.Context) {
			var body struct {
				Status string `json:"status"`
			}
			c.ShouldBindJSON(&body)
			database.Model(&oasmodel.DomainRegistry{}).Where("id = ?", c.Param("id")).Update("status", body.Status)
			response.OK(c, nil)
		})

		// 治理策略
		owner.GET("/policies", func(c *gin.Context) {
			var items []oasmodel.GovernancePolicy
			database.Order("created_at DESC").Find(&items)
			response.OK(c, items)
		})
		owner.POST("/policies", func(c *gin.Context) {
			var p oasmodel.GovernancePolicy
			if err := c.ShouldBindJSON(&p); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}
			database.Create(&p)
			response.Created(c, p)
		})
		owner.PUT("/policies/:id", func(c *gin.Context) {
			var p oasmodel.GovernancePolicy
			if err := database.First(&p, c.Param("id")).Error; err != nil {
				response.NotFound(c, "policy not found")
				return
			}
			c.ShouldBindJSON(&p)
			database.Save(&p)
			response.OK(c, p)
		})
	}

	// ===== OAuth 2.0 / OIDC Endpoints (OAS-CONSOLE-06) =====

	oas.SeedDefaultOAuthClients(database, log, oasEnv)
	// GET /.well-known/openid-configuration — OIDC discovery
	r.GET("/.well-known/openid-configuration", handlers.H.OpenIDConfiguration)

	// GET /oauth/jwks — JWKS endpoint
	r.GET("/oauth/jwks", handlers.H.OAuthJWKS)

	// GET /oauth/authorize — Authorization endpoint
	r.GET("/oauth/authorize", handlers.H.OAuthAuthorize)

	// POST /oauth/token — Token endpoint (exchange code for tokens)
	r.POST("/oauth/token", handlers.H.OAuthToken)

	// GET /oauth/userinfo — Userinfo endpoint
	r.GET("/oauth/userinfo", middleware.JWTAuth(jwtVerifier, nil, log), handlers.H.Userinfo)

	// ===== Admin Plane (/admin/*) — 白名单 A: OU/AU/OAM =====
	if jwtVerifier == nil {
		log.Fatal("admin routes require JWT verifier, but it is not initialized")
	}
	admin := api.Group("/admin")
	// Combined JWT + API Key authentication
	admin.Use(func(c *gin.Context) {
		// Try API Key first
		var apiKeyStr string

		// Check Authorization header (Bearer)
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			apiKeyStr = authHeader[7:]
		}

		// Check X-API-Key header
		if apiKeyStr == "" {
			apiKeyStr = c.GetHeader("X-API-Key")
		}

		// Check api_key query parameter
		if apiKeyStr == "" {
			apiKeyStr = c.Query("api_key")
		}

		if apiKeyStr != "" {
			// Extract prefix (first part before _)
			parts := strings.SplitN(apiKeyStr, "_", 3)
			if len(parts) >= 2 {
				prefix := parts[0] + "_" + parts[1]

				// Look up API key by prefix
				var key oasmodel.APIKey
				if err := database.Where("key_prefix = ?", prefix).First(&key).Error; err == nil {
					// Check status
					if key.Status == "active" {
						// Check expiry
						if key.ExpiresAt == nil || !key.ExpiresAt.Before(time.Now()) {
							// Verify key hash
							if err := bcrypt.CompareHashAndPassword([]byte(key.KeyHash), []byte(apiKeyStr)); err == nil {
								// API Key auth succeeded
								c.Set("api_key_id", key.ID)
								c.Set("api_key_name", key.KeyName)
								c.Set("api_key_scopes", key.Scopes)
								c.Set("auth_type", "api_key")
								c.Set("username", "api-key:"+key.KeyName)
								c.Next()
								return
							}
						}
					}
				}
			}
		}

		// Otherwise, try JWT
		middleware.JWTAuth(jwtVerifier, nil, log)(c)
	})
	// Allow API keys or admin role users
	admin.Use(func(c *gin.Context) {
		authType, _ := c.Get("auth_type")
		if authType == "api_key" {
			c.Next()
			return
		}

		// Check if user has admin role (SU/OU/AU)
		username, _ := c.Get("username")
		if username == nil {
			response.Unauthorized(c, "unauthorized")
			return
		}

		var user oasmodel.OASUser
		if err := database.Where("username = ?", username).First(&user).Error; err != nil {
			response.Unauthorized(c, "user not found")
			return
		}

		// Check if user has admin role (whitelist A: SU/OU/AU/OAM)
		if user.RoleCode != "SU" && user.RoleCode != "OU" && user.RoleCode != "AU" && user.RoleCode != "OAM" {
			response.Forbidden(c, "access denied: admin role required")
			return
		}

		c.Next()
	})
	{
		// ===== 治理看板 (/admin/dashboard/*) =====
		admin.GET("/dashboard/stats", func(c *gin.Context) {
			username, _ := c.Get("username")

			// 用户统计（仅 active）
			var usersTotal int64
			database.Model(&oasmodel.OASUser{}).Where("status = ?", "active").Count(&usersTotal)

			// 组织统计
			var orgsTotal int64
			database.Model(&model.Organization{}).Count(&orgsTotal)

			// 角色统计
			var rolesTotal int64
			database.Model(&oasmodel.OASRole{}).Count(&rolesTotal)

			// 域分布（基于 organizations）
			type DomainCount struct {
				Domain string `json:"domain"`
				Count  int64  `json:"count"`
			}
			// 初始化全域 0 计数
			domainDistribution := []DomainCount{
				{Domain: "T", Count: 0},
				{Domain: "H", Count: 0},
				{Domain: "Y", Count: 0},
				{Domain: "V", Count: 0},
				{Domain: "O", Count: 0},
				{Domain: "A", Count: 0},
				{Domain: "F", Count: 0},
				{Domain: "G", Count: 0},
			}
			// 查询实际分布并更新
			var actualDistribution []DomainCount
			database.Model(&model.Organization{}).
				Select("domain, COUNT(*) as count").
				Group("domain").
				Scan(&actualDistribution)
			// 更新有数据的域
			for _, actual := range actualDistribution {
				for i := range domainDistribution {
					if domainDistribution[i].Domain == actual.Domain {
						domainDistribution[i].Count = actual.Count
						break
					}
				}
			}

			// 审计摘要（仅 2admin 可见明细，OAM 只返回统计）
			isOUAU := username == "oas-ou-admin" || username == "oas-au-admin"

			result := gin.H{
				"users_total":         usersTotal,
				"orgs_total":          orgsTotal,
				"roles_total":         rolesTotal,
				"domain_distribution": domainDistribution,
			}

			if isOUAU {
				// 2admin 可见审计明细
				var recentAudits []oasmodel.AuditLog
				database.Order("created_at DESC").Limit(10).Find(&recentAudits)

				type ActionCount struct {
					Action string `json:"action"`
					Count  int64  `json:"count"`
				}
				var auditSummary []ActionCount
				database.Model(&oasmodel.AuditLog{}).
					Select("action, COUNT(*) as count").
					Group("action").
					Scan(&auditSummary)

				result["recent_audits"] = recentAudits
				result["audit_summary"] = auditSummary
			}

			response.OK(c, result)
		})

		// ===== 战略审批工作台 (/admin/approvals) =====
		admin.GET("/approvals", func(c *gin.Context) {
			username, _ := c.Get("username")
			usernameStr, _ := username.(string)

			// 白名单 B: SU/OU/AU
			if !authz.IsInAdminWhitelistB(database, usernameStr) {
				response.Forbidden(c, "only SU/OU/AU admin can view approvals")
				return
			}

			var approvals []oasmodel.ApprovalRequest
			database.Order("created_at DESC").Find(&approvals)
			response.OK(c, approvals)
		})

		admin.POST("/approvals", func(c *gin.Context) {
			username, _ := c.Get("username")
			userID, _ := c.Get("user_id")
			domain, _ := c.Get("domain")
			oasEnv, _ := c.Get("oas_env")

			// 类型断言
			usernameStr, _ := username.(string)
			userIDStr, _ := userID.(string)
			domainStr, _ := domain.(string)
			oasEnvStr, _ := oasEnv.(string)

			// 白名单 B: SU/OU/AU
			if !authz.IsInAdminWhitelistB(database, usernameStr) {
				response.Forbidden(c, "only SU/OU/AU admin can create approvals")
				return
			}

			var req struct {
				Title       string `json:"title" binding:"required"`
				Description string `json:"description"`
				Type        string `json:"type" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, err.Error())
				return
			}

			// 验证类型
			validTypes := map[string]bool{
				"high_privilege": true,
				"org_delete":     true,
				"key_operation":  true,
				"federation":     true,
			}
			if !validTypes[req.Type] {
				response.BadRequest(c, "invalid approval type")
				return
			}

			approval := oasmodel.ApprovalRequest{
				Title:       req.Title,
				Description: req.Description,
				Type:        req.Type,
				Status:      "pending",
				RequesterID: userIDStr,
				Domain:      domainStr,
				Environment: oasEnvStr,
			}

			if err := database.Create(&approval).Error; err != nil {
				response.InternalError(c, "create approval failed: "+err.Error())
				return
			}

			// 审计日志
			database.Create(&oasmodel.AuditLog{
				UserID:      usernameStr,
				UserName:    usernameStr,
				Plane:       "admin",
				Action:      "governance.approval.create",
				Resource:    "approval_request",
				ResourceID:  fmt.Sprintf("%d", approval.ID),
				Detail:      fmt.Sprintf("type=%s, title=%s", approval.Type, approval.Title),
				IP:          c.ClientIP(),
				UserAgent:   c.Request.UserAgent(),
				Environment: oasEnvStr,
				Domain:      domainStr,
			})

			response.Created(c, approval)
		})

		admin.PUT("/approvals/:id/approve", func(c *gin.Context) {
			username, _ := c.Get("username")
			userID, _ := c.Get("user_id")
			domain, _ := c.Get("domain")
			oasEnv, _ := c.Get("oas_env")

			// 类型断言
			usernameStr, _ := username.(string)
			userIDStr, _ := userID.(string)
			domainStr, _ := domain.(string)
			oasEnvStr, _ := oasEnv.(string)

			// 白名单 B: SU/OU/AU
			if !authz.IsInAdminWhitelistB(database, usernameStr) {
				response.Forbidden(c, "only SU/OU/AU admin can approve")
				return
			}

			id, _ := parseUint(c.Param("id"))
			var approval oasmodel.ApprovalRequest
			if err := database.First(&approval, id).Error; err != nil {
				response.NotFound(c, "approval not found")
				return
			}

			if approval.Status != "pending" {
				response.BadRequest(c, "approval is not pending")
				return
			}

			now := time.Now()
			approval.Status = "approved"
			approval.ApproverID = ptrString(userIDStr)
			approval.ApprovedAt = &now

			if err := database.Save(&approval).Error; err != nil {
				response.InternalError(c, "approve failed: "+err.Error())
				return
			}

			// 审计日志
			database.Create(&oasmodel.AuditLog{
				UserID:      usernameStr,
				UserName:    usernameStr,
				Plane:       "admin",
				Action:      "governance.approval.approve",
				Resource:    "approval_request",
				ResourceID:  fmt.Sprintf("%d", approval.ID),
				Detail:      fmt.Sprintf("title=%s", approval.Title),
				IP:          c.ClientIP(),
				UserAgent:   c.Request.UserAgent(),
				Environment: oasEnvStr,
				Domain:      domainStr,
			})

			response.OK(c, approval)
		})

		admin.PUT("/approvals/:id/reject", func(c *gin.Context) {
			username, _ := c.Get("username")
			userID, _ := c.Get("user_id")
			domain, _ := c.Get("domain")
			oasEnv, _ := c.Get("oas_env")

			// 类型断言
			usernameStr, _ := username.(string)
			userIDStr, _ := userID.(string)
			domainStr, _ := domain.(string)
			oasEnvStr, _ := oasEnv.(string)

			// 仅 OU/AU 可拒绝
			if usernameStr != "oas-ou-admin" && usernameStr != "oas-au-admin" {
				response.Forbidden(c, "only OU/AU admin can reject")
				return
			}

			id, _ := parseUint(c.Param("id"))
			var approval oasmodel.ApprovalRequest
			if err := database.First(&approval, id).Error; err != nil {
				response.NotFound(c, "approval not found")
				return
			}

			if approval.Status != "pending" {
				response.BadRequest(c, "approval is not pending")
				return
			}

			var req struct {
				Notes string `json:"notes"`
			}
			c.ShouldBindJSON(&req)

			approval.Status = "rejected"
			approval.ApproverID = ptrString(userIDStr)
			approval.Notes = req.Notes

			if err := database.Save(&approval).Error; err != nil {
				response.InternalError(c, "reject failed: "+err.Error())
				return
			}

			// 审计日志
			database.Create(&oasmodel.AuditLog{
				UserID:      usernameStr,
				UserName:    usernameStr,
				Plane:       "admin",
				Action:      "governance.approval.reject",
				Resource:    "approval_request",
				ResourceID:  fmt.Sprintf("%d", approval.ID),
				Detail:      fmt.Sprintf("title=%s, notes=%s", approval.Title, req.Notes),
				IP:          c.ClientIP(),
				UserAgent:   c.Request.UserAgent(),
				Environment: oasEnvStr,
				Domain:      domainStr,
			})

			response.OK(c, approval)
		})

		admin.PUT("/approvals/:id/execute", func(c *gin.Context) {
			username, _ := c.Get("username")
			domain, _ := c.Get("domain")
			oasEnv, _ := c.Get("oas_env")

			// 类型断言
			usernameStr, _ := username.(string)
			domainStr, _ := domain.(string)
			oasEnvStr, _ := oasEnv.(string)

			// 仅 OU/AU 可标记执行
			if usernameStr != "oas-ou-admin" && usernameStr != "oas-au-admin" {
				response.Forbidden(c, "only OU/AU admin can execute")
				return
			}

			id, _ := parseUint(c.Param("id"))
			var approval oasmodel.ApprovalRequest
			if err := database.First(&approval, id).Error; err != nil {
				response.NotFound(c, "approval not found")
				return
			}

			if approval.Status != "approved" {
				response.BadRequest(c, "approval is not approved")
				return
			}

			now := time.Now()
			approval.Status = "executed"
			approval.ExecutedAt = &now

			if err := database.Save(&approval).Error; err != nil {
				response.InternalError(c, "execute failed: "+err.Error())
				return
			}

			// 审计日志
			database.Create(&oasmodel.AuditLog{
				UserID:      usernameStr,
				UserName:    usernameStr,
				Plane:       "admin",
				Action:      "governance.approval.execute",
				Resource:    "approval_request",
				ResourceID:  fmt.Sprintf("%d", approval.ID),
				Detail:      fmt.Sprintf("title=%s", approval.Title),
				IP:          c.ClientIP(),
				UserAgent:   c.Request.UserAgent(),
				Environment: oasEnvStr,
				Domain:      domainStr,
			})

			response.OK(c, approval)
		})

		// 删除审批单（仅 OU/AU）
		admin.DELETE("/approvals/:id", func(c *gin.Context) {
			username, _ := c.Get("username")
			domain, _ := c.Get("domain")
			oasEnv, _ := c.Get("oas_env")

			// 类型断言
			usernameStr, _ := username.(string)
			domainStr, _ := domain.(string)
			oasEnvStr, _ := oasEnv.(string)

			// 白名单 B: SU/OU/AU
			if !authz.IsInAdminWhitelistB(database, usernameStr) {
				response.Forbidden(c, "only SU/OU/AU admin can delete approvals")
				return
			}

			id, _ := parseUint(c.Param("id"))
			var approval oasmodel.ApprovalRequest
			if err := database.First(&approval, id).Error; err != nil {
				response.NotFound(c, "approval not found")
				return
			}

			if err := database.Delete(&approval).Error; err != nil {
				response.InternalError(c, "delete failed: "+err.Error())
				return
			}

			// 审计日志
			database.Create(&oasmodel.AuditLog{
				UserID:      usernameStr,
				UserName:    usernameStr,
				Plane:       "admin",
				Action:      "governance.approval.delete",
				Resource:    "approval_request",
				ResourceID:  fmt.Sprintf("%d", approval.ID),
				Detail:      fmt.Sprintf("title=%s, status=%s", approval.Title, approval.Status),
				IP:          c.ClientIP(),
				UserAgent:   c.Request.UserAgent(),
				Environment: oasEnvStr,
				Domain:      domainStr,
			})

			response.OK(c, gin.H{"message": "approval deleted"})
		})

		// ===== 所有权视图 (/admin/ownership) =====
		admin.GET("/ownership/matrix", func(c *gin.Context) {
			// 获取域注册信息
			var domains []oasmodel.DomainRegistry
			database.Order("domain_code").Find(&domains)

			// 获取服务注册信息
			var services []oasmodel.ServiceRegistry
			database.Order("service_name").Find(&services)

			// 构建所有权矩阵
			matrix := make(map[string]interface{})

			// 域所有权
			domainOwnership := make([]map[string]interface{}, 0)
			for _, d := range domains {
				domainOwnership = append(domainOwnership, map[string]interface{}{
					"domain_code":   d.DomainCode,
					"domain_name":   d.DomainName,
					"bos_name":      d.BOSName,
					"owner_user_id": d.OwnerUserID,
					"status":        d.Status,
				})
			}
			matrix["domains"] = domainOwnership

			// 服务归属
			serviceOwnership := make([]map[string]interface{}, 0)
			for _, s := range services {
				// 尝试从 metadata 中提取域信息
				domain := ""
				if s.Metadata != "" {
					// 简单解析 metadata JSON（如果存在）
					var meta map[string]interface{}
					if err := json.Unmarshal([]byte(s.Metadata), &meta); err == nil {
						if d, ok := meta["domain"].(string); ok {
							domain = d
						}
					}
				}

				serviceOwnership = append(serviceOwnership, map[string]interface{}{
					"service_name": s.ServiceName,
					"service_type": s.ServiceType,
					"version":      s.Version,
					"endpoint":     s.Endpoint,
					"status":       s.Status,
					"domain":       domain,
				})
			}
			matrix["services"] = serviceOwnership

			// 如果无数据，返回默认框架
			if len(domains) == 0 && len(services) == 0 {
				matrix["note"] = "暂无注册数据，以下为默认映射框架"
				matrix["default_mapping"] = []map[string]interface{}{
					{"domain": "T", "name": "技术域", "owner": "TAM", "description": "技术研发支撑"},
					{"domain": "H", "name": "人资云", "owner": "HAM", "description": "人事管理"},
					{"domain": "Y", "name": "智场域", "owner": "YAM", "description": "智场运营"},
					{"domain": "V", "name": "经营域", "owner": "VAM", "description": "经营分析"},
					{"domain": "O", "name": "组织域", "owner": "OAM", "description": "组织管理"},
					{"domain": "A", "name": "行政域", "owner": "AU-admin", "description": "行政管理"},
					{"domain": "F", "name": "财务域", "owner": "FAM", "description": "财务管理"},
					{"domain": "G", "name": "商务域", "owner": "GAM", "description": "商务管理"},
				}
			}

			response.OK(c, matrix)
		})

		// ===== Admin 账号管理（2b-1）=====
		// 列表（白名单 B: SU/OU/AU）
		admin.GET("/admin-accounts", func(c *gin.Context) {
			username, _ := c.Get("username")
			usernameStr, _ := username.(string)

			// Check if it's an API key
			authType, _ := c.Get("auth_type")
			if authType != "api_key" {
				// 白名单 B: SU/OU/AU
				if !authz.IsInAdminWhitelistB(database, usernameStr) {
					response.Forbidden(c, "only SU/OU/AU admin can manage admin accounts")
					return
				}
			}

			var admins []oasmodel.OASUser
			// OAS-CONSOLE-08: 包含所有治理角色：OU/AU/SU/OAM
			database.Where("role_code IN ?", []string{"OU", "AU", "SU", "OAM"}).Find(&admins)
			response.OK(c, admins)
		})

		// 创建 admin 账号（仅 OU/AU）
		admin.POST("/admin-accounts", func(c *gin.Context) {
			username, _ := c.Get("username")
			domain, _ := c.Get("domain")
			oasEnv, _ := c.Get("oas_env")

			usernameStr, _ := username.(string)
			domainStr, _ := domain.(string)
			oasEnvStr, _ := oasEnv.(string)

			// Check if it's an API key or whitelist B
			authType, _ := c.Get("auth_type")
			if authType != "api_key" && !authz.IsInAdminWhitelistB(database, usernameStr) {
				response.Forbidden(c, "only OU/AU admin can create admin accounts")
				return
			}

			var req struct {
				Username    string `json:"username" binding:"required"`
				Password    string `json:"password" binding:"required"`
				DisplayName string `json:"display_name"`
				RoleCode    string `json:"role_code" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, "invalid request: "+err.Error())
				return
			}

			// 仅允许 OU/AU 角色
			if req.RoleCode != "OU" && req.RoleCode != "AU" {
				response.BadRequest(c, "role_code must be OU or AU")
				return
			}

			// 检查用户名是否已存在
			var count int64
			database.Model(&oasmodel.OASUser{}).Where("username = ?", req.Username).Count(&count)
			if count > 0 {
				response.BadRequest(c, "username already exists")
				return
			}

			// 生成 user_code
			userCode := fmt.Sprintf("XHPZ#%s-%d", req.RoleCode, time.Now().UnixNano()%100000000)

			// 密码哈希
			passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err != nil {
				response.InternalError(c, "password hash failed: "+err.Error())
				return
			}

			user := oasmodel.OASUser{
				Username:     req.Username,
				UserCode:     userCode,
				DisplayName:  req.DisplayName,
				PasswordHash: string(passwordHash),
				RoleCode:     req.RoleCode,
				Status:       "active",
				Domain:       domainStr,
			}

			if err := database.Create(&user).Error; err != nil {
				response.InternalError(c, "create failed: "+err.Error())
				return
			}

			// 审计日志
			database.Create(&oasmodel.AuditLog{
				UserID:      usernameStr,
				UserName:    usernameStr,
				Plane:       "admin",
				Action:      "admin.account.create",
				Resource:    "user",
				ResourceID:  fmt.Sprintf("%d", user.ID),
				Detail:      fmt.Sprintf("username=%s, role=%s", user.Username, user.RoleCode),
				IP:          c.ClientIP(),
				UserAgent:   c.Request.UserAgent(),
				Environment: oasEnvStr,
				Domain:      domainStr,
			})

			response.OK(c, user)
		})

		// 禁用 admin 账号（仅 OU/AU）
		admin.PUT("/admin-accounts/:id/disable", func(c *gin.Context) {
			username, _ := c.Get("username")
			domain, _ := c.Get("domain")
			oasEnv, _ := c.Get("oas_env")

			usernameStr, _ := username.(string)
			domainStr, _ := domain.(string)
			oasEnvStr, _ := oasEnv.(string)

			if usernameStr != "oas-ou-admin" && usernameStr != "oas-au-admin" {
				response.Forbidden(c, "only OU/AU admin can disable admin accounts")
				return
			}

			id, _ := parseUint(c.Param("id"))
			var user oasmodel.OASUser
			if err := database.First(&user, id).Error; err != nil {
				response.NotFound(c, "user not found")
				return
			}

			// 只能禁用 OU/AU 账号
			if user.RoleCode != "OU" && user.RoleCode != "AU" {
				response.BadRequest(c, "can only disable OU/AU admin accounts")
				return
			}

			user.Status = "disabled"
			if err := database.Save(&user).Error; err != nil {
				response.InternalError(c, "disable failed: "+err.Error())
				return
			}

			// 审计日志
			database.Create(&oasmodel.AuditLog{
				UserID:      usernameStr,
				UserName:    usernameStr,
				Plane:       "admin",
				Action:      "admin.account.disable",
				Resource:    "user",
				ResourceID:  fmt.Sprintf("%d", user.ID),
				Detail:      fmt.Sprintf("username=%s, role=%s", user.Username, user.RoleCode),
				IP:          c.ClientIP(),
				UserAgent:   c.Request.UserAgent(),
				Environment: oasEnvStr,
				Domain:      domainStr,
			})

			response.OK(c, user)
		})

		// 启用 admin 账号（仅 OU/AU）
		admin.PUT("/admin-accounts/:id/enable", func(c *gin.Context) {
			username, _ := c.Get("username")
			domain, _ := c.Get("domain")
			oasEnv, _ := c.Get("oas_env")

			usernameStr, _ := username.(string)
			domainStr, _ := domain.(string)
			oasEnvStr, _ := oasEnv.(string)

			if usernameStr != "oas-ou-admin" && usernameStr != "oas-au-admin" {
				response.Forbidden(c, "only OU/AU admin can enable admin accounts")
				return
			}

			id, _ := parseUint(c.Param("id"))
			var user oasmodel.OASUser
			if err := database.First(&user, id).Error; err != nil {
				response.NotFound(c, "user not found")
				return
			}

			if user.RoleCode != "OU" && user.RoleCode != "AU" {
				response.BadRequest(c, "can only enable OU/AU admin accounts")
				return
			}

			user.Status = "active"
			if err := database.Save(&user).Error; err != nil {
				response.InternalError(c, "enable failed: "+err.Error())
				return
			}

			// 审计日志
			database.Create(&oasmodel.AuditLog{
				UserID:      usernameStr,
				UserName:    usernameStr,
				Plane:       "admin",
				Action:      "admin.account.enable",
				Resource:    "user",
				ResourceID:  fmt.Sprintf("%d", user.ID),
				Detail:      fmt.Sprintf("username=%s, role=%s", user.Username, user.RoleCode),
				IP:          c.ClientIP(),
				UserAgent:   c.Request.UserAgent(),
				Environment: oasEnvStr,
				Domain:      domainStr,
			})

			response.OK(c, user)
		})

		// 重置密码（仅 OU/AU）
		admin.PUT("/admin-accounts/:id/reset-password", func(c *gin.Context) {
			username, _ := c.Get("username")
			domain, _ := c.Get("domain")
			oasEnv, _ := c.Get("oas_env")

			usernameStr, _ := username.(string)
			domainStr, _ := domain.(string)
			oasEnvStr, _ := oasEnv.(string)

			if usernameStr != "oas-ou-admin" && usernameStr != "oas-au-admin" {
				response.Forbidden(c, "only OU/AU admin can reset passwords")
				return
			}

			id, _ := parseUint(c.Param("id"))
			var user oasmodel.OASUser
			if err := database.First(&user, id).Error; err != nil {
				response.NotFound(c, "user not found")
				return
			}

			if user.RoleCode != "OU" && user.RoleCode != "AU" {
				response.BadRequest(c, "can only reset OU/AU admin passwords")
				return
			}

			var req struct {
				NewPassword string `json:"new_password" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, "invalid request: "+err.Error())
				return
			}

			passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
			if err != nil {
				response.InternalError(c, "password hash failed: "+err.Error())
				return
			}

			user.PasswordHash = string(passwordHash)
			if err := database.Save(&user).Error; err != nil {
				response.InternalError(c, "reset password failed: "+err.Error())
				return
			}

			// 审计日志
			database.Create(&oasmodel.AuditLog{
				UserID:      usernameStr,
				UserName:    usernameStr,
				Plane:       "admin",
				Action:      "admin.account.reset_password",
				Resource:    "user",
				ResourceID:  fmt.Sprintf("%d", user.ID),
				Detail:      fmt.Sprintf("username=%s, role=%s", user.Username, user.RoleCode),
				IP:          c.ClientIP(),
				UserAgent:   c.Request.UserAgent(),
				Environment: oasEnvStr,
				Domain:      domainStr,
			})

			response.OK(c, gin.H{"message": "password reset successfully"})
		})

		// 系统配置只读面板（白名单 B: SU/OU/AU）
		admin.GET("/system-config", func(c *gin.Context) {
			username, _ := c.Get("username")
			usernameStr, _ := username.(string)

			// 白名单 B: SU/OU/AU
			if !authz.IsInAdminWhitelistB(database, usernameStr) {
				response.Forbidden(c, "only SU/OU/AU admin can view system config")
				return
			}

			// 读取环境变量（非敏感项）
			appEnv := os.Getenv("APP_ENV")
			if appEnv == "" {
				appEnv = "dev"
			}
			oasEnv := os.Getenv("OAS_ENV")
			if oasEnv == "" {
				oasEnv = "DEV"
			}
			dbDriver := os.Getenv("ZIWAY_DATABASE_DRIVER")
			if dbDriver == "" {
				dbDriver = "sqlite"
			}

			// 构建配置信息（不暴露敏感项）
			config := gin.H{
				"app_env":    appEnv,
				"oas_env":    oasEnv,
				"db_driver":  dbDriver,
				"go_version": runtime.Version(),
				"build_time": "2026-09-08", // 可改为实际构建时间
				"git_commit": "fa6cb5d",    // 可改为实际 commit
			}

			response.OK(c, config)
		})

		// 系统配置
		admin.GET("/configs", func(c *gin.Context) {
			var items []oasmodel.SystemConfig
			database.Order("category, key").Find(&items)
			response.OK(c, items)
		})
		admin.PUT("/configs/:key", func(c *gin.Context) {
			var cfg oasmodel.SystemConfig
			if err := database.Where("key = ?", c.Param("key")).First(&cfg).Error; err != nil {
				cfg.Key = c.Param("key")
			}
			c.ShouldBindJSON(&cfg)
			database.Save(&cfg)
			response.OK(c, cfg)
		})

		// 服务注册
		admin.GET("/services", func(c *gin.Context) {
			var items []oasmodel.ServiceRegistry
			database.Order("service_name").Find(&items)
			response.OK(c, items)
		})
		admin.POST("/services", func(c *gin.Context) {
			var s oasmodel.ServiceRegistry
			if err := c.ShouldBindJSON(&s); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}
			s.RegisteredAt = time.Now()
			database.Create(&s)
			response.Created(c, s)
		})
		admin.PUT("/services/:id/heartbeat", func(c *gin.Context) {
			now := time.Now()
			database.Model(&oasmodel.ServiceRegistry{}).Where("id = ?", c.Param("id")).Updates(map[string]interface{}{
				"status":       "healthy",
				"last_seen_at": &now,
			})
			response.OK(c, nil)
		})

		// API密钥
		// API Key 全生命周期管理 — 白名单 B (OU/AU)
		admin.GET("/api-keys", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}
			var items []oasmodel.APIKey
			database.Order("created_at DESC").Find(&items)
			response.OK(c, items)
		})

		admin.GET("/api-keys/:id", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}
			id := c.Param("id")
			var k oasmodel.APIKey
			if err := database.First(&k, id).Error; err != nil {
				response.NotFound(c, "key not found")
				return
			}
			response.OK(c, k)
		})

		admin.POST("/api-keys", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			var req struct {
				KeyName   string     `json:"key_name"`
				Scopes    string     `json:"scopes"`
				ExpiresAt *time.Time `json:"expires_at"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}

			// Generate API key: prefix + random part
			prefix := "oas_" + fmt.Sprintf("%d", time.Now().UnixNano()%10000)
			randomPart := fmt.Sprintf("%x", time.Now().UnixNano()) + fmt.Sprintf("%x", big.NewInt(time.Now().UnixNano()).Int64())
			fullKey := prefix + "_" + randomPart[:32]

			// Hash the key
			hash, err := bcrypt.GenerateFromPassword([]byte(fullKey), bcrypt.DefaultCost)
			if err != nil {
				response.InternalError(c, "failed to hash key")
				return
			}

			k := oasmodel.APIKey{
				KeyName:   req.KeyName,
				KeyPrefix: prefix,
				KeyHash:   string(hash),
				Scopes:    req.Scopes,
				ExpiresAt: req.ExpiresAt,
				Status:    "active",
				CreatedBy: username.(string),
			}
			database.Create(&k)

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "governance.apikey.create",
				Resource:   "apikey",
				ResourceID: fmt.Sprintf("%d", k.ID),
				Detail:     fmt.Sprintf("created api key: %s", req.KeyName),
				Domain:     "OAS",
			})

			// Return full key only once
			response.Created(c, gin.H{
				"id":         k.ID,
				"key_name":   k.KeyName,
				"key_prefix": k.KeyPrefix,
				"full_key":   fullKey,
				"scopes":     k.Scopes,
				"expires_at": k.ExpiresAt,
				"status":     k.Status,
				"message":    "Save this key now. You won't be able to see it again.",
			})
		})

		admin.PUT("/api-keys/:id/rotate", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var oldKey oasmodel.APIKey
			if err := database.First(&oldKey, id).Error; err != nil {
				response.NotFound(c, "key not found")
				return
			}

			if oldKey.Status != "active" {
				response.BadRequest(c, "can only rotate active keys")
				return
			}

			// Generate new key
			prefix := "oas_" + fmt.Sprintf("%d", time.Now().UnixNano()%10000)
			randomPart := fmt.Sprintf("%x", time.Now().UnixNano()) + fmt.Sprintf("%x", big.NewInt(time.Now().UnixNano()).Int64())
			fullKey := prefix + "_" + randomPart[:32]

			hash, err := bcrypt.GenerateFromPassword([]byte(fullKey), bcrypt.DefaultCost)
			if err != nil {
				response.InternalError(c, "failed to hash key")
				return
			}

			// Atomic rotation: disable old key + create new key
			newKey := oasmodel.APIKey{
				KeyName:   oldKey.KeyName + " (rotated)",
				KeyPrefix: prefix,
				KeyHash:   string(hash),
				Scopes:    oldKey.Scopes,
				ExpiresAt: oldKey.ExpiresAt,
				Status:    "active",
				CreatedBy: username.(string),
			}

			// Transaction for atomicity
			tx := database.Begin()
			if err := tx.Model(&oasmodel.APIKey{}).Where("id = ?", oldKey.ID).Update("status", "disabled").Error; err != nil {
				tx.Rollback()
				response.InternalError(c, "failed to disable old key")
				return
			}
			if err := tx.Create(&newKey).Error; err != nil {
				tx.Rollback()
				response.InternalError(c, "failed to create new key")
				return
			}
			tx.Commit()

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "governance.apikey.rotate",
				Resource:   "apikey",
				ResourceID: fmt.Sprintf("%d", newKey.ID),
				Detail:     fmt.Sprintf("rotated api key from id=%d to id=%d", oldKey.ID, newKey.ID),
				Domain:     "OAS",
			})

			response.OK(c, gin.H{
				"id":         newKey.ID,
				"key_name":   newKey.KeyName,
				"key_prefix": newKey.KeyPrefix,
				"full_key":   fullKey,
				"scopes":     newKey.Scopes,
				"expires_at": newKey.ExpiresAt,
				"status":     newKey.Status,
				"old_key_id": oldKey.ID,
				"message":    "Key rotated. Old key disabled. Save new key now.",
			})
		})

		admin.PUT("/api-keys/:id/disable", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var k oasmodel.APIKey
			if err := database.First(&k, id).Error; err != nil {
				response.NotFound(c, "key not found")
				return
			}

			database.Model(&k).Update("status", "disabled")

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "governance.apikey.disable",
				Resource:   "apikey",
				ResourceID: id,
				Detail:     fmt.Sprintf("disabled api key: %s", k.KeyName),
				Domain:     "OAS",
			})

			response.OK(c, gin.H{"message": "key disabled"})
		})

		admin.PUT("/api-keys/:id/enable", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var k oasmodel.APIKey
			if err := database.First(&k, id).Error; err != nil {
				response.NotFound(c, "key not found")
				return
			}

			// Check expiry
			if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
				response.BadRequest(c, "cannot enable expired key")
				return
			}

			database.Model(&k).Update("status", "active")

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "governance.apikey.enable",
				Resource:   "apikey",
				ResourceID: id,
				Detail:     fmt.Sprintf("enabled api key: %s", k.KeyName),
				Domain:     "OAS",
			})

			response.OK(c, gin.H{"message": "key enabled"})
		})

		admin.DELETE("/api-keys/:id", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var k oasmodel.APIKey
			if err := database.First(&k, id).Error; err != nil {
				response.NotFound(c, "key not found")
				return
			}

			database.Delete(&k)

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "governance.apikey.delete",
				Resource:   "apikey",
				ResourceID: id,
				Detail:     fmt.Sprintf("deleted api key: %s", k.KeyName),
				Domain:     "OAS",
			})

			response.OK(c, gin.H{"message": "key deleted"})
		})

		// Federation Node 联邦节点管理 — 白名单 B (OU/AU)
		admin.GET("/federation-nodes", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}
			var items []oasmodel.FederationNode
			database.Order("created_at DESC").Find(&items)
			response.OK(c, items)
		})

		admin.GET("/federation-nodes/:id", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}
			id := c.Param("id")
			var node oasmodel.FederationNode
			if err := database.First(&node, id).Error; err != nil {
				response.NotFound(c, "node not found")
				return
			}
			response.OK(c, node)
		})

		admin.POST("/federation-nodes", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			var node oasmodel.FederationNode
			if err := c.ShouldBindJSON(&node); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}

			// Validate trust level
			if node.TrustLevel != "basic" && node.TrustLevel != "standard" && node.TrustLevel != "full" {
				response.BadRequest(c, "trust_level must be basic, standard, or full")
				return
			}

			node.CreatedBy = username.(string)
			node.Status = "active"
			database.Create(&node)

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "governance.federation.create",
				Resource:   "federation_node",
				ResourceID: fmt.Sprintf("%d", node.ID),
				Detail:     fmt.Sprintf("registered federation node: %s (trust: %s)", node.NodeName, node.TrustLevel),
				Domain:     "OAS",
			})

			response.Created(c, node)
		})

		admin.PUT("/federation-nodes/:id", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var node oasmodel.FederationNode
			if err := database.First(&node, id).Error; err != nil {
				response.NotFound(c, "node not found")
				return
			}

			var req struct {
				NodeName     string `json:"node_name"`
				TrustLevel   string `json:"trust_level"`
				PublicKey    string `json:"public_key"`
				Endpoint     string `json:"endpoint"`
				Capabilities string `json:"capabilities"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}

			// Validate trust level
			if req.TrustLevel != "" && req.TrustLevel != "basic" && req.TrustLevel != "standard" && req.TrustLevel != "full" {
				response.BadRequest(c, "trust_level must be basic, standard, or full")
				return
			}

			updates := map[string]interface{}{}
			if req.NodeName != "" {
				updates["node_name"] = req.NodeName
			}
			if req.TrustLevel != "" {
				updates["trust_level"] = req.TrustLevel
			}
			if req.PublicKey != "" {
				updates["public_key"] = req.PublicKey
			}
			if req.Endpoint != "" {
				updates["endpoint"] = req.Endpoint
			}
			if req.Capabilities != "" {
				updates["capabilities"] = req.Capabilities
			}

			database.Model(&node).Updates(updates)

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "governance.federation.update",
				Resource:   "federation_node",
				ResourceID: id,
				Detail:     fmt.Sprintf("updated federation node: %s", node.NodeName),
				Domain:     "OAS",
			})

			database.First(&node, id)
			response.OK(c, node)
		})

		admin.PUT("/federation-nodes/:id/suspend", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var node oasmodel.FederationNode
			if err := database.First(&node, id).Error; err != nil {
				response.NotFound(c, "node not found")
				return
			}

			database.Model(&node).Update("status", "suspended")

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "governance.federation.suspend",
				Resource:   "federation_node",
				ResourceID: id,
				Detail:     fmt.Sprintf("suspended federation node: %s", node.NodeName),
				Domain:     "OAS",
			})

			response.OK(c, gin.H{"message": "node suspended"})
		})

		admin.PUT("/federation-nodes/:id/activate", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var node oasmodel.FederationNode
			if err := database.First(&node, id).Error; err != nil {
				response.NotFound(c, "node not found")
				return
			}

			database.Model(&node).Update("status", "active")

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "governance.federation.activate",
				Resource:   "federation_node",
				ResourceID: id,
				Detail:     fmt.Sprintf("activated federation node: %s", node.NodeName),
				Domain:     "OAS",
			})

			response.OK(c, gin.H{"message": "node activated"})
		})

		admin.PUT("/federation-nodes/:id/trust", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var node oasmodel.FederationNode
			if err := database.First(&node, id).Error; err != nil {
				response.NotFound(c, "node not found")
				return
			}

			var req struct {
				TrustLevel string `json:"trust_level" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, "trust_level required")
				return
			}

			// Validate trust level
			if req.TrustLevel != "basic" && req.TrustLevel != "standard" && req.TrustLevel != "full" {
				response.BadRequest(c, "trust_level must be basic, standard, or full")
				return
			}

			oldTrust := node.TrustLevel
			database.Model(&node).Update("trust_level", req.TrustLevel)

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "governance.federation.trust",
				Resource:   "federation_node",
				ResourceID: id,
				Detail:     fmt.Sprintf("changed trust level from %s to %s for node: %s", oldTrust, req.TrustLevel, node.NodeName),
				Domain:     "OAS",
			})

			response.OK(c, gin.H{"message": "trust level updated", "old_trust": oldTrust, "new_trust": req.TrustLevel})
		})

		admin.DELETE("/federation-nodes/:id", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var node oasmodel.FederationNode
			if err := database.First(&node, id).Error; err != nil {
				response.NotFound(c, "node not found")
				return
			}

			database.Delete(&node)

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "governance.federation.delete",
				Resource:   "federation_node",
				ResourceID: id,
				Detail:     fmt.Sprintf("deleted federation node: %s", node.NodeName),
				Domain:     "OAS",
			})

			response.OK(c, gin.H{"message": "node deleted"})
		})

		// OAuth 客户端管理 — 白名单 B (OU/AU)
		admin.GET("/oauth-clients", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}
			var clients []oasmodel.OAuthClient
			database.Order("created_at DESC").Find(&clients)
			response.OK(c, clients)
		})

		admin.GET("/oauth-clients/:id", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}
			id := c.Param("id")
			var client oasmodel.OAuthClient
			if err := database.First(&client, id).Error; err != nil {
				response.NotFound(c, "client not found")
				return
			}
			response.OK(c, client)
		})

		admin.POST("/oauth-clients", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			var req struct {
				ClientName  string `json:"client_name"`
				RedirectURI string `json:"redirect_uri"`
				Scopes      string `json:"scopes"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}
			if req.ClientName == "" || req.RedirectURI == "" {
				response.BadRequest(c, "client_name and redirect_uri required")
				return
			}

			// Generate client_id and client_secret
			clientID := "oauth_" + fmt.Sprintf("%d", time.Now().UnixNano()%100000) + "_" + fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFFFF)
			plainSecret := "ocs_" + fmt.Sprintf("%x", time.Now().UnixNano()) + fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFFFF)
			hashedSecret, _ := password.Hash(plainSecret)

			if req.Scopes == "" {
				req.Scopes = "openid profile email"
			}

			client := oasmodel.OAuthClient{
				ClientID:     clientID,
				ClientName:   req.ClientName,
				ClientSecret: string(hashedSecret),
				RedirectURI:  req.RedirectURI,
				Scopes:       req.Scopes,
				Status:       "active",
				CreatedBy:    username.(string),
			}
			if err := database.Create(&client).Error; err != nil {
				response.InternalError(c, "failed to create client")
				return
			}

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "oauth.client.create",
				Resource:   "oauth_client",
				ResourceID: clientID,
				Detail:     fmt.Sprintf("created OAuth client: %s (%s)", req.ClientName, clientID),
				Domain:     "OAS",
			})

			// Return client with plain secret (only shown once)
			response.OK(c, gin.H{
				"client":        client,
				"client_secret": plainSecret,
				"message":       "Save the client_secret now. It will not be shown again.",
			})
		})

		admin.PUT("/oauth-clients/:id", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var client oasmodel.OAuthClient
			if err := database.First(&client, id).Error; err != nil {
				response.NotFound(c, "client not found")
				return
			}

			var req struct {
				ClientName  string `json:"client_name"`
				RedirectURI string `json:"redirect_uri"`
				Scopes      string `json:"scopes"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}

			updates := map[string]interface{}{}
			if req.ClientName != "" {
				updates["client_name"] = req.ClientName
			}
			if req.RedirectURI != "" {
				updates["redirect_uri"] = req.RedirectURI
			}
			if req.Scopes != "" {
				updates["scopes"] = req.Scopes
			}

			database.Model(&client).Updates(updates)

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "oauth.client.update",
				Resource:   "oauth_client",
				ResourceID: client.ClientID,
				Detail:     fmt.Sprintf("updated OAuth client: %s", client.ClientName),
				Domain:     "OAS",
			})

			database.First(&client, id)
			response.OK(c, client)
		})

		admin.PUT("/oauth-clients/:id/regenerate-secret", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var client oasmodel.OAuthClient
			if err := database.First(&client, id).Error; err != nil {
				response.NotFound(c, "client not found")
				return
			}

			// Generate new secret
			plainSecret := "ocs_" + fmt.Sprintf("%x", time.Now().UnixNano()) + fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFFFF)
			hashedSecret, _ := password.Hash(plainSecret)

			database.Model(&client).Update("client_secret", string(hashedSecret))

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "oauth.client.rotate_secret",
				Resource:   "oauth_client",
				ResourceID: client.ClientID,
				Detail:     fmt.Sprintf("regenerated secret for OAuth client: %s", client.ClientName),
				Domain:     "OAS",
			})

			response.OK(c, gin.H{
				"client_secret": plainSecret,
				"message":       "Save the new client_secret now. It will not be shown again.",
			})
		})

		admin.PUT("/oauth-clients/:id/suspend", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var client oasmodel.OAuthClient
			if err := database.First(&client, id).Error; err != nil {
				response.NotFound(c, "client not found")
				return
			}

			database.Model(&client).Update("status", "inactive")

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "oauth.client.suspend",
				Resource:   "oauth_client",
				ResourceID: client.ClientID,
				Detail:     fmt.Sprintf("suspended OAuth client: %s", client.ClientName),
				Domain:     "OAS",
			})

			response.OK(c, gin.H{"message": "client suspended"})
		})

		admin.PUT("/oauth-clients/:id/activate", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var client oasmodel.OAuthClient
			if err := database.First(&client, id).Error; err != nil {
				response.NotFound(c, "client not found")
				return
			}

			database.Model(&client).Update("status", "active")

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "oauth.client.activate",
				Resource:   "oauth_client",
				ResourceID: client.ClientID,
				Detail:     fmt.Sprintf("activated OAuth client: %s", client.ClientName),
				Domain:     "OAS",
			})

			response.OK(c, gin.H{"message": "client activated"})
		})

		admin.DELETE("/oauth-clients/:id", func(c *gin.Context) {
			username, _ := c.Get("username")
			if !authz.IsInAdminWhitelistB(database, username.(string)) {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}

			id := c.Param("id")
			var client oasmodel.OAuthClient
			if err := database.First(&client, id).Error; err != nil {
				response.NotFound(c, "client not found")
				return
			}

			database.Delete(&client)

			// Audit log
			database.Create(&oasmodel.AuditLog{
				UserID:     username.(string),
				UserName:   username.(string),
				Plane:      "admin",
				Action:     "oauth.client.delete",
				Resource:   "oauth_client",
				ResourceID: client.ClientID,
				Detail:     fmt.Sprintf("deleted OAuth client: %s", client.ClientName),
				Domain:     "OAS",
			})

			response.OK(c, gin.H{"message": "client deleted"})
		})

		// 审计日志路由组 — 白名单 A + XAM 角色放行，handler 内再做细粒度检查
		adminAuditLogs := api.Group("/admin/audit-logs", func(c *gin.Context) {
			// Try API Key first
			var apiKeyStr string

			// Check Authorization header (Bearer)
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				apiKeyStr = authHeader[7:]
			}

			// Check X-API-Key header
			if apiKeyStr == "" {
				apiKeyStr = c.GetHeader("X-API-Key")
			}

			// Check api_key query parameter
			if apiKeyStr == "" {
				apiKeyStr = c.Query("api_key")
			}

			if apiKeyStr != "" {
				// Extract prefix (first part before _)
				parts := strings.SplitN(apiKeyStr, "_", 3)
				if len(parts) >= 2 {
					prefix := parts[0] + "_" + parts[1]

					// Look up API key by prefix
					var key oasmodel.APIKey
					if err := database.Where("key_prefix = ?", prefix).First(&key).Error; err == nil {
						// Check status
						if key.Status == "active" {
							// Check expiry
							if key.ExpiresAt == nil || !key.ExpiresAt.Before(time.Now()) {
								// Verify key hash
								if err := bcrypt.CompareHashAndPassword([]byte(key.KeyHash), []byte(apiKeyStr)); err == nil {
									// API Key auth succeeded
									c.Set("api_key_id", key.ID)
									c.Set("api_key_name", key.KeyName)
									c.Set("api_key_scopes", key.Scopes)
									c.Set("auth_type", "api_key")
									c.Set("username", "api-key:"+key.KeyName)
									c.Next()
									return
								}
							}
						}
					}
				}
			}

			// Otherwise, try JWT
			middleware.JWTAuth(jwtVerifier, nil, log)(c)
		}, func(c *gin.Context) {
			// Allow API keys
			authType, _ := c.Get("auth_type")
			if authType == "api_key" {
				c.Next()
				return
			}

			username, _ := c.Get("username")
			rolesRaw, _ := c.Get("roles")
			var roles []string
			if rolesRaw != nil {
				if rolesSlice, ok := rolesRaw.([]string); ok {
					roles = rolesSlice
				} else if rolesStr, ok := rolesRaw.(string); ok && rolesStr != "" {
					roles = strings.Split(rolesStr, ",")
				}
			}

			isWhitelistA := authz.IsInAdminWhitelistA(database, username.(string))
			isXAM := false
			for _, role := range roles {
				if role == "TAM" || role == "HAM" || role == "YAM" || role == "VAM" {
					isXAM = true
					break
				}
			}

			if !isWhitelistA && !isXAM {
				response.Forbidden(c, "access denied")
				c.Abort()
				return
			}
			c.Next()
		})

		// 审计日志 — 白名单 B (OU/AU) 全量，OAM 不可读
		// OAS-CONSOLE-08: 移除 XAM 域过滤，仅白名单 B 可访问
		adminAuditLogs.GET("", func(c *gin.Context) {
			username, _ := c.Get("username")
			// OAS-CONSOLE-08: roles variable removed (no longer needed for XAM filtering)

			// Check access: whitelist B (OU/AU) only
			// OAS-CONSOLE-08: XAM roles no longer have access
			isWhitelistB := authz.IsInAdminWhitelistB(database, username.(string))

			if !isWhitelistB {
				response.Forbidden(c, "audit logs restricted to OU/AU admin")
				return
			}

			var items []oasmodel.AuditLog
			page, _ := parseInt(c.DefaultQuery("page", "1"))
			size, _ := parseInt(c.DefaultQuery("size", "20"))
			q := database.Model(&oasmodel.AuditLog{})

			// OAS-CONSOLE-08: XAM domain filtering removed
			// if isXAM && !isWhitelistB {
			// 	domain, _ := c.Get("domain")
			// 	if domainStr, ok := domain.(string); ok && domainStr != "" {
			// 		q = q.Where("domain = ?", domainStr)
			// 	} else {
			// 		response.OK(c, gin.H{"items": []oasmodel.AuditLog{}, "total": 0, "page": page, "size": size})
			// 		return
			// 	}
			// }

			if uid := c.Query("user_id"); uid != "" {
				q = q.Where("user_id = ?", uid)
			}
			if plane := c.Query("plane"); plane != "" {
				q = q.Where("plane = ?", plane)
			}
			var total int64
			q.Count(&total)
			q.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&items)
			response.OK(c, gin.H{"items": items, "total": total, "page": page, "size": size})
		})

		// ===== RBAC 策略管理 (/admin/rbac/*) — OAS 权威源 =====
		admin.GET("/rbac/policies", func(c *gin.Context) {
			var items []oasmodel.RBACPolicy
			q := database.Model(&oasmodel.RBACPolicy{}).Where("policy_type = ?", "rbac")
			if role := c.Query("role_type"); role != "" {
				q = q.Where("role_type = ?", role)
			}
			if subject := c.Query("subject"); subject != "" {
				q = q.Where("subject = ?", subject)
			}
			q.Order("subject, resource").Find(&items)
			response.OK(c, gin.H{"items": items, "total": len(items)})
		})

		admin.POST("/rbac/policies", func(c *gin.Context) {
			var p oasmodel.RBACPolicy
			if err := c.ShouldBindJSON(&p); err != nil {
				response.BadRequest(c, "invalid request")
				return
			}
			p.PolicyType = "rbac"
			if p.Effect == "" {
				p.Effect = "allow"
			}
			if p.Status == "" {
				p.Status = "active"
			}
			if err := database.Create(&p).Error; err != nil {
				response.BadRequest(c, "create policy failed: "+err.Error())
				return
			}
			oas.RegeneratePolicyCSV(database, log)
			response.Created(c, p)
		})

		admin.PUT("/rbac/policies/:id", func(c *gin.Context) {
			var p oasmodel.RBACPolicy
			if err := database.First(&p, c.Param("id")).Error; err != nil {
				response.NotFound(c, "policy not found")
				return
			}
			c.ShouldBindJSON(&p)
			p.ID, _ = parseUint(c.Param("id"))
			database.Save(&p)
			oas.RegeneratePolicyCSV(database, log)
			response.OK(c, p)
		})

		admin.DELETE("/rbac/policies/:id", func(c *gin.Context) {
			database.Delete(&oasmodel.RBACPolicy{}, c.Param("id"))
			oas.RegeneratePolicyCSV(database, log)
			response.OK(c, nil)
		})

		admin.POST("/rbac/sync", func(c *gin.Context) {
			oas.RegeneratePolicyCSV(database, log)
			response.OK(c, gin.H{"message": "policy CSV regenerated"})
		})
	}

	// Seed default RBAC policies if empty
	oas.SeedRBACPolicies(database, log)
	// Seed test users based on product edition
	edition = v.GetString("app.edition")
	if edition == "" {
		edition = "production"
	}
	oas.SeedTestUsers(database, log, edition)

	// ===== Root Path (GET /) — Redirect to login or admin overview =====
	r.GET("/", func(c *gin.Context) {
		if jwtVerifier == nil {
			// No JWT verifier, redirect to login
			c.Redirect(302, "/login?redirect=/")
			return
		}
		// Try to get JWT from query param or Authorization header
		tokenStr := c.Query("token")
		if tokenStr == "" {
			auth := c.GetHeader("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				tokenStr = auth[7:]
			}
		}
		if tokenStr == "" {
			// No JWT, redirect to login
			c.Redirect(302, "/login?redirect=/")
			return
		}
		// Verify JWT
		_, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			// Invalid JWT, redirect to login
			c.Redirect(302, "/login?redirect=/")
			return
		}
		// Valid JWT, redirect to admin overview
		c.Redirect(302, "/admin/overview?token="+tokenStr)
	})

	// ===== Login Page (GET /login) =====
	r.GET("/login", func(c *gin.Context) {
		redirect := c.Query("redirect")
		// OAuth context
		oauthClientID := c.Query("client_id")
		oauthRedirectURI := c.Query("redirect_uri")
		oauthResponseType := c.Query("response_type")
		oauthScope := c.Query("scope")
		oauthState := c.Query("state")
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, loginPageHTML(redirect, oasEnv, devTokenEnabled, oauthClientID, oauthRedirectURI, oauthResponseType, oauthScope, oauthState))
	})

	// ===== GET /admin — OAS Console 管理控制台首页 =====
	// Requires JWT + whitelist A (OU/AU/OAM)
	r.GET("/admin", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "JWT verifier not configured")
			return
		}
		// Manual JWT check for HTML page (redirect to login if not authenticated)
		tokenStr := c.Query("token")
		if tokenStr == "" {
			// Try to get from Authorization header
			auth := c.GetHeader("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				tokenStr = auth[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin")
			return
		}
		// Verify JWT
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin")
			return
		}
		// Check whitelist A
		username := claims.Username
		if !authz.IsInAdminWhitelistA(database, username) {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to system administrators.</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, consoleHomePageHTML(username, oasEnv.String(), tokenStr))
	})

	// ===== GET /admin/overview — OU 治理看板 =====
	// Requires JWT + whitelist A (OU/AU/OAM)
	r.GET("/admin/overview", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "JWT verifier not configured")
			return
		}
		tokenStr := c.Query("token")
		if tokenStr == "" {
			auth := c.GetHeader("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				tokenStr = auth[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin/overview")
			return
		}
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/overview")
			return
		}
		username := claims.Username
		if !authz.IsInAdminWhitelistA(database, username) {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to system administrators.</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, overviewPageHTML(username, oasEnv.String(), tokenStr))
	})

	// ===== GET /admin/approvals — 战略审批工作台（白名单 A：OU/AU/OAM，OAM 只读）=====
	r.GET("/admin/approvals", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "JWT verifier not configured")
			return
		}
		tokenStr := c.Query("token")
		if tokenStr == "" {
			auth := c.GetHeader("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				tokenStr = auth[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin/approvals")
			return
		}
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/approvals")
			return
		}
		username := claims.Username
		if !authz.IsInAdminWhitelistA(database, username) {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to system administrators.</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, approvalsPageHTML(username, oasEnv.String(), tokenStr))
	})

	// ===== GET /admin/ownership — 所有权视图（白名单 A）=====
	r.GET("/admin/ownership", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "JWT verifier not configured")
			return
		}
		tokenStr := c.Query("token")
		if tokenStr == "" {
			auth := c.GetHeader("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				tokenStr = auth[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin/ownership")
			return
		}
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/ownership")
			return
		}
		if !authz.IsInAdminWhitelistA(database, claims.Username) {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to whitelist A (OU/AU/OAM).</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, ownershipPageHTML(claims.Username))
	})

	// Admin 账号管理页面（仅 OU/AU）
	r.GET("/admin/admin-accounts", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "JWT verifier not configured")
			return
		}
		tokenStr := c.Query("token")
		if tokenStr == "" {
			auth := c.GetHeader("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				tokenStr = auth[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin/admin-accounts")
			return
		}
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/admin-accounts")
			return
		}
		// 仅 OU/AU 可访问（白名单 B）
		if claims.Username != "oas-ou-admin" && claims.Username != "oas-au-admin" {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to OU/AU admin.</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, adminAccountsPageHTML(claims.Username))
	})

	// 系统配置只读面板（仅 OU/AU）
	r.GET("/admin/system-config", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "JWT verifier not configured")
			return
		}
		tokenStr := c.Query("token")
		if tokenStr == "" {
			auth := c.GetHeader("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				tokenStr = auth[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin/system-config")
			return
		}
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/system-config")
			return
		}
		// 仅 OU/AU 可访问（白名单 B）
		if claims.Username != "oas-ou-admin" && claims.Username != "oas-au-admin" {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to OU/AU admin.</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, systemConfigPageHTML(claims.Username))
	})

	// API Key 管理页面（仅 OU/AU）
	r.GET("/admin/api-keys", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "JWT verifier not configured")
			return
		}
		tokenStr := c.Query("token")
		if tokenStr == "" {
			auth := c.GetHeader("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				tokenStr = auth[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin/api-keys")
			return
		}
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/api-keys")
			return
		}
		// 仅 OU/AU 可访问（白名单 B）
		if claims.Username != "oas-ou-admin" && claims.Username != "oas-au-admin" {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to OU/AU admin.</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, apiKeysPageHTML(claims.Username))
	})

	// 联邦节点管理页面（仅 OU/AU）
	r.GET("/admin/federation-nodes", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "JWT verifier not configured")
			return
		}
		tokenStr := c.Query("token")
		if tokenStr == "" {
			auth := c.GetHeader("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				tokenStr = auth[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin/federation-nodes")
			return
		}
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/federation-nodes")
			return
		}
		// 仅 OU/AU 可访问（白名单 B）
		if claims.Username != "oas-ou-admin" && claims.Username != "oas-au-admin" {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to OU/AU admin.</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, federationNodesPageHTML(claims.Username))
	})

	// ===== GET /admin/oauth-clients — OAuth 客户端管理页面（白名单 B：仅 OU/AU）=====
	r.GET("/admin/oauth-clients", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "JWT verifier not configured")
			return
		}
		tokenStr := c.Query("token")
		if tokenStr == "" {
			auth := c.GetHeader("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				tokenStr = auth[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin/oauth-clients")
			return
		}
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/oauth-clients")
			return
		}
		// Whitelist B: only OU/AU admin can access
		username := claims.Username
		if !authz.IsInAdminWhitelistB(database, username) {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to OU/AU admin.</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, oauthClientsPageHTML(username))
	})

	// ===== GET /admin/audit-logs — 审计日志页面（白名单 B：仅 2 admin）=====
	r.GET("/admin/audit-logs", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "JWT verifier not configured")
			return
		}
		tokenStr := c.Query("token")
		if tokenStr == "" {
			auth := c.GetHeader("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				tokenStr = auth[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin/audit-logs")
			return
		}
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/audit-logs")
			return
		}
		// Whitelist B: only OU/AU admin can access audit logs
		username := claims.Username
		if !authz.IsInAdminWhitelistB(database, username) {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Audit logs restricted to OU/AU admin.</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, auditLogsPageHTML())
	})

	// ===== POST /api/v1/auth/login — username+password login =====
	api.POST("/auth/login", handlers.H.Login)

	// ===== User Management API (/api/v1/admin/users/*) — JWT + SU/AU only =====
	adminUsers := api.Group("/admin")
	if jwtVerifier != nil {
		adminUsers.Use(middleware.JWTAuth(jwtVerifier, nil, log))
		// 白名单 A：系统管理访问（/admin/*）= SU/OU/AU/OAM
		adminUsers.Use(func(c *gin.Context) {
			username, _ := c.Get("username")
			roleCode, _ := c.Get("role_code")
			// API Key 视为 admin 级别
			if strings.HasPrefix(fmt.Sprintf("%v", username), "api-key:") {
				c.Next()
				return
			}
			// 查询用户 role_code
			var user oasmodel.OASUser
			if database.Where("username = ?", username).First(&user).Error == nil && user.RoleCode != "" {
				roleCode = user.RoleCode
			}
			rc := fmt.Sprintf("%v", roleCode)
			if rc == "SU" || rc == "OU" || rc == "AU" || rc == "OAM" {
				c.Set("role_code", rc)
				c.Next()
				return
			}
			response.Forbidden(c, "access denied")
			c.Abort()
		})
	}
	adminUsers.GET("/users", func(c *gin.Context) {
		// 白名单 B：用户管理仅 SU/OU/AU 可访问
		operatorUsername, _ := c.Get("username")
		if !authz.IsInAdminWhitelistB(database, fmt.Sprintf("%v", operatorUsername)) {
			response.Forbidden(c, "only SU/OU/AU admin can manage users")
			return
		}
		type UserVO struct {
			ID          uint64   `json:"id"`
			UserCode    string   `json:"user_code"`
			Username    string   `json:"username"`
			DisplayName string   `json:"display_name"`
			Status      string   `json:"status"`
			Roles       []string `json:"roles"`
			LastLoginAt *string  `json:"last_login_at,omitempty"`
			CreatedAt   string   `json:"created_at"`
		}
		var users []oasmodel.OASUser
		database.Order("created_at DESC").Find(&users)
		var result []UserVO
		for _, u := range users {
			var roles []string
			database.Table("user_roles").
				Select("r.role_code").
				Joins("JOIN roles r ON r.id = user_roles.role_id").
				Where("user_roles.user_id = ?", u.ID).
				Pluck("r.role_code", &roles)
			vo := UserVO{
				ID:          u.ID,
				UserCode:    u.UserCode,
				Username:    u.Username,
				DisplayName: u.DisplayName,
				Status:      u.Status,
				Roles:       roles,
				CreatedAt:   u.CreatedAt.Format(time.RFC3339),
			}
			if u.LastLoginAt != nil {
				s := u.LastLoginAt.Format(time.RFC3339)
				vo.LastLoginAt = &s
			}
			result = append(result, vo)
		}
		response.OK(c, gin.H{"items": result, "total": len(result)})
	})

	adminUsers.POST("/users", func(c *gin.Context) {
		var req struct {
			Username    string   `json:"username" binding:"required"`
			Password    string   `json:"password" binding:"required"`
			DisplayName string   `json:"display_name"`
			RoleCode    string   `json:"role_code" binding:"required"`
			Roles       []string `json:"roles"`
			Domain      string   `json:"domain"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "username, password, role_code required")
			return
		}
		// 白名单 B：创建 admin 账号仅 OU/AU admin 可操作
		operatorUsername, _ := c.Get("username")
		if authz.IsAdminAccount(req.Username) && !authz.CanOperateAdminAccount(database, fmt.Sprintf("%v", operatorUsername)) {
			response.Forbidden(c, "only oas-ou-admin and oas-au-admin can create admin accounts")
			return
		}
		var existing oasmodel.OASUser
		if database.Where("username = ?", req.Username).First(&existing).Error == nil {
			response.BadRequest(c, "username already exists")
			return
		}
		// 校验 role_code 是否存在
		var roleCheck oasmodel.OASRole
		if database.Where("role_code = ?", req.RoleCode).First(&roleCheck).Error != nil {
			response.BadRequest(c, "role_code not found: "+req.RoleCode)
			return
		}
		hash, err := password.Hash(req.Password)
		if err != nil {
			response.InternalError(c, "failed to hash password")
			return
		}
		displayName := req.DisplayName
		if displayName == "" {
			displayName = req.Username
		}
		userCode := fmt.Sprintf("XHPZ#%s-%d", req.RoleCode, time.Now().UnixNano()%100000)
		user := oasmodel.OASUser{
			UserCode:     userCode,
			Username:     req.Username,
			PasswordHash: string(hash),
			DisplayName:  displayName,
			IdentityType: req.RoleCode,
			EntityType:   "H",
			Status:       "active",
			Domain:       req.Domain,
		}
		if err := database.Create(&user).Error; err != nil {
			response.BadRequest(c, "create user failed: "+err.Error())
			return
		}
		roleCodes := req.Roles
		if len(roleCodes) == 0 {
			roleCodes = []string{req.RoleCode}
		}
		for _, rc := range roleCodes {
			var role oasmodel.OASRole
			if database.Where("role_code = ?", rc).First(&role).Error == nil {
				database.Table("user_roles").Create(&oasmodel.OASUserRole{
					UserID:    user.ID,
					RoleID:    role.ID,
					GrantedBy: "admin",
					GrantedAt: time.Now(),
				})
			}
		}
		database.Create(&oasmodel.AuditLog{
			Plane:       "admin",
			Action:      "user.create",
			UserID:      user.UserCode,
			UserName:    user.DisplayName,
			Resource:    "user",
			Detail:      fmt.Sprintf("env=%s, username=%s, roles=%v", oasEnv.String(), req.Username, roleCodes),
			IP:          c.ClientIP(),
			UserAgent:   c.Request.UserAgent(),
			Environment: oasEnv.String(),
			Domain:      user.Domain,
		})
		response.Created(c, gin.H{"id": user.ID, "username": user.Username, "user_code": user.UserCode})
	})

	adminUsers.PUT("/users/:id/roles", func(c *gin.Context) {
		var req struct {
			Roles []string `json:"roles" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "roles required")
			return
		}
		id, _ := parseUint(c.Param("id"))
		// 白名单 B：修改 admin 账号角色仅 OU/AU admin 可操作
		var targetUser oasmodel.OASUser
		database.First(&targetUser, id)
		operatorUsername, _ := c.Get("username")
		if authz.IsAdminAccount(targetUser.Username) && !authz.CanOperateAdminAccount(database, fmt.Sprintf("%v", operatorUsername)) {
			response.Forbidden(c, "only oas-ou-admin and oas-au-admin can modify admin account roles")
			return
		}
		database.Where("user_id = ?", id).Delete(&oasmodel.OASUserRole{})
		for _, rc := range req.Roles {
			var role oasmodel.OASRole
			if database.Where("role_code = ?", rc).First(&role).Error == nil {
				database.Table("user_roles").Create(&oasmodel.OASUserRole{
					UserID:    id,
					RoleID:    role.ID,
					GrantedBy: "admin",
					GrantedAt: time.Now(),
				})
			}
		}
		database.Create(&oasmodel.AuditLog{
			Plane:       "admin",
			Action:      "user.update_roles",
			Resource:    "user",
			Detail:      fmt.Sprintf("env=%s, user_id=%d, roles=%v", oasEnv.String(), id, req.Roles),
			IP:          c.ClientIP(),
			UserAgent:   c.Request.UserAgent(),
			Environment: oasEnv.String(),
		})
		response.OK(c, nil)
	})

	adminUsers.PUT("/users/:id/status", func(c *gin.Context) {
		var req struct {
			Status string `json:"status" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "status required")
			return
		}
		if req.Status != "active" && req.Status != "disabled" {
			response.BadRequest(c, "status must be active or disabled")
			return
		}
		id, _ := parseUint(c.Param("id"))
		// 白名单 B：修改 admin 账号状态仅 OU/AU admin 可操作
		var targetUser oasmodel.OASUser
		database.First(&targetUser, id)
		operatorUsername, _ := c.Get("username")
		if authz.IsAdminAccount(targetUser.Username) && !authz.CanOperateAdminAccount(database, fmt.Sprintf("%v", operatorUsername)) {
			response.Forbidden(c, "only oas-ou-admin and oas-au-admin can modify admin account status")
			return
		}
		database.Model(&oasmodel.OASUser{}).Where("id = ?", id).Update("status", req.Status)
		database.Create(&oasmodel.AuditLog{
			Plane:       "admin",
			Action:      "user.update_status",
			Resource:    "user",
			Detail:      fmt.Sprintf("env=%s, user_id=%d, status=%s", oasEnv.String(), id, req.Status),
			IP:          c.ClientIP(),
			UserAgent:   c.Request.UserAgent(),
			Environment: oasEnv.String(),
		})
		response.OK(c, nil)
	})

	// DELETE /api/v1/admin/users/:id — 删除用户（白名单 B：仅 SU/OU/AU）
	adminUsers.DELETE("/users/:id", func(c *gin.Context) {
		// 白名单 B：删除用户仅 SU/OU/AU 可操作
		operatorUsername, _ := c.Get("username")
		if !authz.IsInAdminWhitelistB(database, fmt.Sprintf("%v", operatorUsername)) {
			response.Forbidden(c, "only SU/OU/AU admin can delete users")
			return
		}
		id, _ := parseUint(c.Param("id"))
		var targetUser oasmodel.OASUser
		if database.First(&targetUser, id).Error != nil {
			response.NotFound(c, "user not found")
			return
		}
		// 删除用户
		if err := database.Delete(&targetUser).Error; err != nil {
			response.InternalError(c, "failed to delete user: "+err.Error())
			return
		}
		// 删除用户角色关联
		database.Where("user_id = ?", id).Delete(&oasmodel.OASUserRole{})
		// 审计日志 - 记录操作者身份，resource_id 为被删用户 id
		var operator oasmodel.OASUser
		operatorUserCode := ""
		operatorDisplayName := ""
		if database.Where("username = ?", operatorUsername).First(&operator).Error == nil {
			operatorUserCode = operator.UserCode
			operatorDisplayName = operator.DisplayName
		}
		database.Create(&oasmodel.AuditLog{
			Plane:       "admin",
			Action:      "admin.account.delete",
			UserID:      operatorUserCode,
			UserName:    operatorDisplayName,
			Resource:    "user",
			ResourceID:  fmt.Sprintf("%d", id),
			Detail:      fmt.Sprintf("env=%s, deleted_user_id=%d, deleted_username=%s, deleted_role=%s", oasEnv.String(), id, targetUser.Username, targetUser.RoleCode),
			IP:          c.ClientIP(),
			UserAgent:   c.Request.UserAgent(),
			Environment: oasEnv.String(),
			Domain:      operator.Domain,
		})
		response.OK(c, gin.H{"id": id, "username": targetUser.Username})
	})

	// GET /api/v1/auth/roles — list available roles
	api.GET("/auth/roles", handlers.H.AuthRoles)

	// ===== Role Management API (JWT + Whitelist A) =====
	adminRoles := api.Group("/admin/roles")
	if jwtVerifier != nil {
		adminRoles.Use(middleware.JWTAuth(jwtVerifier, nil, log))
		adminRoles.Use(middleware.RequireUsers("oas-ou-admin", "oas-au-admin", "oas-oam-admin"))
	}
	// GET /api/v1/admin/roles — list all roles
	adminRoles.GET("", func(c *gin.Context) {
		var roles []oasmodel.OASRole
		database.Order("role_code").Find(&roles)
		type RoleDetail struct {
			ID          uint64 `json:"id"`
			RoleCode    string `json:"role_code"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Permissions string `json:"permissions"`
			CreatedAt   string `json:"created_at"`
			UpdatedAt   string `json:"updated_at"`
		}
		var result []RoleDetail
		for _, r := range roles {
			result = append(result, RoleDetail{
				ID:          r.ID,
				RoleCode:    r.RoleCode,
				Name:        r.Name,
				Description: r.Description,
				Permissions: r.Permissions,
				CreatedAt:   r.CreatedAt.Format(time.RFC3339),
				UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
			})
		}
		response.OK(c, gin.H{"items": result, "total": len(result)})
	})

	// POST /api/v1/admin/roles — create role
	adminRoles.POST("", func(c *gin.Context) {
		var req struct {
			RoleCode    string `json:"role_code" binding:"required"`
			Name        string `json:"name" binding:"required"`
			Description string `json:"description"`
			Permissions string `json:"permissions"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "invalid request: "+err.Error())
			return
		}
		role := oasmodel.OASRole{
			RoleCode:    req.RoleCode,
			Name:        req.Name,
			Description: req.Description,
			Permissions: req.Permissions,
		}
		if err := database.Create(&role).Error; err != nil {
			response.InternalError(c, "create role failed: "+err.Error())
			return
		}
		// Audit log
		operator, _ := c.Get("username")
		database.Create(&oasmodel.AuditLog{
			Action:     "role.create",
			Plane:      "admin",
			UserID:     fmt.Sprintf("%v", operator),
			ResourceID: fmt.Sprintf("role-%d", role.ID),
			Detail:     fmt.Sprintf("role_code=%s, name=%s", role.RoleCode, role.Name),
			IP:         c.ClientIP(),
		})
		response.Created(c, gin.H{"id": role.ID, "role_code": role.RoleCode, "name": role.Name})
	})

	// PUT /api/v1/admin/roles/:id — update role
	adminRoles.PUT("/:id", func(c *gin.Context) {
		id := c.Param("id")
		var role oasmodel.OASRole
		if err := database.First(&role, id).Error; err != nil {
			response.NotFound(c, "role not found")
			return
		}
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Permissions string `json:"permissions"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "invalid request: "+err.Error())
			return
		}
		updates := map[string]interface{}{}
		if req.Name != "" {
			updates["name"] = req.Name
		}
		if req.Description != "" {
			updates["description"] = req.Description
		}
		if req.Permissions != "" {
			updates["permissions"] = req.Permissions
		}
		if len(updates) > 0 {
			database.Model(&role).Updates(updates)
		}
		// Audit log
		operator, _ := c.Get("username")
		database.Create(&oasmodel.AuditLog{
			Action:     "role.update",
			Plane:      "admin",
			UserID:     fmt.Sprintf("%v", operator),
			ResourceID: fmt.Sprintf("role-%s", id),
			Detail:     fmt.Sprintf("updates=%v", updates),
			IP:         c.ClientIP(),
		})
		response.OK(c, gin.H{"message": "role updated"})
	})

	// DELETE /api/v1/admin/roles/:id — delete role
	adminRoles.DELETE("/:id", func(c *gin.Context) {
		id := c.Param("id")
		var role oasmodel.OASRole
		if err := database.First(&role, id).Error; err != nil {
			response.NotFound(c, "role not found")
			return
		}
		// Check if role is assigned to any users
		var count int64
		database.Table("user_roles").Where("role_id = ?", id).Count(&count)
		if count > 0 {
			response.BadRequest(c, fmt.Sprintf("role is assigned to %d users, cannot delete", count))
			return
		}
		database.Delete(&role, id)
		// Audit log
		operator, _ := c.Get("username")
		database.Create(&oasmodel.AuditLog{
			Action:     "role.delete",
			Plane:      "admin",
			UserID:     fmt.Sprintf("%v", operator),
			ResourceID: fmt.Sprintf("role-%s", id),
			Detail:     fmt.Sprintf("role_code=%s", role.RoleCode),
			IP:         c.ClientIP(),
		})
		response.OK(c, gin.H{"message": "role deleted"})
	})

	// ===== Organization Management (GET/POST/PUT/DELETE /admin/orgs) — JWT + whitelist A + XAM roles =====
	adminOrgs := api.Group("/admin/orgs", middleware.JWTAuth(jwtVerifier, nil, log), func(c *gin.Context) {
		username, _ := c.Get("username")
		rolesRaw, _ := c.Get("roles")
		var roles []string
		if rolesRaw != nil {
			// JWT middleware sets roles as []string
			if rolesSlice, ok := rolesRaw.([]string); ok {
				roles = rolesSlice
			} else if rolesStr, ok := rolesRaw.(string); ok && rolesStr != "" {
				// Fallback: comma-separated string
				roles = strings.Split(rolesStr, ",")
			}
		}
		if !authz.CanAccessOrgManagement(database, fmt.Sprintf("%v", username), roles) {
			response.Forbidden(c, "access denied")
			c.Abort()
			return
		}
		c.Next()
	}, middleware.DomainFilter())
	{
		// List organizations (with tree structure)
		adminOrgs.GET("", func(c *gin.Context) {
			var orgs []model.Organization
			query := database.Preload("Parent").Preload("Children")

			// Apply domain filter if present
			if filterDomain, ok := middleware.GetFilterDomain(c); ok && filterDomain != "" {
				query = query.Where("domain = ?", filterDomain)
			}

			if err := query.Find(&orgs).Error; err != nil {
				response.InternalError(c, "load orgs failed: "+err.Error())
				return
			}
			// Build tree structure (only top-level orgs with children)
			orgMap := make(map[uint]*model.Organization)
			var topOrgs []model.Organization
			for i := range orgs {
				orgMap[orgs[i].ID] = &orgs[i]
			}
			for i := range orgs {
				if orgs[i].ParentID == nil {
					topOrgs = append(topOrgs, orgs[i])
				}
			}
			response.OK(c, gin.H{"items": topOrgs, "total": len(topOrgs)})
		})

		// Get organization detail
		adminOrgs.GET("/:id", func(c *gin.Context) {
			id, _ := parseUint(c.Param("id"))
			var org model.Organization
			query := database.Preload("Parent").Preload("Children").Preload("Members")

			// Apply domain filter if present
			if filterDomain, ok := middleware.GetFilterDomain(c); ok && filterDomain != "" {
				query = query.Where("domain = ?", filterDomain)
			}

			if err := query.First(&org, id).Error; err != nil {
				response.NotFound(c, "org not found")
				return
			}
			response.OK(c, org)
		})

		// Create organization
		adminOrgs.POST("", func(c *gin.Context) {
			var req struct {
				Code        string `json:"code" binding:"required"`
				Name        string `json:"name" binding:"required"`
				Description string `json:"description"`
				ParentID    *uint  `json:"parent_id"`
				Domain      string `json:"domain"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, "invalid request: "+err.Error())
				return
			}
			org := model.Organization{
				Code:        req.Code,
				Name:        req.Name,
				Description: req.Description,
				ParentID:    req.ParentID,
				Domain:      req.Domain,
				Status:      "active",
			}
			if err := database.Create(&org).Error; err != nil {
				response.InternalError(c, "create org failed: "+err.Error())
				return
			}
			operator, _ := c.Get("user_id")
			domain, _ := c.Get("domain")
			database.Create(&oasmodel.AuditLog{
				Plane:       "admin",
				Action:      "org.create",
				UserID:      fmt.Sprintf("%v", operator),
				ResourceID:  fmt.Sprintf("org-%d", org.ID),
				Detail:      fmt.Sprintf("code=%s, name=%s, domain=%s", org.Code, org.Name, org.Domain),
				IP:          c.ClientIP(),
				Environment: oasEnv.String(),
				Domain:      fmt.Sprintf("%v", domain),
			})
			response.Created(c, org)
		})

		// Update organization
		adminOrgs.PUT("/:id", func(c *gin.Context) {
			id, _ := parseUint(c.Param("id"))
			var org model.Organization
			if err := database.First(&org, id).Error; err != nil {
				response.NotFound(c, "org not found")
				return
			}
			var req struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				ParentID    *uint  `json:"parent_id"`
				Domain      string `json:"domain"`
				Status      string `json:"status"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, "invalid request: "+err.Error())
				return
			}
			updates := map[string]interface{}{}
			if req.Name != "" {
				updates["name"] = req.Name
			}
			if req.Description != "" {
				updates["description"] = req.Description
			}
			if req.ParentID != nil {
				updates["parent_id"] = *req.ParentID
			}
			if req.Domain != "" {
				updates["domain"] = req.Domain
			}
			if req.Status != "" {
				updates["status"] = req.Status
			}
			if err := database.Model(&org).Updates(updates).Error; err != nil {
				response.InternalError(c, "update org failed: "+err.Error())
				return
			}
			operator, _ := c.Get("user_id")
			domain, _ := c.Get("domain")
			database.Create(&oasmodel.AuditLog{
				Plane:       "admin",
				Action:      "org.update",
				UserID:      fmt.Sprintf("%v", operator),
				ResourceID:  fmt.Sprintf("org-%d", org.ID),
				Detail:      fmt.Sprintf("updates=%v", updates),
				IP:          c.ClientIP(),
				Environment: oasEnv.String(),
				Domain:      fmt.Sprintf("%v", domain),
			})
			response.OK(c, org)
		})

		// Delete organization
		adminOrgs.DELETE("/:id", func(c *gin.Context) {
			id, _ := parseUint(c.Param("id"))
			var org model.Organization
			if err := database.First(&org, id).Error; err != nil {
				response.NotFound(c, "org not found")
				return
			}
			// Check if has children
			var childCount int64
			database.Model(&model.Organization{}).Where("parent_id = ?", org.ID).Count(&childCount)
			if childCount > 0 {
				response.BadRequest(c, "cannot delete org with children")
				return
			}
			// Check if has members
			var memberCount int64
			database.Model(&model.UserOrganization{}).Where("organization_id = ?", org.ID).Count(&memberCount)
			if memberCount > 0 {
				response.BadRequest(c, "cannot delete org with members")
				return
			}
			if err := database.Delete(&org).Error; err != nil {
				response.InternalError(c, "delete org failed: "+err.Error())
				return
			}
			operator, _ := c.Get("user_id")
			domain, _ := c.Get("domain")
			database.Create(&oasmodel.AuditLog{
				Plane:       "admin",
				Action:      "org.delete",
				UserID:      fmt.Sprintf("%v", operator),
				ResourceID:  fmt.Sprintf("org-%d", org.ID),
				Detail:      fmt.Sprintf("code=%s", org.Code),
				IP:          c.ClientIP(),
				Environment: oasEnv.String(),
				Domain:      fmt.Sprintf("%v", domain),
			})
			response.OK(c, gin.H{"message": "org deleted"})
		})

		// Get organization members
		adminOrgs.GET("/:id/members", func(c *gin.Context) {
			id, _ := parseUint(c.Param("id"))

			// First check if org exists and belongs to user's domain
			var org model.Organization
			orgQuery := database
			if filterDomain, ok := middleware.GetFilterDomain(c); ok && filterDomain != "" {
				orgQuery = orgQuery.Where("domain = ?", filterDomain)
			}
			if err := orgQuery.First(&org, id).Error; err != nil {
				response.NotFound(c, "org not found")
				return
			}

			var members []model.UserOrganization
			if err := database.Where("organization_id = ?", id).Find(&members).Error; err != nil {
				response.InternalError(c, "load members failed: "+err.Error())
				return
			}
			response.OK(c, gin.H{"items": members, "total": len(members)})
		})

		// Add member to organization
		adminOrgs.POST("/:id/members", func(c *gin.Context) {
			id, _ := parseUint(c.Param("id"))

			// First check if org exists and belongs to user's domain
			var org model.Organization
			orgQuery := database
			if filterDomain, ok := middleware.GetFilterDomain(c); ok && filterDomain != "" {
				orgQuery = orgQuery.Where("domain = ?", filterDomain)
			}
			if err := orgQuery.First(&org, id).Error; err != nil {
				response.NotFound(c, "org not found")
				return
			}

			var req struct {
				UserID uint   `json:"user_id" binding:"required"`
				Role   string `json:"role"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				response.BadRequest(c, "invalid request: "+err.Error())
				return
			}
			// Check if user exists
			var user oasmodel.OASUser
			if err := database.First(&user, req.UserID).Error; err != nil {
				response.NotFound(c, "user not found")
				return
			}
			// Check if already member
			var existing model.UserOrganization
			if err := database.Where("user_id = ? AND organization_id = ?", req.UserID, org.ID).First(&existing).Error; err == nil {
				response.BadRequest(c, "user already member of this org")
				return
			}
			member := model.UserOrganization{
				UserID:         req.UserID,
				OrganizationID: org.ID,
				Role:           req.Role,
			}
			if err := database.Create(&member).Error; err != nil {
				response.InternalError(c, "add member failed: "+err.Error())
				return
			}
			operator, _ := c.Get("user_id")
			domain, _ := c.Get("domain")
			database.Create(&oasmodel.AuditLog{
				Plane:       "admin",
				Action:      "org.member.add",
				UserID:      fmt.Sprintf("%v", operator),
				ResourceID:  fmt.Sprintf("org-%d", org.ID),
				Detail:      fmt.Sprintf("user_id=%d, role=%s", req.UserID, req.Role),
				IP:          c.ClientIP(),
				Environment: oasEnv.String(),
				Domain:      fmt.Sprintf("%v", domain),
			})
			response.Created(c, member)
		})

		// Remove member from organization
		adminOrgs.DELETE("/:id/members/:userId", func(c *gin.Context) {
			id, _ := parseUint(c.Param("id"))
			userId, _ := parseUint(c.Param("userId"))

			// First check if org exists and belongs to user's domain
			var org model.Organization
			orgQuery := database
			if filterDomain, ok := middleware.GetFilterDomain(c); ok && filterDomain != "" {
				orgQuery = orgQuery.Where("domain = ?", filterDomain)
			}
			if err := orgQuery.First(&org, id).Error; err != nil {
				response.NotFound(c, "org not found")
				return
			}

			var member model.UserOrganization
			if err := database.Where("organization_id = ? AND user_id = ?", org.ID, userId).First(&member).Error; err != nil {
				response.NotFound(c, "member not found")
				return
			}
			if err := database.Delete(&member).Error; err != nil {
				response.InternalError(c, "remove member failed: "+err.Error())
				return
			}
			operator, _ := c.Get("user_id")
			domain, _ := c.Get("domain")
			database.Create(&oasmodel.AuditLog{
				Plane:       "admin",
				Action:      "org.member.remove",
				UserID:      fmt.Sprintf("%v", operator),
				ResourceID:  fmt.Sprintf("org-%d", org.ID),
				Detail:      fmt.Sprintf("user_id=%d", userId),
				IP:          c.ClientIP(),
				Environment: oasEnv.String(),
				Domain:      fmt.Sprintf("%v", domain),
			})
			response.OK(c, gin.H{"message": "member removed"})
		})
	}

	// ===== User Management Page (GET /admin/users) — JWT required =====
	r.GET("/admin/users", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "jwt verifier not configured")
			return
		}
		// Support both ?token= parameter and Authorization header
		tokenStr := c.Query("token")
		if tokenStr == "" {
			authHeader := c.GetHeader("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				tokenStr = authHeader[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin/users")
			return
		}
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/users")
			return
		}
		// 白名单 A：系统管理访问 = OU-admin + AU-admin + OAM
		if claims.Username != "oas-ou-admin" && claims.Username != "oas-au-admin" && claims.Username != "oas-oam-admin" {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Access denied. System management restricted to OU/AU/OAM admins.</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, userMgmtPageHTML())
	})

	// ===== Role Management Page (GET /admin/roles) — JWT required =====
	r.GET("/admin/roles", func(c *gin.Context) {
		if jwtVerifier == nil {
			response.InternalError(c, "jwt verifier not configured")
			return
		}
		// Support both ?token= parameter and Authorization header
		tokenStr := c.Query("token")
		if tokenStr == "" {
			authHeader := c.GetHeader("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				tokenStr = authHeader[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin/roles")
			return
		}
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/roles")
			return
		}
		// 白名单 A：系统管理访问 = OU-admin + AU-admin + OAM
		if claims.Username != "oas-ou-admin" && claims.Username != "oas-au-admin" && claims.Username != "oas-oam-admin" {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Access denied. System management restricted to OU/AU/OAM admins.</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, roleMgmtPageHTML())
	})

	// ===== Organization Management Page (GET /admin/orgs) — JWT required =====
	r.GET("/admin/orgs", func(c *gin.Context) {
		if jwtVerifier == nil {
			c.Redirect(302, "/login?redirect=/admin/orgs")
			return
		}
		tokenStr := c.Query("token")
		if tokenStr == "" {
			authHeader := c.GetHeader("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				tokenStr = authHeader[7:]
			}
		}
		if tokenStr == "" {
			c.Redirect(302, "/login?redirect=/admin/orgs")
			return
		}
		claims, err := jwtVerifier.Verify(tokenStr)
		if err != nil {
			c.Redirect(302, "/login?redirect=/admin/orgs")
			return
		}
		if !authz.IsInAdminWhitelistA(database, claims.Username) {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(403, "<h1>403 Forbidden</h1><p>Access denied. System management restricted to OU/AU/OAM admins.</p>")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, orgMgmtPageHTML())
	})

	port := v.GetString("server.http_port")
	if port == "" {
		port = "8080"
	}
	log.Info("ziway-oas starting", zap.String("port", port))
	if err := r.Run(":" + port); err != nil {
		log.Fatal("server failed", zap.Error(err))
	}
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func parseUint(s string) (uint64, error) {
	var n uint64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func ptrUint64(n uint64) *uint64 {
	return &n
}

func ptrString(s string) *string {
	return &s
}

// loginPageHTML returns the login page HTML.
func loginPageHTML(redirect string, oasEnv envpolicy.Environment, devTokenEnabled bool, oauthClientID, oauthRedirectURI, oauthResponseType, oauthScope, oauthState string) string {
	quickLoginSection := ""
	if envpolicy.IsQuickLoginEnabled(oasEnv) {
		quickLoginSection = `
		<div style="margin-top:24px;padding-top:20px;border-top:1px solid #e5e7eb">
			<p style="font-size:13px;color:#6b7280;margin-bottom:12px">内测快捷登录</p>
			<div style="display:flex;gap:8px;flex-wrap:wrap">
				<button onclick="quickLogin('SU')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">SU 管理员</button>
				<button onclick="quickLogin('AU')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">AU 运营</button>
				<button onclick="quickLogin('CU')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">CU 客户</button>
				<button onclick="quickLogin('GU')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">GU 访客</button>
				<button onclick="quickLogin('EM')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">EM 供给</button>
			</div>
		</div>
		<script>
		async function quickLogin(role){
			const btn=document.getElementById('submitBtn');
			btn.disabled=true;
			try{
				const r=await fetch('/api/v1/auth/quick-login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({role})});
				const d=await r.json();
				if(d.code!==200)throw new Error(d.message||'quick login failed');
				handleToken(d.data);
			}catch(ex){alert(ex.message);btn.disabled=false}
		}
		</script>`
	}
	devTokenSection := ""
	if devTokenEnabled {
		devTokenSection = `
		<div style="margin-top:24px;padding-top:20px;border-top:1px solid #e5e7eb">
			<p style="font-size:13px;color:#6b7280;margin-bottom:12px">🔧 开发临时令牌</p>
			<div style="display:flex;gap:8px;align-items:center;margin-bottom:12px">
				<select id="devTokenRole" style="flex:1;padding:8px 12px;border:1px solid #d1d5db;border-radius:6px;font-size:13px">
					<option value="">按角色生成...</option>
					<option value="SU">SU 管理员</option>
					<option value="AU">AU 运营</option>
					<option value="CU">CU 客户</option>
					<option value="GU">GU 访客</option>
					<option value="EM">EM 供给</option>
					<option value="OFM">OFM 域主</option>
					<option value="OVM">OVM 域运营</option>
					<option value="OGM">OGM 域治理</option>
					<option value="OAM">OAM 权限</option>
				</select>
				<button onclick="genDevToken()" style="padding:8px 16px;border:none;border-radius:6px;background:#059669;color:#fff;cursor:pointer;font-size:13px">生成</button>
			</div>
			<div id="devTokenResult" style="display:none">
				<textarea id="devTokenText" readonly style="width:100%;height:80px;padding:8px;border:1px solid #d1d5db;border-radius:6px;font-size:11px;font-family:monospace;resize:none"></textarea>
				<div style="display:flex;gap:8px;margin-top:8px">
					<button onclick="copyDevToken()" style="flex:1;padding:6px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:12px">📋 复制</button>
					<button onclick="redirectWithDevToken()" id="devTokenRedirectBtn" style="flex:1;padding:6px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:12px;display:none">🔗 携带令牌跳转</button>
				</div>
				<p id="devTokenInfo" style="font-size:11px;color:#6b7280;margin-top:8px"></p>
			</div>
		</div>
		<script>
		let devTokenValue='';
		async function genDevToken(){
			const role=document.getElementById('devTokenRole').value;
			if(!role){alert('请选择角色');return}
			try{
				const r=await fetch('/api/v1/auth/dev-token',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({role,expires_minutes:30})});
				const d=await r.json();
				if(d.code!==200)throw new Error(d.message||'dev-token failed');
				devTokenValue=d.data.token;
				document.getElementById('devTokenText').value=devTokenValue;
				document.getElementById('devTokenResult').style.display='block';
				document.getElementById('devTokenInfo').textContent='角色: '+d.data.role+' | 用户: '+d.data.username+' | 过期: '+new Date(d.data.expires_at).toLocaleString();
				const redirect=document.getElementById('form').dataset.redirect;
				if(redirect){
					document.getElementById('devTokenRedirectBtn').style.display='block';
				}
			}catch(ex){alert(ex.message)}
		}
		function copyDevToken(){
			navigator.clipboard.writeText(devTokenValue).then(()=>alert('已复制')).catch(()=>{
				const ta=document.getElementById('devTokenText');
				ta.select();document.execCommand('copy');alert('已复制');
			});
		}
		function redirectWithDevToken(){
			const redirect=document.getElementById('form').dataset.redirect;
			if(redirect){
				const sep=redirect.includes('?')?'&':'?';
				window.location.href=redirect+sep+'token='+devTokenValue;
			}
		}
		</script>`
	}
	redirectAttr := ""
	if redirect != "" {
		redirectAttr = `data-redirect="` + redirect + `"`
	}
	oauthAttrs := ""
	if oauthClientID != "" {
		oauthAttrs = fmt.Sprintf(`data-oauth-client="%s" data-oauth-redirect-uri="%s" data-oauth-state="%s"`,
			oauthClientID, oauthRedirectURI, oauthState)
	}
	return `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>OAS Login</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f3f4f6;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{background:#fff;border-radius:12px;box-shadow:0 1px 3px rgba(0,0,0,.1);padding:40px;width:100%;max-width:400px}
h1{font-size:20px;font-weight:600;margin-bottom:4px;color:#111827}
.sub{font-size:13px;color:#6b7280;margin-bottom:24px}
label{display:block;font-size:13px;font-weight:500;color:#374151;margin-bottom:4px}
input{width:100%;padding:10px 12px;border:1px solid #d1d5db;border-radius:8px;font-size:14px;margin-bottom:16px;outline:none;transition:border .15s}
input:focus{border-color:#2563eb;box-shadow:0 0 0 3px rgba(37,99,235,.1)}
.btn{width:100%;padding:10px;border:none;border-radius:8px;background:#2563eb;color:#fff;font-size:14px;font-weight:500;cursor:pointer;transition:background .15s}
.btn:hover{background:#1d4ed8}
.btn:disabled{background:#93c5fd;cursor:not-allowed}
.err{color:#dc2626;font-size:13px;margin-top:8px;display:none}
</style>
</head>
<body>
<div class="card" id="form" ` + redirectAttr + ` ` + oauthAttrs + `>
	<h1>知味 OAS</h1>
	<p class="sub">统一身份认证</p>
	<form onsubmit="return doLogin(event)">
		<label for="username">用户名</label>
		<input id="username" name="username" autocomplete="username" required>
		<label for="password">密码</label>
		<input id="password" name="password" type="password" autocomplete="current-password" required>
		<button class="btn" type="submit" id="submitBtn">登录</button>
		<p class="err" id="errMsg"></p>
	</form>` + quickLoginSection + devTokenSection + `
</div>
<script>
async function doLogin(e){
	e.preventDefault();
	const btn=document.getElementById('submitBtn');
	const err=document.getElementById('errMsg');
	btn.disabled=true;err.style.display='none';
	try{
		const r=await fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},
			body:JSON.stringify({username:document.getElementById('username').value,password:document.getElementById('password').value})});
		const d=await r.json();
		if(d.code!==200)throw new Error(d.message||'login failed');
		handleToken(d.data);
	}catch(ex){err.textContent=ex.message;err.style.display='block';btn.disabled=false}
}
function handleToken(data){
	const redirect=document.getElementById('form').dataset.redirect;
	const oauthClientId=document.getElementById('form').dataset.oauthClient;
	const oauthRedirectUri=document.getElementById('form').dataset.oauthRedirectUri;
	const oauthState=document.getElementById('form').dataset.oauthState;
	
	if(oauthClientId && oauthRedirectUri){
		// OAuth flow: exchange token for authorization code
		fetch('/api/v1/oauth/authorize-code',{
			method:'POST',
			headers:{'Content-Type':'application/json','Authorization':'Bearer '+data.access_token},
			body:JSON.stringify({client_id:oauthClientId,redirect_uri:oauthRedirectURI,state:oauthState})
		}).then(r=>r.json()).then(d=>{
			if(d.code!==200)throw new Error(d.message||'failed to generate code');
			const sep=oauthRedirectUri.includes('?')?'&':'?';
			window.location.href=oauthRedirectUri+sep+'code='+d.data.code+(oauthState?'&state='+oauthState:'');
		}).catch(ex=>{alert(ex.message);document.getElementById('submitBtn').disabled=false});
	}else if(redirect){
		const sep=redirect.includes('?')?'&':'?';
		window.location.href=redirect+sep+'token='+data.access_token;
	}else{
		document.getElementById('form').innerHTML='<h1>登录成功</h1><p class="sub">角色: '+data.role+'</p><pre style="font-size:11px;word-break:break-all;background:#f9fafb;padding:12px;border-radius:8px;margin-top:12px;max-height:200px;overflow:auto">'+data.access_token+'</pre><p style="margin-top:12px;font-size:13px;color:#6b7280">Token 有效期: '+data.expires_in+'s</p>';
	}
}
</script>
</body>
</html>`
}

// userMgmtPageHTML returns the user management page HTML.
func userMgmtPageHTML() string {
	return `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>OAS User Management</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f3f4f6;color:#111827}
.header{background:#fff;border-bottom:1px solid #e5e7eb;padding:16px 24px;display:flex;align-items:center;justify-content:space-between}
.header h1{font-size:18px;font-weight:600}
.container{max-width:960px;margin:24px auto;padding:0 24px}
.card{background:#fff;border-radius:10px;box-shadow:0 1px 2px rgba(0,0,0,.06);padding:24px;margin-bottom:20px}
.card h2{font-size:16px;font-weight:600;margin-bottom:16px}
table{width:100%;border-collapse:collapse;font-size:14px}
th{text-align:left;padding:10px 12px;border-bottom:2px solid #e5e7eb;font-weight:600;color:#6b7280;font-size:12px;text-transform:uppercase}
td{padding:10px 12px;border-bottom:1px solid #f3f4f6}
.badge{display:inline-block;padding:2px 8px;border-radius:9999px;font-size:12px;font-weight:500}
.badge-active{background:#d1fae5;color:#065f46}
.badge-disabled{background:#fee2e2;color:#991b1b}
.badge-role{background:#e0e7ff;color:#3730a3;margin-right:4px}
.btn{padding:6px 14px;border:none;border-radius:6px;font-size:13px;cursor:pointer;font-weight:500}
.btn-primary{background:#2563eb;color:#fff}.btn-primary:hover{background:#1d4ed8}
.btn-danger{background:#fee2e2;color:#991b1b}.btn-danger:hover{background:#fecaca}
.btn-success{background:#d1fae5;color:#065f46}.btn-success:hover{background:#a7f3d0}
.btn-sm{padding:4px 10px;font-size:12px}
.form-row{display:flex;gap:12px;margin-bottom:12px}
.form-row>div{flex:1}
.form-row label{display:block;font-size:12px;font-weight:500;color:#6b7280;margin-bottom:4px}
.form-row input,.form-row select{width:100%;padding:8px 10px;border:1px solid #d1d5db;border-radius:6px;font-size:13px}
.modal-overlay{display:none;position:fixed;inset:0;background:rgba(0,0,0,.4);z-index:100;align-items:center;justify-content:center}
.modal-overlay.show{display:flex}
.modal{background:#fff;border-radius:12px;padding:24px;width:100%;max-width:420px}
.modal h3{font-size:16px;margin-bottom:16px}
.empty{text-align:center;padding:40px;color:#9ca3af;font-size:14px}
</style>
</head>
<body>
<div class="header">
	<h1>知味 OAS 用户管理</h1>
	<a href="/login" style="font-size:13px;color:#6b7280;text-decoration:none">← 返回登录</a>
</div>
<div class="container">
	<div class="card">
		<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
			<h2>用户列表</h2>
			<button class="btn btn-primary" onclick="showAddModal()">+ 新增用户</button>
		</div>
		<div id="userTable"><div class="empty">加载中...</div></div>
	</div>
</div>

<div class="modal-overlay" id="addModal">
	<div class="modal">
		<h3>新增用户</h3>
		<div class="form-row"><div><label>用户名</label><input id="newUsername" required></div></div>
		<div class="form-row"><div><label>密码</label><input id="newPassword" type="password" required></div></div>
		<div class="form-row"><div><label>显示名</label><input id="newDisplayName"></div></div>
		<div class="form-row"><div><label>角色</label><select id="newRole"></select></div></div>
		<div style="display:flex;gap:8px;justify-content:flex-end;margin-top:16px">
			<button class="btn" style="background:#f3f4f6" onclick="hideAddModal()">取消</button>
			<button class="btn btn-primary" onclick="createUser()">创建</button>
		</div>
	</div>
</div>

<script>
const API='/api/v1';
async function loadUsers(){
	const r=await fetch(API+'/admin/users');
	const d=await r.json();
	const items=d.data.items||[];
	if(!items.length){document.getElementById('userTable').innerHTML='<div class="empty">暂无用户</div>';return}
	let html='<table><thead><tr><th>用户名</th><th>显示名</th><th>角色</th><th>状态</th><th>操作</th></tr></thead><tbody>';
	for(const u of items){
		const roles=u.roles.map(r=>'<span class="badge badge-role">'+r+'</span>').join('');
		const status=u.status==='active'?'<span class="badge badge-active">启用</span>':'<span class="badge badge-disabled">禁用</span>';
		const toggleBtn=u.status==='active'
			?'<button class="btn btn-danger btn-sm" onclick="toggleStatus('+u.id+',\'disabled\')">禁用</button>'
			:'<button class="btn btn-success btn-sm" onclick="toggleStatus('+u.id+',\'active\')">启用</button>';
		html+='<tr><td>'+u.username+'</td><td>'+u.display_name+'</td><td>'+roles+'</td><td>'+status+'</td><td>'+toggleBtn+'</td></tr>';
	}
	html+='</tbody></table>';
	document.getElementById('userTable').innerHTML=html;
}
async function loadRoles(){
	const r=await fetch(API+'/auth/roles');
	const d=await r.json();
	const sel=document.getElementById('newRole');
	sel.innerHTML='';
	for(const role of d.data){sel.innerHTML+='<option value="'+role.code+'">'+role.code+' - '+role.name+'</option>'}
}
function showAddModal(){document.getElementById('addModal').classList.add('show');loadRoles()}
function hideAddModal(){document.getElementById('addModal').classList.remove('show')}
async function createUser(){
	const body={
		username:document.getElementById('newUsername').value,
		password:document.getElementById('newPassword').value,
		display_name:document.getElementById('newDisplayName').value,
		role_code:document.getElementById('newRole').value
	};
	const r=await fetch(API+'/admin/users',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
	const d=await r.json();
	if(d.code===201||d.code===200){hideAddModal();loadUsers()}else{alert(d.message||'create failed')}
}
async function toggleStatus(id,status){
	await fetch(API+'/admin/users/'+id+'/status',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({status})});
	loadUsers();
}
loadUsers();
</script>
</body>
</html>`
}

// consoleHomePageHTML returns the OAS Console home page HTML with navigation.
func consoleHomePageHTML(username, oasEnv, token string) string {
	return `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>OAS Console</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f3f4f6;color:#111827}
.header{background:#fff;border-bottom:1px solid #e5e7eb;padding:16px 24px;display:flex;align-items:center;justify-content:space-between}
.header h1{font-size:18px;font-weight:600}
.user-info{font-size:13px;color:#6b7280}
.container{max-width:1200px;margin:24px auto;padding:0 24px}
.env-badge{display:inline-block;padding:4px 12px;border-radius:9999px;font-size:12px;font-weight:600;margin-left:12px}
.env-DEV{background:#dbeafe;color:#1e40af}
.env-BETA{background:#fef3c7;color:#92400e}
.env-RC{background:#e0e7ff;color:#3730a3}
.env-PROD{background:#fee2e2;color:#991b1b}
.nav-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:20px;margin-top:24px}
.nav-card{background:#fff;border-radius:12px;box-shadow:0 1px 3px rgba(0,0,0,.1);padding:24px;text-decoration:none;color:#111827;transition:all .2s}
.nav-card:hover{box-shadow:0 4px 12px rgba(0,0,0,.15);transform:translateY(-2px)}
.nav-card h3{font-size:16px;font-weight:600;margin-bottom:8px;display:flex;align-items:center;gap:8px}
.nav-card p{font-size:13px;color:#6b7280;line-height:1.5}
.nav-icon{width:32px;height:32px;border-radius:8px;display:flex;align-items:center;justify-content:center;font-size:18px}
.icon-users{background:#dbeafe;color:#2563eb}
.icon-roles{background:#e0e7ff;color:#4f46e5}
.icon-org{background:#d1fae5;color:#059669}
.icon-audit{background:#fee2e2;color:#dc2626}
.icon-config{background:#f3f4f6;color:#6b7280}
.icon-domains{background:#fef3c7;color:#d97706}
.section-title{font-size:14px;font-weight:600;color:#6b7280;text-transform:uppercase;margin-top:32px;margin-bottom:16px;padding-bottom:8px;border-bottom:2px solid #e5e7eb}
</style>
</head>
<body>
<div class="header">
	<h1>知味 OAS Console <span class="env-badge env-` + oasEnv + `">` + oasEnv + `</span></h1>
	<div class="user-info">` + username + ` | <a href="/login" style="color:#2563eb;text-decoration:none">退出</a></div>
</div>
<div class="container">
	<div class="section-title">系统管理（L1）</div>
	<div class="nav-grid">
		<a href="/admin/users?token=` + token + `" class="nav-card">
			<h3><span class="nav-icon icon-users">👥</span>账号管理</h3>
			<p>全局用户列表、新增用户、角色分配、状态管理</p>
		</a>
		<a href="/admin/roles?token=` + token + `" class="nav-card">
			<h3><span class="nav-icon icon-roles">🔐</span>角色权限</h3>
			<p>角色定义、权限点编码（域:操作）、RBAC 策略</p>
		</a>
		<a href="/admin/orgs?token=` + token + `" class="nav-card">
			<h3><span class="nav-icon icon-org">🏢</span>组织管理</h3>
			<p>组织树、成员归属、域间协调</p>
		</a>
		<a href="/admin/audit-logs?token=` + token + `" class="nav-card">
			<h3><span class="nav-icon icon-audit">📋</span>审计日志</h3>
			<p>全量操作审计、按用户/时间/操作类型检索（仅 2 admin 可读）</p>
		</a>
		<a href="/admin/configs?token=` + token + `" class="nav-card">
			<h3><span class="nav-icon icon-config">⚙️</span>系统配置</h3>
			<p>系统参数、环境配置、功能开关</p>
		</a>
		<a href="/admin/services?token=` + token + `" class="nav-card">
			<h3><span class="nav-icon icon-config">🔌</span>服务注册</h3>
			<p>MBS/BOS/OAS 服务注册、健康检查、API 密钥</p>
		</a>
	</div>

	<div class="section-title">域管理（L2 · XAM）</div>
	<div class="nav-grid">
		<a href="/admin/domains/TAM?token=` + token + `" class="nav-card">
			<h3><span class="nav-icon icon-domains">💻</span>TAM 技术域</h3>
			<p>技术域账号、组织、审计（域级管理视图）</p>
		</a>
		<a href="/admin/domains/HAM?token=` + token + `" class="nav-card">
			<h3><span class="nav-icon icon-domains">👔</span>HAM 人资云</h3>
			<p>人资域账号、组织、审计（域级管理视图）</p>
		</a>
		<a href="/admin/domains/YAM?token=` + token + `" class="nav-card">
			<h3><span class="nav-icon icon-domains">🎯</span>YAM 智场域</h3>
			<p>智场域账号、组织、审计（域级管理视图）</p>
		</a>
	</div>

	<div class="section-title">治理看板（L0 · 只读）</div>
	<div class="nav-grid">
		<a href="/governance?token=` + token + `" class="nav-card">
			<h3><span class="nav-icon icon-config">📊</span>治理总览</h3>
			<p>战略审批、所有权视图、业务系统跳转（O*M 只读）</p>
		</a>
	</div>
</div>
</body>
</html>`
}

// overviewPageHTML returns the OU governance overview dashboard HTML.
// overviewPageHTML returns the OU governance overview dashboard HTML.
func overviewPageHTML(username, oasEnv, token string) string {
	return `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>OU 治理看板 - OAS Console</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f3f4f6;color:#111827}
.header{background:#fff;border-bottom:1px solid #e5e7eb;padding:16px 24px;display:flex;align-items:center;justify-content:space-between}
.header h1{font-size:18px;font-weight:600}
.user-info{font-size:13px;color:#6b7280}
.container{max-width:1200px;margin:24px auto;padding:0 24px}
.env-badge{display:inline-block;padding:4px 12px;border-radius:9999px;font-size:12px;font-weight:600;margin-left:12px}
.env-DEV{background:#dbeafe;color:#1e40af}
.env-BETA{background:#fef3c7;color:#92400e}
.env-RC{background:#e0e7ff;color:#3730a3}
.env-PROD{background:#fee2e2;color:#991b1b}
.stats-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:16px;margin-bottom:32px}
.stat-card{background:#fff;border-radius:12px;box-shadow:0 1px 3px rgba(0,0,0,.1);padding:20px}
.stat-card h3{font-size:13px;color:#6b7280;font-weight:500;margin-bottom:8px}
.stat-card .value{font-size:32px;font-weight:700;color:#111827}
.stat-card .sub{font-size:12px;color:#9ca3af;margin-top:4px}
.section{background:#fff;border-radius:12px;box-shadow:0 1px 3px rgba(0,0,0,.1);padding:24px;margin-bottom:24px}
.section h2{font-size:16px;font-weight:600;margin-bottom:16px;display:flex;align-items:center;gap:8px}
.domain-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(120px,1fr));gap:12px}
.domain-item{background:#f9fafb;border-radius:8px;padding:12px;text-align:center}
.domain-item .code{font-size:14px;font-weight:600;color:#374151}
.domain-item .count{font-size:24px;font-weight:700;color:#2563eb;margin-top:4px}
.audit-list{list-style:none}
.audit-list li{padding:12px 0;border-bottom:1px solid #f3f4f6;font-size:13px}
.audit-list li:last-child{border-bottom:none}
.audit-list .time{color:#9ca3af;font-size:12px}
.audit-list .action{font-weight:500;color:#374151}
.audit-list .user{color:#6b7280}
.breadcrumb{font-size:13px;color:#6b7280;margin-bottom:16px}
.breadcrumb a{color:#2563eb;text-decoration:none}
</style>
</head>
<body>
<div class="header">
	<h1>OU 治理看板 <span class="env-badge env-` + oasEnv + `">` + oasEnv + `</span></h1>
	<div class="user-info">` + username + ` | <a href="/admin?token=` + token + `" style="color:#2563eb;text-decoration:none">返回 Console</a></div>
</div>
<div class="container">
	<div class="breadcrumb"><a href="/admin?token=` + token + `">Console</a> / 治理看板</div>
	
	<div class="stats-grid" id="stats">
		<div class="stat-card"><h3>加载中...</h3></div>
	</div>
	
	<div class="section">
		<h2>🌐 域分布</h2>
		<div class="domain-grid" id="domains">
			<div class="domain-item"><div class="code">加载中...</div></div>
		</div>
	</div>
	
	<div class="section" id="audit-section" style="display:none">
		<h2>📋 近期审计（仅 2 admin 可见）</h2>
		<ul class="audit-list" id="audits"></ul>
	</div>
</div>

<script>
var token = '` + token + `';
var isOUAU = ('` + username + `' === 'oas-ou-admin' || '` + username + `' === 'oas-au-admin');

fetch('/api/v1/admin/dashboard/stats', {
	headers: {'Authorization': 'Bearer ' + token}
}).then(function(r){ return r.json(); }).then(function(data){
	if(data.code !== 200){
		document.getElementById('stats').innerHTML = '<div class="stat-card"><h3>加载失败</h3></div>';
		return;
	}
	var d = data.data;
	document.getElementById('stats').innerHTML = '<div class="stat-card"><h3>启用账号</h3><div class="value">'+d.users_total+'</div><div class="sub">active users</div></div><div class="stat-card"><h3>组织总数</h3><div class="value">'+d.orgs_total+'</div><div class="sub">organizations</div></div><div class="stat-card"><h3>角色总数</h3><div class="value">'+d.roles_total+'</div><div class="sub">roles</div></div>';
	
	if(d.domain_distribution && d.domain_distribution.length > 0){
		document.getElementById('domains').innerHTML = d.domain_distribution.map(function(item){
			return '<div class="domain-item"><div class="code">'+(item.domain || '(未分配)')+'</div><div class="count">'+item.count+'</div></div>';
		}).join('');
	}else{
		document.getElementById('domains').innerHTML = '<div class="domain-item"><div class="code">暂无数据</div></div>';
	}
	
	if(isOUAU && d.recent_audits && d.recent_audits.length > 0){
		document.getElementById('audit-section').style.display = 'block';
		document.getElementById('audits').innerHTML = d.recent_audits.map(function(a){
			return '<li><span class="time">'+new Date(a.created_at).toLocaleString('zh-CN')+'</span> <span class="action">'+a.action+'</span> by <span class="user">'+(a.username || 'system')+'</span>'+(a.domain ? ' ['+a.domain+']' : '')+'</li>';
		}).join('');
	}
}).catch(function(err){
	document.getElementById('stats').innerHTML = '<div class="stat-card"><h3>加载失败: ' + err.message + '</h3></div>';
});
</script>
</body>
</html>`
}

// approvalsPageHTML returns the approvals workbench page HTML (whitelist A: OU/AU/OAM, OAM read-only).
func approvalsPageHTML(username, oasEnv, token string) string {
	isOUAU := username == "oas-ou-admin" || username == "oas-au-admin"
	canWrite := "false"
	if isOUAU {
		canWrite = "true"
	}

	return `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>战略审批工作台 - OAS Console</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f3f4f6;color:#111827}
.header{background:#1e40af;color:#fff;padding:16px 24px;display:flex;justify-content:space-between;align-items:center}
.header h1{font-size:20px;font-weight:600}
.header .env{background:rgba(255,255,255,0.2);padding:4px 12px;border-radius:12px;font-size:13px}
.container{max-width:1200px;margin:24px auto;padding:0 24px}
.card{background:#fff;border-radius:8px;box-shadow:0 1px 3px rgba(0,0,0,0.1);padding:20px;margin-bottom:20px}
.card h2{font-size:16px;font-weight:600;margin-bottom:16px;color:#1e40af}
.btn{padding:8px 16px;border:none;border-radius:6px;cursor:pointer;font-size:14px;font-weight:500;transition:all 0.2s}
.btn-primary{background:#1e40af;color:#fff}
.btn-primary:hover{background:#1e3a8a}
.btn-success{background:#059669;color:#fff}
.btn-success:hover{background:#047857}
.btn-danger{background:#dc2626;color:#fff}
.btn-danger:hover{background:#b91c1c}
.btn-warning{background:#d97706;color:#fff}
.btn-warning:hover{background:#b45309}
.btn-sm{padding:4px 10px;font-size:12px}
table{width:100%;border-collapse:collapse}
th,td{padding:12px;text-align:left;border-bottom:1px solid #e5e7eb}
th{background:#f9fafb;font-weight:600;font-size:13px;color:#6b7280}
td{font-size:14px}
.status{padding:4px 10px;border-radius:12px;font-size:12px;font-weight:500;display:inline-block}
.status-pending{background:#fef3c7;color:#92400e}
.status-approved{background:#d1fae5;color:#065f46}
.status-rejected{background:#fee2e2;color:#991b1b}
.status-executed{background:#dbeafe;color:#1e40af}
.modal{display:none;position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.5);z-index:1000;justify-content:center;align-items:center}
.modal.active{display:flex}
.modal-content{background:#fff;border-radius:8px;padding:24px;max-width:500px;width:90%;max-height:90vh;overflow-y:auto}
.modal-content h3{font-size:18px;font-weight:600;margin-bottom:16px}
.form-group{margin-bottom:16px}
.form-group label{display:block;font-size:14px;font-weight:500;margin-bottom:6px;color:#374151}
.form-group input,.form-group select,.form-group textarea{width:100%;padding:8px 12px;border:1px solid #d1d5db;border-radius:6px;font-size:14px}
.form-group textarea{resize:vertical;min-height:80px}
.actions{display:flex;gap:8px;flex-wrap:wrap}
.readonly-notice{background:#fef3c7;border:1px solid #fbbf24;border-radius:6px;padding:12px;margin-bottom:16px;color:#92400e;font-size:14px}
.nav{display:flex;gap:16px;margin-bottom:24px}
.nav a{color:#6b7280;text-decoration:none;font-size:14px;font-weight:500;padding:8px 16px;border-radius:6px}
.nav a:hover{background:#f3f4f6;color:#111827}
.nav a.active{background:#1e40af;color:#fff}
</style>
</head>
<body>
<div class="header">
  <h1>战略审批工作台</h1>
  <div class="env">` + oasEnv + `</div>
</div>
<div class="container">
  <div class="nav">
    <a href="/admin/overview?token=` + token + `">治理看板</a>
    <a href="/admin/approvals?token=` + token + `" class="active">审批工作台</a>
    <a href="/admin/audit-logs?token=` + token + `">审计日志</a>
    <a href="/admin/users?token=` + token + `">用户管理</a>
    <a href="/admin/roles?token=` + token + `">角色权限</a>
    <a href="/admin/orgs?token=` + token + `">组织管理</a>
  </div>
  
  ` + func() string {
		if !isOUAU {
			return `<div class="readonly-notice">您以只读身份访问（OAM），仅 OU/AU 管理员可发起/审批操作。</div>`
		}
		return ""
	}() + `
  
  <div class="card">
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
      <h2>审批单列表</h2>
      ` + func() string {
		if isOUAU {
			return `<button class="btn btn-primary" onclick="showCreateModal()">+ 发起审批</button>`
		}
		return ""
	}() + `
    </div>
    <table id="approvalsTable">
      <thead>
        <tr>
          <th>ID</th>
          <th>标题</th>
          <th>类型</th>
          <th>状态</th>
          <th>发起人</th>
          <th>创建时间</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody id="approvalsBody">
        <tr><td colspan="7" style="text-align:center;padding:40px;color:#9ca3af">加载中...</td></tr>
      </tbody>
    </table>
  </div>
</div>

<div id="createModal" class="modal">
  <div class="modal-content">
    <h3>发起战略审批</h3>
    <div class="form-group">
      <label>标题 *</label>
      <input type="text" id="approvalTitle" placeholder="例：删除 YAM 域组织">
    </div>
    <div class="form-group">
      <label>类型 *</label>
      <select id="approvalType">
        <option value="high_privilege">高权限授予</option>
        <option value="org_delete">组织删除</option>
        <option value="key_operation">密钥操作</option>
      </select>
    </div>
    <div class="form-group">
      <label>描述</label>
      <textarea id="approvalDesc" placeholder="详细说明审批原因和背景"></textarea>
    </div>
    <div style="display:flex;gap:8px;justify-content:flex-end">
      <button class="btn" onclick="closeCreateModal()">取消</button>
      <button class="btn btn-primary" onclick="createApproval()">提交</button>
    </div>
  </div>
</div>

<div id="detailModal" class="modal">
  <div class="modal-content">
    <h3>审批单详情</h3>
    <div id="detailContent"></div>
    <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:16px">
      <button class="btn" onclick="closeDetailModal()">关闭</button>
    </div>
  </div>
</div>

<script>
const TOKEN = '` + token + `';
const CAN_WRITE = ` + canWrite + `;

function loadApprovals() {
  fetch('/api/v1/admin/approvals', {
    headers: { 'Authorization': 'Bearer ' + TOKEN }
  })
  .then(r => r.json())
  .then(data => {
    const tbody = document.getElementById('approvalsBody');
    if (!data.data || data.data.length === 0) {
      tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;padding:40px;color:#9ca3af">暂无审批单</td></tr>';
      return;
    }
    tbody.innerHTML = data.data.map(a => ` + "`" + `
      <tr>
        <td>${a.id}</td>
        <td>${a.title}</td>
        <td>${typeLabel(a.type)}</td>
        <td><span class="status status-${a.status}">${statusLabel(a.status)}</span></td>
        <td>${a.requester_id}</td>
        <td>${new Date(a.created_at).toLocaleString('zh-CN')}</td>
        <td class="actions">
          <button class="btn btn-sm" onclick="showDetail(${a.id})">详情</button>
          ${CAN_WRITE && a.status === 'pending' ? ` + "`" + `
            <button class="btn btn-sm btn-success" onclick="approve(${a.id})">通过</button>
            <button class="btn btn-sm btn-danger" onclick="reject(${a.id})">拒绝</button>
          ` + "`" + ` : ''}
          ${CAN_WRITE && a.status === 'approved' ? ` + "`" + `
            <button class="btn btn-sm btn-warning" onclick="execute(${a.id})">标记执行</button>
          ` + "`" + ` : ''}
        </td>
      </tr>
    ` + "`" + `).join('');
  });
}

function typeLabel(t) {
  const map = { high_privilege: '高权限', org_delete: '组织删除', key_operation: '密钥操作', federation: '联邦节点' };
  return map[t] || t;
}

function statusLabel(s) {
  const map = { pending: '待审批', approved: '已通过', rejected: '已拒绝', executed: '已执行' };
  return map[s] || s;
}

function showCreateModal() {
  document.getElementById('createModal').classList.add('active');
}

function closeCreateModal() {
  document.getElementById('createModal').classList.remove('active');
}

function createApproval() {
  const title = document.getElementById('approvalTitle').value;
  const type = document.getElementById('approvalType').value;
  const description = document.getElementById('approvalDesc').value;
  
  if (!title) { alert('请输入标题'); return; }
  
  fetch('/api/v1/admin/approvals', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + TOKEN },
    body: JSON.stringify({ title, type, description })
  })
  .then(r => r.json())
  .then(data => {
    if (data.code === 200) {
      alert('审批单已创建');
      closeCreateModal();
      loadApprovals();
    } else {
      alert('创建失败: ' + (data.message || '未知错误'));
    }
  });
}

function showDetail(id) {
  fetch('/api/v1/admin/approvals', {
    headers: { 'Authorization': 'Bearer ' + TOKEN }
  })
  .then(r => r.json())
  .then(data => {
    const a = data.data.find(x => x.id === id);
    if (!a) return;
    
    let html = ` + "`" + `
      <div style="margin-bottom:12px"><strong>ID:</strong> ${a.id}</div>
      <div style="margin-bottom:12px"><strong>标题:</strong> ${a.title}</div>
      <div style="margin-bottom:12px"><strong>类型:</strong> ${typeLabel(a.type)}</div>
      <div style="margin-bottom:12px"><strong>状态:</strong> <span class="status status-${a.status}">${statusLabel(a.status)}</span></div>
      <div style="margin-bottom:12px"><strong>描述:</strong> ${a.description || '-'}</div>
      <div style="margin-bottom:12px"><strong>发起人 ID:</strong> ${a.requester_id}</div>
      <div style="margin-bottom:12px"><strong>审批人 ID:</strong> ${a.approver_id || '-'}</div>
      <div style="margin-bottom:12px"><strong>创建时间:</strong> ${new Date(a.created_at).toLocaleString('zh-CN')}</div>
      ${a.approved_at ? "<div style=\"margin-bottom:12px\"><strong>审批时间:</strong> " + new Date(a.approved_at).toLocaleString('zh-CN') + "</div>" : ''}
      ${a.executed_at ? "<div style=\"margin-bottom:12px\"><strong>执行时间:</strong> " + new Date(a.executed_at).toLocaleString('zh-CN') + "</div>" : ''}
      ${a.notes ? "<div style=\"margin-bottom:12px\"><strong>备注:</strong> " + a.notes + "</div>" : ''}
    ` + "`" + `;
    
    document.getElementById('detailContent').innerHTML = html;
    document.getElementById('detailModal').classList.add('active');
  });
}

function closeDetailModal() {
  document.getElementById('detailModal').classList.remove('active');
}

function approve(id) {
  if (!confirm('确认通过此审批？')) return;
  
  fetch("/api/v1/admin/approvals/" + id + "/approve", {
    method: 'PUT',
    headers: { 'Authorization': 'Bearer ' + TOKEN }
  })
  .then(r => r.json())
  .then(data => {
    if (data.code === 200) {
      alert('审批已通过');
      loadApprovals();
    } else {
      alert('操作失败: ' + (data.message || '未知错误'));
    }
  });
}

function reject(id) {
  const notes = prompt('请输入拒绝原因:');
  if (notes === null) return;
  
  fetch("/api/v1/admin/approvals/" + id + "/reject", {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + TOKEN },
    body: JSON.stringify({ notes })
  })
  .then(r => r.json())
  .then(data => {
    if (data.code === 200) {
      alert('审批已拒绝');
      loadApprovals();
    } else {
      alert('操作失败: ' + (data.message || '未知错误'));
    }
  });
}

function execute(id) {
  if (!confirm('确认标记此审批为已执行？')) return;
  
  fetch("/api/v1/admin/approvals/" + id + "/execute", {
    method: 'PUT',
    headers: { 'Authorization': 'Bearer ' + TOKEN }
  })
  .then(r => r.json())
  .then(data => {
    if (data.code === 200) {
      alert('已标记执行');
      loadApprovals();
    } else {
      alert('操作失败: ' + (data.message || '未知错误'));
    }
  });
}

loadApprovals();
</script>
</body>
</html>`
}

// ownershipPageHTML returns the ownership view page HTML (whitelist A).
func ownershipPageHTML(username string) string {
	return `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>所有权视图 - OAS Console</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif; background: #f5f5f5; color: #333; }
.header { background: #1a73e8; color: white; padding: 16px 24px; display: flex; justify-content: space-between; align-items: center; }
.header h1 { font-size: 20px; font-weight: 500; }
.header .user { font-size: 14px; opacity: 0.9; }
.container { max-width: 1200px; margin: 24px auto; padding: 0 24px; }
.card { background: white; border-radius: 8px; padding: 24px; margin-bottom: 24px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
.card h2 { font-size: 18px; margin-bottom: 16px; color: #1a73e8; }
.matrix-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
.domain-card { background: #f8f9fa; border: 1px solid #e0e0e0; border-radius: 6px; padding: 16px; }
.domain-card h3 { font-size: 16px; margin-bottom: 8px; color: #333; }
.domain-card .code { font-size: 12px; color: #666; margin-bottom: 8px; }
.domain-card .info { font-size: 14px; color: #555; margin-bottom: 4px; }
.service-table { width: 100%; border-collapse: collapse; }
.service-table th, .service-table td { padding: 12px; text-align: left; border-bottom: 1px solid #e0e0e0; }
.service-table th { background: #f8f9fa; font-weight: 500; color: #333; }
.service-table td { font-size: 14px; }
.status-badge { display: inline-block; padding: 2px 8px; border-radius: 12px; font-size: 12px; }
.status-active { background: #e6f4ea; color: #1e8e3e; }
.status-healthy { background: #e6f4ea; color: #1e8e3e; }
.status-inactive { background: #fce8e6; color: #d93025; }
.note { background: #fff3cd; border: 1px solid #ffc107; border-radius: 6px; padding: 16px; margin-bottom: 24px; font-size: 14px; color: #856404; }
.default-mapping { margin-top: 16px; }
.default-mapping table { width: 100%; border-collapse: collapse; }
.default-mapping th, .default-mapping td { padding: 10px; text-align: left; border-bottom: 1px solid #e0e0e0; }
.default-mapping th { background: #f8f9fa; font-weight: 500; }
</style>
</head>
<body>
<div class="header">
  <h1>🏛️ 所有权视图</h1>
  <div class="user">` + username + `</div>
</div>
<div class="container">
  <div id="note" class="note" style="display:none;"></div>
  
  <div class="card">
    <h2>📊 域所有权</h2>
    <div id="domains" class="matrix-grid"></div>
  </div>
  
  <div class="card">
    <h2>🔧 服务归属</h2>
    <table class="service-table">
      <thead>
        <tr><th>服务名</th><th>类型</th><th>版本</th><th>端点</th><th>域</th><th>状态</th></tr>
      </thead>
      <tbody id="services"></tbody>
    </table>
  </div>
  
  <div id="default-mapping" class="card" style="display:none;">
    <h2>📋 默认映射框架</h2>
    <div class="default-mapping">
      <table>
        <thead>
          <tr><th>域代码</th><th>域名</th><th>管理者</th><th>描述</th></tr>
        </thead>
        <tbody id="default-table"></tbody>
      </table>
    </div>
  </div>
</div>
<script>
const token = new URLSearchParams(location.search).get('token') || '';
const headers = token ? {'Authorization':'Bearer '+token} : {};

async function loadOwnership() {
  try {
    const res = await fetch('/api/v1/admin/ownership/matrix', {headers});
    const data = await res.json();
    
    if (data.code !== 200) {
      alert('加载失败: ' + data.message);
      return;
    }
    
    const matrix = data.data;
    
    // 显示备注
    if (matrix.note) {
      document.getElementById('note').textContent = matrix.note;
      document.getElementById('note').style.display = 'block';
    }
    
    // 渲染域所有权
    const domainsDiv = document.getElementById('domains');
    if (matrix.domains && matrix.domains.length > 0) {
      matrix.domains.forEach(d => {
        const card = document.createElement('div');
        card.className = 'domain-card';
        card.innerHTML = "
          <h3>" + (d.domain_name || d.domain_code) + "</h3>
          <div class="code">代码: " + d.domain_code + "</div>
          <div class="info">BOS: " + (d.bos_name || "-") + "</div>
          <div class="info">负责人: " + (d.owner_user_id || "-") + "</div>
          <div class="info">状态: <span class="status-badge status-" + d.status + "">" + d.status + "</span></div>
        ";
        domainsDiv.appendChild(card);
      });
    } else {
      domainsDiv.innerHTML = '<p style="color:#666;">暂无域注册数据</p>';
    }
    
    // 渲染服务归属
    const servicesTbody = document.getElementById('services');
    if (matrix.services && matrix.services.length > 0) {
      matrix.services.forEach(s => {
        const row = document.createElement('tr');
        row.innerHTML = "
          <td>" + s.service_name + "</td>
          <td>" + s.service_type + "</td>
          <td>" + (s.version || "-") + "</td>
          <td style="max-width:200px;overflow:hidden;text-overflow:ellipsis;">" + (s.endpoint || "-") + "</td>
          <td>" + (s.domain || "-") + "</td>
          <td><span class="status-badge status-" + s.status + "">" + s.status + "</span></td>
        ";
        servicesTbody.appendChild(row);
      });
    } else {
      servicesTbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:#666;">暂无服务注册数据</td></tr>';
    }
    
    // 渲染默认映射
    if (matrix.default_mapping) {
      document.getElementById('default-mapping').style.display = 'block';
      const defaultTable = document.getElementById('default-table');
      matrix.default_mapping.forEach(m => {
        const row = document.createElement('tr');
        row.innerHTML = "
          <td>" + m.domain + "</td>
          <td>" + m.name + "</td>
          <td>" + m.owner + "</td>
          <td>" + m.description + "</td>
        ";
        defaultTable.appendChild(row);
      });
    }
  } catch (err) {
    alert('加载失败: ' + err.message);
  }
}

loadOwnership();
</script>
</body>
</html>`
}

func adminAccountsPageHTML(username string) string {
	return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Admin 账号管理 - OAS Console</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
.container { max-width: 1200px; margin: 0 auto; }
h1 { color: #333; margin-bottom: 20px; }
.card { background: white; border-radius: 8px; padding: 20px; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 12px; text-align: left; border-bottom: 1px solid #eee; }
th { background: #f9f9f9; font-weight: 600; }
.btn { padding: 6px 12px; border: none; border-radius: 4px; cursor: pointer; font-size: 14px; }
.btn-primary { background: #007bff; color: white; }
.btn-danger { background: #dc3545; color: white; }
.btn-success { background: #28a745; color: white; }
.btn:hover { opacity: 0.9; }
.status-active { color: #28a745; }
.status-disabled { color: #dc3545; }
</style>
</head>
<body>
<div class="container">
<h1>Admin 账号管理</h1>
<div class="card">
<button class="btn btn-primary" onclick="showCreateDialog()">创建 Admin 账号</button>
</div>
<div class="card">
<table id="accountsTable">
<thead>
<tr>
  <th>ID</th>
  <th>用户名</th>
  <th>用户编码</th>
  <th>显示名</th>
  <th>角色</th>
  <th>状态</th>
  <th>操作</th>
</tr>
</thead>
<tbody id="accountsBody"></tbody>
</table>
</div>
</div>

<script>
const token = new URLSearchParams(window.location.search).get('token');

async function loadAccounts() {
  try {
    const res = await fetch('/api/v1/admin/admin-accounts', {
      headers: { 'Authorization': 'Bearer ' + token }
    });
    if (!res.ok) throw new Error('加载失败');
    const data = await res.json();
    const tbody = document.getElementById('accountsBody');
    tbody.innerHTML = '';
    data.forEach(function(acc) {
      const row = document.createElement('tr');
      row.innerHTML = 
        '<td>' + acc.id + '</td>' +
        '<td>' + acc.username + '</td>' +
        '<td>' + acc.user_code + '</td>' +
        '<td>' + (acc.display_name || '-') + '</td>' +
        '<td>' + acc.role_code + '</td>' +
        '<td class="status-' + acc.status + '">' + acc.status + '</td>' +
        '<td>' +
          (acc.status === 'active' 
            ? '<button class="btn btn-danger" onclick="disableAccount(' + acc.id + ')">禁用</button>'
            : '<button class="btn btn-success" onclick="enableAccount(' + acc.id + ')">启用</button>') +
          ' <button class="btn btn-primary" onclick="resetPassword(' + acc.id + ')">重置密码</button>' +
        '</td>';
      tbody.appendChild(row);
    });
  } catch (err) {
    alert('加载失败: ' + err.message);
  }
}

function showCreateDialog() {
  const username = prompt('用户名:');
  if (!username) return;
  const password = prompt('密码:');
  if (!password) return;
  const displayName = prompt('显示名:');
  const roleCode = prompt('角色 (OU/AU):');
  if (roleCode !== 'OU' && roleCode !== 'AU') {
    alert('角色必须是 OU 或 AU');
    return;
  }
  
  fetch('/api/v1/admin/admin-accounts', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + token
    },
    body: JSON.stringify({
      username: username,
      password: password,
      display_name: displayName,
      role_code: roleCode
    })
  }).then(function(res) {
    if (!res.ok) throw new Error('创建失败');
    alert('创建成功');
    loadAccounts();
  }).catch(function(err) {
    alert('创建失败: ' + err.message);
  });
}

function disableAccount(id) {
  if (!confirm('确认禁用此账号？')) return;
  fetch('/api/v1/admin/admin-accounts/' + id + '/disable', {
    method: 'PUT',
    headers: { 'Authorization': 'Bearer ' + token }
  }).then(function(res) {
    if (!res.ok) throw new Error('禁用失败');
    alert('禁用成功');
    loadAccounts();
  }).catch(function(err) {
    alert('禁用失败: ' + err.message);
  });
}

function enableAccount(id) {
  fetch('/api/v1/admin/admin-accounts/' + id + '/enable', {
    method: 'PUT',
    headers: { 'Authorization': 'Bearer ' + token }
  }).then(function(res) {
    if (!res.ok) throw new Error('启用失败');
    alert('启用成功');
    loadAccounts();
  }).catch(function(err) {
    alert('启用失败: ' + err.message);
  });
}

function resetPassword(id) {
  const newPassword = prompt('新密码:');
  if (!newPassword) return;
  
  fetch('/api/v1/admin/admin-accounts/' + id + '/reset-password', {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + token
    },
    body: JSON.stringify({ new_password: newPassword })
  }).then(function(res) {
    if (!res.ok) throw new Error('重置失败');
    alert('密码重置成功');
  }).catch(function(err) {
    alert('重置失败: ' + err.message);
  });
}

loadAccounts();
</script>
</body>
</html>`
}

func systemConfigPageHTML(username string) string {
	return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>系统配置 - OAS Console</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
.container { max-width: 800px; margin: 0 auto; }
h1 { color: #333; margin-bottom: 20px; }
.card { background: white; border-radius: 8px; padding: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
.config-item { margin-bottom: 15px; padding: 10px; background: #f9f9f9; border-radius: 4px; }
.config-label { font-weight: 600; color: #555; margin-bottom: 5px; }
.config-value { color: #333; font-family: monospace; }
</style>
</head>
<body>
<div class="container">
<h1>系统配置（只读）</h1>
<div class="card" id="configCard">
<p>加载中...</p>
</div>
</div>

<script>
const token = new URLSearchParams(window.location.search).get('token');

async function loadConfig() {
  try {
    const res = await fetch('/api/v1/admin/system-config', {
      headers: { 'Authorization': 'Bearer ' + token }
    });
    if (!res.ok) throw new Error('加载失败');
    const data = await res.json();
    
    const card = document.getElementById('configCard');
    card.innerHTML = '';
    
    const items = [
      { label: '应用环境', key: 'app_env' },
      { label: 'OAS 环境', key: 'oas_env' },
      { label: '数据库驱动', key: 'db_driver' },
      { label: 'Go 版本', key: 'go_version' },
      { label: '构建时间', key: 'build_time' },
      { label: 'Git Commit', key: 'git_commit' }
    ];
    
    items.forEach(function(item) {
      const div = document.createElement('div');
      div.className = 'config-item';
      div.innerHTML = 
        '<div class="config-label">' + item.label + '</div>' +
        '<div class="config-value">' + (data[item.key] || '-') + '</div>';
      card.appendChild(div);
    });
  } catch (err) {
    alert('加载失败: ' + err.message);
  }
}

loadConfig();
</script>
</body>
</html>`
}

func apiKeysPageHTML(username string) string {
	return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>API Key 管理 - OAS Console</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
.container { max-width: 1200px; margin: 0 auto; }
h1 { color: #333; margin-bottom: 20px; }
.card { background: white; border-radius: 8px; padding: 20px; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 12px; text-align: left; border-bottom: 1px solid #eee; }
th { background: #f9f9f9; font-weight: 600; }
.btn { padding: 6px 12px; border: none; border-radius: 4px; cursor: pointer; font-size: 14px; margin-right: 5px; }
.btn-primary { background: #007bff; color: white; }
.btn-danger { background: #dc3545; color: white; }
.btn-success { background: #28a745; color: white; }
.btn-warning { background: #ffc107; color: #333; }
.btn:hover { opacity: 0.9; }
.status-active { color: #28a745; font-weight: 600; }
.status-disabled { color: #dc3545; font-weight: 600; }
.key-display { font-family: monospace; background: #f8f9fa; padding: 8px; border-radius: 4px; word-break: break-all; }
</style>
</head>
<body>
<div class="container">
<h1>API Key 管理</h1>
<div class="card">
<button class="btn btn-primary" onclick="showCreateDialog()">创建 API Key</button>
</div>
<div class="card">
<table id="keysTable">
<thead>
<tr>
  <th>ID</th>
  <th>名称</th>
  <th>前缀</th>
  <th>权限范围</th>
  <th>过期时间</th>
  <th>状态</th>
  <th>创建者</th>
  <th>操作</th>
</tr>
</thead>
<tbody id="keysBody"></tbody>
</table>
</div>
</div>

<script>
const token = new URLSearchParams(window.location.search).get('token');

async function loadKeys() {
  try {
    const res = await fetch('/api/v1/admin/api-keys', {
      headers: { 'Authorization': 'Bearer ' + token }
    });
    if (!res.ok) throw new Error('加载失败');
    const data = await res.json();
    const tbody = document.getElementById('keysBody');
    tbody.innerHTML = '';
    data.forEach(function(key) {
      const row = document.createElement('tr');
      row.innerHTML = 
        '<td>' + key.id + '</td>' +
        '<td>' + key.key_name + '</td>' +
        '<td class="key-display">' + key.key_prefix + '</td>' +
        '<td>' + (key.scopes || '-') + '</td>' +
        '<td>' + (key.expires_at ? new Date(key.expires_at).toLocaleString() : '永不过期') + '</td>' +
        '<td class="status-' + key.status + '">' + key.status + '</td>' +
        '<td>' + key.created_by + '</td>' +
        '<td>' +
          '<button class="btn btn-warning" onclick="rotateKey(' + key.id + ')">轮换</button>' +
          (key.status === 'active' 
            ? '<button class="btn btn-danger" onclick="disableKey(' + key.id + ')">禁用</button>'
            : '<button class="btn btn-success" onclick="enableKey(' + key.id + ')">启用</button>') +
          '<button class="btn btn-danger" onclick="deleteKey(' + key.id + ')">删除</button>' +
        '</td>';
      tbody.appendChild(row);
    });
  } catch (err) {
    alert('加载失败: ' + err.message);
  }
}

function showCreateDialog() {
  const keyName = prompt('密钥名称:');
  if (!keyName) return;
  const scopes = prompt('权限范围 (如: read:all,write:domain1):');
  const expiresAt = prompt('过期时间 (ISO 格式，留空表示永不过期):');
  
  const body = { key_name: keyName, scopes: scopes };
  if (expiresAt) body.expires_at = expiresAt;
  
  fetch('/api/v1/admin/api-keys', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + token
    },
    body: JSON.stringify(body)
  }).then(function(res) {
    if (!res.ok) throw new Error('创建失败');
    return res.json();
  }).then(function(data) {
    alert('创建成功！请立即保存密钥（只显示一次）：\n\n' + data.full_key);
    loadKeys();
  }).catch(function(err) {
    alert('创建失败: ' + err.message);
  });
}

function rotateKey(id) {
  if (!confirm('确认轮换此密钥？旧密钥将立即失效。')) return;
  
  fetch('/api/v1/admin/api-keys/' + id + '/rotate', {
    method: 'PUT',
    headers: { 'Authorization': 'Bearer ' + token }
  }).then(function(res) {
    if (!res.ok) throw new Error('轮换失败');
    return res.json();
  }).then(function(data) {
    alert('轮换成功！新密钥（只显示一次）：\n\n' + data.full_key + '\n\n旧密钥 ID: ' + data.old_key_id + ' 已禁用。');
    loadKeys();
  }).catch(function(err) {
    alert('轮换失败: ' + err.message);
  });
}

function disableKey(id) {
  if (!confirm('确认禁用此密钥？')) return;
  
  fetch('/api/v1/admin/api-keys/' + id + '/disable', {
    method: 'PUT',
    headers: { 'Authorization': 'Bearer ' + token }
  }).then(function(res) {
    if (!res.ok) throw new Error('禁用失败');
    alert('禁用成功');
    loadKeys();
  }).catch(function(err) {
    alert('禁用失败: ' + err.message);
  });
}

function enableKey(id) {
  fetch('/api/v1/admin/api-keys/' + id + '/enable', {
    method: 'PUT',
    headers: { 'Authorization': 'Bearer ' + token }
  }).then(function(res) {
    if (!res.ok) throw new Error('启用失败');
    alert('启用成功');
    loadKeys();
  }).catch(function(err) {
    alert('启用失败: ' + err.message);
  });
}

function deleteKey(id) {
  if (!confirm('确认删除此密钥？此操作不可恢复。')) return;
  
  fetch('/api/v1/admin/api-keys/' + id, {
    method: 'DELETE',
    headers: { 'Authorization': 'Bearer ' + token }
  }).then(function(res) {
    if (!res.ok) throw new Error('删除失败');
    alert('删除成功');
    loadKeys();
  }).catch(function(err) {
    alert('删除失败: ' + err.message);
  });
}

loadKeys();
</script>
</body>
</html>`
}

func federationNodesPageHTML(username string) string {
	return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>联邦节点管理 - OAS Console</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
.container { max-width: 1200px; margin: 0 auto; }
h1 { color: #333; margin-bottom: 20px; }
.card { background: white; border-radius: 8px; padding: 20px; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 12px; text-align: left; border-bottom: 1px solid #eee; }
th { background: #f9f9f9; font-weight: 600; }
.btn { padding: 6px 12px; border: none; border-radius: 4px; cursor: pointer; font-size: 14px; margin-right: 5px; }
.btn-primary { background: #007bff; color: white; }
.btn-danger { background: #dc3545; color: white; }
.btn-success { background: #28a745; color: white; }
.btn-warning { background: #ffc107; color: #333; }
.btn:hover { opacity: 0.9; }
.status-active { color: #28a745; font-weight: 600; }
.status-suspended { color: #ffc107; font-weight: 600; }
.status-inactive { color: #6c757d; font-weight: 600; }
.trust-basic { color: #6c757d; }
.trust-standard { color: #007bff; }
.trust-full { color: #28a745; font-weight: 600; }
</style>
</head>
<body>
<div class="container">
<h1>联邦节点管理</h1>
<div class="card">
<button class="btn btn-primary" onclick="showCreateDialog()">注册联邦节点</button>
</div>
<div class="card">
<table id="nodesTable">
<thead>
<tr>
  <th>ID</th>
  <th>节点名称</th>
  <th>节点ID</th>
  <th>信任级别</th>
  <th>状态</th>
  <th>端点</th>
  <th>创建者</th>
  <th>操作</th>
</tr>
</thead>
<tbody id="nodesBody"></tbody>
</table>
</div>
</div>

<script>
const token = new URLSearchParams(window.location.search).get('token');

async function loadNodes() {
  try {
    const res = await fetch('/api/v1/admin/federation-nodes', {
      headers: { 'Authorization': 'Bearer ' + token }
    });
    if (!res.ok) throw new Error('加载失败');
    const data = await res.json();
    const tbody = document.getElementById('nodesBody');
    tbody.innerHTML = '';
    data.forEach(function(node) {
      const row = document.createElement('tr');
      row.innerHTML = 
        '<td>' + node.id + '</td>' +
        '<td>' + node.node_name + '</td>' +
        '<td>' + node.node_id + '</td>' +
        '<td class="trust-' + node.trust_level + '">' + node.trust_level + '</td>' +
        '<td class="status-' + node.status + '">' + node.status + '</td>' +
        '<td>' + (node.endpoint || '-') + '</td>' +
        '<td>' + node.created_by + '</td>' +
        '<td>' +
          '<button class="btn btn-primary" onclick="editNode(' + node.id + ')">编辑</button>' +
          (node.status === 'active' 
            ? '<button class="btn btn-warning" onclick="suspendNode(' + node.id + ')">暂停</button>'
            : '<button class="btn btn-success" onclick="activateNode(' + node.id + ')">激活</button>') +
          '<button class="btn btn-danger" onclick="deleteNode(' + node.id + ')">删除</button>' +
        '</td>';
      tbody.appendChild(row);
    });
  } catch (err) {
    alert('加载失败: ' + err.message);
  }
}

function showCreateDialog() {
  const nodeName = prompt('节点名称:');
  if (!nodeName) return;
  const nodeId = prompt('节点ID (唯一标识):');
  if (!nodeId) return;
  const trustLevel = prompt('信任级别 (basic/standard/full):');
  if (trustLevel !== 'basic' && trustLevel !== 'standard' && trustLevel !== 'full') {
    alert('信任级别必须是 basic、standard 或 full');
    return;
  }
  const publicKey = prompt('公钥 (PEM 格式):');
  const endpoint = prompt('端点 URL:');
  const capabilities = prompt('能力描述:');
  
  fetch('/api/v1/admin/federation-nodes', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + token
    },
    body: JSON.stringify({
      node_name: nodeName,
      node_id: nodeId,
      trust_level: trustLevel,
      public_key: publicKey,
      endpoint: endpoint,
      capabilities: capabilities
    })
  }).then(function(res) {
    if (!res.ok) throw new Error('创建失败');
    alert('注册成功');
    loadNodes();
  }).catch(function(err) {
    alert('创建失败: ' + err.message);
  });
}

function editNode(id) {
  const nodeName = prompt('节点名称:');
  if (!nodeName) return;
  const trustLevel = prompt('信任级别 (basic/standard/full):');
  if (trustLevel && trustLevel !== 'basic' && trustLevel !== 'standard' && trustLevel !== 'full') {
    alert('信任级别必须是 basic、standard 或 full');
    return;
  }
  const publicKey = prompt('公钥 (PEM 格式):');
  const endpoint = prompt('端点 URL:');
  const capabilities = prompt('能力描述:');
  
  fetch('/api/v1/admin/federation-nodes/' + id, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + token
    },
    body: JSON.stringify({
      node_name: nodeName,
      trust_level: trustLevel,
      public_key: publicKey,
      endpoint: endpoint,
      capabilities: capabilities
    })
  }).then(function(res) {
    if (!res.ok) throw new Error('更新失败');
    alert('更新成功');
    loadNodes();
  }).catch(function(err) {
    alert('更新失败: ' + err.message);
  });
}

function suspendNode(id) {
  if (!confirm('确认暂停此节点？')) return;
  
  fetch('/api/v1/admin/federation-nodes/' + id + '/suspend', {
    method: 'PUT',
    headers: { 'Authorization': 'Bearer ' + token }
  }).then(function(res) {
    if (!res.ok) throw new Error('暂停失败');
    alert('暂停成功');
    loadNodes();
  }).catch(function(err) {
    alert('暂停失败: ' + err.message);
  });
}

function activateNode(id) {
  fetch('/api/v1/admin/federation-nodes/' + id + '/activate', {
    method: 'PUT',
    headers: { 'Authorization': 'Bearer ' + token }
  }).then(function(res) {
    if (!res.ok) throw new Error('激活失败');
    alert('激活成功');
    loadNodes();
  }).catch(function(err) {
    alert('激活失败: ' + err.message);
  });
}

function deleteNode(id) {
  if (!confirm('确认删除此节点？此操作不可恢复。')) return;
  
  fetch('/api/v1/admin/federation-nodes/' + id, {
    method: 'DELETE',
    headers: { 'Authorization': 'Bearer ' + token }
  }).then(function(res) {
    if (!res.ok) throw new Error('删除失败');
    alert('删除成功');
    loadNodes();
  }).catch(function(err) {
    alert('删除失败: ' + err.message);
  });
}

loadNodes();
</script>
</body>
</html>`
}

// oauthClientsPageHTML returns the OAuth clients management page HTML (whitelist B: only OU/AU).
func oauthClientsPageHTML(username string) string {
	return `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>OAS OAuth 客户端管理</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f3f4f6;color:#111827}
.header{background:#fff;border-bottom:1px solid #e5e7eb;padding:16px 24px;display:flex;align-items:center;justify-content:space-between}
.header h1{font-size:18px;font-weight:600}
.user-info{font-size:13px;color:#6b7280}
.container{max-width:1200px;margin:24px auto;padding:0 24px}
.card{background:#fff;border-radius:10px;box-shadow:0 1px 2px rgba(0,0,0,.06);padding:24px;margin-bottom:20px}
.card h2{font-size:16px;font-weight:600;margin-bottom:16px}
.btn{padding:8px 16px;border:none;border-radius:6px;font-size:13px;font-weight:500;cursor:pointer;transition:all .15s}
.btn-primary{background:#3b82f6;color:#fff}.btn-primary:hover{background:#2563eb}
.btn-danger{background:#ef4444;color:#fff}.btn-danger:hover{background:#dc2626}
.btn-secondary{background:#e5e7eb;color:#374151}.btn-secondary:hover{background:#d1d5db}
.btn-sm{padding:4px 10px;font-size:12px}
table{width:100%;border-collapse:collapse}
th,td{padding:10px 12px;text-align:left;border-bottom:1px solid #e5e7eb;font-size:13px}
th{font-weight:600;color:#374151;background:#f9fafb}
tr:hover{background:#f9fafb}
.status-active{color:#059669;font-weight:500}
.status-disabled{color:#dc2626;font-weight:500}
.modal-overlay{display:none;position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.5);z-index:100;align-items:center;justify-content:center}
.modal-overlay.active{display:flex}
.modal{background:#fff;border-radius:12px;padding:24px;width:90%;max-width:500px;max-height:90vh;overflow-y:auto}
.modal h3{font-size:16px;font-weight:600;margin-bottom:16px}
.form-group{margin-bottom:14px}
.form-group label{display:block;font-size:13px;font-weight:500;margin-bottom:4px;color:#374151}
.form-group input,.form-group select,.form-group textarea{width:100%;padding:8px 12px;border:1px solid #d1d5db;border-radius:6px;font-size:13px}
.form-group textarea{resize:vertical;min-height:60px}
.form-actions{display:flex;gap:8px;justify-content:flex-end;margin-top:16px}
.secret-display{background:#f0fdf4;border:1px solid #86efac;border-radius:6px;padding:12px;margin-top:12px;font-family:monospace;font-size:12px;word-break:break-all}
.empty{text-align:center;padding:40px;color:#9ca3af}
</style>
</head>
<body>
<div class="header">
  <h1>知味 OAS · OAuth 客户端管理</h1>
  <span class="user-info">` + username + `</span>
</div>
<div class="container">
  <div class="card">
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
      <h2 style="margin:0">客户端列表</h2>
      <button class="btn btn-primary" onclick="showCreateModal()">+ 新建客户端</button>
    </div>
    <table>
      <thead><tr><th>Client ID</th><th>名称</th><th>Redirect URI</th><th>Scopes</th><th>状态</th><th>操作</th></tr></thead>
      <tbody id="clientTable"><tr><td colspan="6" class="empty">加载中...</td></tr></tbody>
    </table>
  </div>
</div>

<div class="modal-overlay" id="createModal">
  <div class="modal">
    <h3>新建 OAuth 客户端</h3>
    <div class="form-group"><label>Client ID（留空自动生成）</label><input id="newClientId" placeholder="自动生成"></div>
    <div class="form-group"><label>客户端名称</label><input id="newClientName" placeholder="如：My App"></div>
    <div class="form-group"><label>Redirect URI</label><input id="newRedirectUri" placeholder="https://app.example.com/callback"></div>
    <div class="form-group"><label>Scopes（空格分隔）</label><input id="newScopes" value="openid profile email"></div>
    <div class="form-actions">
      <button class="btn btn-secondary" onclick="hideCreateModal()">取消</button>
      <button class="btn btn-primary" onclick="createClient()">创建</button>
    </div>
    <div id="secretResult" style="display:none"></div>
  </div>
</div>

<script>
const API = '';
let allClients = [];

async function loadClients() {
  const r = await fetch(API + '/api/v1/admin/oauth-clients');
  const d = await r.json();
  if (d.code !== 200) { alert(d.message || 'load failed'); return; }
  allClients = d.data || [];
  renderClients(allClients);
}

function renderClients(clients) {
  if (!clients || clients.length === 0) {
    document.getElementById('clientTable').innerHTML = '<tr><td colspan="6" class="empty">暂无客户端</td></tr>';
    return;
  }
  let html = '';
  for (const c of clients) {
    const statusClass = c.status === 'active' ? 'status-active' : 'status-disabled';
    const statusText = c.status === 'active' ? '启用' : '禁用';
    html += '<tr><td><code>' + c.client_id + '</code></td>' +
      '<td>' + (c.client_name || '-') + '</td>' +
      '<td>' + (c.redirect_uri || '-') + '</td>' +
      '<td>' + (c.scopes || '-') + '</td>' +
      '<td><span class="' + statusClass + '">' + statusText + '</span></td>' +
      '<td>' +
      '<button class="btn btn-sm btn-secondary" onclick="toggleStatus(\'' + c.client_id + '\',\'' + (c.status === 'active' ? 'disable' : 'enable') + '\')">' + (c.status === 'active' ? '禁用' : '启用') + '</button> ' +
      '<button class="btn btn-sm btn-secondary" onclick="rotateSecret(\'' + c.client_id + '\')">轮换密钥</button> ' +
      '<button class="btn btn-sm btn-danger" onclick="deleteClient(\'' + c.client_id + '\')">删除</button>' +
      '</td></tr>';
  }
  document.getElementById('clientTable').innerHTML = html;
}

function showCreateModal() { document.getElementById('createModal').classList.add('active'); document.getElementById('secretResult').style.display = 'none'; }
function hideCreateModal() { document.getElementById('createModal').classList.remove('active'); }

async function createClient() {
  const clientId = document.getElementById('newClientId').value.trim();
  const clientName = document.getElementById('newClientName').value.trim();
  const redirectUri = document.getElementById('newRedirectUri').value.trim();
  const scopes = document.getElementById('newScopes').value.trim();
  if (!clientName) { alert('请输入客户端名称'); return; }
  if (!redirectUri) { alert('请输入 Redirect URI'); return; }
  const body = { client_name: clientName, redirect_uri: redirectUri, scopes: scopes };
  if (clientId) body.client_id = clientId;
  const r = await fetch(API + '/api/v1/admin/oauth-clients', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  const d = await r.json();
  if (d.code !== 200) { alert(d.message || 'create failed'); return; }
  const result = document.getElementById('secretResult');
  result.style.display = 'block';
  result.innerHTML = '<div class="secret-display"><strong>Client ID:</strong> ' + d.data.client.client_id + '<br><strong>Client Secret:</strong> ' + d.data.client_secret + '<br><br><span style="color:#dc2626">⚠ 请妥善保存密钥，此密钥仅显示一次！</span></div>';
  loadClients();
}

async function toggleStatus(clientId, action) {
  const r = await fetch(API + '/api/v1/admin/oauth-clients/' + clientId + '/' + action, { method: 'PUT' });
  const d = await r.json();
  if (d.code !== 200) { alert(d.message || 'operation failed'); return; }
  loadClients();
}

async function rotateSecret(clientId) {
  if (!confirm('确认轮换此客户端的密钥？旧密钥将立即失效。')) return;
  const r = await fetch(API + '/api/v1/admin/oauth-clients/' + clientId + '/rotate', { method: 'PUT' });
  const d = await r.json();
  if (d.code !== 200) { alert(d.message || 'rotate failed'); return; }
  alert('新密钥: ' + d.data.new_client_secret + '\n\n请妥善保存，此密钥仅显示一次！');
}

async function deleteClient(clientId) {
  if (!confirm('确认删除此客户端？此操作不可恢复。')) return;
  const r = await fetch(API + '/api/v1/admin/oauth-clients/' + clientId, { method: 'DELETE' });
  const d = await r.json();
  if (d.code !== 200) { alert(d.message || 'delete failed'); return; }
  loadClients();
}

loadClients();
</script>
</body>
</html>`
}

// auditLogsPageHTML returns the audit logs page HTML (whitelist B: only 2 admins).
func auditLogsPageHTML() string {
	return `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>OAS Audit Logs</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f3f4f6;color:#111827}
.header{background:#fff;border-bottom:1px solid #e5e7eb;padding:16px 24px;display:flex;align-items:center;justify-content:space-between}
.header h1{font-size:18px;font-weight:600}
.container{max-width:1200px;margin:24px auto;padding:0 24px}
.card{background:#fff;border-radius:10px;box-shadow:0 1px 2px rgba(0,0,0,.06);padding:24px;margin-bottom:20px}
.card h2{font-size:16px;font-weight:600;margin-bottom:16px}
.filters{display:flex;gap:12px;margin-bottom:16px;flex-wrap:wrap}
.filters input,.filters select{padding:8px 12px;border:1px solid #d1d5db;border-radius:6px;font-size:13px}
.filters button{padding:8px 16px;border:none;border-radius:6px;background:#2563eb;color:#fff;cursor:pointer;font-size:13px}
table{width:100%;border-collapse:collapse;font-size:13px}
th{text-align:left;padding:10px 12px;border-bottom:2px solid #e5e7eb;font-weight:600;color:#6b7280;font-size:12px;text-transform:uppercase}
td{padding:10px 12px;border-bottom:1px solid #f3f4f6;vertical-align:top}
.badge{display:inline-block;padding:2px 8px;border-radius:9999px;font-size:11px;font-weight:500}
.badge-admin{background:#e0e7ff;color:#3730a3}
.badge-owner{background:#fef3c7;color:#92400e}
.pagination{display:flex;gap:8px;justify-content:center;margin-top:16px}
.pagination button{padding:6px 12px;border:1px solid #d1d5db;border-radius:6px;background:#fff;cursor:pointer;font-size:12px}
.pagination button:disabled{opacity:.5;cursor:not-allowed}
.pagination button.active{background:#2563eb;color:#fff;border-color:#2563eb}
.empty{text-align:center;padding:40px;color:#9ca3af;font-size:14px}
.detail{font-size:12px;color:#6b7280;max-width:300px;word-break:break-all}
</style>
</head>
<body>
<div class="header">
	<h1>📋 审计日志</h1>
	<a href="/admin" style="font-size:13px;color:#6b7280;text-decoration:none">← 返回 Console</a>
</div>
<div class="container">
	<div class="card">
		<div class="filters">
			<input type="text" id="filterUser" placeholder="用户 ID">
			<select id="filterPlane">
				<option value="">全部平面</option>
				<option value="admin">Admin</option>
				<option value="owner">Owner</option>
			</select>
			<select id="filterAction">
				<option value="">全部操作</option>
				<option value="auth.login">登录</option>
				<option value="auth.quick-login">快速登录</option>
				<option value="auth.dev-token">开发令牌</option>
				<option value="user.create">创建用户</option>
				<option value="user.update_roles">修改角色</option>
				<option value="user.update_status">修改状态</option>
			</select>
			<button onclick="loadLogs()">筛选</button>
		</div>
		<table>
			<thead>
				<tr>
					<th>时间</th>
					<th>用户</th>
					<th>平面</th>
					<th>操作</th>
					<th>资源</th>
					<th>详情</th>
					<th>环境</th>
					<th>IP</th>
				</tr>
			</thead>
			<tbody id="logTable">
				<tr><td colspan="8" class="empty">加载中...</td></tr>
			</tbody>
		</table>
		<div class="pagination" id="pagination"></div>
	</div>
</div>
<script>
const API='/api/v1';
let currentPage=1;
let pageSize=20;
let totalLogs=0;

async function loadLogs(){
	const user=document.getElementById('filterUser').value;
	const plane=document.getElementById('filterPlane').value;
	const action=document.getElementById('filterAction').value;
	let url=API+'/admin/audit-logs?page='+currentPage+'&size='+pageSize;
	if(user)url+='&user_id='+encodeURIComponent(user);
	if(plane)url+='&plane='+encodeURIComponent(plane);
	if(action)url+='&action='+encodeURIComponent(action);
	const r=await fetch(url);
	const d=await r.json();
	if(d.code!==200){alert(d.message||'load failed');return}
	totalLogs=d.data.total;
	renderLogs(d.data.items);
	renderPagination();
}

function renderLogs(items){
	if(!items||items.length===0){
		document.getElementById('logTable').innerHTML='<tr><td colspan="8" class="empty">暂无数据</td></tr>';
		return;
	}
	let html='';
	for(const log of items){
		const time=new Date(log.created_at).toLocaleString('zh-CN');
		const planeBadge=log.plane==='admin'?'<span class="badge badge-admin">Admin</span>':'<span class="badge badge-owner">Owner</span>';
		html+='<tr>';
		html+='<td>'+time+'</td>';
		html+='<td>'+log.user_name+'<br><small style="color:#9ca3af">'+log.user_id+'</small></td>';
		html+='<td>'+planeBadge+'</td>';
		html+='<td><code style="font-size:11px;background:#f3f4f6;padding:2px 6px;border-radius:4px">'+log.action+'</code></td>';
		html+='<td>'+log.resource+'</td>';
		html+='<td class="detail">'+log.detail+'</td>';
		html+='<td><span class="badge" style="background:#dbeafe;color:#1e40af">'+(log.environment||'DEV')+'</span></td>';
		html+='<td>'+log.ip+'</td>';
		html+='</tr>';
	}
	document.getElementById('logTable').innerHTML=html;
}

function renderPagination(){
	const totalPages=Math.ceil(totalLogs/pageSize);
	if(totalPages<=1){document.getElementById('pagination').innerHTML='';return}
	let html='<button onclick="changePage('+(currentPage-1)+')" '+(currentPage===1?'disabled':'')+'>上一页</button>';
	for(let i=1;i<=totalPages&&i<=5;i++){
		html+='<button onclick="changePage('+i+')" class="'+(i===currentPage?'active':'')+'">'+i+'</button>';
	}
	if(totalPages>5)html+='<span style="padding:6px">...</span>';
	html+='<button onclick="changePage('+(currentPage+1)+')" '+(currentPage===totalPages?'disabled':'')+'>下一页</button>';
	document.getElementById('pagination').innerHTML=html;
}

function changePage(page){
	currentPage=page;
	loadLogs();
}

loadLogs();
</script>
</body>
</html>`
}

func roleMgmtPageHTML() string {
	return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>OAS Console - 角色权限管理</title>
<script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-gray-50">
<div class="max-w-7xl mx-auto px-4 py-8">
	<div class="mb-6">
		<a href="/admin" class="text-blue-600 hover:underline">← 返回 Console</a>
	</div>
	<div class="bg-white rounded-lg shadow p-6">
		<h1 class="text-2xl font-bold mb-6">角色权限管理</h1>
		<div class="mb-4">
			<button onclick="showCreateModal()" class="bg-blue-600 text-white px-4 py-2 rounded hover:bg-blue-700">新增角色</button>
		</div>
		<table class="w-full">
			<thead class="bg-gray-100">
				<tr>
					<th class="px-4 py-2 text-left">角色编码</th>
					<th class="px-4 py-2 text-left">角色名称</th>
					<th class="px-4 py-2 text-left">描述</th>
					<th class="px-4 py-2 text-left">权限点</th>
					<th class="px-4 py-2 text-left">操作</th>
				</tr>
			</thead>
			<tbody id="roleTable"></tbody>
		</table>
	</div>
</div>

<!-- Create/Edit Modal -->
<div id="modal" class="hidden fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center">
	<div class="bg-white rounded-lg p-6 w-96">
		<h2 id="modalTitle" class="text-xl font-bold mb-4">新增角色</h2>
		<input type="hidden" id="editId">
		<div class="mb-3">
			<label class="block text-sm mb-1">角色编码</label>
			<input id="roleCode" class="w-full border rounded px-3 py-2" placeholder="如 TAM">
		</div>
		<div class="mb-3">
			<label class="block text-sm mb-1">角色名称</label>
			<input id="roleName" class="w-full border rounded px-3 py-2" placeholder="如 技术域管理员">
		</div>
		<div class="mb-3">
			<label class="block text-sm mb-1">描述</label>
			<textarea id="roleDesc" class="w-full border rounded px-3 py-2" rows="2"></textarea>
		</div>
		<div class="mb-3">
			<label class="block text-sm mb-1">权限点（JSON 数组）</label>
			<textarea id="rolePerms" class="w-full border rounded px-3 py-2 font-mono text-sm" rows="3" placeholder='["sys:user:read","tam:org:manage"]'></textarea>
		</div>
		<div class="flex justify-end gap-2">
			<button onclick="closeModal()" class="px-4 py-2 border rounded">取消</button>
			<button onclick="saveRole()" class="px-4 py-2 bg-blue-600 text-white rounded">保存</button>
		</div>
	</div>
</div>

<script>
const API='/api/v1';
let roles=[];

async function loadRoles(){
	const r=await fetch(API+'/admin/roles');
	const d=await r.json();
	if(d.code!==200){alert(d.message||'load failed');return}
	roles=d.data.items;
	renderRoles(roles);
}

function renderRoles(items){
	if(!items||items.length===0){
		document.getElementById('roleTable').innerHTML='<tr><td colspan="5" class="text-center py-8 text-gray-400">暂无数据</td></tr>';
		return;
	}
	let html='';
	for(const role of items){
		const perms=role.permissions?role.permissions.substring(0,50)+(role.permissions.length>50?'...':''):'-';
		html+='<tr class="border-b hover:bg-gray-50">';
		html+='<td class="px-4 py-3 font-mono text-sm">'+role.role_code+'</td>';
		html+='<td class="px-4 py-3">'+role.name+'</td>';
		html+='<td class="px-4 py-3 text-sm text-gray-600">'+(role.description||'-')+'</td>';
		html+='<td class="px-4 py-3 text-xs font-mono">'+perms+'</td>';
		html+='<td class="px-4 py-3">';
		html+='<button onclick="editRole('+role.id+')" class="text-blue-600 hover:underline mr-2">编辑</button>';
		html+='<button onclick="deleteRole('+role.id+',\''+role.role_code+'\')" class="text-red-600 hover:underline">删除</button>';
		html+='</td>';
		html+='</tr>';
	}
	document.getElementById('roleTable').innerHTML=html;
}

function showCreateModal(){
	document.getElementById('modalTitle').textContent='新增角色';
	document.getElementById('editId').value='';
	document.getElementById('roleCode').value='';
	document.getElementById('roleCode').disabled=false;
	document.getElementById('roleName').value='';
	document.getElementById('roleDesc').value='';
	document.getElementById('rolePerms').value='';
	document.getElementById('modal').classList.remove('hidden');
}

function editRole(id){
	const role=roles.find(r=>r.id===id);
	if(!role)return;
	document.getElementById('modalTitle').textContent='编辑角色';
	document.getElementById('editId').value=id;
	document.getElementById('roleCode').value=role.role_code;
	document.getElementById('roleCode').disabled=true;
	document.getElementById('roleName').value=role.name;
	document.getElementById('roleDesc').value=role.description||'';
	document.getElementById('rolePerms').value=role.permissions||'';
	document.getElementById('modal').classList.remove('hidden');
}

function closeModal(){
	document.getElementById('modal').classList.add('hidden');
}

async function saveRole(){
	const id=document.getElementById('editId').value;
	const data={
		name:document.getElementById('roleName').value,
		description:document.getElementById('roleDesc').value,
		permissions:document.getElementById('rolePerms').value
	};
	if(!id){
		data.role_code=document.getElementById('roleCode').value;
		if(!data.role_code){alert('请输入角色编码');return}
		const r=await fetch(API+'/admin/roles',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data)});
		const d=await r.json();
		if(d.code!==201){alert(d.message||'create failed');return}
	}else{
		const r=await fetch(API+'/admin/roles/'+id,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(data)});
		const d=await r.json();
		if(d.code!==200){alert(d.message||'update failed');return}
	}
	closeModal();
	loadRoles();
}

async function deleteRole(id,code){
	if(!confirm('确认删除角色 '+code+'？'))return;
	const r=await fetch(API+'/admin/roles/'+id,{method:'DELETE'});
	const d=await r.json();
	if(d.code!==200){alert(d.message||'delete failed');return}
	loadRoles();
}

loadRoles();
</script>
</body>
</html>`
}

func orgMgmtPageHTML() string {
	return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>OAS Console - 组织管理</title>
<script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-gray-50">
<div class="max-w-7xl mx-auto px-4 py-8">
	<div class="mb-6">
		<a href="/admin" class="text-blue-600 hover:underline">← 返回 Console</a>
	</div>
	<div class="bg-white rounded-lg shadow p-6">
		<h1 class="text-2xl font-bold mb-6">组织管理</h1>
		<div class="mb-4">
			<button onclick="showCreateModal()" class="bg-blue-600 text-white px-4 py-2 rounded hover:bg-blue-700">新增组织</button>
		</div>
		<table class="w-full">
			<thead class="bg-gray-100">
				<tr>
					<th class="px-4 py-2 text-left">组织编码</th>
					<th class="px-4 py-2 text-left">组织名称</th>
					<th class="px-4 py-2 text-left">域</th>
					<th class="px-4 py-2 text-left">状态</th>
					<th class="px-4 py-2 text-left">操作</th>
				</tr>
			</thead>
			<tbody id="orgTable"></tbody>
		</table>
	</div>
</div>

<!-- Create/Edit Modal -->
<div id="modal" class="hidden fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center">
	<div class="bg-white rounded-lg p-6 w-96">
		<h2 id="modalTitle" class="text-xl font-bold mb-4">新增组织</h2>
		<input type="hidden" id="editId">
		<div class="mb-3">
			<label class="block text-sm mb-1">组织编码</label>
			<input id="orgCode" class="w-full border rounded px-3 py-2" placeholder="如 TAM-DEV">
		</div>
		<div class="mb-3">
			<label class="block text-sm mb-1">组织名称</label>
			<input id="orgName" class="w-full border rounded px-3 py-2" placeholder="如 技术域研发中心">
		</div>
		<div class="mb-3">
			<label class="block text-sm mb-1">描述</label>
			<textarea id="orgDesc" class="w-full border rounded px-3 py-2" rows="2"></textarea>
		</div>
		<div class="mb-3">
			<label class="block text-sm mb-1">所属域</label>
			<select id="orgDomain" class="w-full border rounded px-3 py-2">
				<option value="">请选择</option>
				<option value="T">T - 技术域 (TAM)</option>
				<option value="H">H - 人资域 (HAM)</option>
				<option value="Y">Y - 智场域 (YAM)</option>
				<option value="O">O - 经营域 (OAM)</option>
				<option value="A">A - 行政域 (AU-admin)</option>
				<option value="F">F - 财务域 (FAM)</option>
				<option value="V">V - 商务域 (VAM)</option>
				<option value="G">G - 治理域 (GAM)</option>
			</select>
		</div>
		<div class="mb-3">
			<label class="block text-sm mb-1">父组织 ID（可选）</label>
			<input id="orgParent" type="number" class="w-full border rounded px-3 py-2" placeholder="留空表示顶级组织">
		</div>
		<div class="flex justify-end gap-2">
			<button onclick="closeModal()" class="px-4 py-2 border rounded">取消</button>
			<button onclick="saveOrg()" class="px-4 py-2 bg-blue-600 text-white rounded">保存</button>
		</div>
	</div>
</div>

<script>
const API='/api/v1';
let orgs=[];

async function loadOrgs(){
	const r=await fetch(API+'/admin/orgs');
	const d=await r.json();
	if(d.code!==200){alert(d.message||'load failed');return}
	orgs=d.data.items;
	renderOrgs(orgs);
}

function renderOrgs(items){
	if(!items||items.length===0){
		document.getElementById('orgTable').innerHTML='<tr><td colspan="5" class="text-center py-8 text-gray-400">暂无数据</td></tr>';
		return;
	}
	let html='';
	for(const org of items){
		const statusBadge=org.status==='active'?'<span class="bg-green-100 text-green-800 px-2 py-1 rounded text-xs">active</span>':'<span class="bg-gray-100 text-gray-800 px-2 py-1 rounded text-xs">inactive</span>';
		html+='<tr class="border-b hover:bg-gray-50">';
		html+='<td class="px-4 py-3 font-mono text-sm">'+org.code+'</td>';
		html+='<td class="px-4 py-3">'+org.name+'</td>';
		html+='<td class="px-4 py-3 text-sm">'+(org.domain||'-')+'</td>';
		html+='<td class="px-4 py-3">'+statusBadge+'</td>';
		html+='<td class="px-4 py-3">';
		html+='<button onclick="editOrg('+org.id+')" class="text-blue-600 hover:underline mr-2">编辑</button>';
		html+='<button onclick="viewMembers('+org.id+',\''+org.code+'\')" class="text-green-600 hover:underline mr-2">成员</button>';
		html+='<button onclick="deleteOrg('+org.id+',\''+org.code+'\')" class="text-red-600 hover:underline">删除</button>';
		html+='</td>';
		html+='</tr>';
		// Render children if any
		if(org.children&&org.children.length>0){
			for(const child of org.children){
				const childStatus=child.status==='active'?'<span class="bg-green-100 text-green-800 px-2 py-1 rounded text-xs">active</span>':'<span class="bg-gray-100 text-gray-800 px-2 py-1 rounded text-xs">inactive</span>';
				html+='<tr class="border-b hover:bg-gray-50 bg-gray-50">';
				html+='<td class="px-4 py-3 pl-8 font-mono text-sm text-gray-600">└ '+child.code+'</td>';
				html+='<td class="px-4 py-3 text-gray-600">'+child.name+'</td>';
				html+='<td class="px-4 py-3 text-sm">'+(child.domain||'-')+'</td>';
				html+='<td class="px-4 py-3">'+childStatus+'</td>';
				html+='<td class="px-4 py-3">';
				html+='<button onclick="editOrg('+child.id+')" class="text-blue-600 hover:underline mr-2">编辑</button>';
				html+='<button onclick="viewMembers('+child.id+',\''+child.code+'\')" class="text-green-600 hover:underline mr-2">成员</button>';
				html+='<button onclick="deleteOrg('+child.id+',\''+child.code+'\')" class="text-red-600 hover:underline">删除</button>';
				html+='</td>';
				html+='</tr>';
			}
		}
	}
	document.getElementById('orgTable').innerHTML=html;
}

function showCreateModal(){
	document.getElementById('modalTitle').textContent='新增组织';
	document.getElementById('editId').value='';
	document.getElementById('orgCode').value='';
	document.getElementById('orgCode').disabled=false;
	document.getElementById('orgName').value='';
	document.getElementById('orgDesc').value='';
	document.getElementById('orgDomain').value='';
	document.getElementById('orgParent').value='';
	document.getElementById('modal').classList.remove('hidden');
}

function editOrg(id){
	const org=orgs.find(o=>o.id===id);
	if(!org)return;
	document.getElementById('modalTitle').textContent='编辑组织';
	document.getElementById('editId').value=id;
	document.getElementById('orgCode').value=org.code;
	document.getElementById('orgCode').disabled=true;
	document.getElementById('orgName').value=org.name;
	document.getElementById('orgDesc').value=org.description||'';
	document.getElementById('orgDomain').value=org.domain||'';
	document.getElementById('orgParent').value=org.parent_id||'';
	document.getElementById('modal').classList.remove('hidden');
}

function closeModal(){
	document.getElementById('modal').classList.add('hidden');
}

async function saveOrg(){
	const id=document.getElementById('editId').value;
	const data={
		name:document.getElementById('orgName').value,
		description:document.getElementById('orgDesc').value,
		domain:document.getElementById('orgDomain').value,
		parent_id:document.getElementById('orgParent').value?parseInt(document.getElementById('orgParent').value):null
	};
	if(!id){
		data.code=document.getElementById('orgCode').value;
		if(!data.code){alert('请输入组织编码');return}
		const r=await fetch(API+'/admin/orgs',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data)});
		const d=await r.json();
		if(d.code!==201){alert(d.message||'create failed');return}
	}else{
		const r=await fetch(API+'/admin/orgs/'+id,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(data)});
		const d=await r.json();
		if(d.code!==200){alert(d.message||'update failed');return}
	}
	closeModal();
	loadOrgs();
}

async function deleteOrg(id,code){
	if(!confirm('确认删除组织 '+code+'？'))return;
	const r=await fetch(API+'/admin/orgs/'+id,{method:'DELETE'});
	const d=await r.json();
	if(d.code!==200){alert(d.message||'delete failed');return}
	loadOrgs();
}

async function viewMembers(id,code){
	alert('成员管理功能待实现（1b-3 域过滤中间件完成后）');
}

loadOrgs();
</script>
</body>
</html>`
}
