package community

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	communityadmin "github.com/router-for-me/CLIProxyAPI/v6/internal/community/admin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/community/credential"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/community/quota"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/community/security"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/community/stats"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/community/user"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/db"
	log "github.com/sirupsen/logrus"
)

// ============================================================
// 公益站平台核心 — 统一初始化器
// 聚合所有子模块（user / quota / credential / security / stats）
// ============================================================

// Community 公益站平台核心
type Community struct {
	store db.Store

	// -- 用户子系统 --
	userSvc  *user.Service
	jwtMgr   *user.JWTManager
	emailSvc *user.EmailService

	// -- 额度子系统 --
	quotaEngine      *quota.Engine
	poolMgr          *quota.PoolManager
	riskEngine       *quota.RiskEngine
	contributorRPM   *quota.RPMLimiter
	nonContributorRPM *quota.RPMLimiter

	// -- 安全子系统 --
	secStack *security.SecurityStack

	// -- 凭证子系统 --
	redemptionSvc *credential.RedemptionService
	templateSvc   *credential.TemplateService
	referralSvc   *credential.ReferralService

	// -- 统计子系统 --
	statsCollector *stats.Collector
	statsAggregator *stats.Aggregator
	statsExporter  *stats.Exporter
}

// New 初始化社区模块
// 按依赖顺序初始化: 数据库 → 用户 → 额度 → 安全 → 凭证 → 统计
func New(ctx context.Context, cfg config.CommunityConfig) (*Community, error) {
	// ---- 1. 数据库 ----
	store, err := db.NewStore(ctx, cfg.Database.Driver, cfg.Database.DSN)
	if err != nil {
		return nil, fmt.Errorf("初始化社区数据库失败: %w", err)
	}
	if err := store.Migrate(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("社区数据库迁移失败: %w", err)
	}
	log.Info("社区平台数据库初始化完成")
	db.ConfigureCredentialProtection(cfg.Auth.CredentialSecret, cfg.Auth.JWTSecret)

	// ---- 2. 用户子系统 ----
	userSvc := user.NewService(store)

	// 确保默认管理员账户存在
	if err := ensureAdminUser(ctx, userSvc, store); err != nil {
		log.Warnf("确保管理员账户失败: %v", err)
	}

	accessTTL := time.Duration(cfg.Auth.AccessTokenTTL) * time.Second
	if accessTTL == 0 {
		accessTTL = 2 * time.Hour
	}
	refreshTTL := time.Duration(cfg.Auth.RefreshTokenTTL) * time.Second
	if refreshTTL == 0 {
		refreshTTL = 7 * 24 * time.Hour
	}
	jwtMgr, err := user.NewJWTManager(cfg.Auth.JWTSecret, accessTTL, refreshTTL)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("初始化 JWT 管理器失败: %w", err)
	}

	emailSvc := user.NewEmailService(
		cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.Username,
		cfg.SMTP.Password, cfg.SMTP.From, cfg.SMTP.UseTLS,
	)

	// ---- 3. 额度子系统 ----
	quotaEngine := quota.NewEngine(store)
	poolMgr := quota.NewPoolManager(store, store)

	riskEngine := quota.NewRiskEngine(store, quota.RiskConfig{
		Enabled:              cfg.Quota.RiskRule.Enabled,
		RPMExceedThreshold:   cfg.Quota.RiskRule.RPMExceedThreshold,
		RPMExceedWindow:      time.Duration(cfg.Quota.RiskRule.RPMExceedWindowSec) * time.Second,
		PenaltyDuration:      time.Duration(cfg.Quota.RiskRule.PenaltyDurationSec) * time.Second,
		PenaltyProbability:   cfg.Quota.RiskRule.PenaltyProbability,
		ProbEnabled:          cfg.Quota.ProbabilityLimit.Enabled,
		ContributorWeight:    cfg.Quota.ProbabilityLimit.ContributorWeight,
		NonContributorWeight: cfg.Quota.ProbabilityLimit.NonContributorWeight,
	})

	// RPM 限流器
	var contributorRPM, nonContributorRPM *quota.RPMLimiter
	if cfg.Quota.RPM.Enabled {
		if cfg.Quota.RPM.ContributorRPM > 0 {
			contributorRPM = quota.NewRPMLimiter(cfg.Quota.RPM.ContributorRPM)
		}
		if cfg.Quota.RPM.NonContributorRPM > 0 {
			nonContributorRPM = quota.NewRPMLimiter(cfg.Quota.RPM.NonContributorRPM)
		}
	}

	// ---- 4. 安全子系统 ----
	secStack := security.NewSecurityStack(store, cfg.Security)

	// ---- 5. 凭证子系统 ----
	redemptionSvc := credential.NewRedemptionService(store, quotaEngine, store)
	templateSvc := credential.NewTemplateService(store, store, quotaEngine)
	referralSvc := credential.NewReferralService(store, store, quotaEngine)

	// ---- 6. 统计子系统 ----
	statsCollector := stats.NewCollector(store)
	statsAggregator := stats.NewAggregator(store)
	statsExporter := stats.NewExporter(store)

	log.Info("社区平台所有子模块初始化完成")

	return &Community{
		store:             store,
		userSvc:           userSvc,
		jwtMgr:            jwtMgr,
		emailSvc:          emailSvc,
		quotaEngine:       quotaEngine,
		poolMgr:           poolMgr,
		riskEngine:        riskEngine,
		contributorRPM:    contributorRPM,
		nonContributorRPM: nonContributorRPM,
		secStack:          secStack,
		redemptionSvc:     redemptionSvc,
		templateSvc:       templateSvc,
		referralSvc:       referralSvc,
		statsCollector:    statsCollector,
		statsAggregator:   statsAggregator,
		statsExporter:     statsExporter,
	}, nil
}

