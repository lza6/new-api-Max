package training_capture

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// failAfterSink 在第 N 次 WritePart 之后开始失败，并记录删除调用。
//
// 用于验证「部分上传失败 → 补偿删除本批已写分片」。
type failAfterSink struct {
	mu         sync.Mutex
	files      map[string][]byte
	failAfter  int // 成功写够这么多次后开始失败
	writeCalls int
	deleted    []string
}

func newFailAfterSink(failAfter int) *failAfterSink {
	return &failAfterSink{files: map[string][]byte{}, failAfter: failAfter}
}

func (f *failAfterSink) Name() string { return "fail-after" }

func (f *failAfterSink) EnsureDir(context.Context, string) error { return nil }

func (f *failAfterSink) WritePart(_ context.Context, path string, data []byte) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.writeCalls >= f.failAfter {
		return "", fmt.Errorf("injected write failure")
	}
	f.writeCalls++
	f.files[path] = append([]byte(nil), data...)
	return path, nil
}

func (f *failAfterSink) List(context.Context, string) ([]string, error) { return nil, nil }

func (f *failAfterSink) Stat(context.Context) (SinkStat, error) {
	return SinkStat{Provider: "fail-after"}, nil
}

func (f *failAfterSink) DeleteFile(_ context.Context, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, path)
	delete(f.files, path)
	return nil
}

func (f *failAfterSink) snapshot() (files map[string][]byte, deleted []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	files = make(map[string][]byte, len(f.files))
	for k := range f.files {
		files[k] = nil
	}
	return files, append([]string(nil), f.deleted...)
}

// 回归（盲点 B4）：部分上传失败必须**补偿删除**本批已写分片。
//
// 不删除的后果：水位不推进（不丢数据，这点原本就对）但孤儿留在云盘 →
// 下轮重试把整段增量按**更大序号**再写一遍 → 数据集里既有重复内容、
// 又有永远不该存在的残片，而两者都会被分析/导出当作有效增量读回。
func TestPartialUploadFailureCleansUpOrphans(t *testing.T) {
	// processSignal 会读水位（需要 DB）—— 不初始化会 nil deref，
	// 那会让失败原因看起来像「功能坏了」，其实只是测试装置缺失。
	if err := ensureTestDB(t); err != nil {
		t.Skipf("跳过：%v", err)
	}

	// 第 1 块成功、第 2 块开始失败。
	sink := newFailAfterSink(1)
	restore := SetSinkForTest(sink)
	t.Cleanup(restore)

	// 复位预算并按 env 推导分片上限，保证能切出 ≥2 块。
	ResetBudgetForTest()
	t.Setenv(EnvCaptureMaxPartMB, "1")
	t.Cleanup(ResetBudgetForTest)

	limiter := NewByteLimiter(uploadRateLimitBytesPerSec, nil)
	ctx := context.Background()

	// 多轮 + 不可压缩内容，稳定切出多块。
	var msgs []Message
	for i := range 900 {
		var b strings.Builder
		for j := 0; b.Len() < 4096; j++ {
			fmt.Fprintf(&b, "%08x%08x%08x\n", i*131+j, i*7919+j, i*2654435761+j)
		}
		msgs = append(msgs, Message{Role: "user", Content: b.String()})
	}

	sig := signal(902, "orphan", "m", "conv-orphan", msgs, "")
	n, uploaded, err := processSignal(ctx, sig, limiter)
	if err == nil {
		t.Fatal("注入的写入失败必须向上报错")
	}
	if n != 0 || uploaded != 0 {
		t.Fatalf("失败时必须报告 0 上传（而不是「部分成功」的字节数），实际 n=%d bytes=%d",
			n, uploaded)
	}

	files, deleted := sink.snapshot()
	if len(deleted) == 0 {
		t.Fatal("部分上传失败后必须补偿删除本批已写分片，实际一次都没删")
	}
	if len(files) != 0 {
		names := make([]string, 0, len(files))
		for k := range files {
			names = append(names, k)
		}
		t.Fatalf("补偿后云盘上不应残留本批分片，实际还剩 %d 个：%v", len(files), names)
	}
	t.Logf("补偿删除 %d 个孤儿分片", len(deleted))
}
