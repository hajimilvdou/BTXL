package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/community/credential"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/community/security"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/community/stats"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/community/user"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/db"
)

const (
	settingSMTPConfig     = "admin.smtp_config"
	settingOAuthProviders = "admin.oauth_providers"
	settingGeneral        = "admin.general_settings"
	settingPoolMode       = "admin.pool_mode"
	settingRPM            = "admin.rpm_settings"
	settingRouterStrategy = "admin.router_strategy"
	settingRouterHealth   = "admin.router_health"
	settingSecurity       = "admin.security_modules"
)

type Handler struct {
	store         db.Store
	userSvc       *user.Service
	secStack      *security.SecurityStack
	statsAgg      *stats.Aggregator
	statsExporter *stats.Exporter
	redemptionSvc *credential.RedemptionService
	templateSvc   *credential.TemplateService
}

func NewHandler(store db.Store, userSvc *user.Service, secStack *security.SecurityStack, statsAgg *stats.Aggregator, statsExporter *stats.Exporter, redemptionSvc *credential.RedemptionService, templateSvc *credential.TemplateService) *Handler {
	return &Handler{
		store:         store,
		userSvc:       userSvc,
		secStack:      secStack,
		statsAgg:      statsAgg,
		statsExporter: statsExporter,
		redemptionSvc: redemptionSvc,
		templateSvc:   templateSvc,
	}
}

type smtpConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	UseTLS   bool   `json:"use_tls"`
}

type oauthProvider struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Enabled      bool   `json:"enabled"`
}

type generalSettings struct {
	JWTSecretMasked      string `json:"jwt_secret_masked"`
	AccessTokenTTL       int    `json:"access_token_ttl"`
	RefreshTokenTTL      int    `json:"refresh_token_ttl"`
	EmailRegisterEnabled bool   `json:"email_register_enabled"`
	InviteRequired       bool   `json:"invite_required"`
	ReferralEnabled      bool   `json:"referral_enabled"`
	DailyRegisterLimit   int    `json:"daily_register_limit"`
}

type rpmSettings struct {
	ContributorRPM    int `json:"contributor_rpm"`
	NonContributorRPM int `json:"non_contributor_rpm"`
}

type securityModules struct {
	IPControl        bool `json:"ip_control"`
	RateLimit        bool `json:"rate_limit"`
	AnomalyDetection bool `json:"anomaly_detection"`
	Audit            bool `json:"audit"`
}

type routerHealth struct {
	CheckIntervalSec   int `json:"check_interval_sec"`
	FailureThreshold   int `json:"failure_threshold"`
	RecoveryTimeoutSec int `json:"recovery_timeout_sec"`
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/admin/quota-configs", h.ListQuotaConfigs)
	rg.POST("/admin/quota-configs", h.CreateQuotaConfig)
	rg.PUT("/admin/quota-configs/:id", h.UpdateQuotaConfig)
	rg.DELETE("/admin/quota-configs/:id", h.DeleteQuotaConfig)

	rg.GET("/admin/credentials", h.ListCredentials)
	rg.POST("/admin/credentials", h.CreateCredential)
	rg.DELETE("/admin/credentials/:id", h.DeleteCredential)
	rg.POST("/admin/credentials/health-check", h.BulkHealthCheck)

	rg.POST("/admin/redemption/generate", h.GenerateCodes)
	rg.GET("/admin/redemption/templates", h.ListTemplates)
	rg.POST("/admin/redemption/templates", h.CreateTemplate)
	rg.PUT("/admin/redemption/templates/:id/toggle", h.ToggleTemplate)
	rg.GET("/admin/redemption/stats", h.CodeUsageStats)
	rg.GET("/admin/redemption/codes", h.ListRedemptionCodes)

	rg.GET("/admin/invites", h.ListInviteCodes)
	rg.POST("/admin/invites", h.CreateInviteCode)
	rg.PUT("/admin/invites/:id/disable", h.DisableInviteCode)

	rg.GET("/admin/security/status", h.GetSecurityStatus)
	rg.PUT("/admin/security/toggle", h.ToggleSecurityModule)
	rg.GET("/admin/security/ip-rules", h.ListIPRules)
	rg.POST("/admin/security/ip-rules", h.CreateIPRule)
	rg.DELETE("/admin/security/ip-rules/:id", h.DeleteIPRule)
	rg.GET("/admin/security/risk-marks", h.ListRiskMarks)
	rg.POST("/admin/security/risk-marks", h.CreateRiskMark)
	rg.DELETE("/admin/security/risk-marks/:id", h.DeleteRiskMark)
	rg.GET("/admin/security/anomaly-events", h.ListAnomalyEvents)
	rg.GET("/admin/security/audit-logs", h.ListAuditLogs)

	rg.GET("/admin/settings/smtp", h.GetSMTPConfig)
	rg.PUT("/admin/settings/smtp", h.PutSMTPConfig)
	rg.POST("/admin/settings/smtp/test", h.TestSMTPConfig)
	rg.GET("/admin/settings/oauth-providers", h.ListOAuthProviders)
	rg.POST("/admin/settings/oauth-providers", h.CreateOAuthProvider)
	rg.DELETE("/admin/settings/oauth-providers/:id", h.DeleteOAuthProvider)
	rg.PUT("/admin/settings/oauth-providers/:id/toggle", h.ToggleOAuthProvider)
	rg.GET("/admin/settings/general", h.GetGeneralSettings)
	rg.PUT("/admin/settings/general", h.PutGeneralSettings)
	rg.GET("/admin/settings/pool-mode", h.GetPoolMode)
	rg.PUT("/admin/settings/pool-mode", h.PutPoolMode)
	rg.GET("/admin/settings/rpm", h.GetRPMSettings)
	rg.PUT("/admin/settings/rpm", h.PutRPMSettings)

	rg.GET("/admin/router/config", h.GetRouterConfig)
	rg.PUT("/admin/router/strategy", h.PutRouterStrategy)
	rg.PUT("/admin/router/credentials/:id/weight", h.PutCredentialWeight)
	rg.PUT("/admin/router/health-settings", h.PutRouterHealth)
	rg.POST("/admin/router/apply", h.ApplyRouter)

	rg.GET("/admin/dashboard/stats", h.GetDashboardStats)
	rg.GET("/admin/dashboard/request-trends", h.GetRequestTrends)
	rg.GET("/admin/dashboard/model-distribution", h.GetModelDistribution)
	rg.GET("/admin/dashboard/recent-logs", h.GetRecentAuditLogs)
}

