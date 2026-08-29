package ams

import (
	"context"
	"fmt"
	"time"

	"ziway/backend/pkg/cache"
	"ziway/backend/pkg/response"

	"github.com/gin-gonic/gin"
)

// PolicyRecord represents a cached RBAC policy row.
type PolicyRecord struct {
	Subject  string `json:"subject"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Effect   string `json:"effect"`
	Domain   string `json:"domain"`
	RoleType string `json:"role_type"`
	Status   string `json:"status"`
}

// policyCache is the L1→L2→L3 cache for RBAC policies.
// L3 origin queries the DB (shared with OAS in P0 dev mode).
var policyCache *cache.ThreeLevelCache[[]PolicyRecord]

func (s *Service) initPolicyCache() {
	policyCache = cache.NewThreeLevelCache[[]PolicyRecord](
		nil, // Redis not available in P0 dev
		30*time.Second,
		5*time.Minute,
		"ziway:rbac:policies",
		s.loadPoliciesFromDB,
	)
}

// loadPoliciesFromDB is the L3 origin — reads active RBAC policies from the shared rbac_policies table.
func (s *Service) loadPoliciesFromDB(ctx context.Context) ([]PolicyRecord, error) {
	var records []PolicyRecord
	type rbacRow struct {
		Subject  string
		Resource string
		Action   string
		Effect   string
		Domain   string
		RoleType string
		Status   string
	}
	var rows []rbacRow
	if err := s.DB().Table("rbac_policies").
		Where("status = ? AND deleted_at IS NULL", "active").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load rbac_policies: %w", err)
	}
	records = make([]PolicyRecord, 0, len(rows))
	for _, r := range rows {
		records = append(records, PolicyRecord{
			Subject:  r.Subject,
			Resource: r.Resource,
			Action:   r.Action,
			Effect:   r.Effect,
			Domain:   r.Domain,
			RoleType: r.RoleType,
			Status:   r.Status,
		})
	}
	return records, nil
}

// registerPolicyCacheRoutes adds cache management endpoints.
func (s *Service) registerPolicyCacheRoutes(rg *gin.RouterGroup) {
	rg.GET("/policies/cached", func(c *gin.Context) {
		if policyCache == nil {
			response.OK(c, gin.H{"policies": []PolicyRecord{}, "source": "uninitialized"})
			return
		}
		records, err := policyCache.Get(c.Request.Context(), "all")
		if err != nil {
			response.InternalError(c, "cache get failed: "+err.Error())
			return
		}
		response.OK(c, gin.H{"policies": records, "source": "L1/L2/L3"})
	})

	rg.POST("/policies/cache/invalidate", func(c *gin.Context) {
		if policyCache != nil {
			policyCache.InvalidateAll(c.Request.Context())
		}
		response.OK(c, gin.H{"message": "policy cache invalidated (L1+L2 cleared)"})
	})
}
