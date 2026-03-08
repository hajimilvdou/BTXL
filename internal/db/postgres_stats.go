package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ============================================================
// PostgresStore — StatsStore 接口实现
// 基于 request_logs 表的请求日志与聚合统计
// PG 差异: $N 占位符替代 ?
// ============================================================

// RecordRequest 写入单条请求日志
func (s *PostgresStore) RecordRequest(ctx context.Context, log *RequestLog) error {
	const query = `
		INSERT INTO request_logs (
			user_id, model, provider, credential_id,
			input_tokens, output_tokens, latency_ms,
			status_code, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`

	// 若调用方未指定时间，自动填充当前时刻
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}

	err := s.db.QueryRowContext(ctx, query,
		log.UserID, log.Model, log.Provider, log.CredentialID,
		log.InputTokens, log.OutputTokens, log.Latency,
		log.StatusCode, log.CreatedAt,
	).Scan(&log.ID)
	if err != nil {
		return fmt.Errorf("写入请求日志失败: %w", err)
	}
	return nil
}

// GetRequestStats 获取全局请求统计
// 分三步查询: 聚合指标 -> 按模型分组 -> 按提供商分组
func (s *PostgresStore) GetRequestStats(ctx context.Context, opts RequestStatsOpts) (*RequestStats, error) {
	where, args := pgBuildStatsWhere(opts)

	stats := &RequestStats{
		ByModel:    make(map[string]int64),
		ByProvider: make(map[string]int64),
	}

	// 第一步: 聚合指标（总请求数、总 token、平均延迟）
	aggQuery := fmt.Sprintf(`
		SELECT
			COALESCE(COUNT(*), 0),
			COALESCE(SUM(input_tokens + output_tokens), 0),
			COALESCE(AVG(latency_ms), 0)
		FROM request_logs
		%s
	`, where)

	err := s.db.QueryRowContext(ctx, aggQuery, args...).Scan(
		&stats.TotalRequests,
		&stats.TotalTokens,
		&stats.AvgLatency,
	)
	if err != nil {
		return nil, fmt.Errorf("查询聚合统计失败: %w", err)
	}

	// 第二步: 按模型分组
	if err := s.pgFillGroupStats(ctx, stats.ByModel, "model", where, args); err != nil {
		return nil, fmt.Errorf("查询模型分组统计失败: %w", err)
	}

	// 第三步: 按提供商分组
	if err := s.pgFillGroupStats(ctx, stats.ByProvider, "provider", where, args); err != nil {
		return nil, fmt.Errorf("查询提供商分组统计失败: %w", err)
	}

	return stats, nil
}

// GetUserRequestStats 获取指定用户的请求统计
func (s *PostgresStore) GetUserRequestStats(ctx context.Context, userID int64, opts RequestStatsOpts) (*UserRequestStats, error) {
	where, args := pgBuildUserStatsWhere(userID, opts)

	stats := &UserRequestStats{
		UserID:  userID,
		ByModel: make(map[string]int64),
	}

	// 聚合指标
	aggQuery := fmt.Sprintf(`
		SELECT
			COALESCE(COUNT(*), 0),
			COALESCE(SUM(input_tokens + output_tokens), 0)
		FROM request_logs
		%s
	`, where)

	err := s.db.QueryRowContext(ctx, aggQuery, args...).Scan(
		&stats.TotalRequests,
		&stats.TotalTokens,
	)
	if err != nil {
		return nil, fmt.Errorf("查询用户 [%d] 聚合统计失败: %w", userID, err)
	}

	// 按模型分组
	if err := s.pgFillGroupStats(ctx, stats.ByModel, "model", where, args); err != nil {
		return nil, fmt.Errorf("查询用户 [%d] 模型分组统计失败: %w", userID, err)
	}

	return stats, nil
}

// ============================================================
// 内部辅助函数
// ============================================================

// pgFillGroupStats 通用分组聚合查询（PG 版）
func (s *PostgresStore) pgFillGroupStats(
	ctx context.Context,
	target map[string]int64,
	groupCol string,
	where string,
	args []any,
) error {
	query := fmt.Sprintf(`
		SELECT %s, COUNT(*)
		FROM request_logs
		%s
		GROUP BY %s
	`, groupCol, where, groupCol)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		var count int64
		if err := rows.Scan(&key, &count); err != nil {
			return err
		}
		target[key] = count
	}
	return rows.Err()
}

// pgBuildStatsWhere 根据 RequestStatsOpts 构建 WHERE 子句（$N 占位符）
func pgBuildStatsWhere(opts RequestStatsOpts) (string, []any) {
	var conds []string
	var args []any
	paramN := 1

	if opts.After != nil {
		conds = append(conds, fmt.Sprintf("created_at >= $%d", paramN))
		args = append(args, *opts.After)
		paramN++
	}
	if opts.Before != nil {
		conds = append(conds, fmt.Sprintf("created_at < $%d", paramN))
		args = append(args, *opts.Before)
		paramN++
	}
	if opts.Model != nil {
		conds = append(conds, fmt.Sprintf("model = $%d", paramN))
		args = append(args, *opts.Model)
		paramN++
	}

	if len(conds) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// pgBuildUserStatsWhere 在 pgBuildStatsWhere 基础上追加 user_id 条件
func pgBuildUserStatsWhere(userID int64, opts RequestStatsOpts) (string, []any) {
	conds := []string{"user_id = $1"}
	args := []any{userID}
	paramN := 2

	if opts.After != nil {
		conds = append(conds, fmt.Sprintf("created_at >= $%d", paramN))
		args = append(args, *opts.After)
		paramN++
	}
	if opts.Before != nil {
		conds = append(conds, fmt.Sprintf("created_at < $%d", paramN))
		args = append(args, *opts.Before)
		paramN++
	}
	if opts.Model != nil {
		conds = append(conds, fmt.Sprintf("model = $%d", paramN))
		args = append(args, *opts.Model)
		paramN++
	}

	return "WHERE " + strings.Join(conds, " AND "), args
}
