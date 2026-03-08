package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ============================================================
// PostgresStore — QuotaStore 接口实现
// 管理模型额度配置 (quota_configs) 与用户额度状态 (user_quotas)
// PG 差异: ON CONFLICT 替代 INSERT OR REPLACE / CURRENT_DATE 替代 date()
// ============================================================

// ============================================================
// 额度配置 CRUD
// ============================================================

// CreateQuotaConfig 创建新的模型额度配置
// model_pattern 具有唯一约束，重复插入将返回错误
func (s *PostgresStore) CreateQuotaConfig(ctx context.Context, cfg *QuotaConfig) error {
	const query = `
		INSERT INTO quota_configs (model_pattern, quota_type, max_requests, request_period, max_tokens, token_period, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`

	err := s.db.QueryRowContext(ctx, query,
		cfg.ModelPattern,
		cfg.QuotaType,
		cfg.MaxRequests,
		cfg.RequestPeriod,
		cfg.MaxTokens,
		cfg.TokenPeriod,
	).Scan(&cfg.ID)
	if err != nil {
		return fmt.Errorf("创建额度配置失败: %w", err)
	}

	cfg.CreatedAt = time.Now()
	cfg.UpdatedAt = cfg.CreatedAt
	return nil
}

// GetQuotaConfigs 获取全部额度配置列表
// 按 ID 升序排列，结果集为空时返回空切片而非 nil
func (s *PostgresStore) GetQuotaConfigs(ctx context.Context) ([]*QuotaConfig, error) {
	const query = `
		SELECT id, model_pattern, quota_type, max_requests, request_period, max_tokens, token_period, created_at, updated_at
		FROM quota_configs
		ORDER BY id ASC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询额度配置列表失败: %w", err)
	}
	defer rows.Close()

	var configs []*QuotaConfig
	for rows.Next() {
		cfg := &QuotaConfig{}
		if err := rows.Scan(
			&cfg.ID,
			&cfg.ModelPattern,
			&cfg.QuotaType,
			&cfg.MaxRequests,
			&cfg.RequestPeriod,
			&cfg.MaxTokens,
			&cfg.TokenPeriod,
			&cfg.CreatedAt,
			&cfg.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描额度配置行失败: %w", err)
		}
		configs = append(configs, cfg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历额度配置结果集失败: %w", err)
	}
	return configs, nil
}

// GetQuotaConfigByModel 根据模型名称精确匹配额度配置
// 未找到时返回 sql.ErrNoRows
func (s *PostgresStore) GetQuotaConfigByModel(ctx context.Context, model string) (*QuotaConfig, error) {
	const query = `
		SELECT id, model_pattern, quota_type, max_requests, request_period, max_tokens, token_period, created_at, updated_at
		FROM quota_configs
		WHERE model_pattern = $1
	`

	cfg := &QuotaConfig{}
	err := s.db.QueryRowContext(ctx, query, model).Scan(
		&cfg.ID,
		&cfg.ModelPattern,
		&cfg.QuotaType,
		&cfg.MaxRequests,
		&cfg.RequestPeriod,
		&cfg.MaxTokens,
		&cfg.TokenPeriod,
		&cfg.CreatedAt,
		&cfg.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("查询模型 [%s] 额度配置失败: %w", model, err)
	}
	return cfg, nil
}

// UpdateQuotaConfig 更新已有的额度配置
// 同时刷新 updated_at 字段
func (s *PostgresStore) UpdateQuotaConfig(ctx context.Context, cfg *QuotaConfig) error {
	const query = `
		UPDATE quota_configs
		SET model_pattern = $1, quota_type = $2, max_requests = $3, request_period = $4,
		    max_tokens = $5, token_period = $6, updated_at = CURRENT_TIMESTAMP
		WHERE id = $7
	`

	_, err := s.db.ExecContext(ctx, query,
		cfg.ModelPattern,
		cfg.QuotaType,
		cfg.MaxRequests,
		cfg.RequestPeriod,
		cfg.MaxTokens,
		cfg.TokenPeriod,
		cfg.ID,
	)
	if err != nil {
		return fmt.Errorf("更新额度配置 [%d] 失败: %w", cfg.ID, err)
	}
	cfg.UpdatedAt = time.Now()
	return nil
}

// DeleteQuotaConfig 根据 ID 删除额度配置
// 删除不存在的记录时静默成功（幂等操作）
func (s *PostgresStore) DeleteQuotaConfig(ctx context.Context, id int64) error {
	const query = `DELETE FROM quota_configs WHERE id = $1`

	_, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("删除额度配置 [%d] 失败: %w", id, err)
	}
	return nil
}

// ============================================================
// 用户额度操作
// ============================================================

// GetUserQuota 查询用户在指定模型下的额度使用状态
// 未找到时返回 sql.ErrNoRows
func (s *PostgresStore) GetUserQuota(ctx context.Context, userID int64, model string) (*UserQuota, error) {
	const query = `
		SELECT id, user_id, model_pattern, used_requests, used_tokens, bonus_requests, bonus_tokens, period_start
		FROM user_quotas
		WHERE user_id = $1 AND model_pattern = $2
	`

	q := &UserQuota{}
	err := s.db.QueryRowContext(ctx, query, userID, model).Scan(
		&q.ID,
		&q.UserID,
		&q.ModelPattern,
		&q.UsedRequests,
		&q.UsedTokens,
		&q.BonusRequests,
		&q.BonusTokens,
		&q.PeriodStart,
	)
	if err != nil {
		return nil, fmt.Errorf("查询用户 [%d] 模型 [%s] 额度失败: %w", userID, model, err)
	}
	return q, nil
}

// UpsertUserQuota 插入或更新用户额度记录
// 利用 user_id + model_pattern 唯一约束 + ON CONFLICT 实现 upsert
func (s *PostgresStore) UpsertUserQuota(ctx context.Context, quota *UserQuota) error {
	const query = `
		INSERT INTO user_quotas (user_id, model_pattern, used_requests, used_tokens, bonus_requests, bonus_tokens, period_start)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, model_pattern) DO UPDATE SET
			used_requests = EXCLUDED.used_requests,
			used_tokens = EXCLUDED.used_tokens,
			bonus_requests = EXCLUDED.bonus_requests,
			bonus_tokens = EXCLUDED.bonus_tokens,
			period_start = EXCLUDED.period_start
		RETURNING id
	`

	err := s.db.QueryRowContext(ctx, query,
		quota.UserID,
		quota.ModelPattern,
		quota.UsedRequests,
		quota.UsedTokens,
		quota.BonusRequests,
		quota.BonusTokens,
		quota.PeriodStart,
	).Scan(&quota.ID)
	if err != nil {
		return fmt.Errorf("upsert 用户 [%d] 模型 [%s] 额度失败: %w", quota.UserID, quota.ModelPattern, err)
	}
	return nil
}

// DeductUserQuota 扣减用户额度（原子递增已用计数）
// requests 和 tokens 为本次消耗增量，非绝对值
func (s *PostgresStore) DeductUserQuota(ctx context.Context, userID int64, model string, requests int64, tokens int64) error {
	const query = `
		UPDATE user_quotas
		SET used_requests = used_requests + $1, used_tokens = used_tokens + $2
		WHERE user_id = $3 AND model_pattern = $4
	`

	result, err := s.db.ExecContext(ctx, query, requests, tokens, userID, model)
	if err != nil {
		return fmt.Errorf("扣减用户 [%d] 模型 [%s] 额度失败: %w", userID, model, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取扣减影响行数失败: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ResetExpiredQuotas 重置过期周期的用户额度
// 将 period_start 早于当天零点的记录清零并更新周期起点
// 使用 CURRENT_DATE 替代 SQLite 的 date('now','start of day')
func (s *PostgresStore) ResetExpiredQuotas(ctx context.Context) (int64, error) {
	const query = `
		UPDATE user_quotas
		SET used_requests = 0, used_tokens = 0, period_start = NOW()
		WHERE period_start < CURRENT_DATE
	`

	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("重置过期额度失败: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("获取重置影响行数失败: %w", err)
	}
	return affected, nil
}
