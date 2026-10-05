package service

import (
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"

	"github.com/bytedance/gopkg/util/gopool"
)

// §4.7.1 轻量用户记忆层（规则 + KV；**不引入向量库**，避免 pgvector 与三库兼容负担）。
//
// 首个能力：记录用户「最近一次成功调用的模型」，供前端回填默认模型（减少重复选择）。
// 存储复用既有 `UserSetting`（JSON text 列）的 `last_used_model` 字段，**无需迁移**。
//
// 防写放大：同一 (userId, model) 在 TTL 内只写一次；写操作异步（gopool），不阻塞请求收尾。
const (
	userLastModelDedupTTL  = 30 * time.Second
	userLastModelMemoryCap = 10000 // 进程内去重表上限，防无界增长
)

var (
	userLastModelMu   sync.Mutex
	userLastModelSeen = map[int]userLastModelEntry{}
)

type userLastModelEntry struct {
	model string
	at    time.Time
}

// RecordUserLastModel 记录用户最近使用的模型（异步、去重、有界）。空模型名忽略。
// 失败仅记日志，绝不影响请求主链路。
func RecordUserLastModel(userId int, modelName string) {
	if userId <= 0 || modelName == "" {
		return
	}
	if !markUserLastModelSeen(userId, modelName) {
		return
	}
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				common.SysLog("RecordUserLastModel panic recovered")
			}
		}()
		user, err := model.GetUserById(userId, false)
		if err != nil || user == nil {
			return
		}
		setting := user.GetSetting()
		if setting.LastUsedModel == modelName {
			return
		}
		setting.LastUsedModel = modelName
		if err := model.UpdateUserSetting(userId, setting); err != nil {
			common.SysLog("failed to record last used model: " + err.Error())
		}
	})
}

// markUserLastModelSeen 去重：同一用户同一模型在 TTL 内只写一次；返回是否应写入。
func markUserLastModelSeen(userId int, modelName string) bool {
	userLastModelMu.Lock()
	defer userLastModelMu.Unlock()
	now := time.Now()
	if e, ok := userLastModelSeen[userId]; ok && e.model == modelName && now.Sub(e.at) < userLastModelDedupTTL {
		return false
	}
	if len(userLastModelSeen) >= userLastModelMemoryCap {
		// 超上限：清空（近似 LRU 的粗粒度回收，避免无界增长）。
		userLastModelSeen = map[int]userLastModelEntry{}
	}
	userLastModelSeen[userId] = userLastModelEntry{model: modelName, at: now}
	return true
}

// ResetUserLastModelMemoryForTest 清空去重表（测试用）。
func ResetUserLastModelMemoryForTest() {
	userLastModelMu.Lock()
	userLastModelSeen = map[int]userLastModelEntry{}
	userLastModelMu.Unlock()
}
