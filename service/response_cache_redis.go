package service

import (
	"encoding/base64"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// 响应缓存的 Redis 后端（Batch-9 / G3）。
//
// 为什么把 body 用 base64 再塞进 JSON：上游响应体是**任意字节**，直接当 JSON 字符串
// 会引入转义与非法 UTF-8 风险。base64 让整条条目成为一个安全的 JSON 文档，
// 且 `common.RedisSet` 自身有 `MaxRedisValueBytes` 上限兜底 —— 超限会返回错误，
// 调用方按「缓存写入失败」处理（降级为不缓存），不会撑爆 Redis。

const responseCacheRedisKeyPrefix = "newapi:rc:"

// responseCacheRedisPayload 是落到 Redis 的条目结构。
type responseCacheRedisPayload struct {
	Body        string `json:"body"` // base64
	TotalTokens int64  `json:"total_tokens"`
	StoredAtMs  int64  `json:"stored_at_ms"`
}

func redisResponseCacheGet(key string) (*responseCacheEntry, bool) {
	raw, err := common.RedisGet(responseCacheRedisKeyPrefix + key)
	if err != nil || raw == "" {
		return nil, false
	}
	var payload responseCacheRedisPayload
	if err := common.UnmarshalJsonStr(raw, &payload); err != nil {
		return nil, false
	}
	body, err := base64.StdEncoding.DecodeString(payload.Body)
	if err != nil {
		return nil, false
	}
	return &responseCacheEntry{
		key:         key,
		body:        body,
		totalTokens: payload.TotalTokens,
		storedAt:    time.UnixMilli(payload.StoredAtMs),
	}, true
}

func redisResponseCacheSet(key string, entry *responseCacheEntry, ttl time.Duration) error {
	encoded, err := common.Marshal(responseCacheRedisPayload{
		Body:        base64.StdEncoding.EncodeToString(entry.body),
		TotalTokens: entry.totalTokens,
		StoredAtMs:  entry.storedAt.UnixMilli(),
	})
	if err != nil {
		return err
	}
	return common.RedisSet(responseCacheRedisKeyPrefix+key, string(encoded), ttl)
}
