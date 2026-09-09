// admin_mw.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
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

func (h *Handlers) AdminRoleGate() gin.HandlerFunc {
	return func(c *gin.Context) {
		authType, _ := c.Get("auth_type")
		if authType == "api_key" {
			c.Next()
			return
		}

		// Check if user has admin role (SU/OU/AU)
		username, _ := c.Get("username")
		if username == nil {
			response.Unauthorized(c, "unauthorized")
			c.Abort()
			return
		}

		var user oasmodel.OASUser
		if err := h.DB.Where("username = ?", username).First(&user).Error; err != nil {
			response.Unauthorized(c, "user not found")
			c.Abort()
			return
		}

		// Check if user has admin role (whitelist A: SU/OU/AU/OAM)
		if user.RoleCode != "SU" && user.RoleCode != "OU" && user.RoleCode != "AU" && user.RoleCode != "OAM" {
			response.Forbidden(c, "access denied: admin role required")
			c.Abort()
			return
		}

		c.Next()
	}
}

func (h *Handlers) AdminAuth() gin.HandlerFunc {
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

// APIKeyScopeGate enforces least-privilege for API-key requests (SEC-1):
// read methods require scope read/admin, write methods require write/admin;
// empty or unknown scopes are denied (fail-closed). JWT users pass through
// and are governed by their own RBAC layer.
func (h *Handlers) APIKeyScopeGate() gin.HandlerFunc {
	return func(c *gin.Context) {
		authType, _ := c.Get("auth_type")
		if authType != "api_key" {
			c.Next()
			return
		}

		scopes, _ := c.Get("api_key_scopes")
		scopeStr, _ := scopes.(string)
		granted := map[string]bool{}
		for _, s := range strings.FieldsFunc(scopeStr, func(r rune) bool {
			return r == ',' || r == ';' || r == ' ' || r == '\t'
		}) {
			granted[strings.ToLower(strings.TrimSpace(s))] = true
		}

		method := c.Request.Method
		need := "read"
		if method != "GET" && method != "HEAD" && method != "OPTIONS" {
			need = "write"
		}

		if granted[need] || granted["admin"] {
			c.Next()
			return
		}

		response.Forbidden(c, "api key scope does not allow "+method)
		c.Abort()
	}
}

// OwnerGate — SEC-1 扩大面：owner plane 仅白名单 A（SU/OU/AU/OAM）可访问。
// API key 无法进入（owner 组只挂 JWTAuth，Bearer api-key 解析失败 401）。
func (h *Handlers) OwnerGate() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, _ := c.Get("username")
		usernameStr, _ := username.(string)
		if usernameStr == "" || !authz.IsInAdminWhitelistA(h.DB, usernameStr) {
			response.Forbidden(c, "owner plane requires system management access")
			c.Abort()
			return
		}
		c.Next()
	}
}
