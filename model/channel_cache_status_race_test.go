package model

import (
	"sync"
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/stretchr/testify/require"
)

// TestCacheGetChannelStatusRaceWithUpdate 复现并守护 be-audit P1：内存缓存模式
// 下 CacheGetChannel 返回共享 *Channel 指针（读锁在返回前释放），调用方在锁外读
// 其 .Status；而 CacheUpdateChannelStatus 在写锁内就地写同一字段 → data race。
//
// CacheGetChannelStatus 在读锁内返回值副本，本用例持续并发「读状态」×「改状态」，
// -race 下必须 0 race。反向验证：把读改回 CacheGetChannel(...).Status 会报
// WARNING: DATA RACE（见提交说明）。
func TestCacheGetChannelStatusRaceWithUpdate(t *testing.T) {
	setupChannelRuntimeTest(t)
	ch := &Channel{
		Name:   "status-race",
		Key:    "status-race-key",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-4o",
	}
	ch.SetSetting(dto.ChannelSettings{})
	ch.SetOtherSettings(dto.ChannelOtherSettings{})
	require.NoError(t, DB.Create(ch).Error)
	require.NoError(t, ch.AddAbilities(nil))
	InitChannelCache()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 读侧：并发读状态（锁保护）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_, _ = CacheGetChannelStatus(ch.Id)
		}
	}()

	// 写侧：并发就地改状态（写锁内）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 400 {
			status := common.ChannelStatusEnabled
			if i%2 == 0 {
				status = common.ChannelStatusAutoDisabled
			}
			CacheUpdateChannelStatus(ch.Id, status)
		}
		close(stop)
	}()

	wg.Wait()
}