// RegisterRoutes 注册所有社区 API 路由
// 路由层级: /api/v1 → 认证(无 JWT) + 已认证用户 + 管理员
func (c *Community) RegisterRoutes(engine *gin.Engine) {
	api := engine.Group("/api/v1")

	// -- 安全中间件（IP控制 + 全局限流 + 审计） --
	if c.secStack != nil {
		for _, mw := range c.secStack.Middlewares() {
			api.Use(mw)
		}
	}

	// -- 认证路由（无需 JWT） --
	authHandler := user.NewAuthHandler(c.userSvc, c.jwtMgr, c.emailSvc)
	authHandler.RegisterRoutes(api)

	// -- 需要 JWT 的路由 --
	authed := api.Group("")
	authed.Use(user.JWTMiddleware(c.jwtMgr))

	// 用户路由（不需要额度中间件）
	userHandler := user.NewUserHandler(c.userSvc, c.store, c.store)
	userHandler.RegisterRoutes(authed)

	// 凭证路由（用户端）
	credHandler := credential.NewHandler(c.redemptionSvc, c.templateSvc, c.referralSvc)
	credHandler.RegisterRoutes(authed)

	// 统计路由（用户端 + 管理员端）
	statsHandler := stats.NewHandler(c.statsAggregator, c.statsExporter)
	statsHandler.RegisterRoutes(authed)

	// -- 管理员路由 --
	admin := authed.Group("")
	admin.Use(user.AdminMiddleware())

	adminUserHandler := user.NewAdminUserHandler(c.userSvc)
	adminUserHandler.RegisterRoutes(admin)

	// 凭证路由（管理端，需 Admin 权限）
	credHandler.RegisterAdminRoutes(admin)
	adminHandler := communityadmin.NewHandler(c.store, c.userSvc, c.secStack, c.statsAggregator, c.statsExporter, c.redemptionSvc, c.templateSvc)
	adminHandler.RegisterRoutes(admin)
}

// Close 清理资源
func (c *Community) Close() error {
	if c.store != nil {
		return c.store.Close()
	}
	return nil
}

// ============================================================
// 默认管理员账户
// ============================================================

const (
	defaultAdminUsername = "admin"
	defaultAdminPassword = "admin114514"
)

// ensureAdminUser 确保默认管理员账户存在
// 如果管理员账户不存在，则创建一个
func ensureAdminUser(ctx context.Context, userSvc *user.Service, store db.Store) error {
	// 检查是否已存在管理员用户
	existingAdmin, err := store.GetUserByUsername(ctx, defaultAdminUsername)
	if err == nil && existingAdmin != nil {
		// 管理员已存在
		return nil
	}

	// 创建默认管理员账户
	_, err = userSvc.Register(ctx, user.RegisterInput{
		Username: defaultAdminUsername,
		Password: defaultAdminPassword,
		Email:    "", // 可选，留空
	})
	if err != nil {
		return fmt.Errorf("创建默认管理员失败: %w", err)
	}

	// 将用户角色提升为管理员
	adminUser, err := store.GetUserByUsername(ctx, defaultAdminUsername)
	if err != nil {
		return fmt.Errorf("查询新创建的管理员失败: %w", err)
	}

	adminUser.Role = db.RoleAdmin
	if err := store.UpdateUser(ctx, adminUser); err != nil {
		return fmt.Errorf("提升管理员权限失败: %w", err)
	}

	log.Info("默认管理员账户已创建 (用户名: admin)")
	return nil
}

// Store 暴露底层存储（供外部组件使用）
func (c *Community) Store() db.Store {
	return c.store
}

// QuotaMiddleware 返回额度中间件，供代理 API 路由层挂载
// 仅应用于实际消耗额度的代理请求路由，不应用于管理路由
func (c *Community) QuotaMiddleware() gin.HandlerFunc {
	return quota.Middleware(
		c.quotaEngine, c.contributorRPM, c.nonContributorRPM,
		c.riskEngine, c.poolMgr,
	)
}
