package db

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ============================================================
// PostgreSQL 共享工具函数
// 占位符生成、PG 专用扫描器等复用工具
// ============================================================

// pgPlaceholders 生成 PostgreSQL 参数占位符序列
// 例如: pgPlaceholders(1, 3) → "$1, $2, $3"
func pgPlaceholders(start, count int) string {
	parts := make([]string, count)
	for i := 0; i < count; i++ {
		parts[i] = fmt.Sprintf("$%d", start+i)
	}
	return strings.Join(parts, ", ")
}

// pgPlaceholderList 生成带括号的占位符列表
// 例如: pgPlaceholderList(1, 3) → "($1, $2, $3)"
func pgPlaceholderList(start, count int) string {
	return "(" + pgPlaceholders(start, count) + ")"
}

// ============================================================
// PG 专用扫描器 — 处理与 SQLite 的类型差异
// ============================================================

// pgScanTemplate PostgreSQL 专用模板扫描器
// PG 的 BOOLEAN 直接扫描为 bool，JSONB 扫描为 string
func pgScanTemplate(sc interface{ Scan(dest ...any) error }) (*RedemptionTemplate, error) {
	var tpl RedemptionTemplate
	var bonusJSON string

	err := sc.Scan(
		&tpl.ID, &tpl.Name, &tpl.Description, &bonusJSON,
		&tpl.MaxPerUser, &tpl.TotalLimit, &tpl.IssuedCount,
		&tpl.Enabled, &tpl.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("扫描模板行失败: %w", err)
	}

	if err := json.Unmarshal([]byte(bonusJSON), &tpl.BonusQuota); err != nil {
		return nil, fmt.Errorf("反序列化 bonus_quota 失败: %w", err)
	}
	return &tpl, nil
}