func (h *Handler) ListQuotaConfigs(c *gin.Context) {
	items, err := h.store.GetQuotaConfigs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询额度配置失败"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) CreateQuotaConfig(c *gin.Context) {
	var req db.QuotaConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	req.CreatedAt = time.Now()
	req.UpdatedAt = req.CreatedAt
	if err := h.store.CreateQuotaConfig(c.Request.Context(), &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, req)
}

func (h *Handler) UpdateQuotaConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的配置 ID"})
		return
	}
	items, err := h.store.GetQuotaConfigs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询额度配置失败"})
		return
	}
	var target *db.QuotaConfig
	for _, item := range items {
		if item.ID == id {
			target = item
			break
		}
	}
	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "额度配置不存在"})
		return
	}
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	applyQuotaConfigPatch(target, req)
	target.UpdatedAt = time.Now()
	if err := h.store.UpdateQuotaConfig(c.Request.Context(), target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, target)
}

func (h *Handler) DeleteQuotaConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的配置 ID"})
		return
	}
	if err := h.store.DeleteQuotaConfig(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) ListCredentials(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	opts := db.ListCredentialsOpts{Page: page, PageSize: pageSize}
	if provider := strings.TrimSpace(c.Query("provider")); provider != "" {
		opts.Provider = &provider
	}
	if health := strings.TrimSpace(c.Query("health")); health != "" {
		opts.Health = &health
	}
	items, total, err := h.store.ListCredentials(c.Request.Context(), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询凭证失败"})
		return
	}
	data := make([]gin.H, 0, len(items))
	for _, item := range items {
		ownerName := ""
		if item.OwnerID != nil {
			if u, err := h.userSvc.GetByID(c.Request.Context(), *item.OwnerID); err == nil && u != nil {
				ownerName = u.Username
			}
		}
		data = append(data, gin.H{
			"id":           item.ID,
			"provider":     item.Provider,
			"owner_id":     item.OwnerID,
			"health":       item.Health,
			"weight":       item.Weight,
			"enabled":      item.Enabled,
			"added_at":     item.AddedAt,
			"success_rate": healthToSuccessRate(item.Health),
			"last_check":   nil,
			"owner_name":   ownerName,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) CreateCredential(c *gin.Context) {
	var req struct {
		Provider       string      `json:"provider" binding:"required"`
		CredentialData string      `json:"credential_data" binding:"required"`
		PoolMode       db.PoolMode `json:"pool_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	var ownerID *int64
	if req.PoolMode == db.PoolPrivate {
		uid := c.GetInt64("userID")
		ownerID = &uid
	}
	cred := &db.Credential{ID: uuid.NewString(), Provider: req.Provider, OwnerID: ownerID, Data: req.CredentialData, Health: db.HealthHealthy, Weight: 1, Enabled: true, AddedAt: time.Now()}
	if err := h.store.CreateCredential(c.Request.Context(), cred); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": cred.ID, "provider": cred.Provider, "owner_id": cred.OwnerID, "health": cred.Health, "weight": cred.Weight, "enabled": cred.Enabled, "added_at": cred.AddedAt, "success_rate": 100, "last_check": nil, "owner_name": ""})
}

func (h *Handler) DeleteCredential(c *gin.Context) {
	if err := h.store.DeleteCredential(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) BulkHealthCheck(c *gin.Context) {
	items, _, err := h.store.ListCredentials(c.Request.Context(), db.ListCredentialsOpts{Page: 1, PageSize: 1000})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询凭证失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"checked": len(items)})
}

func (h *Handler) GetSecurityStatus(c *gin.Context) {
	status, _ := h.loadSecurityModules(c.Request.Context())
	c.JSON(http.StatusOK, status)
}

func (h *Handler) ToggleSecurityModule(c *gin.Context) {
	var req struct {
		Module  string `json:"module" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	status, _ := h.loadSecurityModules(c.Request.Context())
	switch req.Module {
	case "ip_control":
		status.IPControl = req.Enabled
		if h.secStack != nil && h.secStack.IPCtrl != nil {
			h.secStack.IPCtrl.SetEnabled(req.Enabled)
		}
	case "rate_limit":
		status.RateLimit = req.Enabled
	case "anomaly_detection":
		status.AnomalyDetection = req.Enabled
		if h.secStack != nil && h.secStack.Anomaly != nil {
			if req.Enabled {
				h.secStack.Anomaly.UpdateRules(security.DefaultAnomalyRules())
			} else {
				h.secStack.Anomaly.UpdateRules(nil)
			}
		}
	case "audit":
		status.Audit = req.Enabled
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "未知模块"})
		return
	}
	if err := saveJSONSetting(c.Request.Context(), h.store, settingSecurity, status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存安全设置失败"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) ListIPRules(c *gin.Context) {
	items, err := h.store.ListIPRules(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询 IP 规则失败"})
		return
	}
	rows := make([]gin.H, 0, len(items))
	for _, item := range items {
		rows = append(rows, gin.H{"id": item.ID, "cidr": item.CIDR, "rule_type": item.RuleType, "description": item.Comment, "created_at": item.CreatedAt})
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) CreateIPRule(c *gin.Context) {
	var req struct {
		CIDR        string `json:"cidr" binding:"required"`
		RuleType    string `json:"rule_type" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	rule := &db.IPRule{CIDR: req.CIDR, RuleType: req.RuleType, Comment: req.Description, CreatedAt: time.Now()}
	if err := h.store.CreateIPRule(c.Request.Context(), rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.secStack != nil {
		_ = h.secStack.ReloadIPRules(c.Request.Context())
	}
	c.JSON(http.StatusOK, gin.H{"id": rule.ID, "cidr": rule.CIDR, "rule_type": rule.RuleType, "description": rule.Comment, "created_at": rule.CreatedAt})
}

func (h *Handler) DeleteIPRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}
	if err := h.store.DeleteIPRule(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.secStack != nil {
		_ = h.secStack.ReloadIPRules(c.Request.Context())
	}
	c.Status(http.StatusOK)
}

func (h *Handler) ListRiskMarks(c *gin.Context) {
	items, _, err := h.store.ListRiskMarks(c.Request.Context(), db.ListRiskMarksOpts{Page: 1, PageSize: 200})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询风险标记失败"})
		return
	}
	rows := make([]gin.H, 0, len(items))
	for _, item := range items {
		username := ""
		if u, err := h.userSvc.GetByID(c.Request.Context(), item.UserID); err == nil && u != nil {
			username = u.Username
		}
		rows = append(rows, gin.H{"id": item.ID, "user_id": item.UserID, "username": username, "type": mapRiskType(item.MarkType), "reason": item.Reason, "expires_at": item.ExpiresAt, "created_at": item.MarkedAt})
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) CreateRiskMark(c *gin.Context) {
	var req struct {
		UserID        int64  `json:"user_id" binding:"required"`
		Type          string `json:"type" binding:"required"`
		Reason        string `json:"reason"`
		DurationHours int    `json:"duration_hours"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	mark := &db.UserRiskMark{UserID: req.UserID, MarkType: parseRiskType(req.Type), Reason: req.Reason, MarkedAt: time.Now(), ExpiresAt: time.Now().Add(time.Duration(req.DurationHours) * time.Hour), AutoApplied: false}
	if err := h.store.CreateRiskMark(c.Request.Context(), mark); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": mark.ID, "user_id": mark.UserID, "username": "", "type": mapRiskType(mark.MarkType), "reason": mark.Reason, "expires_at": mark.ExpiresAt, "created_at": mark.MarkedAt})
}

func (h *Handler) DeleteRiskMark(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的风险标记 ID"})
		return
	}
	if err := h.store.DeleteRiskMark(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) ListAnomalyEvents(c *gin.Context) {
	limit := queryInt(c, "limit", 50)
	items, _, err := h.store.ListAnomalyEvents(c.Request.Context(), db.ListAnomalyEventsOpts{Page: 1, PageSize: limit})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询异常事件失败"})
		return
	}
	rows := make([]gin.H, 0, len(items))
	for _, item := range items {
		var userID int64
		username := ""
		if item.UserID != nil {
			userID = *item.UserID
			if u, err := h.userSvc.GetByID(c.Request.Context(), *item.UserID); err == nil && u != nil {
				username = u.Username
			}
		}
		rows = append(rows, gin.H{"id": item.ID, "user_id": userID, "username": username, "event_type": item.EventType, "severity": mapAnomalySeverity(item.Action), "detail": item.Detail, "created_at": item.CreatedAt})
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) ListAuditLogs(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	opts := db.ListAuditLogsOpts{Page: page, PageSize: pageSize}
	if action := strings.TrimSpace(c.Query("action")); action != "" {
		opts.Action = &action
	}
	items, total, err := h.store.ListAuditLogs(c.Request.Context(), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询审计日志失败"})
		return
	}
	rows := make([]gin.H, 0, len(items))
	for _, item := range items {
		username := ""
		if item.UserID != nil {
			if u, err := h.userSvc.GetByID(c.Request.Context(), *item.UserID); err == nil && u != nil {
				username = u.Username
			}
		}
		rows = append(rows, gin.H{"id": item.ID, "user_id": item.UserID, "username": username, "action": item.Action, "target": item.Target, "detail": item.Detail, "ip": item.IP, "created_at": item.CreatedAt})
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "total": total})
}

func (h *Handler) GetSMTPConfig(c *gin.Context) {
	var cfg smtpConfig
	_ = loadJSONSetting(c.Request.Context(), h.store, settingSMTPConfig, &cfg)
	c.JSON(http.StatusOK, cfg)
}

func (h *Handler) PutSMTPConfig(c *gin.Context) {
	var cfg smtpConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if err := saveJSONSetting(c.Request.Context(), h.store, settingSMTPConfig, cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 SMTP 配置失败"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) TestSMTPConfig(c *gin.Context) {
	var cfg smtpConfig
	_ = loadJSONSetting(c.Request.Context(), h.store, settingSMTPConfig, &cfg)
	if strings.TrimSpace(cfg.Host) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "SMTP Host 未配置"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "SMTP 配置格式有效"})
}

func (h *Handler) ListOAuthProviders(c *gin.Context) {
	providers, _ := h.loadOAuthProviders(c.Request.Context())
	c.JSON(http.StatusOK, providers)
}

func (h *Handler) CreateOAuthProvider(c *gin.Context) {
	providers, _ := h.loadOAuthProviders(c.Request.Context())
	var req oauthProvider
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	req.ID = nextOAuthProviderID(providers)
	req.Enabled = true
	providers = append(providers, req)
	if err := saveJSONSetting(c.Request.Context(), h.store, settingOAuthProviders, providers); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 OAuth 提供商失败"})
		return
	}
	c.JSON(http.StatusOK, req)
}

func (h *Handler) DeleteOAuthProvider(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 OAuth ID"})
		return
	}
	providers, _ := h.loadOAuthProviders(c.Request.Context())
	filtered := make([]oauthProvider, 0, len(providers))
	for _, item := range providers {
		if item.ID != id {
			filtered = append(filtered, item)
		}
	}
	if err := saveJSONSetting(c.Request.Context(), h.store, settingOAuthProviders, filtered); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除 OAuth 提供商失败"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) ToggleOAuthProvider(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 OAuth ID"})
		return
	}
	var req struct{ Enabled bool `json:"enabled"` }
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	providers, _ := h.loadOAuthProviders(c.Request.Context())
	for i := range providers {
		if providers[i].ID == id {
			providers[i].Enabled = req.Enabled
		}
	}
	if err := saveJSONSetting(c.Request.Context(), h.store, settingOAuthProviders, providers); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 OAuth 提供商失败"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) GetGeneralSettings(c *gin.Context) {
	settings, _ := h.loadGeneralSettings(c.Request.Context())
	c.JSON(http.StatusOK, settings)
}

