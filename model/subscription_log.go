/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// SubscriptionLogItem 是管理员「订阅日志」视图中的一条记录：谁、何时、
// 通过什么来源购买了哪个套餐、状态、档位与分组变化。字段来自
// user_subscriptions 联表 users（用户名）与 subscription_plans（套餐信息）。
type SubscriptionLogItem struct {
	Id                  int     `json:"id"`
	UserId              int     `json:"user_id"`
	Username            string  `json:"username"`
	PlanId              int     `json:"plan_id"`
	PlanTitle           string  `json:"plan_title"`
	PriceAmount         float64 `json:"price_amount"`
	Currency            string  `json:"currency"`
	DurationUnit        string  `json:"duration_unit"`
	DurationValue       int     `json:"duration_value"`
	Source              string  `json:"source"`
	Status              string  `json:"status"`
	StartTime           int64   `json:"start_time"`
	EndTime             int64   `json:"end_time"`
	CreatedAt           int64   `json:"created_at"`
	AmountTotal         int64   `json:"amount_total"`
	AmountUsed          int64   `json:"amount_used"`
	RpmOverride         int     `json:"rpm_override"`
	ConcurrencyOverride int     `json:"concurrency_override"`
	UpgradeGroup        string  `json:"upgrade_group"`
	PrevUserGroup       string  `json:"prev_user_group"`
	DowngradeGroup      string  `json:"downgrade_group"`
}

// SubscriptionLogFilter 订阅日志筛选条件（管理员）。
type SubscriptionLogFilter struct {
	Username  string
	Status    string
	Source    string
	StartTime int64
	EndTime   int64
}

// SubscriptionLogResult 分页结果。
type SubscriptionLogResult struct {
	Items []SubscriptionLogItem `json:"items"`
	Total int64                 `json:"total"`
}

// GetAllSubscriptionLogs 返回管理员视角的全量订阅日志（联表用户名与套餐）。
// 跨 SQLite/MySQL/PostgreSQL 使用 GORM Joins，避免方言差异。
func GetAllSubscriptionLogs(page, pageSize int, filter SubscriptionLogFilter) (*SubscriptionLogResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	query := DB.Table("user_subscriptions AS s").
		Select(`s.id, s.user_id, u.username, s.plan_id, p.title AS plan_title,
			p.price_amount, p.currency, p.duration_unit, p.duration_value,
			s.source, s.status, s.start_time, s.end_time, s.created_at,
			s.amount_total, s.amount_used, s.rpm_override, s.concurrency_override,
			s.upgrade_group, s.prev_user_group, s.downgrade_group`).
		Joins("LEFT JOIN users AS u ON u.id = s.user_id").
		Joins("LEFT JOIN subscription_plans AS p ON p.id = s.plan_id")

	query = applySubscriptionLogFilters(query, filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	items := make([]SubscriptionLogItem, 0)
	if err := query.Order("s.id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Scan(&items).Error; err != nil {
		return nil, err
	}

	return &SubscriptionLogResult{Items: items, Total: total}, nil
}

func applySubscriptionLogFilters(query *gorm.DB, filter SubscriptionLogFilter) *gorm.DB {
	if username := strings.TrimSpace(filter.Username); username != "" {
		query = query.Where("u.username LIKE ?", "%"+username+"%")
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("s.status = ?", status)
	}
	if source := strings.TrimSpace(filter.Source); source != "" {
		query = query.Where("s.source = ?", source)
	}
	if filter.StartTime > 0 {
		query = query.Where("s.created_at >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		query = query.Where("s.created_at <= ?", filter.EndTime)
	}
	return query
}

// SubscriptionLogStartOfDay 返回当日 00:00 的 Unix 时间戳（管理员筛选默认起点）。
func SubscriptionLogStartOfDay() int64 {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start.Unix()
}
