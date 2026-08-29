package repository

import (
	"gorm.io/gorm"
	"ziway.ams/internal/model"
)

// RoleRepo 角色数据访问
type RoleRepo struct {
	db *gorm.DB
}

func NewRoleRepo(db *gorm.DB) *RoleRepo {
	return &RoleRepo{db: db}
}

// GetByCode 按角色编码查询
func (r *RoleRepo) GetByCode(code string) (*model.Role, error) {
	var role model.Role
	err := r.db.Where("role_code = ?", code).First(&role).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// ListAll 查询所有角色
func (r *RoleRepo) ListAll() ([]model.Role, error) {
	var roles []model.Role
	err := r.db.Find(&roles).Error
	return roles, err
}

// ListByType 按类型查询角色（base/hat/nhi）
func (r *RoleRepo) ListByType(roleType string) ([]model.Role, error) {
	var roles []model.Role
	err := r.db.Where("role_type = ? AND status = 1", roleType).Find(&roles).Error
	return roles, err
}

// Create 创建角色
func (r *RoleRepo) Create(role *model.Role) error {
	return r.db.Create(role).Error
}

// GetByDomain 按domain查询角色
func (r *RoleRepo) GetByDomain(domain string) ([]model.Role, error) {
	var roles []model.Role
	err := r.db.Where("domain = ? OR domain = '*' OR domain IS NULL", domain).
		Where("status = 1").Find(&roles).Error
	return roles, err
}

// EnsureDefaultRoles 确保12U + 2帽子 + NHI默认角色存在
// 依据：知味生态12U角色系统正式定义（LOCKED v4.9）
func (r *RoleRepo) EnsureDefaultRoles() error {
	defaults := []model.Role{
		// === 12U 基础角色 ===
		{RoleCode: "CU", RoleName: "消费者", RoleType: model.RoleTypeBase, Domain: "mall",
			Description: "浏览、下单、支付、评价、退款申请"},
		{RoleCode: "DU", RoleName: "门店经营者", RoleType: model.RoleTypeBase, Domain: "shop",
			Description: "商品管理、接单、库存、经营看板、经营级VCASE"},
		{RoleCode: "PU", RoleName: "创研者", RoleType: model.RoleTypeBase, Domain: "lab",
			Description: "NPI项目、产品定义、原型研发、知识产权"},
		{RoleCode: "EU", RoleName: "供应商/资源管理者", RoleType: model.RoleTypeBase, Domain: "market",
			Description: "货源发布、采购、账期、仓储、产能、设备调度（Dyard协同）"},
		{RoleCode: "HU", RoleName: "工作者", RoleType: model.RoleTypeBase, Domain: "mate",
			Description: "接单、排班、工单执行、打卡、提现、技能认证"},
		{RoleCode: "OU", RoleName: "治理者/董事长", RoleType: model.RoleTypeBase, Domain: "owner",
			Description: "VCASE/FCASE/ICASE/XCASE审批、多重签名、战略决策（主入口：ziway-Owner）"},
		{RoleCode: "GU", RoleName: "监管者", RoleType: model.RoleTypeBase, Domain: "case",
			Description: "合规审计、神案执行监控、异常告警（只读）"},
		{RoleCode: "AU", RoleName: "运维/系统管理员", RoleType: model.RoleTypeBase, Domain: "admin",
			Description: "服务启停、配置变更、API网关、消息队列管理、Agent熔断（主入口：ziway-Admin）"},
		{RoleCode: "FU", RoleName: "财务", RoleType: model.RoleTypeBase, Domain: "case",
			Description: "财务报表、对账、月结、税务、GP分配执行（财务中心）"},
		{RoleCode: "IU", RoleName: "投资人", RoleType: model.RoleTypeBase, Domain: "case",
			Description: "投资专案查看、尽调、投资决议投票、分红查看（投资中心）"},
		{RoleCode: "VU", RoleName: "运营执行官", RoleType: model.RoleTypeBase, Domain: "vms",
			Description: "VCASE预算编制、资源调度执行、运营监控、报告生成（主入口：VMBS价值运营管控服务）"},
		{RoleCode: "SU", RoleName: "孵化创业者", RoleType: model.RoleTypeBase, Domain: "lab",
			Description: "孵化项目创建、资源申请、路演、接受投资、毕业（Lab+Market+Case孵化中心）"},

		// === 帽子角色（可穿戴，非独立U） ===
		{RoleCode: "CX", RoleName: "客户体验专员", RoleType: model.RoleTypeHat, Domain: "mall",
			ParentRole: "HU", Description: "客诉处理、工单跟进、客户回访、满意度调查（HU戴帽子）"},
		{RoleCode: "FX", RoleName: "退款专员", RoleType: model.RoleTypeHat, Domain: "case",
			ParentRole: "HU", Description: "退款审核、执行、争议处理、退款报表（可由HU或FU戴帽子）"},

		// === 非人类身份 ===
		{RoleCode: "NHI", RoleName: "非人类身份(Agent)", RoleType: model.RoleTypeNHI, Domain: "ams",
			Description: "ziway-Agent运行时身份，继承调用用户权限边界，本身无独立权限"},
	}

	for _, role := range defaults {
		var existing model.Role
		err := r.db.Where("role_code = ?", role.RoleCode).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := r.db.Create(&role).Error; err != nil {
				return err
			}
		} else if err == nil {
			// 更新已有角色的名称和描述（VU/SU等修正）
			updates := map[string]interface{}{
				"role_name":    role.RoleName,
				"role_type":    role.RoleType,
				"description":  role.Description,
				"parent_role":  role.ParentRole,
			}
			if err := r.db.Model(&existing).Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
