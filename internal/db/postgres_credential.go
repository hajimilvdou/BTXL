package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ============================================================
// PostgresStore — CredentialStore 接口实现
// 管理 API 凭证 (credentials) 及其健康记录 (credential_health)
// 支持公共池与用户私有凭证的分层管理
// PG 差异: enabled = TRUE 替代 enabled = 1 / RETURNING id
// ============================================================

// ============================================================
// 凭证 CRUD
// ============================================================

// CreateCredential 创建新的 API 凭证
// ID 由调用方在外部生成（通常为 UUID）
func (s *PostgresStore) CreateCredential(ctx context.Context, cred *Credential) error {
	const query = `
		INSERT INTO credentials (id, provider, owner_id, data, health, weight, enabled, added_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP)
	`

	var ownerID any
	if cred.OwnerID != nil {
		ownerID = *cred.OwnerID
	}

	_, err := s.db.ExecContext(ctx, query,
		cred.ID,
		cred.Provider,
		ownerID,
		cred.Data,
		cred.Health,
		cred.Weight,
		cred.Enabled,
	)
	if err != nil {
		return fmt.Errorf("创建凭证 [%s] 失败: %w", cred.ID, err)
	}
	cred.AddedAt = time.Now()
	return nil
}

// GetCredentialByID 根据 ID 获取单条凭证
// 未找到时返回 sql.ErrNoRows
func (s *PostgresStore) GetCredentialByID(ctx context.Context, id string) (*Credential, error) {
	query := fmt.Sprintf(`SELECT %s FROM credentials WHERE id = $1`, credentialColumns)

	cred, err := scanCredential(s.db.QueryRowContext(ctx, query, id))
	if err != nil {
		return nil, fmt.Errorf("查询凭证 [%s] 失败: %w", id, err)
	}
	return cred, nil
}

// ListCredentials 分页查询凭证列表
// 支持按 owner_id / provider / health 筛选
func (s *PostgresStore) ListCredentials(ctx context.Context, opts ListCredentialsOpts) ([]*Credential, int64, error) {
	// 动态构建 WHERE 子句（$N 占位符）
	var conditions []string
	var args []any
	paramN := 1

	if opts.OwnerID != nil {
		conditions = append(conditions, fmt.Sprintf("owner_id = $%d", paramN))
		args = append(args, *opts.OwnerID)
		paramN++
	}
	if opts.Provider != nil {
		conditions = append(conditions, fmt.Sprintf("provider = $%d", paramN))
		args = append(args, *opts.Provider)
		paramN++
	}
	if opts.Health != nil {
		conditions = append(conditions, fmt.Sprintf("health = $%d", paramN))
		args = append(args, *opts.Health)
		paramN++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// 查询总数
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM credentials %s`, where)
	var total int64
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计凭证总数失败: %w", err)
	}

	// 分页查询
	page, pageSize := normalizePage(opts.Page, opts.PageSize)
	offset := (page - 1) * pageSize

	dataQuery := fmt.Sprintf(
		`SELECT %s FROM credentials %s ORDER BY added_at DESC LIMIT $%d OFFSET $%d`,
		credentialColumns, where, paramN, paramN+1,
	)
	dataArgs := make([]any, 0, len(args)+2)
	dataArgs = append(dataArgs, args...)
	dataArgs = append(dataArgs, pageSize, offset)

	rows, err := s.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询凭证列表失败: %w", err)
	}
	defer rows.Close()

	var creds []*Credential
	for rows.Next() {
		cred, err := scanCredential(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("扫描凭证行失败: %w", err)
		}
		creds = append(creds, cred)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历凭证结果集失败: %w", err)
	}
	return creds, total, nil
}

// UpdateCredential 更新凭证信息
func (s *PostgresStore) UpdateCredential(ctx context.Context, cred *Credential) error {
	const query = `
		UPDATE credentials
		SET provider = $1, owner_id = $2, data = $3, health = $4, weight = $5, enabled = $6
		WHERE id = $7
	`

	var ownerID any
	if cred.OwnerID != nil {
		ownerID = *cred.OwnerID
	}

	_, err := s.db.ExecContext(ctx, query,
		cred.Provider,
		ownerID,
		cred.Data,
		cred.Health,
		cred.Weight,
		cred.Enabled,
		cred.ID,
	)
	if err != nil {
		return fmt.Errorf("更新凭证 [%s] 失败: %w", cred.ID, err)
	}
	return nil
}

// DeleteCredential 根据 ID 删除凭证
// 删除不存在的记录时静默成功（幂等操作）
func (s *PostgresStore) DeleteCredential(ctx context.Context, id string) error {
	const query = `DELETE FROM credentials WHERE id = $1`

	_, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("删除凭证 [%s] 失败: %w", id, err)
	}
	return nil
}

// ============================================================
// 凭证池查询
// ============================================================

// GetPublicPoolCredentials 获取公共池中可用的凭证
// 筛选条件：无 owner、指定 provider、已启用、健康状态非 down
func (s *PostgresStore) GetPublicPoolCredentials(ctx context.Context, provider string) ([]*Credential, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM credentials WHERE owner_id IS NULL AND provider = $1 AND enabled = TRUE AND health != 'down' ORDER BY weight DESC`,
		credentialColumns,
	)

	rows, err := s.db.QueryContext(ctx, query, provider)
	if err != nil {
		return nil, fmt.Errorf("查询公共池凭证 [%s] 失败: %w", provider, err)
	}
	defer rows.Close()

	var creds []*Credential
	for rows.Next() {
		cred, err := scanCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描公共池凭证行失败: %w", err)
		}
		creds = append(creds, cred)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历公共池凭证结果集失败: %w", err)
	}
	return creds, nil
}

