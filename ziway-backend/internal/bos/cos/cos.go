// Package cos is the retail BOS — orchestrates cms + dms for ziway-Mall.
// ZW-ARC-017: COS编排CMBS(客户)+dms(经营)，Mall消费端跨域编排。
package cos

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/bos"
	"ziway/backend/pkg/response"
)

type Orchestrator struct{ bos.BaseOrchestrator }

func New(deps bos.Dependencies) *Orchestrator {
	return &Orchestrator{BaseOrchestrator: bos.NewBaseOrchestrator("cos", deps)}
}

func (o *Orchestrator) MBSDependencies() []string { return []string{"cms", "dms"} }

func (o *Orchestrator) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"os": "cos", "status": "ok",
			"desc":     "零售事业场编排器（客户+经营跨域编排）",
			"mbs_deps": o.MBSDependencies(),
		})
	})
	// P0 transparent proxies
	o.MountProxy(rg, "cms")
	o.MountProxy(rg, "dms")
	// Orchestration endpoints
	rg.GET("/customers/summary", o.CustomerSummary)
	rg.GET("/orders/today", o.TodayOrders)
}

// CustomerSummary — 跨CMBS+DMBS聚合：客户数/活跃/新增
func (o *Orchestrator) CustomerSummary(c *gin.Context) {
	result, err := o.Caller().CallMBS("cms", "CustomerSummary", nil)
	if err != nil {
		response.OK(c, gin.H{
			"total_customers":  0,
			"active_today":     0,
			"new_this_week":    0,
			"note":             "P0 placeholder — cms+dms aggregation pending gRPC",
		})
		return
	}
	response.OK(c, result)
}

func (o *Orchestrator) TodayOrders(c *gin.Context) {
	result, err := o.Caller().CallMBS("cms", "TodayOrders", nil)
	if err != nil {
		response.OK(c, gin.H{
			"total_orders": 0, "revenue": 0,
			"note": "P0 placeholder — cms order aggregation pending gRPC",
		})
		return
	}
	response.OK(c, result)
}
