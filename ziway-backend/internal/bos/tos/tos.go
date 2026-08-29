// Package tos is the R&D/innovation BOS — orchestrates tms for ziway-Lab.
package tos

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/bos"
	"ziway/backend/pkg/response"
)

type Orchestrator struct{ bos.BaseOrchestrator }

func New(deps bos.Dependencies) *Orchestrator {
	return &Orchestrator{BaseOrchestrator: bos.NewBaseOrchestrator("tos", deps)}
}

func (o *Orchestrator) MBSDependencies() []string { return []string{"tms"} }

func (o *Orchestrator) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"os": "tos", "status": "ok",
			"desc":     "创研事业场编排器",
			"mbs_deps": o.MBSDependencies(),
		})
	})
	o.MountProxy(rg, "tms")
	rg.GET("/npi/pipeline", o.NPIPipeline)
	rg.GET("/products/catalog", o.ProductCatalog)
}

// NPIPipeline — NPI项目管线视图
func (o *Orchestrator) NPIPipeline(c *gin.Context) {
	response.OK(c, gin.H{
		"pipeline": []interface{}{},
		"note":     "P0 placeholder — tms NPI pipeline pending gRPC",
	})
}

func (o *Orchestrator) ProductCatalog(c *gin.Context) {
	response.OK(c, gin.H{
		"products": []interface{}{},
		"note":     "P0 placeholder — tms product catalog pending gRPC",
	})
}