func (h *Handler) PutGeneralSettings(c *gin.Context) {
	current, _ := h.loadGeneralSettings(c.Request.Context())
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	applyGeneralSettingsPatch(&current, req)
	if err := saveJSONSetting(c.Request.Context(), h.store, settingGeneral, current); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存通用设置失败"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) GetPoolMode(c *gin.Context) {
	mode, _ := h.loadPoolMode(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"mode": mode})
}

func (h *Handler) PutPoolMode(c *gin.Context) {
	var req struct{ Mode db.PoolMode `json:"mode"` }
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if err := h.store.SetSetting(c.Request.Context(), settingPoolMode, string(req.Mode)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存池模式失败"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) GetRPMSettings(c *gin.Context) {
	settings, _ := h.loadRPMSettings(c.Request.Context())
	c.JSON(http.StatusOK, settings)
}

func (h *Handler) PutRPMSettings(c *gin.Context) {
	var req rpmSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if err := saveJSONSetting(c.Request.Context(), h.store, settingRPM, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 RPM 设置失败"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) GetRouterConfig(c *gin.Context) {
	strategy, _ := h.loadRouterStrategy(c.Request.Context())
	health, _ := h.loadRouterHealth(c.Request.Context())
	items, _, err := h.store.ListCredentials(c.Request.Context(), db.ListCredentialsOpts{Page: 1, PageSize: 1000})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询凭证失败"})
		return
	}
	creds := make([]gin.H, 0, len(items))
	for _, item := range items {
		creds = append(creds, gin.H{"credential_id": item.ID, "provider": item.Provider, "total_requests": 0, "success_rate": healthToSuccessRate(item.Health), "avg_latency_ms": 0, "weight": item.Weight, "circuit_state": healthToCircuitState(item.Health)})
	}
	c.JSON(http.StatusOK, gin.H{"strategy": strategy, "credentials": creds, "health": health})
}

func (h *Handler) PutRouterStrategy(c *gin.Context) {
	var req struct{ Strategy string `json:"strategy"` }
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if err := h.store.SetSetting(c.Request.Context(), settingRouterStrategy, req.Strategy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存路由策略失败"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) PutCredentialWeight(c *gin.Context) {
	id := c.Param("id")
	var req struct{ Weight int `json:"weight"` }
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	cred, err := h.store.GetCredentialByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "凭证不存在"})
		return
	}
	cred.Weight = req.Weight
	if err := h.store.UpdateCredential(c.Request.Context(), cred); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新权重失败"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) PutRouterHealth(c *gin.Context) {
	var req routerHealth
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if err := saveJSONSetting(c.Request.Context(), h.store, settingRouterHealth, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存健康检查配置失败"})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) ApplyRouter(c *gin.Context) {
	c.Status(http.StatusOK)
}

