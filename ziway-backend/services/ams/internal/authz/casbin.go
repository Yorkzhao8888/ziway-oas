package authz

import (
	"fmt"

	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Enforcer Casbin鉴权引擎封装
type Enforcer struct {
	enforcer *casbin.Enforcer
	logger   *zap.Logger
}

// NewEnforcer 初始化Casbin RBAC+ABAC引擎
func NewEnforcer(db *gorm.DB, modelPath string, logger *zap.Logger) (*Enforcer, error) {
	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		return nil, fmt.Errorf("create casbin adapter: %w", err)
	}

	e, err := casbin.NewEnforcer(modelPath, adapter)
	if err != nil {
		return nil, fmt.Errorf("create casbin enforcer: %w", err)
	}

	if err := e.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("load casbin policy: %w", err)
	}

	logger.Info("casbin enforcer initialized",
		zap.Int("policies", len(e.GetPolicy())),
		zap.Int("roles", len(e.GetGroupingPolicy())),
	)

	return &Enforcer{enforcer: e, logger: logger}, nil
}

// Enforce 检查权限
// userID: 用户X*PZ#编号
// domain: 事业场（mall/shop/bos...）
// resource: API路径
// action: HTTP方法
func (e *Enforcer) Enforce(userID, domain, resource, action string) (bool, error) {
	return e.enforcer.Enforce(userID, domain, resource, action)
}

// AddRoleForUser 在指定domain下给用户授予角色（戴帽子）
func (e *Enforcer) AddRoleForUser(userID, role, domain string) error {
	_, err := e.enforcer.AddGroupingPolicy(userID, role, domain)
	if err != nil {
		return fmt.Errorf("add role for user: %w", err)
	}
	return e.enforcer.SavePolicy()
}

// RemoveRoleForUser 移除用户在指定domain下的角色（摘帽子）
func (e *Enforcer) RemoveRoleForUser(userID, role, domain string) error {
	_, err := e.enforcer.RemoveGroupingPolicy(userID, role, domain)
	if err != nil {
		return fmt.Errorf("remove role for user: %w", err)
	}
	return e.enforcer.SavePolicy()
}

// GetRolesForUser 获取用户在指定domain下的所有角色
func (e *Enforcer) GetRolesForUser(userID, domain string) ([]string, error) {
	return e.enforcer.GetRolesForUser(userID, domain)
}

// AddPolicy 添加权限策略
func (e *Enforcer) AddPolicy(role, domain, resource, action string) error {
	_, err := e.enforcer.AddPolicy(role, domain, resource, action, "allow")
	if err != nil {
		return fmt.Errorf("add policy: %w", err)
	}
	return e.enforcer.SavePolicy()
}

// ReloadPolicy 热加载策略
func (e *Enforcer) ReloadPolicy() error {
	return e.enforcer.LoadPolicy()
}

// AddPolicyWithEffect 添加带effect的策略（allow/deny）
func (e *Enforcer) AddPolicyWithEffect(role, domain, resource, action, effect string) error {
	if effect == "" {
		effect = "allow"
	}
	_, err := e.enforcer.AddPolicy(role, domain, resource, action, effect)
	if err != nil {
		return fmt.Errorf("add policy: %w", err)
	}
	return e.enforcer.SavePolicy()
}

// RemovePolicy 移除策略
func (e *Enforcer) RemovePolicy(role, domain, resource, action, effect string) error {
	rules := []string{role, domain, resource, action}
	if effect != "" {
		rules = append(rules, effect)
	}
	_, err := e.enforcer.RemovePolicy(rules)
	if err != nil {
		return fmt.Errorf("remove policy: %w", err)
	}
	return e.enforcer.SavePolicy()
}

// AddGroupingPolicy 添加角色继承（g规则）
func (e *Enforcer) AddGroupingPolicy(user, role, domain string) error {
	_, err := e.enforcer.AddGroupingPolicy(user, role, domain)
	if err != nil {
		return fmt.Errorf("add grouping policy: %w", err)
	}
	return e.enforcer.SavePolicy()
}

// RemoveGroupingPolicy 移除角色继承
func (e *Enforcer) RemoveGroupingPolicy(user, role, domain string) error {
	_, err := e.enforcer.RemoveGroupingPolicy(user, role, domain)
	if err != nil {
		return fmt.Errorf("remove grouping policy: %w", err)
	}
	return e.enforcer.SavePolicy()
}

// GetAllPolicies 获取所有权限策略
func (e *Enforcer) GetAllPolicies() [][]string {
	return e.enforcer.GetPolicy()
}

// GetAllGroupingPolicies 获取所有角色继承关系
func (e *Enforcer) GetAllGroupingPolicies() [][]string {
	return e.enforcer.GetGroupingPolicy()
}
