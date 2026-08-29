// Package sos is the startup-incubation BOS — orchestrates sms.
// ZW-ARC-017: NEW — 创业孵化事业场，面向SU孵化创业者，产出新公司/业务主体。
// 注意：sos(创业孵化) ≠ tos(产品孵化/Lab)。
// APP: 孵化园APP（未来开发）。
package sos

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/bos"
	"ziway/backend/pkg/response"
)

type Orchestrator struct{ bos.BaseOrchestrator }

func New(deps bos.Dependencies) *Orchestrator {
	return &Orchestrator{BaseOrchestrator: bos.NewBaseOrchestrator("sos", deps)}
}

func (o *Orchestrator) MBSDependencies() []string { return []string{"sms"} }

func (o *Orchestrator) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"os": "sos", "status": "ok",
			"desc":     "创业孵化事业场编排器（SMBS孵化管理）",
			"mbs_deps": o.MBSDependencies(),
			"app":      "孵化园APP（未来开发）",
		})
	})
	// P0 transparent proxy: /proxy/sms/* → MBS /api/v1/sms/*
	o.MountProxy(rg, "sms")
	// Startup incubation endpoints
	rg.GET("/startups/pipeline", o.StartupPipeline)
	rg.GET("/incubators/list", o.IncubatorList)
}

// StartupPipeline — 孵化项目管线
func (o *Orchestrator) StartupPipeline(c *gin.Context) {
	response.OK(c, gin.H{
		"applied":    0,
		"screening":  0,
		"incubating": 0,
		"graduated":  0,
		"note":       "P0 placeholder — sms startup pipeline pending gRPC",
	})
}

// IncubatorList — 孵化器列表
func (o *Orchestrator) IncubatorList(c *gin.Context) {
	response.OK(c, gin.H{
		"incubators": []interface{}{},
		"note":       "P0 placeholder — sms incubator catalog pending gRPC",
	})
}
