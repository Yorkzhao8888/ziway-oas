// Package fos is the finance BOS — orchestrates fms.
// ZW-ARC-017: NEW — 财务事业场，直通模式（1:1映射FMBS）。
// FMBS同时有FOS直通+被DOS/IOS编排（非严格1:1）。
// 无独立APP，FMBS能力同时被DOS(经营)和IOS(投资)编排消费。
package fos

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/bos"
	"ziway/backend/pkg/response"
)

type Orchestrator struct{ bos.BaseOrchestrator }

func New(deps bos.Dependencies) *Orchestrator {
	return &Orchestrator{BaseOrchestrator: bos.NewBaseOrchestrator("fos", deps)}
}

func (o *Orchestrator) MBSDependencies() []string { return []string{"fms"} }

func (o *Orchestrator) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"os": "fos", "status": "ok",
			"desc":     "财务事业场编排器（FMBS财务直通）",
			"mbs_deps": o.MBSDependencies(),
			"app":      "无独立APP（被DOS/IOS编排消费）",
		})
	})
	// P0 transparent proxy: /proxy/fms/* → MBS /api/v1/fms/*
	o.MountProxy(rg, "fms")
	// Finance endpoints
	rg.GET("/finance/summary", o.FinanceSummary)
	rg.GET("/ledger/overview", o.LedgerOverview)
}

// FinanceSummary — 财务汇总
func (o *Orchestrator) FinanceSummary(c *gin.Context) {
	response.OK(c, gin.H{
		"total_revenue":   0,
		"total_expenses":  0,
		"net_profit":      0,
		"note":            "P0 placeholder — fms finance summary pending gRPC",
	})
}

// LedgerOverview — 账本概览
func (o *Orchestrator) LedgerOverview(c *gin.Context) {
	response.OK(c, gin.H{
		"accounts":      []interface{}{},
		"pending_posts": 0,
		"note":          "P0 placeholder — fms ledger overview pending gRPC",
	})
}
