// Package vos is the governance/operations BOS — NEW in ZW-ARC-015.
// Orchestrates gms (risk) + oms (approval/governance) + vms (value operations).
// Serves ziway-Xcase governance/risk tab and ziway-Dyard.
package vos

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/bos"
	"ziway/backend/pkg/response"
)

type Orchestrator struct{ bos.BaseOrchestrator }

func New(deps bos.Dependencies) *Orchestrator {
	return &Orchestrator{BaseOrchestrator: bos.NewBaseOrchestrator("vos", deps)}
}

func (o *Orchestrator) MBSDependencies() []string { return []string{"gms", "oms", "vms"} }

func (o *Orchestrator) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"os": "vos", "status": "ok",
			"desc":     "治理运营事业场编排器（风控+审批+价值运营）",
			"mbs_deps": o.MBSDependencies(),
		})
	})
	// P0 transparent proxies — vos orchestrates governance trio
	o.MountProxy(rg, "gms")
	o.MountProxy(rg, "oms")
	o.MountProxy(rg, "vms")
	// Governance dashboard
	rg.GET("/dashboard", o.GovernanceDashboard)
	// VCASE approval pipeline (vms → oms)
	rg.GET("/vcases/pipeline", o.VCASEPipeline)
	rg.POST("/vcases/:id/approve", o.ApproveVCASE)
	rg.POST("/vcases/:id/reject", o.RejectVCASE)
	// Risk monitoring (gms)
	rg.GET("/risks/active", o.ActiveRisks)
	// Approval timeout (72h)
	rg.GET("/approvals/timeout", o.ApprovalTimeouts)
}

// GovernanceDashboard — 跨GMBS+oms+VMBS聚合
func (o *Orchestrator) GovernanceDashboard(c *gin.Context) {
	response.OK(c, gin.H{
		"open_risks":         0,
		"pending_approvals":  0,
		"vcase_in_flight":    0,
		"overdue_approvals":  0,
		"note":               "P0 placeholder — gms+oms+vms governance dashboard pending gRPC",
	})
}

// VCASEPipeline — VMBS提交 → OMBS审批队列
func (o *Orchestrator) VCASEPipeline(c *gin.Context) {
	response.OK(c, gin.H{
		"draft":     0,
		"submitted": 0,
		"approved":  0,
		"executing": 0,
		"completed": 0,
		"rejected":  0,
		"failed":    0,
		"note":      "P0 placeholder — vms VCASE pipeline pending gRPC",
	})
}

// ApproveVCASE — OMBS审批通过 → 触发VMBS执行
// Saga: oms publishes VCASE_APPROVED → vms starts execution
func (o *Orchestrator) ApproveVCASE(c *gin.Context) {
	response.OK(c, gin.H{
		"status":  "pending",
		"message": "VCASE approval Saga (oms→vms) will be implemented in P1; 72h timeout applies",
	})
}

// RejectVCASE — oms publishes VCASE_REJECTED → vms marks rejected
func (o *Orchestrator) RejectVCASE(c *gin.Context) {
	response.OK(c, gin.H{
		"status":  "pending",
		"message": "VCASE rejection Saga will be implemented in P1",
	})
}

func (o *Orchestrator) ActiveRisks(c *gin.Context) {
	response.OK(c, gin.H{
		"risks": []interface{}{},
		"note":  "P0 placeholder — gms active risks pending gRPC",
	})
}

// ApprovalTimeouts — 检查72h审批超时
func (o *Orchestrator) ApprovalTimeouts(c *gin.Context) {
	response.OK(c, gin.H{
		"timeout_count": 0,
		"window_hours":  72,
		"note":          "P0 placeholder — oms 72h timeout scanner pending P1 scheduler",
	})
}
