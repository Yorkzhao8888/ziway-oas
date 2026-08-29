// Package aos is the system-development BOS — orchestrates ams for ziway-Xcase (ops/dev tab).
// ZW-ARC-015: aos now focuses solely on ams (system/auth); governance moved to vos.
package aos

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/bos"
	"ziway/backend/pkg/response"
)

type Orchestrator struct{ bos.BaseOrchestrator }

func New(deps bos.Dependencies) *Orchestrator {
	return &Orchestrator{BaseOrchestrator: bos.NewBaseOrchestrator("aos", deps)}
}

func (o *Orchestrator) MBSDependencies() []string { return []string{"ams"} }

func (o *Orchestrator) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"os": "aos", "status": "ok",
			"desc":     "系统开发事业场编排器（AMS鉴权/用户）",
			"mbs_deps": o.MBSDependencies(),
		})
	})
	o.MountProxy(rg, "ams")
	rg.GET("/users/summary", o.UsersSummary)
	rg.GET("/roles/list", o.RolesList)
	rg.GET("/system/status", o.SystemStatus)
}

func (o *Orchestrator) UsersSummary(c *gin.Context) {
	response.OK(c, gin.H{
		"total_users":  0,
		"active_users": 0,
		"by_type":      gin.H{},
		"note":         "P0 placeholder — ams user aggregation pending gRPC",
	})
}

func (o *Orchestrator) RolesList(c *gin.Context) {
	response.OK(c, gin.H{
		"roles": []interface{}{},
		"note":  "P0 placeholder — ams role list pending gRPC",
	})
}

func (o *Orchestrator) SystemStatus(c *gin.Context) {
	response.OK(c, gin.H{
		"auth_provider": "ams",
		"jwt_enabled":   false,
		"rbac_mode":     "basic",
		"casbin":        false,
		"note":          "P0 basic auth; JWT+Casbin hardening in P1 (see services/ams (P1 independent service))",
	})
}
