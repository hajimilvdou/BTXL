package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ============================================================
// PostgresStore — TemplateStore 接口实现
// 兑换码模板 CRUD、原子递增/递减、用户领取次数统计
// PG 差异: BOOLEAN 直接映射 bool / JSONB 原生支持
// ============================================================

// CreateTemplate 创建兑换码模板
func (s *PostgresStore) CreateTemplate(ctx context.Context, tpl *RedemptionTemplate) error {
	bonusJSON, err := json.Marshal(tpl.BonusQuota)
	if err != nil {
		return fmt.Errorf("序列化 bonus_quota 失败: %w", err)
	}

	now := time.Now()

	err = s.db.QueryRowContext(ctx,
		`INSERT INTO redemption_templates
			(name, description, bonus_quota, max_per_user, total_limit, issued_count, enabled, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id`,
		tpl.Name, tpl.Description, string(bonusJSON),
		tpl.MaxPerUser, tpl.TotalLimit, tpl.IssuedCount,
		tpl.Enabled, now,
	).Scan(&tpl.ID)
	if err != nil {
		return fmt.Errorf("插入模板失败: %w", err)
	}

	tpl.CreatedAt = now
	return nil
}

// GetTemplateByID 根据 ID 查询模板
func (s *PostgresStore) GetTemplateByID(ctx context.Context, id int64) (*RedemptionTemplate, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, bonus_quota, max_per_user,
		        total_limit, issued_count, enabled, created_at
		 FROM redemption_templates WHERE id = $1`, id)
	return pgScanTemplate(row)
}

// ListTemplates 列出所有已启用的模板
func (s *PostgresStore) ListTemplates(ctx context.Context) ([]*RedemptionTemplate, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, bonus_quota, max_per_user,
		        total_limit, issued_count, enabled, created_at
		 FROM redemption_templates WHERE enabled = TRUE
		 ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("查询模板列表失败: %w", err)
	}
	defer rows.Close()

	var list []*RedemptionTemplate
	for rows.Next() {
		tpl, err := pgScanTemplate(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, tpl)
	}
	return list, rows.Err()
}

// IncrementTemplateIssuedCount 原子递增模板已发放数
// WHERE 条件同时检查 issued_count < total_limit，防止 TOCTOU 竞态
func (s *PostgresStore) IncrementTemplateIssuedCount(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE redemption_templates SET issued_count = issued_count + 1
		 WHERE id = $1 AND issued_count < total_limit`, id)
	if err != nil {
		return fmt.Errorf("更新模板发放数失败: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("模板已发放完毕或不存在 (id=%d)", id)
	}
	return nil
}

// CountTemplateClaimsByUser 统计用户对某模板的领取次数
// 通过 invite_codes 表统计: bonus_quota JSONB 匹配
func (s *PostgresStore) CountTemplateClaimsByUser(ctx context.Context, userID, templateID int64) (int, error) {
	// 获取模板的 bonus_quota 作为匹配条件
	tpl, err := s.GetTemplateByID(ctx, templateID)
	if err != nil {
		return 0, err
	}

	bonusJSON, err := json.Marshal(tpl.BonusQuota)
	if err != nil {
		return 0, fmt.Errorf("序列化 bonus_quota 失败: %w", err)
	}

	var count int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM invite_codes
		 WHERE creator_id = $1 AND type = $2 AND bonus_quota = $3::jsonb`,
		userID, string(InviteAdminCreated), string(bonusJSON),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("统计用户领取数失败: %w", err)
	}
	return count, nil
}
