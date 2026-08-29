package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"ziway.ams/internal/authz"
	"ziway.ams/internal/idgen"
	"ziway.ams/internal/jwt"
	"ziway.ams/internal/model"
	"ziway.ams/internal/repository"
)

// AuthService 认证鉴权业务逻辑
type AuthService struct {
	userRepo *repository.UserRepo
	roleRepo *repository.RoleRepo
	issuer   *jwt.Issuer
	verifier *jwt.Verifier
	enforcer *authz.Enforcer
	rdb      *redis.Client
	logger   *zap.Logger
}

func NewAuthService(
	userRepo *repository.UserRepo,
	roleRepo *repository.RoleRepo,
	issuer *jwt.Issuer,
	verifier *jwt.Verifier,
	enforcer *authz.Enforcer,
	rdb *redis.Client,
	logger *zap.Logger,
) *AuthService {
	return &AuthService{
		userRepo: userRepo,
		roleRepo: roleRepo,
		issuer:   issuer,
		verifier: verifier,
		enforcer: enforcer,
		rdb:      rdb,
		logger:   logger,
	}
}

// Login 登录（人类用户）
func (s *AuthService) Login(ctx context.Context, req *model.LoginRequest) (*model.TokenResponse, error) {
	// NHI登录不使用密码，走IssueNHIToken
	identityType := req.IdentityType
	if identityType == "" {
		identityType = model.IdentityTypeHuman
	}
	if identityType == model.IdentityTypeNHI {
		return nil, errors.New("NHI must use agent token endpoint, not password login")
	}

	user, err := s.userRepo.GetByAccount(req.Account)
	if err != nil {
		s.logger.Error("login query error", zap.Error(err))
		return nil, fmt.Errorf("internal error")
	}
	if user == nil {
		return nil, errors.New("account not found")
	}
	if user.Status != 1 {
		return nil, errors.New("account disabled")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, errors.New("invalid password")
	}

	domain := req.Domain
	if domain == "" {
		domain = "mall"
	}

	roles, activeRole := s.resolveRoles(user, domain, "")
	tokenID := uuid.New().String()

	claims := &jwt.Claims{
		UserID:       user.UserID,
		IdentityType: model.IdentityTypeHuman,
		Roles:        roles,
		ActiveRole:   activeRole,
		Domain:       domain,
		TokenID:      tokenID,
	}

	accessToken, expiresIn, err := s.issuer.IssueAccessToken(claims)
	if err != nil {
		return nil, fmt.Errorf("issue token: %w", err)
	}
	refreshToken, err := s.issuer.IssueRefreshToken(user.UserID, model.IdentityTypeHuman, tokenID)
	if err != nil {
		return nil, fmt.Errorf("issue refresh token: %w", err)
	}

	s.storeRefresh(ctx, tokenID, user.UserID)

	s.logger.Info("user logged in",
		zap.String("user_id", user.UserID),
		zap.String("domain", domain),
		zap.Strings("roles", roles),
		zap.String("active_role", activeRole),
	)

	return s.buildTokenResponse(accessToken, refreshToken, expiresIn, user, roles, activeRole, domain), nil
}

// IssueNHIToken 为Agent签发NHI Token
// Agent本身无独立权限，继承委托用户(delegatedBy)的权限边界
func (s *AuthService) IssueNHIToken(ctx context.Context, agentService, delegatedBy string) (*model.TokenResponse, error) {
	// 查找委托用户
	delegator, err := s.userRepo.GetByUserID(delegatedBy)
	if err != nil || delegator == nil {
		return nil, errors.New("delegator not found")
	}
	if delegator.Status != 1 {
		return nil, errors.New("delegator disabled")
	}

	// 查找或创建NHI用户记录
	nhiUser, err := s.userRepo.GetByAccount(agentService + "@nhi.ziway")
	if err != nil || nhiUser == nil {
		nhiUser = &model.User{
			UserID:       idgen.Generate(idgen.NHIPrefix),
			IdentityType: model.IdentityTypeNHI,
			Email:        agentService + "@nhi.ziway",
			Nickname:     agentService,
			AgentService: agentService,
			DelegatedBy:  delegatedBy,
			Status:       1,
		}
		if err := s.userRepo.Create(nhiUser); err != nil {
			return nil, fmt.Errorf("create nhi: %w", err)
		}
		// 分配NHI角色
		if nhiRole, e := s.roleRepo.GetByCode("NHI"); e == nil && nhiRole != nil {
			_ = s.userRepo.AssignRole(nhiUser.ID, nhiRole.ID, "ams")
		}
	}

	// NHI继承委托方的角色和domain（取委托方第一个角色domain）
	roles := make([]string, 0)
	activeRole := "NHI"
	domain := "ams"
	if len(delegator.Roles) > 0 {
		domain = delegator.Roles[0].Domain
		for _, r := range delegator.Roles {
			roles = append(roles, r.RoleCode)
		}
	}
	// NHI始终包含自身标识
	roles = append([]string{"NHI"}, roles...)

	tokenID := uuid.New().String()
	claims := &jwt.Claims{
		UserID:       nhiUser.UserID,
		IdentityType: model.IdentityTypeNHI,
		Roles:        roles,
		ActiveRole:   activeRole,
		Domain:       domain,
		AgentService: agentService,
		DelegatedBy:  delegatedBy,
		TokenID:      tokenID,
	}

	// NHI Token有效期较短（1小时）
	accessToken, expiresIn, err := s.issuer.IssueAccessToken(claims)
	if err != nil {
		return nil, err
	}
	refreshToken, _ := s.issuer.IssueRefreshToken(nhiUser.UserID, model.IdentityTypeNHI, tokenID)
	s.storeRefresh(ctx, tokenID, nhiUser.UserID)

	s.logger.Info("NHI token issued",
		zap.String("agent", agentService),
		zap.String("delegated_by", delegatedBy),
		zap.String("nhi_id", nhiUser.UserID),
	)

	return s.buildTokenResponse(accessToken, refreshToken, expiresIn, nhiUser, roles, activeRole, domain), nil
}

