// orgs_mw.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"ziway/backend/internal/oas/authz"
	"ziway/backend/pkg/response"
)

// OrgsAuthz：组织管理组鉴权中间件（JWT + 白名单 A + XAM 角色）。
func (h *Handlers) OrgsAuthz() gin.HandlerFunc {
	return func(c *gin.Context) {
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
		if !authz.CanAccessOrgManagement(h.DB, fmt.Sprintf("%v", username), roles) {
			response.Forbidden(c, "access denied")
			c.Abort()
			return
		}
		c.Next()
	}
}
