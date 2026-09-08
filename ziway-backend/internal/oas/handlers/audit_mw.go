// audit_mw.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"ziway/backend/internal/oas/authz"
	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/middleware"
	"ziway/backend/pkg/response"
)

// AuditLogsAuth：API Key 优先，未命中回退 JWT 校验。
func (h *Handlers) AuditLogsAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
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
				if err := h.DB.Where("key_prefix = ?", prefix).First(&key).Error; err == nil {
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
		middleware.JWTAuth(h.JWTVerifier, nil, h.Log)(c)
	}
}

// AuditLogsGate：白名单 A 或 XAM 角色放行（audit-logs 组第二中间件）。
func (h *Handlers) AuditLogsGate() gin.HandlerFunc {
	return func(c *gin.Context) {
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

		isWhitelistA := authz.IsInAdminWhitelistA(h.DB, username.(string))
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
	}
}