func (h *Handler) GetDashboardStats(c *gin.Context) {
	totalUsers, _ := h.store.CountUsers(c.Request.Context())
	now := time.Now()
	last24h, _ := h.statsAgg.GetGlobalStats(c.Request.Context(), timePtr(now.Add(-24*time.Hour)), timePtr(now))
	prev24h, _ := h.statsAgg.GetGlobalStats(c.Request.Context(), timePtr(now.Add(-48*time.Hour)), timePtr(now.Add(-24*time.Hour)))
	creds, _, _ := h.store.ListCredentials(c.Request.Context(), db.ListCredentialsOpts{Page: 1, PageSize: 1000})
	activeCreds := 0
	hasDown := false
	hasDegraded := false
	for _, item := range creds {
		if item.Enabled {
			activeCreds++
		}
		if item.Health == db.HealthDown {
			hasDown = true
		}
		if item.Health == db.HealthDegraded {
			hasDegraded = true
		}
	}
	systemHealth := "healthy"
	if activeCreds == 0 {
		systemHealth = "down"
	} else if hasDown || hasDegraded {
		systemHealth = "degraded"
	}
	c.JSON(http.StatusOK, gin.H{"total_users": totalUsers, "total_requests_24h": safeRequestCount(last24h), "active_credentials": activeCreds, "system_health": systemHealth, "users_trend": 0, "requests_trend": calculateTrend(safeRequestCount(last24h), safeRequestCount(prev24h)), "credentials_trend": 0})
}

