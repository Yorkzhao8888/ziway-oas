package repository

import (
	"errors"

	"gorm.io/gorm"
	"ziway.ams/internal/model"
)

// UserRepo 用户数据访问
type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

// Create 创建用户
func (r *UserRepo) Create(u *model.User) error {
	return r.db.Create(u).Error
}

// GetByID 按主键查询
func (r *UserRepo) GetByID(id int64) (*model.User, error) {
	var u model.User
	err := r.db.Preload("Roles").First(&u, id).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByUserID 按X*PZ#编号查询
func (r *UserRepo) GetByUserID(userID string) (*model.User, error) {
	var u model.User
	err := r.db.Preload("Roles").Where("user_id = ?", userID).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByAccount 按手机号或邮箱查询（登录用）
func (r *UserRepo) GetByAccount(account string) (*model.User, error) {
	var u model.User
	err := r.db.Preload("Roles").
		Where("phone = ? OR email = ?", account, account).
		First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// GetByPhone 按手机号查询
func (r *UserRepo) GetByPhone(phone string) (*model.User, error) {
	var u model.User
	err := r.db.Where("phone = ?", phone).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// Update 更新用户
func (r *UserRepo) Update(u *model.User) error {
	return r.db.Save(u).Error
}

// List 分页查询
func (r *UserRepo) List(page, size int) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	r.db.Model(&model.User{}).Count(&total)
	offset := (page - 1) * size
	err := r.db.Preload("Roles").Offset(offset).Limit(size).Find(&users).Error
	return users, total, err
}

// AssignRole 在指定domain下分配角色
func (r *UserRepo) AssignRole(userID int64, roleID int64, domain string) error {
	ur := model.UserRole{
		UserID: userID,
		RoleID: roleID,
		Domain: domain,
	}
	return r.db.Where("user_id = ? AND role_id = ? AND domain = ?", userID, roleID, domain).
		FirstOrCreate(&ur).Error
}

// GetRolesByDomain 获取用户在指定domain下的角色
func (r *UserRepo) GetRolesByDomain(userID int64, domain string) ([]model.Role, error) {
	var roles []model.Role
	err := r.db.Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ? AND user_roles.domain = ?", userID, domain).
		Find(&roles).Error
	return roles, err
}
