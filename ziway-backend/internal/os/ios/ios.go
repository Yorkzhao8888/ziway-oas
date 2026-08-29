// Package ios is the capital BOS — orchestrates ims + fms (investment-grade finance) for ziway-Xcase.
package ios

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/bos"
	"ziway/backend/pkg/response"
)

type Orchestrator struct{ bos.BaseOrchestrator }

func New(deps bos.Dependencies) *Orchestrator {
	return &Orchestrator{BaseOrchestrator: bos.NewBaseOrchestrator("ios", deps)}
}

func (o *Orchestrator) MBSDependencies() []string { return []string{"ims", "fms"} }

func (o *Orchestrator) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"os": "ios", "status": "ok",
			"desc":     "资本事业场编排器（T43三方会签）",
			"mbs_deps": o.MBSDependencies(),
		})
	})
	o.MountProxy(rg, "ims")
	o.MountProxy(rg, "fms")
	rg.GET("/portfolio/summary", o.PortfolioSummary)
	rg.GET("/icases/pending", o.PendingICases)
	rg.POST("/icases/:id/deploy", o.DeployCapital)
}

func (o *Orchestrator) PortfolioSummary(c *gin.Context) {
	response.OK(c, gin.H{
		"total_committed":  0,
		"total_deployed":   0,
		"total_returned":   0,
		"unrealized_pl":    0,
		"note":             "P0 placeholder — ims+fms portfolio pending gRPC",
	})
}

func (o *Orchestrator) PendingICases(c *gin.Context) {
	response.OK(c, gin.H{
		"awaiting_oas":  0,
		"awaiting_sms": 0,
		"awaiting_ims": 0,
		"note":          "P0 placeholder — T43 three-party approval queue pending gRPC",
	})
}

// DeployCapital — Saga编排：IMBS创建仓位 → FMBS记账
func (o *Orchestrator) DeployCapital(c *gin.Context) {
	response.OK(c, gin.H{
		"status":  "pending",
		"message": "Capital deployment Saga (ims→fms) will be implemented in P1",
	})
}
