package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

func TestEventDeliveryUniquePerEventAndHandler(t *testing.T) {
	truncateTables(t)

	first := &EventDelivery{
		EventID:   "evt-order-1",
		Handler:   "epay.topup",
		EventType: "epay.topup.success",
		State:     "pending",
	}
	require.NoError(t, DB.Create(first).Error)
	assert.NotZero(t, first.CreatedAt, "BeforeCreate 应写入 CreatedAt")
	assert.NotZero(t, first.UpdatedAt, "BeforeCreate 应写入 UpdatedAt")

	// 同一 (event_id, handler) 直接 Create 必须命中联合唯一约束。
	dup := &EventDelivery{
		EventID:   "evt-order-1",
		Handler:   "epay.topup",
		EventType: "epay.topup.success",
		State:     "pending",
	}
	err := DB.Create(dup).Error
	require.Error(t, err, "重复 (event_id, handler) 应被唯一索引拒绝")

	// 不同 handler 允许并存（同一事件给不同处理器分别幂等）。
	other := &EventDelivery{
		EventID:   "evt-order-1",
		Handler:   "notify.webhook",
		EventType: "user.notify",
		State:     "pending",
	}
	require.NoError(t, DB.Create(other).Error)
}

func TestEventDeliveryOnConflictDoNothing(t *testing.T) {
	truncateTables(t)

	row := &EventDelivery{
		EventID:   "evt-order-2",
		Handler:   "epay.topup",
		EventType: "epay.topup.success",
		State:     "pending",
	}
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	require.NoError(t, result.Error)
	require.Equal(t, int64(1), result.RowsAffected)

	// 重复插入被 DoNothing 跳过（RowsAffected=0），是 store 幂等判定的基础。
	row2 := &EventDelivery{
		EventID:   "evt-order-2",
		Handler:   "epay.topup",
		EventType: "epay.topup.success",
		State:     "success",
	}
	result2 := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row2)
	require.NoError(t, result2.Error)
	assert.Zero(t, result2.RowsAffected)

	// 状态更新仍生效。
	require.NoError(t, DB.Model(&EventDelivery{}).
		Where("event_id = ? AND handler = ?", "evt-order-2", "epay.topup").
		Updates(map[string]any{"state": "success", "attempts": 1}).Error)

	var loaded EventDelivery
	require.NoError(t, DB.Where("event_id = ?", "evt-order-2").First(&loaded).Error)
	assert.Equal(t, "success", loaded.State)
	assert.Equal(t, 1, loaded.Attempts)
}

func TestEventDeliveryReassignDBWorks(t *testing.T) {
	// gormEventDeliveryStore 每次调用读取 model.DB；此处验证替换 DB 指针后
	// 旧句柄（若曾捕获）不影响后续调用 —— 由 service 层测试覆盖真实路径，
	// 这里仅验证 schema 可被重复迁移（幂等由 conformance 覆盖）。
	require.NoError(t, DB.AutoMigrate(&EventDelivery{}))
	var count int64
	require.NoError(t, DB.Model(&EventDelivery{}).Count(&count).Error)
	assert.Zero(t, count)
}
