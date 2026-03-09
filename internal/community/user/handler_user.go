package user

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/db"
)

// ============================================================
// 用户端 Handler — 个人信息 / API Key 重置
// ============================================================

// UserHandler 用户端 HTTP Handler
type UserHandler struct {
	userSvc         *Service
	quotaStore      db.QuotaStore
	credentialStore db.CredentialStore
}

// NewUserHandler 创建用户端 Handler
func NewUserHandler(userSvc *Service, quotaStore db.QuotaStore, credentialStore db.CredentialStore) *UserHandler {
	return &UserHandler{userSvc: userSvc, quotaStore: quotaStore, credentialStore: credentialStore}
}

// RegisterRoutes 注册用户端路由
func (h *UserHandler) RegisterRoutes(rg *gin.RouterGroup) {
	user := rg.Group("/user")
	user.GET("/profile", h.GetProfile)
	user.PUT("/profile", h.UpdateProfile)
	user.POST("/reset-api-key", h.ResetAPIKey)
	user.GET("/quota", h.GetQuota)
	user.GET("/credentials", h.ListCredentials)
	user.POST("/credentials", h.CreateCredential)
	user.DELETE("/credentials/:id", h.DeleteCredential)
	user.POST("/settings/password", h.ChangePassword)
	user.POST("/settings/api-key/regenerate", h.RegenerateAPIKey)
}

// GetProfile 获取个人信息
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID := c.GetInt64("userID")
	user, err := h.userSvc.GetByID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	c.JSON(http.StatusOK, user)
}

// UpdateProfile 更新个人信息
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	// 后续实现密码修改、邮箱绑定等
	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// ResetAPIKey 重置 API Key
func (h *UserHandler) ResetAPIKey(c *gin.Context) {
	userID := c.GetInt64("userID")
	newKey, err := h.userSvc.ResetAPIKey(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"api_key": newKey})
}

type quotaModelResponse struct {
	Model     string `json:"model"`
	Total     int64  `json:"total"`
	Used      int64  `json:"used"`
	Remaining int64  `json:"remaining"`
}

// GetQuota 获取用户额度概览。
func (h *UserHandler) GetQuota(c *gin.Context) {
	if h.quotaStore == nil {
		c.JSON(http.StatusOK, gin.H{"total": 0, "used": 0, "remaining": 0, "reset_at": "", "models": []quotaModelResponse{}})
		return
	}
	userID := c.GetInt64("userID")
	configs, err := h.quotaStore.GetQuotaConfigs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询额度配置失败"})
		return
	}

	models := make([]quotaModelResponse, 0, len(configs))
	var total, used int64
	for _, cfg := range configs {
		quota, errQuota := h.quotaStore.GetUserQuota(c.Request.Context(), userID, cfg.ModelPattern)
		if errQuota != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户额度失败"})
			return
		}
		maxAllowed := cfg.MaxRequests
		usedRequests := int64(0)
		if quota != nil {
			maxAllowed += quota.BonusRequests
			usedRequests = quota.UsedRequests
		}
		remaining := maxAllowed - usedRequests
		if remaining < 0 {
			remaining = 0
		}
		models = append(models, quotaModelResponse{
			Model:     cfg.ModelPattern,
			Total:     maxAllowed,
			Used:      usedRequests,
			Remaining: remaining,
		})
		total += maxAllowed
		used += usedRequests
	}

	remaining := total - used
	if remaining < 0 {
		remaining = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"used":      used,
		"remaining": remaining,
		"reset_at":  "",
		"models":    models,
	})
}

type credentialCreateRequest struct {
	Provider string `json:"provider" binding:"required"`
	APIKey   string `json:"api_key" binding:"required"`
	Endpoint string `json:"endpoint"`
	Notes    string `json:"notes"`
}

// ListCredentials 返回当前用户凭证列表。
func (h *UserHandler) ListCredentials(c *gin.Context) {
	if h.credentialStore == nil {
		c.JSON(http.StatusOK, gin.H{"credentials": []gin.H{}})
		return
	}
	userID := c.GetInt64("userID")
	creds, _, err := h.credentialStore.ListCredentials(c.Request.Context(), db.ListCredentialsOpts{
		Page:     1,
		PageSize: 200,
		OwnerID:  &userID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询凭证列表失败"})
		return
	}
	items := make([]gin.H, 0, len(creds))
	for _, cred := range creds {
		health := "unknown"
		switch cred.Health {
		case db.HealthHealthy:
			health = "healthy"
		case db.HealthDown, db.HealthDegraded:
			health = "unhealthy"
		}
		items = append(items, gin.H{
			"id":           cred.ID,
			"provider":     cred.Provider,
			"health":       health,
			"created_at":   cred.AddedAt,
			"last_checked": nil,
		})
	}
	c.JSON(http.StatusOK, gin.H{"credentials": items})
}

// CreateCredential 创建当前用户凭证。
func (h *UserHandler) CreateCredential(c *gin.Context) {
	if h.credentialStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "凭证存储未初始化"})
		return
	}
	var req credentialCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	userID := c.GetInt64("userID")
	ownerID := userID
	cred := &db.Credential{
		ID:       uuid.NewString(),
		Provider: req.Provider,
		OwnerID:  &ownerID,
		Data:     req.APIKey,
		Health:   db.HealthHealthy,
		Weight:   1,
		Enabled:  true,
		AddedAt:  time.Now(),
	}
	if err := h.credentialStore.CreateCredential(c.Request.Context(), cred); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建凭证失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "凭证上传成功",
		"credential": gin.H{
			"id":           cred.ID,
			"provider":     cred.Provider,
			"health":       "healthy",
			"created_at":   cred.AddedAt,
			"last_checked": nil,
		},
	})
}

// DeleteCredential 删除当前用户凭证。
func (h *UserHandler) DeleteCredential(c *gin.Context) {
	if h.credentialStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "凭证存储未初始化"})
		return
	}
	userID := c.GetInt64("userID")
	id := c.Param("id")
	cred, err := h.credentialStore.GetCredentialByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "凭证不存在"})
		return
	}
	if cred.OwnerID == nil || *cred.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权删除该凭证"})
		return
	}
	if err := h.credentialStore.DeleteCredential(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除凭证失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// ChangePassword 修改当前用户密码。
func (h *UserHandler) ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if len(req.NewPassword) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新密码长度至少为 6 位"})
		return
	}
	userID := c.GetInt64("userID")
	if err := h.userSvc.ChangePassword(c.Request.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
}

// RegenerateAPIKey 兼容设置页的 API Key 重新生成接口。
func (h *UserHandler) RegenerateAPIKey(c *gin.Context) {
	h.ResetAPIKey(c)
}