func (h *Handler) GetRequestTrends(c *gin.Context) {
	now := time.Now()
	rows := make([]gin.H, 0, 7)
	for i := 6; i >= 0; i-- {
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -i)
		end := start.Add(24 * time.Hour)
		statsRow, _ := h.statsAgg.GetGlobalStats(c.Request.Context(), timePtr(start), timePtr(end))
		rows = append(rows, gin.H{"date": start.Format("2006-01-02"), "count": safeRequestCount(statsRow)})
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) GetModelDistribution(c *gin.Context) {
	statsRow, _ := h.statsAgg.GetGlobalStats(c.Request.Context(), nil, nil)
	rows := make([]gin.H, 0)
	if statsRow != nil {
		for model, count := range statsRow.ByModel {
			rows = append(rows, gin.H{"model": model, "count": count})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["count"].(int64) > rows[j]["count"].(int64) })
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) GetRecentAuditLogs(c *gin.Context) {
	items, _, err := h.store.ListAuditLogs(c.Request.Context(), db.ListAuditLogsOpts{Page: 1, PageSize: 10})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询审计日志失败"})
		return
	}
	rows := make([]gin.H, 0, len(items))
	for _, item := range items {
		rows = append(rows, gin.H{"id": item.ID, "user_id": item.UserID, "action": item.Action, "target": item.Target, "detail": item.Detail, "ip": item.IP, "created_at": item.CreatedAt})
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) GenerateCodes(c *gin.Context) {
	var req struct {
		TemplateID int64   `json:"template_id" binding:"required"`
		Count      int     `json:"count" binding:"required"`
		ExpiresAt  *string `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	tpl, err := h.store.GetTemplateByID(c.Request.Context(), req.TemplateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "模板不存在"})
		return
	}
	cfg := credential.CodeGenConfig{MaxUses: 1, BonusQuota: &tpl.BonusQuota}
	if req.ExpiresAt != nil && strings.TrimSpace(*req.ExpiresAt) != "" {
		if ts, errParse := time.Parse(time.RFC3339, strings.TrimSpace(*req.ExpiresAt)); errParse == nil {
			cfg.ExpiresIn = time.Until(ts)
		}
	}
	codes, err := h.redemptionSvc.GenerateCodes(c.Request.Context(), int(c.GetInt64("userID")), req.Count, cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"codes": codes})
}

func (h *Handler) ListTemplates(c *gin.Context) {
	items, err := h.store.ListAllTemplates(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询模板失败"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) CreateTemplate(c *gin.Context) {
	var req db.RedemptionTemplate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if err := h.templateSvc.CreateTemplate(c.Request.Context(), req.Name, req.Description, req.BonusQuota, req.MaxPerUser, req.TotalLimit); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	items, _ := h.store.ListAllTemplates(c.Request.Context())
	for _, item := range items {
		if item.Name == req.Name && item.Description == req.Description {
			c.JSON(http.StatusOK, item)
			return
		}
	}
	c.JSON(http.StatusOK, req)
}

func (h *Handler) ToggleTemplate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模板 ID"})
		return
	}
	var req struct{ Enabled bool `json:"enabled"` }
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	if err := h.store.UpdateTemplateEnabled(c.Request.Context(), id, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func (h *Handler) CodeUsageStats(c *gin.Context) {
	templates, _ := h.store.ListAllTemplates(c.Request.Context())
	rows := make([]gin.H, 0, len(templates))
	for _, tpl := range templates {
		rows = append(rows, gin.H{"template_id": tpl.ID, "template_name": tpl.Name, "total_generated": tpl.IssuedCount, "total_used": 0, "total_expired": 0})
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) ListRedemptionCodes(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	typeStr := string(db.InviteAdminCreated)
	items, total, err := h.store.ListInviteCodes(c.Request.Context(), db.ListInviteCodesOpts{Page: page, PageSize: pageSize, Type: &typeStr})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询兑换码失败"})
		return
	}
	templates, _ := h.store.ListAllTemplates(c.Request.Context())
	rows := make([]gin.H, 0, len(items))
	for _, item := range items {
		tplID, tplName := matchTemplate(item, templates)
		rows = append(rows, gin.H{"id": item.ID, "code": item.Code, "template_id": tplID, "template_name": tplName, "used_by": nil, "used_at": nil, "expires_at": item.ExpiresAt, "created_at": item.CreatedAt})
	}
	c.JSON(http.StatusOK, gin.H{"data": rows, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) ListInviteCodes(c *gin.Context) {
	items, _, err := h.store.ListInviteCodes(c.Request.Context(), db.ListInviteCodesOpts{Page: 1, PageSize: 500})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询邀请码失败"})
		return
	}
	rows := make([]gin.H, 0, len(items))
	for _, item := range items {
		creatorName := ""
		if u, err := h.userSvc.GetByID(c.Request.Context(), item.CreatorID); err == nil && u != nil {
			creatorName = u.Username
		}
		rows = append(rows, gin.H{"id": item.ID, "code": item.Code, "type": item.Type, "creator_id": item.CreatorID, "creator_name": creatorName, "max_uses": item.MaxUses, "used_count": item.UsedCount, "expires_at": item.ExpiresAt, "status": item.Status, "created_at": item.CreatedAt})
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) CreateInviteCode(c *gin.Context) {
	var req struct {
		MaxUses    int     `json:"max_uses" binding:"required"`
		BonusQuota *int64  `json:"bonus_quota"`
		ExpiresAt  *string `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}
	code := &db.InviteCode{Code: uuid.NewString()[:8], Type: db.InviteAdminCreated, CreatorID: c.GetInt64("userID"), MaxUses: req.MaxUses, Status: db.InviteActive, CreatedAt: time.Now()}
	if req.BonusQuota != nil && *req.BonusQuota > 0 {
		code.BonusQuota = &db.QuotaGrant{ModelPattern: "*", Requests: *req.BonusQuota, QuotaType: db.QuotaCount}
	}
	if req.ExpiresAt != nil && strings.TrimSpace(*req.ExpiresAt) != "" {
		if ts, errParse := time.Parse(time.RFC3339, strings.TrimSpace(*req.ExpiresAt)); errParse == nil {
			code.ExpiresAt = &ts
		}
	}
	if err := h.store.CreateInviteCode(c.Request.Context(), code); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	creatorName := ""
	if u, err := h.userSvc.GetByID(c.Request.Context(), code.CreatorID); err == nil && u != nil {
		creatorName = u.Username
	}
	c.JSON(http.StatusOK, gin.H{"id": code.ID, "code": code.Code, "type": code.Type, "creator_id": code.CreatorID, "creator_name": creatorName, "max_uses": code.MaxUses, "used_count": code.UsedCount, "expires_at": code.ExpiresAt, "status": code.Status, "created_at": code.CreatedAt})
}

