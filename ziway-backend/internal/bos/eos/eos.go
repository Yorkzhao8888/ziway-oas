// Package eos is the supply BOS — orchestrates ems for ziway-Market (supply + four shop types).
package eos

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/bos"
	"ziway/backend/pkg/response"
)

type Orchestrator struct{ bos.BaseOrchestrator }

func New(deps bos.Dependencies) *Orchestrator {
	return &Orchestrator{BaseOrchestrator: bos.NewBaseOrchestrator("eos", deps)}
}

func (o *Orchestrator) MBSDependencies() []string { return []string{"ems"} }

func (o *Orchestrator) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"os": "eos", "status": "ok",
			"desc":     "供给事业场编排器（集市+四种铺面）",
			"mbs_deps": o.MBSDependencies(),
		})
	})
	o.MountProxy(rg, "ems")
	rg.GET("/supply/pool", o.SupplyPool)
	rg.GET("/stores/summary", o.StoresSummary)
	rg.GET("/orders/pending", o.PendingOrders)
}

func (o *Orchestrator) SupplyPool(c *gin.Context) {
	response.OK(c, gin.H{
		"items": []interface{}{},
		"note":  "P0 placeholder — ems supply pool pending gRPC",
	})
}

func (o *Orchestrator) StoresSummary(c *gin.Context) {
	response.OK(c, gin.H{
		"supplier_stores":  0,
		"producer_stores":  0,
		"warehouse_stores": 0,
		"logistics_stores": 0,
		"note":             "P0 placeholder — ems four shop types pending gRPC",
	})
}

func (o *Orchestrator) PendingOrders(c *gin.Context) {
	response.OK(c, gin.H{
		"purchase_orders": 0,
		"work_orders":     0,
		"outbound_orders": 0,
		"shipments":       0,
		"note":            "P0 placeholder — ems order aggregation pending gRPC",
	})
}
