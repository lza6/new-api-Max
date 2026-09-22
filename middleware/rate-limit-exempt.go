package middleware

import (
	"bytes"
	"io"
	"slices"
	"strings"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/setting/relay_setting"

	"github.com/gin-gonic/gin"
)

// requestModelNamePeek 提取当前中继请求的 model 字段（仅当豁免清单非空时被调用）。
// 读取请求体后完整还原 c.Request.Body，不影响后续 relay 解析；结果缓存进
// context，链上多次判断只 peek 一次。超过 1MB 探测窗口时用 MultiReader 保留
// 剩余流，绝不丢体。
func requestModelNamePeek(c *gin.Context) string {
	if v := common.GetContextKeyString(c, constant.ContextKeyRequestModelPeek); v != "" {
		return v
	}
	const probeN = 1 << 20 // 1MB 探测窗口
	buf := make([]byte, probeN+1)
	n, _ := io.ReadFull(c.Request.Body, buf)
	if n > probeN {
		// 请求体超过探测窗口：还原完整流（已读前缀 + 剩余），避免破坏大请求体。
		c.Request.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf), c.Request.Body))
		buf = buf[:probeN]
	} else {
		c.Request.Body = io.NopCloser(bytes.NewReader(buf[:n]))
		buf = buf[:n]
	}
	var probe struct {
		Model string `json:"model"`
	}
	model := ""
	if common.Unmarshal(buf, &probe) == nil {
		model = strings.TrimSpace(probe.Model)
	}
	common.SetContextKey(c, constant.ContextKeyRequestModelPeek, model)
	return model
}

// isRateLimitExemptModel 判断当前请求的模型是否命中「限流豁免模型」配置
// （如免费翻译模型 google-translate = 不限并发/不限速率，对所有用户生效）。
// 豁免清单为空时零开销直接返回 false，热路径无额外读取。
func isRateLimitExemptModel(c *gin.Context) bool {
	exempt := relay_setting.GetUserRateLimitExemptModels()
	if len(exempt) == 0 {
		return false
	}
	model := requestModelNamePeek(c)
	if model == "" {
		return false
	}
	return slices.Contains(exempt, model)
}