// Register 注册
func (s *AuthService) Register(ctx context.Context, phone, email, password, nickname string) (*model.User, error) {
	if phone != "" {
		existing, _ := s.userRepo.GetByPhone(phone)
		if existing != nil {
			return nil, errors.New("phone already registered")
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &model.User{
		UserID:       idgen.Generate("CU"), // 默认消费者
		IdentityType: model.IdentityTypeHuman,
		Phone:        phone,
		Email:        email,
		PasswordHash: string(hash),
		Nickname:     nickname,
		Status:       1,
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	cuRole, err := s.roleRepo.GetByCode("CU")
	if err == nil && cuRole != nil {
		_ = s.userRepo.AssignRole(user.ID, cuRole.ID, "mall")
		_ = s.enforcer.AddRoleForUser(user.UserID, "CU", "mall")
	}

	return user, nil
}

// VerifyToken 验证Token
func (s *AuthService) VerifyToken(tokenString string) (*jwt.Claims, error) {
	return s.verifier.Verify(tokenString)
}

// CheckPermission 检查权限
func (s *AuthService) CheckPermission(userID, domain, resource, action string) (bool, error) {
	return s.enforcer.Enforce(userID, domain, resource, action)
}

// SwitchHat 切换帽子（在不同事业场切换角色，可指定帽子角色）
func (s *AuthService) SwitchHat(ctx context.Context, userID, newDomain, roleCode string) (*model.TokenResponse, error) {
	user, err := s.userRepo.GetByUserID(userID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	roles, activeRole := s.resolveRoles(user, newDomain, roleCode)
	if len(roles) == 0 {
		return nil, errors.New("no role in target domain")
	}

	tokenID := uuid.New().String()
	claims := &jwt.Claims{
		UserID:       user.UserID,
		IdentityType: user.IdentityType,
		Roles:        roles,
		ActiveRole:   activeRole,
		Domain:       newDomain,
		TokenID:      tokenID,
	}
	if user.IsNHI() {
		claims.AgentService = user.AgentService
		claims.DelegatedBy = user.DelegatedBy
	}

	accessToken, expiresIn, err := s.issuer.IssueAccessToken(claims)
	if err != nil {
		return nil, err
	}
	refreshToken, _ := s.issuer.IssueRefreshToken(user.UserID, user.IdentityType, tokenID)
	s.storeRefresh(ctx, tokenID, user.UserID)

	return s.buildTokenResponse(accessToken, refreshToken, expiresIn, user, roles, activeRole, newDomain), nil
}

// Logout 登出
func (s *AuthService) Logout(ctx context.Context, tokenID string) error {
	refreshKey := fmt.Sprintf("ams:refresh:%s", tokenID)
	blacklistKey := fmt.Sprintf("ams:blacklist:%s", tokenID)
	s.rdb.Set(ctx, blacklistKey, "1", 2*time.Hour)
	return s.rdb.Del(ctx, refreshKey).Err()
}

// resolveRoles 解析用户在指定domain下的角色，可指定佩戴的帽子
func (s *AuthService) resolveRoles(user *model.User, domain, preferRole string) ([]string, string) {
	dbRoles, err := s.userRepo.GetRolesByDomain(user.ID, domain)
	if err != nil || len(dbRoles) == 0 {
		// fallback: 用户所有角色
		dbRoles = user.Roles
	}
	roleCodes := make([]string, 0, len(dbRoles))
	activeRole := ""
	for _, r := range dbRoles {
		roleCodes = append(roleCodes, r.RoleCode)
		if preferRole != "" && r.RoleCode == preferRole {
			activeRole = r.RoleCode
		}
	}
	if activeRole == "" && len(roleCodes) > 0 {
		activeRole = roleCodes[0]
	}
	return roleCodes, activeRole
}

func (s *AuthService) storeRefresh(ctx context.Context, tokenID, userID string) {
	key := fmt.Sprintf("ams:refresh:%s", tokenID)
	s.rdb.Set(ctx, key, userID, 7*24*time.Hour)
}

func (s *AuthService) buildTokenResponse(access, refresh string, expires int64, user *model.User, roles []string, activeRole, domain string) *model.TokenResponse {
	return &model.TokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    expires,
		User: &model.UserInfo{
			UserID:       user.UserID,
			IdentityType: user.IdentityType,
			Nickname:     user.Nickname,
			AvatarURL:    user.AvatarURL,
			Roles:        roles,
			ActiveRole:   activeRole,
			Domain:       domain,
			AgentService: user.AgentService,
			DelegatedBy:  user.DelegatedBy,
		},
	}
}
