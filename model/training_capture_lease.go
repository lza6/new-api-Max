package model

import (
	"fmt"
	"strconv"
	"time"
)

// 训练数据采集的多实例租约。
//
// 采集 worker 是**进程内单 goroutine**；多实例部署时，同一用户的并发信号
// 可能被两个实例同时处理 → 各自读水位、上传、写水位，后写覆盖前写，
// 造成重复或丢失增量。这里复用 system_task_locks 表做按用户的互斥租约：
// 同一时刻只有一个实例能处理某个用户的上传，租约到期自动可被抢占。
//
// 采集是尽力而为：拿不到租约就跳过本条信号（计入 skipped_lease），
// 不阻塞 worker、不影响主业务。
const trainingCaptureLeaseTTL = 5 * time.Minute

func trainingCaptureLeaseType(userId int) string {
	return fmt.Sprintf("training_capture_user:%d", userId)
}

// AcquireTrainingCaptureLease 尝试获取某用户的采集租约。
func AcquireTrainingCaptureLease(userId int, owner string, ttl time.Duration) (bool, error) {
	if userId <= 0 || owner == "" {
		return false, nil
	}
	if ttl <= 0 {
		ttl = trainingCaptureLeaseTTL
	}
	now := GetDBTimestamp()
	acquired, _, err := acquireSystemTaskLock(
		trainingCaptureLeaseType(userId),
		strconv.Itoa(userId),
		owner,
		now,
		now+int64(ttl.Seconds()),
	)
	return acquired, err
}

// ReleaseTrainingCaptureLease 释放自己持有的采集租约（幂等）。
func ReleaseTrainingCaptureLease(userId int, owner string) error {
	if userId <= 0 || owner == "" {
		return nil
	}
	return DB.Where("type = ? AND locked_by = ?", trainingCaptureLeaseType(userId), owner).
		Delete(&SystemTaskLock{}).Error
}
