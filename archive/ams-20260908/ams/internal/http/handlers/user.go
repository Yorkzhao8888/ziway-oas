package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ziway.ams/internal/pkg"
	"ziway.ams/internal/repository"
)

// UserHandler 用户管理HTTP处理器
type UserHandler struct {
	userRepo *repository.UserRepo
	logger   *zap.Logger
}

func NewUserHandler(userRepo *repository.UserRepo, logger *zap.Logger) *UserHandler {
	return &UserHandler{userRepo: userRepo, logger: logger}
}

// GetUser GET /api/v1/users/:id
func (h *UserHandler) GetUser(c *gin.Context) {
	userID := c.Param("id")
	user, err := h.userRepo.GetByUserID(userID)
	if err != nil {
		pkg.NotFound(c, "user not found")
		return
	}
	pkg.OK(c, user)
}

// ListUsers GET /api/v1/users?page=1&size=20
func (h *UserHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	users, total, err := h.userRepo.List(page, size)
	if err != nil {
		pkg.InternalError(c, "failed to list users")
		return
	}

	pkg.OK(c, pkg.PageResult{
		Items: users,
		Total: total,
		Page:  page,
		Size:  size,
	})
}

// GetMyProfile GET /api/v1/users/me/profile
func (h *UserHandler) GetMyProfile(c *gin.Context) {
	userID := c.GetString("user_id")
	user, err := h.userRepo.GetByUserID(userID)
	if err != nil {
		pkg.NotFound(c, "profile not found")
		return
	}
	pkg.OK(c, user)
}
