package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveRequestTracker_LifecycleAndAggregation(t *testing.T) {
	ResetLiveRequestsForTest()
	defer ResetLiveRequestsForTest()

	// 两个进行中请求，其中一个已压缩并有首字。
	LiveBegin("req-1", LiveRequestEntry{UserId: 1, Model: "m", IsStream: true, StartedAt: 100})
	LiveSetChannel("req-1", 49, "ch49", 0)
	LiveSetCompression("req-1", 2_000_000, 20_000, 300, true)
	LiveSetFirstResponse("req-1", 500)

	LiveBegin("req-2", LiveRequestEntry{UserId: 2, Model: "m", IsStream: false, StartedAt: 100})
	LiveSetChannel("req-2", 50, "ch50", 0)
	LiveSetCompression("req-2", 300_000, 300_000, -1, false)

	snap := GetLiveRequestsSnapshot()
	require.Equal(t, 2, snap.ActiveCount)
	assert.Equal(t, 1, snap.CompressedCount, "只有 req-1 被压缩")
	assert.EqualValues(t, 2_000_000, snap.OriginalBytesSum)
	assert.EqualValues(t, 20_000, snap.CompressedBytesSum)
	assert.InDelta(t, 0.01, snap.AvgCompressionRatio, 0.001)
	assert.EqualValues(t, 500, snap.AvgFirstResponseMs)

	// 结束 req-1：移出 active，进入 finished。
	LiveEnd("req-1", 200, "")
	snap = GetLiveRequestsSnapshot()
	assert.Equal(t, 1, snap.ActiveCount)
	require.Len(t, snap.Finished, 1)
	assert.Equal(t, LivePhaseDone, snap.Finished[0].Phase)
	assert.Equal(t, "req-1", snap.Finished[0].RequestId)

	// 失败请求进入 error 阶段。
	LiveBegin("req-3", LiveRequestEntry{UserId: 3, Model: "m"})
	LiveEnd("req-3", 500, "upstream failed")
	snap = GetLiveRequestsSnapshot()
	var found bool
	for _, f := range snap.Finished {
		if f.RequestId == "req-3" {
			found = true
			assert.Equal(t, LivePhaseError, f.Phase)
			assert.Equal(t, "upstream failed", f.ErrorMsg)
		}
	}
	assert.True(t, found, "req-3 应在最近完成中")
}

func TestLiveRequestTracker_UnknownIdIsNoop(t *testing.T) {
	ResetLiveRequestsForTest()
	defer ResetLiveRequestsForTest()

	// 对不存在的请求做更新/结束不应 panic。
	LiveSetChannel("nope", 1, "x", 0)
	LiveSetCompression("nope", 1, 2, -1, true)
	LiveSetFirstResponse("nope", 1)
	LiveEnd("nope", 200, "")
	assert.Equal(t, 0, GetLiveRequestsSnapshot().ActiveCount)
}

func TestLiveRequestTracker_UncompressedNotCountedInRatio(t *testing.T) {
	ResetLiveRequestsForTest()
	defer ResetLiveRequestsForTest()

	LiveBegin("r", LiveRequestEntry{Model: "m"})
	LiveSetCompression("r", 500_000, 500_000, -1, false) // 未压缩
	snap := GetLiveRequestsSnapshot()
	assert.Equal(t, 0, snap.CompressedCount)
	assert.EqualValues(t, 0, snap.AvgCompressionRatio, "未压缩请求不参与压缩率统计")
}

func TestLiveRequestTracker_UpstreamTimingAggregation(t *testing.T) {
	ResetLiveRequestsForTest()
	defer ResetLiveRequestsForTest()

	LiveBegin("r1", LiveRequestEntry{Model: "m"})
	LiveSetUpstreamTiming("r1", 120, 40, 800)
	LiveBegin("r2", LiveRequestEntry{Model: "m"})
	LiveSetUpstreamTiming("r2", 100, 60, 1200)

	snap := GetLiveRequestsSnapshot()
	// 平均上传 (40+60)/2 = 50；平均上游首字节 (800+1200)/2 = 1000
	assert.EqualValues(t, 50, snap.AvgUploadMs)
	assert.EqualValues(t, 1000, snap.AvgUpstreamTtfbMs)

	// 未采集计时的请求（-1）不计入平均。
	LiveBegin("r3", LiveRequestEntry{Model: "m"})
	snap = GetLiveRequestsSnapshot()
	assert.EqualValues(t, 50, snap.AvgUploadMs, "r3 无计时不应拉低平均")
}

// 无任何请求时，聚合上传/上游计时应为 -1（未采集），而非 0——
// 否则前端会误显示「upload 0ms · upstream 0ms」。
func TestLiveRequestTracker_EmptySnapshotUsesMinusOne(t *testing.T) {
	ResetLiveRequestsForTest()
	defer ResetLiveRequestsForTest()

	snap := GetLiveRequestsSnapshot()
	assert.EqualValues(t, -1, snap.AvgUploadMs)
	assert.EqualValues(t, -1, snap.AvgUpstreamTtfbMs)
	assert.EqualValues(t, 0, snap.AvgCompressionRatio)
}
