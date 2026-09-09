// routes.go — 全部 API 路由注册统一收口（OAS-CONSOLE-09 A1）。
// main() 在依赖初始化与 handlers.H 注入完成后调用 oas.Register(r)；路由相对顺序与拆分前一致。
// 页面路由（/login、/admin/*）暂留 cmd/oas/main.go，OAS-CONSOLE-09 A2 迁移。
package oas

import (
	"os"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ziway/backend/internal/oas/handlers"
	"ziway/backend/pkg/envpolicy"
	"ziway/backend/pkg/middleware"
)

// Register 注册全局中间件与全部 API 路由。
func Register(r *gin.Engine) {
	r.Use(middleware.CORS(), middleware.TraceID(), middleware.Recover(handlers.H.Log))

	r.GET("/health", handlers.H.Health)

	api := r.Group("/api/v1")

	// ===== Auth (public, no JWT required) =====
	// GET /api/v1/auth/public-key — return PEM for RS256 verification
	api.GET("/auth/public-key", handlers.H.PublicKey)

	// GET /.well-known/jwks.json — standard JWK Set endpoint
	r.GET("/.well-known/jwks.json", handlers.H.WellKnownJWKS)

	// POST /api/v1/os/:os/proxy/ams/auth/login — unified login, returns real JWT
	api.POST("/os/:os/proxy/ams/auth/login", handlers.H.ProxyAMSLogin)

	// ===== Quick Login (DEV/BETA only) =====
	// OAS_ENV controls availability: DEV and BETA allow, RC and PROD return 404 (fail-closed)
	if envpolicy.IsQuickLoginEnabled(handlers.H.OASEnv) {
		// POST /api/v1/auth/quick-login — one-click login for testing
		// Request: {"role": "SU"} or {"username": "admin"}
		// Returns JWT without password verification
		api.POST("/auth/quick-login", handlers.H.QuickLogin)
	}

	// ===== GET /api/v1/auth/test-accounts (SEC-2: DEV only, JWT required) =====
	// BETA/RC/PROD fail-closed 404; response never includes passwords
	if envpolicy.IsTestAccountsEnabled(handlers.H.OASEnv) {
		api.GET("/auth/test-accounts", middleware.JWTAuth(handlers.H.JWTVerifier, nil, handlers.H.Log), handlers.H.TestAccounts)
	}

	// ===== POST /api/v1/auth/dev-token — temporary token for development =====
	// Environment-aware: OAS_ENV controls availability (DEV/BETA allow, RC/PROD disabled)
	// Single source of truth: OAS_ENV (not COZE_PROJECT_ENV which may be auto-injected)
	devTokenEnv := os.Getenv("ZIWAY_DEV_TOKEN_ENABLED")
	// DEV/BETA: enabled unless explicitly set to "false"; RC/PROD: always disabled
	devTokenEnabled := envpolicy.IsDevTokenEnabled(handlers.H.OASEnv) && devTokenEnv != "false"
	if devTokenEnabled {
		api.POST("/auth/dev-token", handlers.H.DevToken)
		handlers.H.Log.Info("DEV token endpoint enabled (DEV/BETA environment)", zap.String("oas_env", handlers.H.OASEnv.String()), zap.String("ZIWAY_DEV_TOKEN_ENABLED", devTokenEnv))
	}
	// POST /api/v1/oauth/authorize-code — Generate authorization code (called after login in OAuth flow)
	if handlers.H.JWTVerifier != nil {
		api.POST("/oauth/authorize-code", middleware.JWTAuth(handlers.H.JWTVerifier, nil, handlers.H.Log), handlers.H.AuthorizeCode)
	}

	// ===== Owner Plane (/owner/*) — OU 权限 =====
	// OAS-CONSOLE-12 SEC-1 扩大面：owner plane 曾完全无鉴权（匿名可 CRUD 治理策略/域注册）
	owner := api.Group("/owner")
	if handlers.H.JWTVerifier != nil {
		owner.Use(middleware.JWTAuth(handlers.H.JWTVerifier, nil, handlers.H.Log))
		owner.Use(handlers.H.OwnerGate())
	}
	{
		// 事业场生命周期
		owner.GET("/domains", handlers.H.ListDomains)
		owner.POST("/domains", handlers.H.CreateDomain)
		owner.PUT("/domains/:id/status", handlers.H.UpdateDomainStatus)

		// 治理策略
		owner.GET("/policies", handlers.H.ListPolicies)
		owner.POST("/policies", handlers.H.CreatePolicy)
		owner.PUT("/policies/:id", handlers.H.UpdatePolicy)
	}

	// ===== OAuth 2.0 / OIDC Endpoints (OAS-CONSOLE-06) =====
	// GET /.well-known/openid-configuration — OIDC discovery
	r.GET("/.well-known/openid-configuration", handlers.H.OpenIDConfiguration)

	// GET /oauth/jwks — JWKS endpoint
	r.GET("/oauth/jwks", handlers.H.OAuthJWKS)

	// GET /oauth/authorize — Authorization endpoint
	r.GET("/oauth/authorize", handlers.H.OAuthAuthorize)

	// POST /oauth/token — Token endpoint (exchange code for tokens)
	r.POST("/oauth/token", handlers.H.OAuthToken)

	// GET /oauth/userinfo — Userinfo endpoint
	r.GET("/oauth/userinfo", middleware.JWTAuth(handlers.H.JWTVerifier, nil, handlers.H.Log), handlers.H.Userinfo)

	// ===== Admin Plane (/admin/*) — 白名单 A: OU/AU/OAM =====
	if handlers.H.JWTVerifier == nil {
		handlers.H.Log.Fatal("admin routes require JWT verifier, but it is not initialized")
	}
	admin := api.Group("/admin")
	// Combined JWT + API Key authentication
	admin.Use(handlers.H.AdminAuth())
	// Allow API keys or admin role users
	admin.Use(handlers.H.AdminRoleGate())
	// SEC-1: API keys must present sufficient scope (default least-privilege, writes denied)
	admin.Use(handlers.H.APIKeyScopeGate())
	{
		// ===== 治理看板 (/admin/dashboard/*) =====
		admin.GET("/dashboard/stats", handlers.H.DashboardStats)
		// OAS-CONSOLE-13 A4: 治理概览别名（测试口径 /dashboard/overview，同 handler）
		admin.GET("/dashboard/overview", handlers.H.DashboardStats)

		// ===== 战略审批工作台 (/admin/approvals) =====
		admin.GET("/approvals", handlers.H.ListApprovals)

		admin.POST("/approvals", handlers.H.CreateApproval)

		admin.PUT("/approvals/:id/approve", handlers.H.ApproveApproval)

		admin.PUT("/approvals/:id/reject", handlers.H.RejectApproval)

		admin.PUT("/approvals/:id/execute", handlers.H.ExecuteApproval)

		// 删除审批单（仅 OU/AU）
		admin.DELETE("/approvals/:id", handlers.H.DeleteApproval)

		// ===== 所有权视图 (/admin/ownership) =====
		admin.GET("/ownership/matrix", handlers.H.OwnershipMatrix)
		// OAS-CONSOLE-13 A5: 所有权 API 别名（页面 /admin/ownership 在根段，互不影响）
		admin.GET("/ownership", handlers.H.OwnershipMatrix)

		// ===== Admin 账号管理（2b-1）=====
		// 列表（白名单 B: SU/OU/AU）
		admin.GET("/admin-accounts", handlers.H.ListAdminAccounts)

		// 创建 admin 账号（仅 OU/AU）
		admin.POST("/admin-accounts", handlers.H.CreateAdminAccount)

		// 禁用 admin 账号（仅 OU/AU）
		admin.PUT("/admin-accounts/:id/disable", handlers.H.DisableAdminAccount)

		// 启用 admin 账号（仅 OU/AU）
		admin.PUT("/admin-accounts/:id/enable", handlers.H.EnableAdminAccount)

		// 重置密码（仅 OU/AU）
		admin.PUT("/admin-accounts/:id/reset-password", handlers.H.ResetAdminPassword)

		// 系统配置只读面板（白名单 B: SU/OU/AU）
		admin.GET("/system-config", handlers.H.GetSystemConfig)

		// 系统配置
		admin.GET("/configs", handlers.H.GetConfigs)
		admin.PUT("/configs/:key", handlers.H.UpdateConfig)
		// OAS-CONSOLE-13 D: 配置项删除（白名单 B + 敏感 key 拒删 + 审计）
		admin.DELETE("/configs/:key", handlers.H.DeleteConfig)

		// 服务注册
		admin.GET("/services", handlers.H.ListServices)
		admin.POST("/services", handlers.H.CreateService)
		admin.PUT("/services/:id/heartbeat", handlers.H.ServiceHeartbeat)

		// API密钥
		// API Key 全生命周期管理 — 白名单 B (OU/AU)
		admin.GET("/api-keys", handlers.H.ListAPIKeys)

		admin.GET("/api-keys/:id", handlers.H.GetAPIKey)

		admin.POST("/api-keys", handlers.H.CreateAPIKey)

		admin.PUT("/api-keys/:id/rotate", handlers.H.RotateAPIKey)

		admin.PUT("/api-keys/:id/disable", handlers.H.DisableAPIKey)

		admin.PUT("/api-keys/:id/enable", handlers.H.EnableAPIKey)

		admin.DELETE("/api-keys/:id", handlers.H.DeleteAPIKey)

		// Federation Node 联邦节点管理 — 白名单 B (OU/AU)
		admin.GET("/federation-nodes", handlers.H.ListFederationNodes)

		admin.GET("/federation-nodes/:id", handlers.H.GetFederationNode)

		admin.POST("/federation-nodes", handlers.H.CreateFederationNode)

		admin.PUT("/federation-nodes/:id", handlers.H.UpdateFederationNode)

		admin.PUT("/federation-nodes/:id/suspend", handlers.H.SuspendFederationNode)

		admin.PUT("/federation-nodes/:id/activate", handlers.H.ActivateFederationNode)

		admin.PUT("/federation-nodes/:id/trust", handlers.H.UpdateTrustLevel)

		admin.DELETE("/federation-nodes/:id", handlers.H.DeleteFederationNode)

		// OAuth 客户端管理 — 白名单 B (OU/AU)
		admin.GET("/oauth-clients", handlers.H.ListOAuthClients)

		admin.GET("/oauth-clients/:id", handlers.H.GetOAuthClient)

		admin.POST("/oauth-clients", handlers.H.CreateOAuthClient)

		admin.PUT("/oauth-clients/:id", handlers.H.UpdateOAuthClient)

		admin.PUT("/oauth-clients/:id/regenerate-secret", handlers.H.RegenerateClientSecret)

		admin.PUT("/oauth-clients/:id/suspend", handlers.H.SuspendOAuthClient)

		admin.PUT("/oauth-clients/:id/activate", handlers.H.ActivateOAuthClient)

		admin.DELETE("/oauth-clients/:id", handlers.H.DeleteOAuthClient)

		// 审计日志路由组 — 白名单 A + XAM 角色放行，handler 内再做细粒度检查
		adminAuditLogs := api.Group("/admin/audit-logs", handlers.H.AuditLogsAuth(), handlers.H.AuditLogsGate(), handlers.H.APIKeyScopeGate())

		// 审计日志 — 白名单 B (OU/AU) 全量，OAM 不可读
		// OAS-CONSOLE-08: 移除 XAM 域过滤，仅白名单 B 可访问
		adminAuditLogs.GET("", handlers.H.ListAuditLogs)

		// ===== RBAC 策略管理 (/admin/rbac/*) — OAS 权威源 =====
		admin.GET("/rbac/policies", handlers.H.ListRBACPolicies)

		admin.POST("/rbac/policies", handlers.H.CreateRBACPolicy)

		admin.PUT("/rbac/policies/:id", handlers.H.UpdateRBACPolicy)

		admin.DELETE("/rbac/policies/:id", handlers.H.DeleteRBACPolicy)

		admin.POST("/rbac/sync", handlers.H.SyncRBAC)
	}
	// ===== Root Path (GET /) — Redirect to login or admin overview =====
	r.GET("/", handlers.H.ConsoleHome)

	// ===== POST /api/v1/auth/login — username+password login =====
	api.POST("/auth/login", handlers.H.Login)

	// ===== User Management API (/api/v1/admin/users/*) — JWT + SU/AU only =====
	adminUsers := api.Group("/admin")
	if handlers.H.JWTVerifier != nil {
		adminUsers.Use(middleware.JWTAuth(handlers.H.JWTVerifier, nil, handlers.H.Log))
		// 白名单 A：系统管理访问（/admin/*）= SU/OU/AU/OAM
		adminUsers.Use(handlers.H.UsersAuthz())
	}
	adminUsers.GET("/users", handlers.H.ListUsers)

	adminUsers.POST("/users", handlers.H.CreateUser)

	adminUsers.PUT("/users/:id/roles", handlers.H.UpdateUserRoles)

	adminUsers.PUT("/users/:id/status", handlers.H.UpdateUserStatus)

	// DELETE /api/v1/admin/users/:id — 删除用户（白名单 B：仅 SU/OU/AU）
	adminUsers.DELETE("/users/:id", handlers.H.DeleteUser)

	// GET /api/v1/auth/roles — list available roles
	api.GET("/auth/roles", handlers.H.AuthRoles)

	// ===== Role Management API (JWT + Whitelist A) =====
	adminRoles := api.Group("/admin/roles")
	if handlers.H.JWTVerifier != nil {
		adminRoles.Use(middleware.JWTAuth(handlers.H.JWTVerifier, nil, handlers.H.Log))
		adminRoles.Use(middleware.RequireUsers("oas-ou-admin", "oas-au-admin", "oas-oam-admin"))
	}
	// GET /api/v1/admin/roles — list all roles
	adminRoles.GET("", handlers.H.ListRoles)

	// OAS-CONSOLE-13 A6: /admin/rbac/roles 别名（与 /admin/roles 同鉴权同 handler，测试口径兼容）
	adminRBACRoles := api.Group("/admin/rbac/roles")
	if handlers.H.JWTVerifier != nil {
		adminRBACRoles.Use(middleware.JWTAuth(handlers.H.JWTVerifier, nil, handlers.H.Log))
		adminRBACRoles.Use(middleware.RequireUsers("oas-ou-admin", "oas-au-admin", "oas-oam-admin"))
	}
	adminRBACRoles.GET("", handlers.H.ListRoles)

	// POST /api/v1/admin/roles — create role
	adminRoles.POST("", handlers.H.CreateRole)

	// PUT /api/v1/admin/roles/:id — update role
	adminRoles.PUT("/:id", handlers.H.UpdateRole)

	// DELETE /api/v1/admin/roles/:id — delete role
	adminRoles.DELETE("/:id", handlers.H.DeleteRole)

	// ===== Organization Management (GET/POST/PUT/DELETE /admin/orgs) — JWT + whitelist A + XAM roles =====
	adminOrgs := api.Group("/admin/orgs", middleware.JWTAuth(handlers.H.JWTVerifier, nil, handlers.H.Log), handlers.H.OrgsAuthz(), middleware.DomainFilter())
	{
		// List organizations (with tree structure)
		adminOrgs.GET("", handlers.H.ListOrgs)

		// Get organization detail
		adminOrgs.GET("/:id", handlers.H.GetOrg)

		// Create organization
		adminOrgs.POST("", handlers.H.CreateOrg)

		// Update organization
		adminOrgs.PUT("/:id", handlers.H.UpdateOrg)

		// Delete organization
		adminOrgs.DELETE("/:id", handlers.H.DeleteOrg)

		// Get organization members
		adminOrgs.GET("/:id/members", handlers.H.ListOrgMembers)

		// Add member to organization
		adminOrgs.POST("/:id/members", handlers.H.AddOrgMember)

	}

	// ===== 页面路由（OAS-CONSOLE-09 A2 自 main.go 收口；注册顺序不影响静态路径匹配）=====
	// ===== Login Page (GET /login) =====
	r.GET("/login", handlers.H.PageLogin)

	// OAS-CONSOLE-13 B2: /admin/config-center 已并入 /admin/system-config，旧链接 302 兼容
	r.GET("/admin/config-center", func(c *gin.Context) {
		q := c.Request.URL.RawQuery
		if q != "" {
			c.Redirect(302, "/admin/system-config?"+q)
		} else {
			c.Redirect(302, "/admin/system-config")
		}
	})

	// ===== GET /admin — OAS Console 管理控制台首页 =====
	// Requires JWT + whitelist A (OU/AU/OAM)
	r.GET("/admin", handlers.H.PageConsoleHome)

	// ===== GET /admin/overview — OU 治理看板 =====
	// Requires JWT + whitelist A (OU/AU/OAM)
	r.GET("/admin/overview", handlers.H.PageOverview)

	// ===== GET /admin/approvals — 战略审批工作台（白名单 A：OU/AU/OAM，OAM 只读）=====
	r.GET("/admin/approvals", handlers.H.PageApprovals)

	// ===== GET /admin/ownership — 所有权视图（白名单 A）=====
	r.GET("/admin/ownership", handlers.H.PageOwnership)

	// Admin 账号管理页面（仅 OU/AU）
	r.GET("/admin/admin-accounts", handlers.H.PageAdminAccounts)

	// 系统配置只读面板（仅 OU/AU）
	r.GET("/admin/system-config", handlers.H.PageSystemConfig)

	// API Key 管理页面（仅 OU/AU）
	r.GET("/admin/api-keys", handlers.H.PageAPIKeys)

	// 联邦节点管理页面（仅 OU/AU）
	r.GET("/admin/federation-nodes", handlers.H.PageFederationNodes)

	// ===== GET /admin/services — 服务注册列表页面（白名单 A）OAS-CONSOLE-13 B1 =====
	r.GET("/admin/services", handlers.H.PageServices)

	// ===== GET /admin/oauth-clients — OAuth 客户端管理页面（白名单 B：仅 OU/AU）=====
	r.GET("/admin/oauth-clients", handlers.H.PageOAuthClients)

	// ===== GET /admin/audit-logs — 审计日志页面（白名单 B：仅 2 admin）=====
	r.GET("/admin/audit-logs", handlers.H.PageAuditLogs)

	// ===== User Management Page (GET /admin/users) — JWT required =====
	r.GET("/admin/users", handlers.H.PageUsers)

	// ===== Role Management Page (GET /admin/roles) — JWT required =====
	r.GET("/admin/roles", handlers.H.PageRoles)

	// ===== Organization Management Page (GET /admin/orgs) — JWT required =====
	r.GET("/admin/orgs", handlers.H.PageOrgs)
}
