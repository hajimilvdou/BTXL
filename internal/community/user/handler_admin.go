package user

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/db"
)

// ============================================================
// 管理端 Handler — 用户列表 / 封禁 / 解封
// ============================================================

// AdminUserHandler 管理端用户 Handler
type AdminUserHandler struct {
	userSvc *Service
}

// NewAdminUserHandler 创建管理端用户 Handler
func NewAdminUserHandler(userSvc *Service) *AdminUserHandler {
	return &AdminUserHandler{userSvc: userSvc}
}

// RegisterRoutes 注册管理端路由
func (h *AdminUserHandler) RegisterRoutes(rg *gin.RouterGroup) {
	admin := rg.Group("/admin/users")
	admin.GET("", h.ListUsers)
	admin.POST("/:id/ban", h.BanUser)
	admin.POST("/:id/unban", h.UnbanUser)
	admin.PUT("/:id/role", h.UpdateUserRole)
	admin.PUT("/:id/status", h.UpdateUserStatus)
}

// ListUsers 列出所有用户
func (h *AdminUserHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	search := c.Query("search")

	users, total, err := h.userSvc.ListUsers(c.Request.Context(), db.ListUsersOpts{
		Page:     page,
		PageSize: pageSize,
		Role:     stringPtr(c.Query("role")),
		Status:   stringPtr(c.Query("status")),
		Search:   search,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      users,
		"users":     users,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// BanUser 封禁用户
func (h *AdminUserHandler) BanUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户 ID"})
		return
	}
	if err := h.userSvc.BanUser(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "用户已封禁"})
}

// UnbanUser 解封用户
func (h *AdminUserHandler) UnbanUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户 ID"})
		return
	}
	if err := h.userSvc.UnbanUser(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "用户已解封"})
}

type updateRoleRequest struct {
	Role db.Role `json:"role" binding:"required"`
}

// UpdateUserRole 更新用户角色。
func (h *AdminUserHandler) UpdateUserRole(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户 ID"})
		return
	}
	var req updateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	user, err := h.userSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	user.Role = req.Role
	user.UpdatedAt = time.Now()
	if err := h.userSvc.store.UpdateUser(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "角色更新成功"})
}

type updateStatusRequest struct {
	Status db.UserStatus `json:"status" binding:"required"`
}

// UpdateUserStatus 更新用户状态。
func (h *AdminUserHandler) UpdateUserStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户 ID"})
		return
	}
	var req updateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	user, err := h.userSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	user.Status = req.Status
	user.UpdatedAt = time.Now()
	if err := h.userSvc.store.UpdateUser(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "状态更新成功"})
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
