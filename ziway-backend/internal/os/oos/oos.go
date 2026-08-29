// Package oos is the governance BOS — orchestrates oms.
// ZW-ARC-017: NEW — 治理事业场，直通模式（1:1映射OMBS）。
// 服务于ziway-Xcase治理Tab；OMBS能力同时被VOS编排。
package oos

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/bos"
	"ziway/backend/pkg/response"
)

type Orchestrator struct{ bos.BaseOrchestrator }

func New(deps bos.Dependencies) *Orchestrator {
	return &Orchestrator{BaseOrchestrator: bos.NewBaseOrchestrator("oos", deps)}
}

func (o *Orchestrator) MBSDependencies() []string { return []string{"oms"} }

func (o *Orchestrator) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"os": "oos", "status": "ok",
			"desc":     "治理事业场编排器（OMBS治理直通）",
			"mbs_deps": o.MBSDependencies(),
			"app":      "ziway-Xcase（治理Tab）",
		})
	})
	// P0 transparent proxy: /proxy/oms/* → MBS /api/v1/oms/*
	o.MountProxy(rg, "oms")
	// Governance endpoints
	rg.GET("/approvals/queue", o.ApprovalQueue)
	rg.GET("/policies/list", o.PolicyList)
}

// ApprovalQueue — 审批队列
func (o *Orchestrator) ApprovalQueue(c *gin.Context) {
	response.OK(c, gin.H{
		"pending":    0,
		"approved":   0,
		"rejected":   0,
		"timeout_72h": 0,
		"note":       "P0 placeholder — oms approval queue pending gRPC",
	})
}

// PolicyList — 治理策略列表
func (o *Orchestrator) PolicyList(c *gin.Context) {
	response.OK(c, gin.H{
		"policies": []interface{}{},
		"note":     "P0 placeholder — oms governance policies pending gRPC",
	})
}
