// Package hos is the HR/talent BOS — orchestrates hms for ziway-Mate.
package hos

import (
	"github.com/gin-gonic/gin"

	"ziway/backend/internal/bos"
	"ziway/backend/pkg/response"
)

type Orchestrator struct{ bos.BaseOrchestrator }

func New(deps bos.Dependencies) *Orchestrator {
	return &Orchestrator{BaseOrchestrator: bos.NewBaseOrchestrator("hos", deps)}
}

func (o *Orchestrator) MBSDependencies() []string { return []string{"hms"} }

func (o *Orchestrator) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{
			"os": "hos", "status": "ok",
			"desc":     "职聘事业场编排器",
			"mbs_deps": o.MBSDependencies(),
		})
	})
	o.MountProxy(rg, "hms")
	rg.GET("/org/headcount", o.Headcount)
	rg.GET("/attendance/today", o.AttendanceToday)
	rg.GET("/leave/pending", o.PendingLeaves)
}

func (o *Orchestrator) Headcount(c *gin.Context) {
	response.OK(c, gin.H{
		"total_employees": 0,
		"active":          0,
		"note":            "P0 placeholder — hms headcount pending gRPC",
	})
}

func (o *Orchestrator) AttendanceToday(c *gin.Context) {
	response.OK(c, gin.H{
		"checked_in":  0,
		"absent":      0,
		"late":        0,
		"note":        "P0 placeholder — hms attendance pending gRPC",
	})
}

func (o *Orchestrator) PendingLeaves(c *gin.Context) {
	response.OK(c, gin.H{
		"pending": 0,
		"note":    "P0 placeholder — hms leave approvals pending gRPC",
	})
}
