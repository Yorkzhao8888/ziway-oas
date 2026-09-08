// home.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1），行为零变化。
package handlers

import (
	"github.com/gin-gonic/gin"
)

func (h *Handlers) ConsoleHome(c *gin.Context) {
	if h.JWTVerifier == nil {
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
	_, err := h.JWTVerifier.Verify(tokenStr)
	if err != nil {
		// Invalid JWT, redirect to login
		c.Redirect(302, "/login?redirect=/")
		return
	}
	// Valid JWT, redirect to admin overview
	c.Redirect(302, "/admin/overview?token="+tokenStr)
}