func (h *Handler) DisableInviteCode(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的邀请码 ID"})
		return
	}
	items, _, err := h.store.ListInviteCodes(c.Request.Context(), db.ListInviteCodesOpts{Page: 1, PageSize: 500})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询邀请码失败"})
		return
	}
	for _, item := range items {
		if item.ID == id {
			item.Status = db.InviteDisabled
			if err := h.store.UpdateInviteCode(c.Request.Context(), item); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.Status(http.StatusOK)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "邀请码不存在"})
}

func (h *Handler) loadOAuthProviders(ctx context.Context) ([]oauthProvider, error) {
	var items []oauthProvider
	err := loadJSONSetting(ctx, h.store, settingOAuthProviders, &items)
	return items, err
}

func (h *Handler) loadGeneralSettings(ctx context.Context) (generalSettings, error) {
	settings := generalSettings{JWTSecretMasked: "************************", AccessTokenTTL: 7200, RefreshTokenTTL: 604800, EmailRegisterEnabled: false, InviteRequired: false, ReferralEnabled: true, DailyRegisterLimit: 100}
	err := loadJSONSetting(ctx, h.store, settingGeneral, &settings)
	return settings, err
}

func (h *Handler) loadPoolMode(ctx context.Context) (db.PoolMode, error) {
	value, err := h.store.GetSetting(ctx, settingPoolMode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.PoolPublic, nil
		}
		return db.PoolPublic, err
	}
	if value == "" {
		return db.PoolPublic, nil
	}
	return db.PoolMode(value), nil
}

