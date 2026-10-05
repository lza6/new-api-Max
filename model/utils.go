package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const (
	BatchUpdateTypeUserQuota = iota
	BatchUpdateTypeTokenQuota
	BatchUpdateTypeUsedQuota
	BatchUpdateTypeChannelUsedQuota
	BatchUpdateTypeRequestCount
	BatchUpdateTypeCount // if you add a new type, you need to add a new map and a new lock
)

var batchUpdateStores []map[int]int
var batchUpdateLocks []sync.Mutex

// batchUpdaterCancel/batchUpdaterWG 管理周期落库 worker 的生命周期，供 StopBatchUpdater 排空后退出。
var (
	batchUpdaterMu     sync.Mutex
	batchUpdaterCancel context.CancelFunc
	batchUpdaterWG     sync.WaitGroup
)

func init() {
	for range BatchUpdateTypeCount {
		batchUpdateStores = append(batchUpdateStores, make(map[int]int))
		batchUpdateLocks = append(batchUpdateLocks, sync.Mutex{})
	}
}

func InitBatchUpdater() {
	ctx, cancel := context.WithCancel(context.Background())
	batchUpdaterMu.Lock()
	// 每个进程只应启动一个周期落库 worker；重复调用时保留先启动者。
	if batchUpdaterCancel != nil {
		batchUpdaterMu.Unlock()
		cancel()
		return
	}
	batchUpdaterCancel = cancel
	batchUpdaterMu.Unlock()

	batchUpdaterWG.Add(1)
	gopool.Go(func() {
		defer batchUpdaterWG.Done()
		ticker := time.NewTicker(time.Duration(common.BatchUpdateInterval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				batchUpdate()
			}
		}
	})
}

func addNewRecord(type_ int, id int, value int) {
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	old, ok := batchUpdateStores[type_][id]
	if !ok {
		batchUpdateStores[type_][id] = value
		return
	}

	sum := old + value
	if (value > 0 && sum < old) || (value < 0 && sum > old) {
		common.SysError(fmt.Sprintf("batch update overflow: type=%d id=%d old=%d value=%d", type_, id, old, value))
		if value > 0 {
			sum = math.MaxInt
		} else {
			sum = math.MinInt
		}
	}
	batchUpdateStores[type_][id] = sum
}

func batchUpdate() {
	// check if there's any data to update
	hasData := false
	for i := range BatchUpdateTypeCount {
		batchUpdateLocks[i].Lock()
		if len(batchUpdateStores[i]) > 0 {
			hasData = true
			batchUpdateLocks[i].Unlock()
			break
		}
		batchUpdateLocks[i].Unlock()
	}
	if !hasData {
		return
	}

	common.SysLog("batch update started")
	flushBatchUpdateStores()
	common.SysLog("batch update finished")
}

// flushBatchUpdateStores 加锁 swap 出各类型的增量并落库。供周期 worker 与
// 停机排空（FlushBatchUpdate）共用，保证两者落库语义一致。
func flushBatchUpdateStores() {
	stores := make([]map[int]int, BatchUpdateTypeCount)
	for i := range BatchUpdateTypeCount {
		batchUpdateLocks[i].Lock()
		stores[i] = batchUpdateStores[i]
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
	}

	for i, store := range stores {
		if i == BatchUpdateTypeUserQuota || i == BatchUpdateTypeUsedQuota || i == BatchUpdateTypeRequestCount {
			continue
		}
		for key, value := range store {
			switch i {
			case BatchUpdateTypeTokenQuota:
				err := increaseTokenQuota(key, value)
				if err != nil {
					common.SysLog("failed to batch update token quota: " + err.Error())
				}
			case BatchUpdateTypeChannelUsedQuota:
				updateChannelUsedQuota(key, value)
			}
		}
	}

	userQuotaStore := stores[BatchUpdateTypeUserQuota]
	usedQuotaStore := stores[BatchUpdateTypeUsedQuota]
	requestCountStore := stores[BatchUpdateTypeRequestCount]

	userIDs := make(map[int]struct{}, len(userQuotaStore)+len(usedQuotaStore)+len(requestCountStore))
	for key := range userQuotaStore {
		userIDs[key] = struct{}{}
	}
	for key := range usedQuotaStore {
		userIDs[key] = struct{}{}
	}
	for key := range requestCountStore {
		userIDs[key] = struct{}{}
	}
	for key := range userIDs {
		updateUserQuotaUsedQuotaAndRequestCount(key, userQuotaStore[key], usedQuotaStore[key], requestCountStore[key])
	}
}

// StopBatchUpdater 停止周期落库 worker 并排空存量增量（供优雅退出/测试调用）。
// 与 InitBatchUpdater 配对（mutex 保护）；未启动过 worker 时只做排空（安全 no-op）。
// 排空后仍可再次 InitBatchUpdater。
func StopBatchUpdater() {
	batchUpdaterMu.Lock()
	cancel := batchUpdaterCancel
	batchUpdaterMu.Unlock()
	if cancel != nil {
		cancel()
		batchUpdaterWG.Wait()
		batchUpdaterMu.Lock()
		batchUpdaterCancel = nil
		batchUpdaterMu.Unlock()
	}
	FlushBatchUpdate()
}

// FlushBatchUpdate 同步排空 batchUpdateStores 中已入队但未落库的额度增量。
// 停机路径调用，避免优雅退出丢失最近一个 BATCH_UPDATE_INTERVAL 内的增量。
func FlushBatchUpdate() {
	flushBatchUpdateStores()
}

func RecordExist(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func shouldUpdateRedis(fromDB bool, err error) bool {
	return common.RedisEnabled && fromDB && err == nil
}
