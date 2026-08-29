// Package bos defines the unified interface for all BOS (Business Operating System) modules.
// BOS modules orchestrate MBS services; they do NOT hold their own databases.
package bos

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Orchestrator is the interface every BOS package must implement.
type Orchestrator interface {
	// Name returns the BOS identifier (e.g., "cos", "eos")
	Name() string
	// MBSDependencies returns the list of MBS names this BOS orchestrates
	MBSDependencies() []string
	// RegisterRoutes registers HTTP routes for this BOS
	RegisterRoutes(rg *gin.RouterGroup)
}

// Dependencies holds shared dependencies for BOS orchestrators.
type Dependencies struct {
	Logger *zap.Logger
	// MBSCaller allows BOS to call MBS via gRPC (P0: in-process, P1: network)
	MBSCaller MBSCaller
	// MBSAddr is the address (host:port) of the MBS process for P0 HTTP proxy.
	MBSAddr string
}

// MBSCaller abstracts MBS service invocation.
// P0: direct function call; P1: gRPC.
type MBSCaller interface {
	// CallMBS invokes an MBS method by name and returns the result.
	CallMBS(mbsName, method string, payload interface{}) (interface{}, error)
}

// BaseOrchestrator provides common fields for BOS implementations.
type BaseOrchestrator struct {
	name string
	deps Dependencies
}

func NewBaseOrchestrator(name string, deps Dependencies) BaseOrchestrator {
	return BaseOrchestrator{name: name, deps: deps}
}

func (b *BaseOrchestrator) Name() string        { return b.name }
func (b *BaseOrchestrator) Log() *zap.Logger    { return b.deps.Logger }
func (b *BaseOrchestrator) Caller() MBSCaller   { return b.deps.MBSCaller }

// MountProxy registers a catch-all /proxy/* route that forwards to the
// given MBS on the MBS process. Each BOS should call this for every MBS
// it orchestrates (e.g. cos → cms, dos → dms+fms).
//
// APP call example (cos→cms):
//   GET :8082/api/v1/bos/cos/proxy/cms/customers
//   → proxied to
//   GET :8081/api/v1/cms/customers
func (b *BaseOrchestrator) MountProxy(rg *gin.RouterGroup, mbsName string) {
	if b.deps.MBSAddr == "" {
		// No MBS addr configured — mount a stub that explains the gap.
		rg.Any("/proxy/"+mbsName+"/*any", func(c *gin.Context) {
			c.JSON(503, gin.H{
				"error":  "MBS proxy not configured",
				"ms":    mbsName,
				"hint":   "set mbs.addr in bos config (default localhost:8081)",
			})
		})
		return
	}
	rg.Any("/proxy/"+mbsName+"/*any", MBSProxy(mbsName, b.deps.MBSAddr))
}
