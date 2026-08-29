// Package mbs defines the unified interface for all MBS (Micros Business Service) modules.
// Each MBS package must implement the Service interface and register itself via Register().
// P0: All MBS run in a single process (ziway-mbs), communicating via in-process calls.
// P1: Each MBS can be extracted to independent service with zero code change (gRPC).
package mbs

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Service is the unified interface every MBS module must implement.
type Service interface {
	// Name returns the MBS identifier (e.g., "cms", "ems")
	Name() string
	// Schema returns the database schema name (e.g., "mbs_cms")
	Schema() string
	// RegisterRoutes registers HTTP routes on the given router group.
	RegisterRoutes(rg *gin.RouterGroup)
	// AutoMigrate runs database migrations for this MBS.
	AutoMigrate(db *gorm.DB) error
}

// Dependencies holds shared dependencies injected into each MBS.
type Dependencies struct {
	DB     *gorm.DB
	Logger *zap.Logger
	// EventBus for cross-MBS async events (Kafka in P1, in-process in P0)
	EventBus EventBus
}

// EventBus abstracts cross-MBS event publishing.
// P0: in-process channel; P1: Kafka.
type EventBus interface {
	Publish(topic string, event []byte) error
	Subscribe(topic string, handler func(event []byte)) error
}

// BaseService provides common fields for MBS implementations.
type BaseService struct {
	name   string
	schema string
	deps   Dependencies
}

func NewBaseService(name, schema string, deps Dependencies) BaseService {
	return BaseService{name: name, schema: schema, deps: deps}
}

func (b *BaseService) Name() string   { return b.name }
func (b *BaseService) Schema() string { return b.schema }
func (b *BaseService) DB() *gorm.DB   { return b.deps.DB }
func (b *BaseService) Log() *zap.Logger { return b.deps.Logger }
func (b *BaseService) Events() EventBus { return b.deps.EventBus }
