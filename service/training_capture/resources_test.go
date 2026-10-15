package training_capture

import (
	"os"
	"path/filepath"
	"testing"
)

// 自适应资源预算（B1）的测试。
//
// 三条关键性质：
//  1. **容器内必须认 cgroup**：cgroup 上限 < 宿主机内存时取 cgroup。
//    忽略这一点会让采集在限 512MB 的容器里按宿主机的 80% 预算 → OOM killer。
//  2. 无限制（cgroup "max" / v1 魔法值）→ 退到宿主机内存。
//  3. 全部检测失败 → 安全下限（不是 0，0 会让采集完全失效）。

func TestDetectBudgetPrefersSmallerCgroupOverHost(t *testing.T) {
	// 容器限 512MB，宿主机 64GB → 必须按 512MB 算（这是会杀进程的坑）。
	hostTotal := int64(64) << 30

	t.Run("cgroup v2 限制生效", func(t *testing.T) {
		writeCgroupFiles(t, "536870912\n", "")
		budget, source := detectBudgetBytes(hostTotal, 80, 0)
		if source != "cgroupv2" {
			t.Fatalf("来源应为 cgroupv2，实际 %s", source)
		}
		if want := int64(512) << 20 * 80 / 100; budget != want {
			t.Fatalf("预算应为 %d，实际 %d", want, budget)
		}
	})

	t.Run("cgroup v1 限制生效", func(t *testing.T) {
		writeCgroupFiles(t, "", "536870912\n")
		_, source := detectBudgetBytes(hostTotal, 80, 0)
		if source != "cgroupv1" {
			t.Fatalf("来源应为 cgroupv1，实际 %s", source)
		}
	})

	t.Run("cgroup v2 max 表示无限制", func(t *testing.T) {
		writeCgroupFiles(t, "max\n", "")
		_, source := detectBudgetBytes(hostTotal, 80, 0)
		if source != "host" {
			t.Fatalf("max 应回退宿主机，实际 %s", source)
		}
	})

	t.Run("cgroup v1 魔法值表示无限制", func(t *testing.T) {
		// kernel 的 PAGE_COUNTER_MAX ≈ 9.2e18，远超 2^60。
		writeCgroupFiles(t, "", "9223372036854771712\n")
		_, source := detectBudgetBytes(hostTotal, 80, 0)
		if source != "host" {
			t.Fatalf("魔法值应视为无限制，实际 %s", source)
		}
	})
}

func TestDetectBudgetFloorWhenNothingDetectable(t *testing.T) {
	// 宿主机读不到 + 无 cgroup → 安全下限，不是 0。
	// 预算为 0 会让队列容量/单条上限全为 0，采集完全失效。
	writeCgroupFiles(t, "", "")
	budget, source := detectBudgetBytes(0, 80, 0)
	if budget <= 0 {
		t.Fatalf("全部检测失败时必须给正的预算，实际 %d", budget)
	}
	if source != "floor" {
		t.Fatalf("来源应为 floor，实际 %s", source)
	}
}

func TestDetectBudgetCustomPercent(t *testing.T) {
	writeCgroupFiles(t, "", "")
	hostTotal := int64(10) << 30

	// 用户指定 80%（默认）。
	b80, _ := detectBudgetBytes(hostTotal, 80, 0)
	if want := int64(8) << 30; b80 != want {
		t.Fatalf("80%% 应为 %d，实际 %d", want, b80)
	}
	// 显式 50%。
	b50, _ := detectBudgetBytes(hostTotal, 50, 0)
	if want := int64(5) << 30; b50 != want {
		t.Fatalf("50%% 应为 %d，实际 %d", want, b50)
	}
	// 非法值（150%、0、负数）→ 回退默认 80。
	for _, bad := range []int{150, 0, -5} {
		b, _ := detectBudgetBytes(hostTotal, bad, 0)
		if b != b80 {
			t.Fatalf("非法百分比 %d 应回退默认，实际 %d（期望 %d）", bad, b, b80)
		}
	}
}

