// Package gos is the risk-control BOS — orchestrates gms.
// ZW-ARC-017: NEW — 风控事业场，直通模式（1:1映射GMBS）。
// 无独立APP，GMBS能力同时被VOS编排（治理运营场）。
package gos

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/bos"
	"ziway/backend/pkg/response"
)

type Orchestrator struct{ bos.BaseOrchestrator }

func New(deps bos.Dependencies) *Orchestrator {
	return &Orchestrator{BaseOrchestrator: bos.NewBaseOrchestrator("gos", deps)}
}

func (o *Orchestrator) MBSDependencies() []string { return []string{"gms"} }

func (o *Orchestrator) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"os": "gos", "status": "ok",
			"desc":     "风控事业场编排器（GMBS风控直通）",
			"mbs_deps": o.MBSDependencies(),
			"app":      "无独立APP（被VOS编排消费）",
		})
	})
	// P0 transparent proxy: /proxy/gms/* → MBS /api/v1/gms/*
	o.MountProxy(rg, "gms")
	// Risk control endpoints
	rg.GET("/risks/overview", o.RiskOverview)
	rg.GET("/rules/list", o.RiskRuleList)
}

// RiskOverview — 风控概览
func (o *Orchestrator) RiskOverview(c *gin.Context) {
	response.OK(c, gin.H{
		"active_risks":    0,
		"mitigated_today": 0,
		"rule_hits_24h":   0,
		"note":            "P0 placeholder — gms risk overview pending gRPC",
	})
}

// RiskRuleList — 风控规则列表
func (o *Orchestrator) RiskRuleList(c *gin.Context) {
	response.OK(c, gin.H{
		"rules": []interface{}{},
		"note":  "P0 placeholder — gms risk rules pending gRPC",
	})
}
