package handlers

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ziway.ams/internal/authz"
	"ziway.ams/internal/pkg"
	"ziway.ams/internal/repository"
)

// OASHandler OAS治理底座对接接口
// 处理OAS下发的策略同步、角色模板更新、权限边界变更
type OASHandler struct {
	enforcer *authz.Enforcer
	roleRepo *repository.RoleRepo
	logger   *zap.Logger
}

func NewOASHandler(enforcer *authz.Enforcer, roleRepo *repository.RoleRepo, logger *zap.Logger) *OASHandler {
	return &OASHandler{enforcer: enforcer, roleRepo: roleRepo, logger: logger}
}

// PolicySyncRequest OAS下发的策略变更请求
type PolicySyncRequest struct {
	Version   string         `json:"version" binding:"required"`   // OAS策略版本号
	Operation string         `json:"operation" binding:"required"` // add_p / remove_p / add_g / remove_g / reload
	Ptype     string         `json:"ptype" binding:"required"`     // p 或 g
	Rules     [][]string     `json:"rules" binding:"required,min=1"`
}

// SyncPolicy POST /api/v1/internal/sync-policy
// OAS Owner/Admin Plane 在策略变更后调用此接口，AMS接收后更新Casbin并热加载
// Header: X-Service-Token
// Body示例：
//
//	{
//	  "version": "20260824-001",
//	  "operation": "add_p",
//	  "ptype": "p",
//	  "rules": [["CU","mall","/api/v1/orders","POST","allow"]]
//	}
func (h *OASHandler) SyncPolicy(c *gin.Context) {
	var req PolicySyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	var err error
	switch req.Operation {
	case "add_p":
		for _, rule := range req.Rules {
			if len(rule) < 4 {
				pkg.BadRequest(c, "policy rule must have at least 4 fields: role, domain, resource, action")
				return
			}
			effect := "allow"
			if len(rule) >= 5 {
				effect = rule[4]
			}
			if e := h.enforcer.AddPolicyWithEffect(rule[0], rule[1], rule[2], rule[3], effect); e != nil {
				err = e
				break
			}
		}
	case "remove_p":
		for _, rule := range req.Rules {
			if len(rule) < 4 {
				continue
			}
			effect := "allow"
			if len(rule) >= 5 {
				effect = rule[4]
			}
			if e := h.enforcer.RemovePolicy(rule[0], rule[1], rule[2], rule[3], effect); e != nil {
				err = e
				break
			}
		}
	case "add_g":
		for _, rule := range req.Rules {
			if len(rule) < 3 {
				pkg.BadRequest(c, "grouping rule must have 3 fields: user, role, domain")
				return
			}
			if e := h.enforcer.AddGroupingPolicy(rule[0], rule[1], rule[2]); e != nil {
				err = e
				break
			}
		}
	case "remove_g":
		for _, rule := range req.Rules {
			if len(rule) < 3 {
				continue
			}
			if e := h.enforcer.RemoveGroupingPolicy(rule[0], rule[1], rule[2]); e != nil {
				err = e
				break
			}
		}
	case "reload":
		err = h.enforcer.ReloadPolicy()
	default:
		pkg.BadRequest(c, "unsupported operation: "+req.Operation)
		return
	}

	if err != nil {
		h.logger.Error("policy sync failed",
			zap.String("version", req.Version),
			zap.String("operation", req.Operation),
			zap.Error(err),
		)
		pkg.InternalError(c, "policy sync failed: "+err.Error())
		return
	}

	h.logger.Info("policy synced from OAS",
		zap.String("version", req.Version),
		zap.String("operation", req.Operation),
		zap.Int("rules", len(req.Rules)),
	)

	pkg.OK(c, gin.H{
		"version": req.Version,
		"status":  "applied",
		"rules":   len(req.Rules),
	})
}

// ReloadRoles POST /api/v1/internal/reload-roles
// OAS更新角色模板后调用，AMS重新同步默认角色
func (h *OASHandler) ReloadRoles(c *gin.Context) {
	if err := h.roleRepo.EnsureDefaultRoles(); err != nil {
		pkg.InternalError(c, "reload roles failed: "+err.Error())
		return
	}
	if err := h.enforcer.ReloadPolicy(); err != nil {
		pkg.InternalError(c, "reload policy failed: "+err.Error())
		return
	}
	pkg.OK(c, gin.H{"status": "reloaded"})
}

// PolicySnapshot GET /api/v1/internal/policy-snapshot
// 返回当前AMS缓存的策略快照，供OAS对账使用
func (h *OASHandler) PolicySnapshot(c *gin.Context) {
	policies := h.enforcer.GetAllPolicies()
	grouping := h.enforcer.GetAllGroupingPolicies()
	pkg.OK(c, gin.H{
		"policies":      policies,
		"grouping":      grouping,
		"policy_count":  len(policies),
		"group_count":   len(grouping),
	})
}
