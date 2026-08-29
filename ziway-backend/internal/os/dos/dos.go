// Package dos is the daily-operation BOS — orchestrates dms + hms + fms for ziway-Shop.
// ZW-ARC-017: DOS编排DMBS(经营)+hms(人力)+fms(财务)，门店经营跨域编排。
package dos

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/bos"
	"ziway/backend/pkg/response"
)

type Orchestrator struct{ bos.BaseOrchestrator }

func New(deps bos.Dependencies) *Orchestrator {
	return &Orchestrator{BaseOrchestrator: bos.NewBaseOrchestrator("dos", deps)}
}

func (o *Orchestrator) MBSDependencies() []string { return []string{"dms", "hms", "fms"} }

func (o *Orchestrator) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"os": "dos", "status": "ok",
			"desc":     "经营事业场编排器（经营+人力+财务跨域编排）",
			"mbs_deps": o.MBSDependencies(),
		})
	})
	// P0 transparent proxies
	o.MountProxy(rg, "dms")
	o.MountProxy(rg, "hms")
	o.MountProxy(rg, "fms")
	// Orchestration endpoints
	rg.GET("/daily/gp", o.DailyGP)
	rg.GET("/stores/overview", o.StoresOverview)
	rg.GET("/daily/close", o.DailyClose)
}

// DailyGP — 跨DMBS+FMBS聚合：日经营GP（毛利）
func (o *Orchestrator) DailyGP(c *gin.Context) {
	response.OK(c, gin.H{
		"revenue":      0,
		"cogs":         0,
		"gross_profit": 0,
		"gp_margin":    0,
		"note":         "P0 placeholder — dms+fms GP aggregation pending gRPC",
	})
}

func (o *Orchestrator) StoresOverview(c *gin.Context) {
	response.OK(c, gin.H{
		"total_stores":  0,
		"active_stores": 0,
		"note":          "P0 placeholder — dms store overview pending gRPC",
	})
}

// DailyClose — 日结编排：DMBS日KPI → HMBS工时 → FMBS记账
func (o *Orchestrator) DailyClose(c *gin.Context) {
	response.OK(c, gin.H{
		"status":  "pending",
		"message": "Daily close orchestration (dms→hms→fms) will be implemented in P1 Saga",
	})
}
