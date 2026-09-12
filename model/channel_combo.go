package model

import (
	"errors"
	"time"

	"github.com/lza6/new-api-Max/common"
	"gorm.io/gorm"
)

// ChannelCombo 模型组合（9router combo 思想的 Go 落地）。
// 一个组合名对应多个「渠道#模型」候选（可能跨提供商），按策略依次尝试：
//   - fallback    ：按候选顺序 + 失败自动切下一个
//   - round-robin ：轮询候选 + 粘性（sticky 次请求内保持同一候选）
//   - weighted    ：按权重随机选候选；失败后切同组下一个
//
// 请求 model 命中组合名时，渠道选择先从本组合取候选渠道（跨渠道/提供商），
// 组合内全部失败后才落到普通渠道选择。
type ChannelCombo struct {
	Id        int            `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	Name      string         `json:"name" gorm:"type:varchar(64);uniqueIndex"`
	Strategy  string         `json:"strategy" gorm:"type:varchar(20);default:'fallback'"` // fallback|round-robin|weighted
	Models    string         `json:"models" gorm:"type:text"`                             // json 数组 [{channel_id, model, weight}]
	Status    int            `json:"status" gorm:"default:1"`                             // 1=启用 2=禁用
	Sticky    int            `json:"sticky" gorm:"default:1"`                             // round-robin 粘性：同一候选连续使用次数
	Created   int64          `json:"created" gorm:"bigint"`
	Updated   int64          `json:"updated" gorm:"bigint"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// ComboModelItem 组合候选（models json 元素）。
type ComboModelItem struct {
	ChannelID int    `json:"channel_id"`
	Model     string `json:"model"`
	Weight    int    `json:"weight,omitempty"` // weighted 策略权重，默认 1
}

func (ChannelCombo) TableName() string { return "channel_combos" }

// GetEnabledComboByName 按名称查启用组合。
func GetEnabledComboByName(name string) (*ChannelCombo, error) {
	if name == "" {
		return nil, nil
	}
	var combo ChannelCombo
	if err := DB.Where("name = ? AND status = 1", name).First(&combo).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &combo, nil
}

// ListEnabledCombos 列出全部启用组合（按创建序）。
func ListEnabledCombos() ([]*ChannelCombo, error) {
	var combos []*ChannelCombo
	if err := DB.Where("status = 1").Order("id asc").Find(&combos).Error; err != nil {
		return nil, err
	}
	return combos, nil
}

// ParseComboModels 解析 models json → 候选列表。
func (c *ChannelCombo) ParseComboModels() ([]ComboModelItem, error) {
	var items []ComboModelItem
	if c == nil || c.Models == "" {
		return nil, nil
	}
	if err := common.Unmarshal([]byte(c.Models), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// CreateCombo 新建。Name 唯一、Strategy 合法、Models 非空。
func CreateCombo(combo *ChannelCombo) error {
	if combo.Name == "" || len(combo.Name) > 64 {
		return errors.New("combo name must be 1-64 chars")
	}
	switch combo.Strategy {
	case "fallback", "round-robin", "weighted":
	default:
		return errors.New("combo strategy must be fallback/round-robin/weighted")
	}
	items, err := combo.ParseComboModels()
	if err != nil || len(items) == 0 {
		return errors.New("combo models must be a non-empty channel/model list")
	}
	now := time.Now().Unix()
	combo.Created = now
	combo.Updated = now
	return DB.Create(combo).Error
}

// UpdateCombo 更新组合（全字段）。Status/Sticky 走 SelectUpdate 更新。
func UpdateCombo(combo *ChannelCombo) error {
	if combo == nil || combo.Id == 0 {
		return errors.New("invalid combo id")
	}
	existing, err := GetComboById(combo.Id)
	if err != nil {
		return err
	}
	if existing == nil {
		return errors.New("combo not found")
	}
	if combo.Name == "" {
		combo.Name = existing.Name
	}
	switch combo.Strategy {
	case "fallback", "round-robin", "weighted":
	default:
		combo.Strategy = existing.Strategy
	}
	if combo.Models == "" {
		combo.Models = existing.Models
	}
	items, err := combo.ParseComboModels()
	if err != nil || len(items) == 0 {
		return errors.New("combo models must be a non-empty channel/model list")
	}
	combo.Updated = time.Now().Unix()
	return DB.Model(combo).
		Select("name", "strategy", "models", "status", "sticky", "updated").
		Updates(combo).Error
}

// GetComboById 按主键查询。
func GetComboById(id int) (*ChannelCombo, error) {
	var combo ChannelCombo
	if err := DB.First(&combo, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &combo, nil
}

// ListCombos 分页列出组合（含被禁用）。
func ListCombos(start, num int) ([]*ChannelCombo, int64, error) {
	var total int64
	if err := DB.Model(&ChannelCombo{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var combos []*ChannelCombo
	if err := DB.Order("id asc").Limit(num).Offset(start).Find(&combos).Error; err != nil {
		return nil, 0, err
	}
	return combos, total, nil
}

// DeleteCombo 软删组合。
func DeleteCombo(id int) error {
	if id == 0 {
		return errors.New("invalid combo id")
	}
	return DB.Delete(&ChannelCombo{}, "id = ?", id).Error
}
