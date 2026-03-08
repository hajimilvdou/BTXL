package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// ============================================================
// PostgresStore — InviteCodeStore 接口实现
// 邀请码 / 兑换码 CRUD、使用量递增、使用记录
// ============================================================

// ============================================================
// 创建邀请码
// ============================================================

// CreateInviteCode 插入新邀请码，通过 RETURNING id 回写自增 ID
// BonusQuota / ReferralBonus 序列化为 JSON 存入 JSONB 列
func (s *PostgresStore) CreateInviteCode(ctx context.Context, code *InviteCode) error {
	// 序列化 JSON 字段
	var bonusQuota, referralBonus sql.NullString

	if code.BonusQuota != nil {
		data, err := json.Marshal(code.BonusQuota)
		if err != nil {
			return fmt.Errorf("序列化 bonus_quota 失败: %w", err)
		}
		bonusQuota = sql.NullString{String: string(data), Valid: true}
	}
	if code.ReferralBonus != nil {
		data, err := json.Marshal(code.ReferralBonus)
		if err != nil {
			return fmt.Errorf("序列化 referral_bonus 失败: %w", err)
		}
		referralBonus = sql.NullString{String: string(data), Valid: true}
	}

	const query = `
		INSERT INTO invite_codes (
			code, type, creator_id, max_uses, used_count,
			require_email, bonus_quota, referral_bonus, expires_at,
			status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`
	err := s.db.QueryRowContext(ctx, query,
		code.Code, code.Type, code.CreatorID, code.MaxUses, code.UsedCount,
		code.RequireEmail, bonusQuota, referralBonus, code.ExpiresAt,
		code.Status, code.CreatedAt,
	).Scan(&code.ID)
	if err != nil {
		return fmt.Errorf("插入邀请码失败: %w", err)
	}
	return nil
}

// ============================================================
// 单条查询
// ============================================================

// GetInviteCodeByCode 按邀请码字符串查询
func (s *PostgresStore) GetInviteCodeByCode(ctx context.Context, code string) (*InviteCode, error) {
	query := fmt.Sprintf("SELECT %s FROM invite_codes WHERE code = $1", inviteCodeColumns)
	row := s.db.QueryRowContext(ctx, query, code)
	c, err := scanInviteCode(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("邀请码 code=%s 不存在: %w", code, err)
	}
	return c, err
}

// ============================================================
// 列表查询
// ============================================================

// ListInviteCodes 分页查询邀请码列表，支持按类型和状态过滤
func (s *PostgresStore) ListInviteCodes(ctx context.Context, opts ListInviteCodesOpts) ([]*InviteCode, int64, error) {
	// 动态构建 WHERE 条件（$N 占位符）
	var conditions []string
	var args []interface{}
	paramN := 1

	if opts.Type != nil {
		conditions = append(conditions, fmt.Sprintf("type = $%d", paramN))
		args = append(args, *opts.Type)
		paramN++
	}
	if opts.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", paramN))
		args = append(args, *opts.Status)
		paramN++
	}
	if opts.CreatorID != nil {
		conditions = append(conditions, fmt.Sprintf("creator_id = $%d", paramN))
		args = append(args, *opts.CreatorID)
		paramN++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// 查询总数
	countQuery := "SELECT COUNT(*) FROM invite_codes" + whereClause
	var total int64
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计邀请码总数失败: %w", err)
	}

	// 分页参数
	page := opts.Page
	if page < 1 {
		page = 1
	}
	pageSize := opts.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// 查询分页数据
	dataQuery := fmt.Sprintf(
		"SELECT %s FROM invite_codes%s ORDER BY id DESC LIMIT $%d OFFSET $%d",
		inviteCodeColumns, whereClause, paramN, paramN+1,
	)
	dataArgs := make([]interface{}, 0, len(args)+2)
	dataArgs = append(dataArgs, args...)
	dataArgs = append(dataArgs, pageSize, offset)

	rows, err := s.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询邀请码列表失败: %w", err)
	}
	defer rows.Close()

	var codes []*InviteCode
	for rows.Next() {
		c, err := scanInviteCode(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("扫描邀请码行失败: %w", err)
		}
		codes = append(codes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历邀请码行失败: %w", err)
	}
	return codes, total, nil
}

// ============================================================
// 更新
// ============================================================

// UpdateInviteCode 更新邀请码所有可变字段
func (s *PostgresStore) UpdateInviteCode(ctx context.Context, code *InviteCode) error {
	// 序列化 JSON 字段
	var bonusQuota, referralBonus sql.NullString

	if code.BonusQuota != nil {
		data, err := json.Marshal(code.BonusQuota)
		if err != nil {
			return fmt.Errorf("序列化 bonus_quota 失败: %w", err)
		}
		bonusQuota = sql.NullString{String: string(data), Valid: true}
	}
	if code.ReferralBonus != nil {
		data, err := json.Marshal(code.ReferralBonus)
		if err != nil {
			return fmt.Errorf("序列化 referral_bonus 失败: %w", err)
		}
		referralBonus = sql.NullString{String: string(data), Valid: true}
	}

	const query = `
		UPDATE invite_codes SET
			code = $1, type = $2, creator_id = $3, max_uses = $4, used_count = $5,
			require_email = $6, bonus_quota = $7, referral_bonus = $8,
			expires_at = $9, status = $10
		WHERE id = $11
	`
	result, err := s.db.ExecContext(ctx, query,
		code.Code, code.Type, code.CreatorID, code.MaxUses, code.UsedCount,
		code.RequireEmail, bonusQuota, referralBonus,
		code.ExpiresAt, code.Status,
		code.ID,
	)
	if err != nil {
		return fmt.Errorf("更新邀请码失败: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("邀请码 id=%d 不存在，更新无效", code.ID)
	}
	return nil
}

// ============================================================
// 使用量递增 & 使用记录
// ============================================================

// IncrementInviteCodeUsage 原子递增邀请码使用次数
// WHERE 条件同时检查 used_count < max_uses，防止 TOCTOU 竞态
func (s *PostgresStore) IncrementInviteCodeUsage(ctx context.Context, codeID int64) error {
	const query = `UPDATE invite_codes SET used_count = used_count + 1 WHERE id = $1 AND used_count < max_uses`
	result, err := s.db.ExecContext(ctx, query, codeID)
	if err != nil {
		return fmt.Errorf("递增邀请码使用次数失败: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("邀请码已用尽或不存在 (id=%d)", codeID)
	}
	return nil
}

// RecordInviteCodeUsage 记录邀请码使用详情
func (s *PostgresStore) RecordInviteCodeUsage(ctx context.Context, usage *InviteCodeUsage) error {
	const query = `
		INSERT INTO invite_code_usage (code_id, user_id, used_at)
		VALUES ($1, $2, $3)
		RETURNING id
	`
	err := s.db.QueryRowContext(ctx, query,
		usage.CodeID, usage.UserID, usage.UsedAt,
	).Scan(&usage.ID)
	if err != nil {
		return fmt.Errorf("记录邀请码使用详情失败: %w", err)
	}
	return nil
}

// HasUserUsedCode 检查用户是否已使用过指定邀请码
func (s *PostgresStore) HasUserUsedCode(ctx context.Context, codeID, userID int64) (bool, error) {
	const query = `SELECT COUNT(*) FROM invite_code_usage WHERE code_id = $1 AND user_id = $2`
	var count int
	if err := s.db.QueryRowContext(ctx, query, codeID, userID).Scan(&count); err != nil {
		return false, fmt.Errorf("查询用户兑换记录失败: %w", err)
	}
	return count > 0, nil
}
