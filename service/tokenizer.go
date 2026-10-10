package service

import (
	"sync"

	"github.com/lza6/new-api-Max/common"
	"github.com/tiktoken-go/tokenizer"
	"github.com/tiktoken-go/tokenizer/codec"
)

// tokenEncoderMap won't grow after initialization
var defaultTokenEncoder tokenizer.Codec

// tokenEncoderMap is used to store token encoders for different models
//
// [修复] G10 §12.2.1：这张表的键是**请求里的模型名**（外部可控：任何含 "gpt" 的名字
// 都会被缓存，见 IsOpenAITextModel 的子串匹配）。原注释写「won't grow after
// initialization」并不成立 —— 它是一个无上界的字符串键 map。
// 现在加硬上界：达到上界后**不再新增条目**（编码器本身仍能正常工作，因为
// `tokenizer.ForModel` 返回的是共享词表的 codec，不缓存只是多一次查找）。
var tokenEncoderMap = make(map[string]tokenizer.Codec)

// tokenEncoderMaxEntries 是编码器缓存条目上界。取 512：本项目支持的模型族远少于此，
// 正常永远不会触顶；触顶说明有人在用随机模型名刷这张表。
const tokenEncoderMaxEntries = 512

// tokenEncoderMutex protects tokenEncoderMap for concurrent access
var tokenEncoderMutex sync.RWMutex

func InitTokenEncoders() {
	common.SysLog("initializing token encoders")
	defaultTokenEncoder = codec.NewCl100kBase()
	common.SysLog("token encoders initialized")
}

func getTokenEncoder(model string) tokenizer.Codec {
	// First, try to get the encoder from cache with read lock
	tokenEncoderMutex.RLock()
	if encoder, exists := tokenEncoderMap[model]; exists {
		tokenEncoderMutex.RUnlock()
		return encoder
	}
	tokenEncoderMutex.RUnlock()

	// If not in cache, create new encoder with write lock
	tokenEncoderMutex.Lock()
	defer tokenEncoderMutex.Unlock()

	// Double-check if another goroutine already created the encoder
	if encoder, exists := tokenEncoderMap[model]; exists {
		return encoder
	}

	// Create new encoder
	modelCodec, err := tokenizer.ForModel(tokenizer.Model(model))
	if err != nil {
		// Cache the default encoder for this model to avoid repeated failures
		if len(tokenEncoderMap) < tokenEncoderMaxEntries {
			tokenEncoderMap[model] = defaultTokenEncoder
		}
		return defaultTokenEncoder
	}

	// Cache the new encoder（达上界后不缓存：正确性不受影响，只少一次复用）
	if len(tokenEncoderMap) < tokenEncoderMaxEntries {
		tokenEncoderMap[model] = modelCodec
	}
	return modelCodec
}

func getTokenNum(tokenEncoder tokenizer.Codec, text string) int {
	if text == "" {
		return 0
	}
	tkm, _ := tokenEncoder.Count(text)
	return tkm
}