// GetUserCredentials 获取用户私有的已启用凭证
func (s *PostgresStore) GetUserCredentials(ctx context.Context, userID int64, provider string) ([]*Credential, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM credentials WHERE owner_id = $1 AND provider = $2 AND enabled = TRUE ORDER BY weight DESC`,
		credentialColumns,
	)

	rows, err := s.db.QueryContext(ctx, query, userID, provider)
	if err != nil {
		return nil, fmt.Errorf("查询用户 [%d] 凭证 [%s] 失败: %w", userID, provider, err)
	}
	defer rows.Close()

	var creds []*Credential
	for rows.Next() {
		cred, err := scanCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描用户凭证行失败: %w", err)
		}
		creds = append(creds, cred)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历用户凭证结果集失败: %w", err)
	}
	return creds, nil
}

// ============================================================
// 凭证健康记录
// ============================================================

// RecordCredentialHealth 记录一次凭证健康检查结果
// 同时在事务中更新 credentials 表的 health 字段以保持一致
func (s *PostgresStore) RecordCredentialHealth(ctx context.Context, record *CredentialHealthRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启健康记录事务失败: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // 提交后 Rollback 为无操作

	// 插入健康检查记录（RETURNING id 替代 LastInsertId）
	const insertQuery = `
		INSERT INTO credential_health (credential_id, status, latency_ms, error_msg, checked_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		RETURNING id
	`
	err = tx.QueryRowContext(ctx, insertQuery,
		record.CredentialID,
		record.Status,
		record.Latency,
		record.ErrorMsg,
	).Scan(&record.ID)
	if err != nil {
		return fmt.Errorf("插入健康记录失败: %w", err)
	}
	record.CheckedAt = time.Now()

	// 同步更新凭证主表的健康状态
	const updateQuery = `UPDATE credentials SET health = $1 WHERE id = $2`
	if _, err := tx.ExecContext(ctx, updateQuery, record.Status, record.CredentialID); err != nil {
		return fmt.Errorf("同步更新凭证健康状态失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交健康记录事务失败: %w", err)
	}
	return nil
}
