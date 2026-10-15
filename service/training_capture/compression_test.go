package training_capture

import (
	"bytes"
	"strings"
	"testing"
)

// 分片压缩（B2）的测试。
//
// 核心性质：**压缩是损耗less 的**（解压 == 原文）+ **真的省带宽**
// （对对话类 JSONL 有显著压缩率）+ **防 zip-bomb**（读取侧有限制）。

func TestGzipCompressRoundTripLossless(t *testing.T) {
	// 各种真实形态：多轮中文对话、代码块、emoji、空行。
	payload := encodeJSONL([]Message{
		{Role: "user", Content: "帮我写一个 Go 的 HTTP server"},
		{Role: "assistant", Content: "```go\nsrv := &http.Server{Addr: \":8080\"}\nsrv.ListenAndServe()\n```"},
		{Role: "user", Content: "加个超时和优雅关闭 🚀"},
		{Role: "assistant", Content: strings.Repeat("srv.ReadTimeout = 5 * time.Second\n", 100)},
	})

	compressed, err := gzipCompress(payload)
	if err != nil {
		t.Fatalf("压缩失败：%v", err)
	}
	if len(compressed) == 0 {
		t.Fatal("压缩产物为空")
	}
	if !isValidGzip(compressed) {
		t.Fatal("压缩产物不是合法 gzip")
	}

	plain, err := gzipDecompress(compressed, 1<<30)
	if err != nil {
		t.Fatalf("解压失败：%v", err)
	}
	if !bytes.Equal(plain, payload) {
		t.Fatalf("解压后与原文不一致（有损！）：原文 %d 字节，解压 %d 字节",
			len(payload), len(plain))
	}
}

func TestGzipCompressActuallySavesBandwidth(t *testing.T) {
	// 模拟真实长对话：重复的结构性 token 是压缩率的来源。
	var b strings.Builder
	for range 200 {
		b.WriteString(`{"role":"user","content":"请解释第`)
		b.WriteString(strings.Repeat("x", 100))
		b.WriteString("段代码的作用\"}\n")
		b.WriteString(`{"role":"assistant","content":"这段代码实现了`)
		b.WriteString(strings.Repeat("log output ", 80))
		b.WriteString("功能\"}\n")
	}
	payload := []byte(b.String())

	compressed, err := gzipCompress(payload)
	if err != nil {
		t.Fatal(err)
	}
	ratio := float64(len(compressed)) / float64(len(payload))
	t.Logf("压缩率：%.1f%%（%d → %d 字节）", ratio*100, len(payload), len(compressed))

	// BestSpeed 对这类文本应至少压到 40% 以下。
	// 这是「真的省带宽」的量化证据 —— 如果将来有人把 BestSpeed
	// 换成 NoCompression，这里会立即失败。
	if ratio > 0.4 {
		t.Fatalf("压缩率 %.1f%% 过差 —— 采集的带宽保护目标失效", ratio*100)
	}
	// 往返仍必须无损。
	plain, err := gzipDecompress(compressed, 1<<30)
	if err != nil || !bytes.Equal(plain, payload) {
		t.Fatalf("高压缩率下有损：err=%v equal=%v", err, bytes.Equal(plain, payload))
	}
}

func TestGzipDecompressRejectsZipBomb(t *testing.T) {
	// 高度重复的内容解压比极高：1KB 压缩 → ~10MB 原文。
	bomb := gzipMust(t, strings.Repeat("A", 10<<20))

	// 限制 1MB → 必须拒绝而不是读满内存。
	if _, err := gzipDecompress(bomb, 1<<20); err != ErrDecompressTooLarge {
		t.Fatalf("超限解压必须返回 ErrDecompressTooLarge，实际 %v", err)
	}
	// 限制足够时正常解出。
	plain, err := gzipDecompress(bomb, 32<<20)
	if err != nil {
		t.Fatalf("充足限额下解压失败：%v", err)
	}
	if len(plain) != 10<<20 {
		t.Fatalf("解压长度不对：%d", len(plain))
	}
}

func TestGzipDecompressRejectsCorruptData(t *testing.T) {
	if _, err := gzipDecompress([]byte("this is not gzip"), 1<<20); err == nil {
		t.Fatal("非 gzip 数据必须报错")
	}
	// 截断的 gzip 流。
	valid := gzipMust(t, strings.Repeat("x", 1000))
	if _, err := gzipDecompress(valid[:len(valid)/2], 1<<20); err == nil {
		t.Fatal("截断的 gzip 必须报错（不能静默给半个文件）")
	}
}

func TestGzipCompressEmptyAndSmall(t *testing.T) {
	// 空输入：压缩后仍是合法 gzip（解压为空），不报错。
	compressed, err := gzipCompress(nil)
	if err != nil {
		t.Fatalf("空输入压缩失败：%v", err)
	}
	plain, err := gzipDecompress(compressed, 1<<20)
	if err != nil {
		t.Fatalf("空输入解压失败：%v", err)
	}
	if len(plain) != 0 {
		t.Fatalf("空输入解压应得空，实际 %d 字节", len(plain))
	}

	// 极小输入（< 一条记录）：必须无损。
	small := []byte("{\"role\":\"user\",\"content\":\"hi\"}\n")
	compressed, err = gzipCompress(small)
	if err != nil {
		t.Fatal(err)
	}
	plain, err = gzipDecompress(compressed, 1<<20)
	if err != nil || !bytes.Equal(plain, small) {
		t.Fatalf("小输入无损性被破坏：err=%v", err)
	}
}

// ---------- 工具 ----------

func gzipMust(t *testing.T, s string) []byte {
	t.Helper()
	out, err := gzipCompress([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func isValidGzip(data []byte) bool {
	_, err := gzipDecompress(data, 1<<30)
	return err == nil
}
