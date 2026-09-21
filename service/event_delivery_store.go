package service

import (
	"context"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"gorm.io/gorm/clause"
)

// gormEventDeliveryStore 基于主库 event_deliveries 表的持久化投递记录。
//
// 幂等插入用 GORM clause.OnConflict{DoNothing:true}（SQLite/MySQL/PostgreSQL
// 三库通用，参考 model/auth_flow.go / model/ability.go 既有模式），以
// RowsAffected 判断是否新建：0 行 = (event_id, handler) 已存在。
// 每次调用读取 model.DB，保证测试替换 DB 后依然指向测试库。
type gormEventDeliveryStore struct{}

// NewEventDeliveryStore 创建绑定到主库的投递记录存储。
func NewEventDeliveryStore() EventDeliveryStore {
	return &gormEventDeliveryStore{}
}

func (s *gormEventDeliveryStore) CreateDelivery(ctx context.Context, rec EventDeliveryRecord) (bool, error) {
	row := &model.EventDelivery{
		EventID:   rec.EventID,
		Handler:   rec.Handler,
		EventType: rec.EventType,
		State:     string(rec.State),
		Attempts:  rec.Attempts,
		LastError: rec.LastError,
		CreatedAt: common.GetTimestamp(),
		UpdatedAt: common.GetTimestamp(),
	}
	result := model.DB.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(row)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (s *gormEventDeliveryStore) UpdateDeliveryState(ctx context.Context, eventID, handler string, state EventState, attempts int, lastError string) error {
	return model.DB.WithContext(ctx).
		Model(&model.EventDelivery{}).
		Where("event_id = ? AND handler = ?", eventID, handler).
		Updates(map[string]any{
			"state":      string(state),
			"attempts":   attempts,
			"last_error": lastError,
			"updated_at": common.GetTimestamp(),
		}).Error
}
