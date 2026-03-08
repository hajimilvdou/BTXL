package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ============================================================
// PostgresStore — UserStore 接口实现
// 用户 CRUD、多维度查询、分页列表
// ============================================================

// ============================================================
// 创建用户
// ============================================================

// CreateUser 插入新用户，通过 RETURNING id 回写自增 ID
func (s *PostgresStore) CreateUser(ctx context.Context, user *User) error {
	const query = `
		INSERT INTO users (
			uuid, username, email, password_hash, role, status,
			api_key, oauth_provider, oauth_id, invited_by,
			invite_code, pool_mode, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id
	`
	err := s.db.QueryRowContext(ctx, query,
		user.UUID, user.Username, user.Email, user.PasswordHash,
		user.Role, user.Status, user.APIKey, user.OAuthProvider,
		user.OAuthID, user.InvitedBy, user.InviteCode, user.PoolMode,
		user.CreatedAt, user.UpdatedAt,
	).Scan(&user.ID)
	if err != nil {
		return fmt.Errorf("插入用户失败: %w", err)
	}
	return nil
}

// ============================================================
// 单条查询（多种维度）
// ============================================================

// GetUserByID 按主键查询用户
func (s *PostgresStore) GetUserByID(ctx context.Context, id int64) (*User, error) {
	query := fmt.Sprintf("SELECT %s FROM users WHERE id = $1", userColumns)
	row := s.db.QueryRowContext(ctx, query, id)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("用户 id=%d 不存在: %w", id, err)
	}
	return u, err
}

// GetUserByUUID 按 UUID 查询用户
func (s *PostgresStore) GetUserByUUID(ctx context.Context, uuid string) (*User, error) {
	query := fmt.Sprintf("SELECT %s FROM users WHERE uuid = $1", userColumns)
	row := s.db.QueryRowContext(ctx, query, uuid)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("用户 uuid=%s 不存在: %w", uuid, err)
	}
	return u, err
}

// GetUserByUsername 按用户名查询用户
func (s *PostgresStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	query := fmt.Sprintf("SELECT %s FROM users WHERE username = $1", userColumns)
	row := s.db.QueryRowContext(ctx, query, username)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("用户 username=%s 不存在: %w", username, err)
	}
	return u, err
}

// GetUserByEmail 按邮箱查询用户
func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := fmt.Sprintf("SELECT %s FROM users WHERE email = $1", userColumns)
	row := s.db.QueryRowContext(ctx, query, email)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("用户 email=%s 不存在: %w", email, err)
	}
	return u, err
}

// GetUserByAPIKey 按 API Key 查询用户
func (s *PostgresStore) GetUserByAPIKey(ctx context.Context, apiKey string) (*User, error) {
	query := fmt.Sprintf("SELECT %s FROM users WHERE api_key = $1", userColumns)
	row := s.db.QueryRowContext(ctx, query, apiKey)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("用户 api_key 不存在: %w", err)
	}
	return u, err
}

// GetUserByOAuth 按 OAuth 提供方 + ID 查询用户
func (s *PostgresStore) GetUserByOAuth(ctx context.Context, provider, oauthID string) (*User, error) {
	query := fmt.Sprintf(
		"SELECT %s FROM users WHERE oauth_provider = $1 AND oauth_id = $2",
		userColumns,
	)
	row := s.db.QueryRowContext(ctx, query, provider, oauthID)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("用户 oauth(%s/%s) 不存在: %w", provider, oauthID, err)
	}
	return u, err
}

// ============================================================
// 更新 & 删除
// ============================================================

// UpdateUser 更新用户所有可变字段
func (s *PostgresStore) UpdateUser(ctx context.Context, user *User) error {
	const query = `
		UPDATE users SET
			username = $1, email = $2, password_hash = $3, role = $4,
			status = $5, api_key = $6, oauth_provider = $7, oauth_id = $8,
			invited_by = $9, invite_code = $10, pool_mode = $11, updated_at = $12
		WHERE id = $13
	`
	result, err := s.db.ExecContext(ctx, query,
		user.Username, user.Email, user.PasswordHash, user.Role,
		user.Status, user.APIKey, user.OAuthProvider, user.OAuthID,
		user.InvitedBy, user.InviteCode, user.PoolMode, user.UpdatedAt,
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("更新用户失败: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("用户 id=%d 不存在，更新无效", user.ID)
	}
	return nil
}

// DeleteUser 按主键删除用户
func (s *PostgresStore) DeleteUser(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("删除用户失败: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("用户 id=%d 不存在，删除无效", id)
	}
	return nil
}

// ============================================================
// 列表 & 计数
// ============================================================

// ListUsers 分页查询用户列表，返回用户列表和总数
func (s *PostgresStore) ListUsers(ctx context.Context, opts ListUsersOpts) ([]*User, int64, error) {
	// 动态构建 WHERE 条件（$N 占位符）
	var conditions []string
	var args []interface{}
	paramN := 1

	if opts.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", paramN))
		args = append(args, *opts.Status)
		paramN++
	}
	if opts.Role != nil {
		conditions = append(conditions, fmt.Sprintf("role = $%d", paramN))
		args = append(args, *opts.Role)
		paramN++
	}
	if opts.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(username LIKE $%d OR email LIKE $%d)", paramN, paramN+1))
		pattern := "%" + opts.Search + "%"
		args = append(args, pattern, pattern)
		paramN += 2
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// 查询总数
	countQuery := "SELECT COUNT(*) FROM users" + whereClause
	var total int64
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计用户总数失败: %w", err)
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
		"SELECT %s FROM users%s ORDER BY id DESC LIMIT $%d OFFSET $%d",
		userColumns, whereClause, paramN, paramN+1,
	)
	dataArgs := make([]interface{}, 0, len(args)+2)
	dataArgs = append(dataArgs, args...)
	dataArgs = append(dataArgs, pageSize, offset)

	rows, err := s.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询用户列表失败: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("扫描用户行失败: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历用户行失败: %w", err)
	}
	return users, total, nil
}

// CountUsers 统计用户总数
func (s *PostgresStore) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("统计用户总数失败: %w", err)
	}
	return count, nil
}
