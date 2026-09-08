// users_mw.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	oasmodel "ziway/backend/internal/oas/model"
	"ziway/backend/pkg/response"
)

func (h *Handlers) UsersAuthz() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, _ := c.Get("username")
		roleCode, _ := c.Get("role_code")
		// API Key 视为 admin 级别
		if strings.HasPrefix(fmt.Sprintf("%v", username), "api-key:") {
			c.Next()
			return
		}
		// 查询用户 role_code
		var user oasmodel.OASUser
		if h.DB.Where("username = ?", username).First(&user).Error == nil && user.RoleCode != "" {
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
	}
}
