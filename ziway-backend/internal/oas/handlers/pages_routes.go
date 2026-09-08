// pages_routes.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"os"

	"github.com/gin-gonic/gin"

	"ziway/backend/internal/oas/authz"
	"ziway/backend/pkg/envpolicy"
	"ziway/backend/pkg/response"
)

func (h *Handlers) PageOrgs(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
	if err != nil {
		c.Redirect(302, "/login?redirect=/admin/orgs")
		return
	}
	if !authz.IsInAdminWhitelistA(h.DB, claims.Username) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(403, "<h1>403 Forbidden</h1><p>Access denied. System management restricted to OU/AU/OAM admins.</p>")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, orgMgmtPageHTML())
}

func (h *Handlers) PageRoles(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
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
}

func (h *Handlers) PageUsers(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
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
}

func (h *Handlers) PageAuditLogs(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
	if err != nil {
		c.Redirect(302, "/login?redirect=/admin/audit-logs")
		return
	}
	// Whitelist B: only OU/AU admin can access audit logs
	username := claims.Username
	if !authz.IsInAdminWhitelistB(h.DB, username) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(403, "<h1>403 Forbidden</h1><p>Audit logs restricted to OU/AU admin.</p>")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, auditLogsPageHTML())
}

func (h *Handlers) PageOAuthClients(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
	if err != nil {
		c.Redirect(302, "/login?redirect=/admin/oauth-clients")
		return
	}
	// Whitelist B: only OU/AU admin can access
	username := claims.Username
	if !authz.IsInAdminWhitelistB(h.DB, username) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to OU/AU admin.</p>")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, oauthClientsPageHTML(username))
}

func (h *Handlers) PageFederationNodes(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
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
}

func (h *Handlers) PageAPIKeys(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
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
}

func (h *Handlers) PageSystemConfig(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
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
}

func (h *Handlers) PageAdminAccounts(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
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
}

func (h *Handlers) PageOwnership(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
	if err != nil {
		c.Redirect(302, "/login?redirect=/admin/ownership")
		return
	}
	if !authz.IsInAdminWhitelistA(h.DB, claims.Username) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to whitelist A (OU/AU/OAM).</p>")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, ownershipPageHTML(claims.Username))
}

func (h *Handlers) PageApprovals(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
	if err != nil {
		c.Redirect(302, "/login?redirect=/admin/approvals")
		return
	}
	username := claims.Username
	if !authz.IsInAdminWhitelistA(h.DB, username) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to system administrators.</p>")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, approvalsPageHTML(username, h.OASEnv.String(), tokenStr))
}

func (h *Handlers) PageOverview(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
	if err != nil {
		c.Redirect(302, "/login?redirect=/admin/overview")
		return
	}
	username := claims.Username
	if !authz.IsInAdminWhitelistA(h.DB, username) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to system administrators.</p>")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, overviewPageHTML(username, h.OASEnv.String(), tokenStr))
}

func (h *Handlers) PageConsoleHome(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	claims, err := h.JWTVerifier.Verify(tokenStr)
	if err != nil {
		c.Redirect(302, "/login?redirect=/admin")
		return
	}
	// Check whitelist A
	username := claims.Username
	if !authz.IsInAdminWhitelistA(h.DB, username) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(403, "<h1>403 Forbidden</h1><p>Access restricted to system administrators.</p>")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, consoleHomePageHTML(username, h.OASEnv.String(), tokenStr))
}

func (h *Handlers) PageLogin(c *gin.Context) {
	devTokenEnabled := envpolicy.IsDevTokenEnabled(h.OASEnv) && os.Getenv("ZIWAY_DEV_TOKEN_ENABLED") != "false"
	redirect := c.Query("redirect")
	// OAuth context
	oauthClientID := c.Query("client_id")
	oauthRedirectURI := c.Query("redirect_uri")
	oauthResponseType := c.Query("response_type")
	oauthScope := c.Query("scope")
	oauthState := c.Query("state")
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, loginPageHTML(redirect, h.OASEnv, devTokenEnabled, oauthClientID, oauthRedirectURI, oauthResponseType, oauthScope, oauthState))
}