func (h *Handler) loadRPMSettings(ctx context.Context) (rpmSettings, error) {
	settings := rpmSettings{ContributorRPM: 30, NonContributorRPM: 10}
	err := loadJSONSetting(ctx, h.store, settingRPM, &settings)
	return settings, err
}

func (h *Handler) loadRouterStrategy(ctx context.Context) (string, error) {
	value, err := h.store.GetSetting(ctx, settingRouterStrategy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "FillFirst", nil
		}
		return "FillFirst", err
	}
	if value == "" {
		return "FillFirst", nil
	}
	return value, nil
}

func (h *Handler) loadRouterHealth(ctx context.Context) (routerHealth, error) {
	settings := routerHealth{CheckIntervalSec: 60, FailureThreshold: 3, RecoveryTimeoutSec: 300}
	err := loadJSONSetting(ctx, h.store, settingRouterHealth, &settings)
	return settings, err
}

func (h *Handler) loadSecurityModules(ctx context.Context) (securityModules, error) {
	settings := securityModules{IPControl: false, RateLimit: true, AnomalyDetection: true, Audit: true}
	err := loadJSONSetting(ctx, h.store, settingSecurity, &settings)
	return settings, err
}

func applyQuotaConfigPatch(target *db.QuotaConfig, patch map[string]any) {
	if v, ok := patch["model_pattern"].(string); ok && v != "" { target.ModelPattern = v }
	if v, ok := patch["quota_type"].(string); ok && v != "" { target.QuotaType = db.QuotaType(v) }
	if v, ok := numberFromAny(patch["max_requests"]); ok { target.MaxRequests = v }
	if v, ok := patch["request_period"].(string); ok && v != "" { target.RequestPeriod = db.QuotaPeriod(v) }
	if v, ok := numberFromAny(patch["max_tokens"]); ok { target.MaxTokens = v }
	if v, ok := patch["token_period"].(string); ok && v != "" { target.TokenPeriod = db.QuotaPeriod(v) }
}

