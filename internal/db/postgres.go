package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// ============================================================
// PostgresStore — 基于 PostgreSQL 的存储实现
// 使用 pgx/v5/stdlib 通过 database/sql 接口操作
// ============================================================

// PostgresStore 基于 PostgreSQL 的存储实现
type PostgresStore struct {
	db *sql.DB
}

// 编译时接口检查
var _ Store = (*PostgresStore)(nil)

// NewPostgresStore 创建 PostgreSQL 存储实例
func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if dsn == "" {
		return nil, fmt.Errorf("PostgreSQL DSN 不能为空")
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 PostgreSQL 数据库失败: %w", err)
	}

	// 连接池配置 — 适合中等并发场景
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("PostgreSQL 连接测试失败: %w", err)
	}

	return &PostgresStore{db: sqlDB}, nil
}

// ============================================================
// 迁移
// ============================================================

// Migrate 执行数据库迁移
func (s *PostgresStore) Migrate(ctx context.Context) error {
	data, err := MigrationFS.ReadFile("migrations/001_init_postgres.sql")
	if err != nil {
		return fmt.Errorf("读取 PostgreSQL 迁移脚本失败: %w", err)
	}

	// 按分号拆分并逐条执行（复用 splitSQL 工具函数）
	statements := splitSQL(string(data))
	for _, stmt := range statements {
		if stmt = strings.TrimSpace(stmt); stmt == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("执行 PostgreSQL 迁移语句失败: %w\nSQL: %s", err, stmt)
		}
	}
	return nil
}

// Close 关闭数据库连接
func (s *PostgresStore) Close() error {
	return s.db.Close()
}
