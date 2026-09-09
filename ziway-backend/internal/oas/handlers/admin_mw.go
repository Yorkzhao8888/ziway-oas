// admin_mw.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

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