func applyGeneralSettingsPatch(target *generalSettings, patch map[string]any) {
	if v, ok := numberFromAny(patch["access_token_ttl"]); ok { target.AccessTokenTTL = int(v) }
	if v, ok := numberFromAny(patch["refresh_token_ttl"]); ok { target.RefreshTokenTTL = int(v) }
	if v, ok := patch["email_register_enabled"].(bool); ok { target.EmailRegisterEnabled = v }
	if v, ok := patch["invite_required"].(bool); ok { target.InviteRequired = v }
	if v, ok := patch["referral_enabled"].(bool); ok { target.ReferralEnabled = v }
	if v, ok := numberFromAny(patch["daily_register_limit"]); ok { target.DailyRegisterLimit = int(v) }
}

func healthToSuccessRate(health db.HealthStatus) float64 { switch health { case db.HealthHealthy: return 100; case db.HealthDegraded: return 50; default: return 0 } }
func healthToCircuitState(health db.HealthStatus) string { switch health { case db.HealthDown: return "open"; case db.HealthDegraded: return "half_open"; default: return "closed" } }
func mapRiskType(markType db.RiskMarkType) string { switch markType { case db.RiskRPMAbuse: return "rpm_exceed"; case db.RiskAnomaly: return "anomaly"; default: return "manual" } }
func parseRiskType(markType string) db.RiskMarkType { switch markType { case "rpm_exceed": return db.RiskRPMAbuse; case "anomaly": return db.RiskAnomaly; default: return db.RiskManual } }
func mapAnomalySeverity(action string) string { switch strings.ToLower(action) { case "ban": return "critical"; case "throttle": return "high"; case "warn": return "medium"; default: return "low" } }
func safeRequestCount(statsRow *db.RequestStats) int64 { if statsRow == nil { return 0 }; return statsRow.TotalRequests }
func calculateTrend(current, previous int64) int64 { if previous <= 0 { if current > 0 { return 100 }; return 0 }; return int64(((float64(current) - float64(previous)) / float64(previous)) * 100) }
func nextOAuthProviderID(items []oauthProvider) int64 { var maxID int64; for _, item := range items { if item.ID > maxID { maxID = item.ID } }; return maxID + 1 }

func loadJSONSetting[T any](ctx context.Context, store db.SettingsStore, key string, out *T) error {
	value, err := store.GetSetting(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) { return nil }
		return err
	}
	if strings.TrimSpace(value) == "" { return nil }
	return json.Unmarshal([]byte(value), out)
}

func saveJSONSetting(ctx context.Context, store db.SettingsStore, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil { return err }
	return store.SetSetting(ctx, key, string(data))
}

func queryInt(c *gin.Context, key string, def int) int { if raw := strings.TrimSpace(c.Query(key)); raw != "" { if v, err := strconv.Atoi(raw); err == nil && v > 0 { return v } }; return def }
func numberFromAny(v any) (int64, bool) { switch n := v.(type) { case float64: return int64(n), true; case float32: return int64(n), true; case int: return int64(n), true; case int64: return n, true; case int32: return int64(n), true; case json.Number: parsed, err := n.Int64(); return parsed, err == nil; default: return 0, false } }
func timePtr(t time.Time) *time.Time { return &t }

func matchTemplate(code *db.InviteCode, templates []*db.RedemptionTemplate) (int64, string) {
	if code == nil || code.BonusQuota == nil { return 0, "" }
	for _, tpl := range templates {
		if tpl.BonusQuota.ModelPattern == code.BonusQuota.ModelPattern && tpl.BonusQuota.Requests == code.BonusQuota.Requests && tpl.BonusQuota.Tokens == code.BonusQuota.Tokens && tpl.BonusQuota.QuotaType == code.BonusQuota.QuotaType {
			return tpl.ID, tpl.Name
		}
	}
	return 0, ""
}
