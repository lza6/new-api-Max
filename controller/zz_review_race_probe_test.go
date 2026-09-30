package controller

import (
	"fmt"
	"sync"
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/require"
)

// 临时审查探针（审查结束后删除）：模拟并发读日志路径
// （service/log_info_generate.go:86 读 IsMultiKey；middleware/distributor.go:693
// 读 IsMultiKey 后写 ContextKey）与管理端 ManageMultiKeys 的写路径并发。
func TestReviewProbeMultiKeyInfoJSONRace(t *testing.T) {
	prev := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = prev })

	db := setupKeyStressDB(t)
	channel := seedKeyStressChannel(t, db, 9950, "https://example.invalid",
		[]string{"seed-key", "seed-key-2"})
	model.InitChannelCache()

	cached, err := model.CacheGetChannel(channel.Id)
	require.NoError(t, err)

	const readers, writers, iterations = 8, 4, 60
	var waitGroup sync.WaitGroup
	for range readers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for range iterations {
				// 模拟 log_info_generate / distributor 的读
				_ = cached.ChannelInfo.IsMultiKey
				if _, err := common.Marshal(cached.ChannelInfo); err != nil {
					t.Errorf("marshal: %v", err)
					return
				}
			}
		}()
	}
	for w := range writers {
		waitGroup.Add(1)
		go func(worker int) {
			defer waitGroup.Done()
			for i := range iterations {
				body := fmt.Sprintf(`{"channel_id":%d,"action":"disable_key","key_index":%d}`,
					channel.Id, (worker+i)%2)
				_ = postManageMultiKeys(body)
			}
		}(w)
	}
	waitGroup.Wait()
}

// 临时审查探针：确认轮询索引在并发下确实推进（不是永远同一个 key）。
func TestReviewProbePollingAdvances(t *testing.T) {
	prev := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = prev })

	db := setupKeyStressDB(t)
	keys := []string{"p0", "p1", "p2"}
	channel := seedKeyStressChannel(t, db, 9951, "https://example.invalid", keys)
	model.InitChannelCache()

	cached, err := model.CacheGetChannel(channel.Id)
	require.NoError(t, err)
	require.Equal(t, constant.MultiKeyModePolling, cached.ChannelInfo.MultiKeyMode)

	seen := map[string]int{}
	for range 9 {
		key, _, apiErr := cached.GetNextEnabledKey()
		require.Nil(t, apiErr)
		seen[key]++
	}
	t.Logf("9 次轮询分布：%v", seen)
	require.Len(t, seen, 3, "轮询必须覆盖全部 3 个 key")

	// 缓存权威对象的索引也必须被推进
	require.NotEqual(t, 0, cached.ChannelInfo.MultiKeyPollingIndex,
		"轮询索引必须写回缓存权威对象")
}
