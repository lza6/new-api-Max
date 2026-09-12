package service

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// comboRoutingState 组合路由进程内状态：轮询游标 + 粘性计数。
type comboRoutingState struct {
	mu       sync.Mutex
	cursor   int64 // 轮询下一个候选下标
	stickied int64 // 当前粘性候选 id
	stickyN  int   // 已连续使用次数
}

var (
	comboStateMu sync.RWMutex
	comboStates  = map[string]*comboRoutingState{}
)

func comboStateFor(name string) *comboRoutingState {
	comboStateMu.RLock()
	if s, ok := comboStates[name]; ok {
		comboStateMu.RUnlock()
		return s
	}
	comboStateMu.RUnlock()
	comboStateMu.Lock()
	defer comboStateMu.Unlock()
	if s, ok := comboStates[name]; ok {
		return s
	}
	s := &comboRoutingState{}
	comboStates[name] = s
	return s
}

// ComboCandidate 一次渠道选择的候选（含选中组合名供审计）。
type ComboCandidate struct {
	ChannelID int
	Model     string
	ComboName string
}

// ResolveComboForModel 按请求模型名解析启用组合（nil = 非组合）。
func ResolveComboForModel(modelName string) *model.ChannelCombo {
	if modelName == "" {
		return nil
	}
	combo, err := model.GetEnabledComboByName(modelName)
	if err != nil || combo == nil {
		return nil
	}
	return combo
}

// NextComboCandidate 便捷封装：取组合下一个候选（nil = 组合无效/无候选）。
func NextComboCandidate(combo *model.ChannelCombo) *ComboCandidate {
	if combo == nil {
		return nil
	}
	cand, err := ComboNextCandidate(combo)
	if err != nil {
		return nil
	}
	return cand
}

// RecordComboSelected 记录组合命中候选（供审计/观测）。
func RecordComboSelected(comboName string, channelID int, modelName string) {
	common.SysLog(fmt.Sprintf("combo %q -> channel #%d model %s", comboName, channelID, modelName))
}

// ComboNextCandidate 按组合策略返回下一个候选（下标/粘性逻辑）。
// 不落库、不校验渠道状态；调用方拿到候选后走现有渠道校验。
func ComboNextCandidate(combo *model.ChannelCombo) (*ComboCandidate, error) {
	if combo == nil || combo.Id == 0 {
		return nil, errors.New("nil combo")
	}
	items, err := combo.ParseComboModels()
	if err != nil || len(items) == 0 {
		return nil, errors.New("combo has no models")
	}
	state := comboStateFor(combo.Name)

	switch combo.Strategy {
	case "round-robin":
		state.mu.Lock()
		defer state.mu.Unlock()
		sticky := combo.Sticky
		if sticky <= 0 {
			sticky = 1
		}
		// 粘性：同一候选连续 sticky 次。
		if state.stickied != 0 && state.stickyN < sticky {
			state.stickyN++
			if item, ok := findItem(items, state.stickied); ok {
				return &ComboCandidate{ChannelID: item.ChannelID, Model: item.Model, ComboName: combo.Name}, nil
			}
		}
		// 轮询到下一个候选。
		idx := int(state.cursor) % len(items)
		state.cursor = int64(idx + 1)
		state.stickied = int64(items[idx].ChannelID)
		state.stickyN = 1
		return &ComboCandidate{ChannelID: items[idx].ChannelID, Model: items[idx].Model, ComboName: combo.Name}, nil

	case "weighted":
		state.mu.Lock()
		defer state.mu.Unlock()
		total := 0
		for _, item := range items {
			w := item.Weight
			if w <= 0 {
				w = 1
			}
			total += w
		}
		pick := rand.Intn(total)
		for _, item := range items {
			w := item.Weight
			if w <= 0 {
				w = 1
			}
			pick -= w
			if pick < 0 {
				return &ComboCandidate{ChannelID: item.ChannelID, Model: item.Model, ComboName: combo.Name}, nil
			}
		}
		return &ComboCandidate{ChannelID: items[0].ChannelID, Model: items[0].Model, ComboName: combo.Name}, nil

	default: // fallback
		state.mu.Lock()
		defer state.mu.Unlock()
		// 默认顺序 = models 数组顺序（首个优先）；若已推进过（失败过），
		// 从当前游标取候选。
		idx := int(state.cursor) % len(items)
		item := items[idx]
		return &ComboCandidate{ChannelID: item.ChannelID, Model: item.Model, ComboName: combo.Name}, nil
	}
}

func findItem(items []model.ComboModelItem, channelID int64) (model.ComboModelItem, bool) {
	for _, item := range items {
		if int64(item.ChannelID) == channelID {
			return item, true
		}
	}
	return model.ComboModelItem{}, false
}

// ComboFailAndAdvance 组合内候选失败后推进到下一候选（fallback 语义）。
// round-robin/weighted 不额外推进（next 调用自动轮换）。
func ComboFailAndAdvance(combo *model.ChannelCombo) {
	if combo == nil || combo.Strategy != "fallback" {
		return
	}
	state := comboStateFor(combo.Name)
	items, err := combo.ParseComboModels()
	if err != nil || len(items) == 0 {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.cursor = int64(atomic.AddInt64(&state.cursor, 1) % int64(len(items)))
}

// comboFailCooldownUntil 组合内候选失败后的冷却（避免立刻重试同一候选）。
type comboFailCooldown struct {
	mu          sync.Mutex
	channelCool map[int]time.Time
}

var comboFailCool = &comboFailCooldown{channelCool: map[int]time.Time{}}

// MarkComboCandidateFailed 记录候选渠道失败并返回建议冷却秒数。
func MarkComboCandidateFailed(channelID int) time.Duration {
	comboFailCool.mu.Lock()
	defer comboFailCool.mu.Unlock()
	d := 2 * time.Second
	comboFailCool.channelCool[channelID] = time.Now().Add(d)
	return d
}

// ComboCandidateCoolingDown 报告候选渠道是否处于组合级冷却（组合内专用）。
func ComboCandidateCoolingDown(channelID int) bool {
	comboFailCool.mu.Lock()
	defer comboFailCool.mu.Unlock()
	until, ok := comboFailCool.channelCool[channelID]
	if !ok {
		return false
	}
	if time.Now().Before(until) {
		return true
	}
	delete(comboFailCool.channelCool, channelID)
	return false
}