func TestDeriveLimitsScalesWithBudget(t *testing.T) {
	// 小预算：128MB → 单条 64KB、队列容量被「最坏占用 ≤ 25% 预算」约束。
	small := deriveLimits(128 << 20)
	if small.MaxTextBytes < 64<<10 {
		t.Fatalf("单条上限不得低于 64KB，实际 %d", small.MaxTextBytes)
	}
	if small.QueueCapacity < 16 {
		t.Fatalf("小预算下队列容量仍有下限 16，实际 %d", small.QueueCapacity)
	}

	// 大预算：64GB×80% → 单条上限应放大（64KB 就太浪费了），
	// 队列容量应大于小预算下（预算富余允许更多缓冲）。
	big := deriveLimits(51 << 30)
	if big.MaxTextBytes <= small.MaxTextBytes {
		t.Fatalf("大预算下单条上限必须放大：大 %d vs 小 %d",
			big.MaxTextBytes, small.MaxTextBytes)
	}
	if big.QueueCapacity <= small.QueueCapacity {
		t.Fatalf("大预算下队列容量应更大：大 %d vs 小 %d",
			big.QueueCapacity, small.QueueCapacity)
	}
	if big.QueueCapacity > 4096 {
		t.Fatalf("队列容量必须被硬顶在 4096（再大只是让积压在内存放更久），实际 %d",
			big.QueueCapacity)
	}
	if big.MaxPartRawBytes < 1<<20 {
		t.Fatalf("分片原始上限不得低于 1MB（否则压缩后仍远小于上传上限，纯属浪费），实际 %d",
			big.MaxPartRawBytes)
	}
}

func TestDeriveLimitsEnvOverrides(t *testing.T) {
	t.Setenv(EnvCaptureMaxTextMB, "4")
	t.Setenv(EnvCaptureMaxPartMB, "32")
	t.Setenv(EnvCaptureQueueCap, "777")

	b := deriveLimits(128 << 20)
	if b.MaxTextBytes != 4<<20 {
		t.Fatalf("env 覆盖单条上限失败：%d", b.MaxTextBytes)
	}
	if b.MaxPartRawBytes != 32<<20 {
		t.Fatalf("env 覆盖分片上限失败：%d", b.MaxPartRawBytes)
	}
	if b.QueueCapacity != 777 {
		t.Fatalf("env 覆盖队列容量失败：%d", b.QueueCapacity)
	}

	// 非法值 → 不覆盖（回退推导值）。
	t.Setenv(EnvCaptureMaxTextMB, "abc")
	b2 := deriveLimits(128 << 20)
	if b2.MaxTextBytes != b.MaxTextBytes-0 && b2.MaxTextBytes > 8<<20 {
		// 只验证「非法值没有把上限变成 0 或负数」这个硬底线。
		t.Fatalf("非法 env 不应破坏下限：%d", b2.MaxTextBytes)
	}
	if b2.MaxTextBytes < 64<<10 {
		t.Fatalf("非法 env 后仍必须 ≥64KB，实际 %d", b2.MaxTextBytes)
	}
}

func TestInitResourceBudgetIsIdempotent(t *testing.T) {
	// 两次初始化结果一致（幂等）——启动时被调多次也不应产生不同预算。
	a := initResourceBudget(8 << 30)
	b := initResourceBudget(8 << 30)
	if a != b {
		t.Fatalf("初始化应幂等：%+v vs %+v", a, b)
	}
}

// writeCgroupFiles 把 cgroup 探测路径指到临时文件。
// 传空串表示「删除该文件」（模拟该代 cgroup 不存在）。
func writeCgroupFiles(t *testing.T, v2Content, v1Content string) {
	t.Helper()
	dir := t.TempDir()

	v2 := filepath.Join(dir, "memory.max")
	if v2Content == "" {
		// 指向一个不存在的路径。
		v2 = filepath.Join(dir, "nonexistent-v2")
	} else if err := os.WriteFile(v2, []byte(v2Content), 0644); err != nil {
		t.Fatal(err)
	}
	v1 := filepath.Join(dir, "limit_in_bytes")
	if v1Content == "" {
		v1 = filepath.Join(dir, "nonexistent-v1")
	} else if err := os.WriteFile(v1, []byte(v1Content), 0644); err != nil {
		t.Fatal(err)
	}

	origV2, origV1 := cgroupV2MemoryMaxPath, cgroupV1MemoryLimitPath
	cgroupV2MemoryMaxPath, cgroupV1MemoryLimitPath = v2, v1
	t.Cleanup(func() {
		cgroupV2MemoryMaxPath, cgroupV1MemoryLimitPath = origV2, origV1
	})
}
