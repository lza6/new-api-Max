package training_capture

import (
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

// 采集模块的**自适应资源预算**（Spec-008 增量需求 B1）。
//
// ## 为什么不能写死 900MB
//
// 用户明确指出：容器预算锁死 900MB 不行，服务器内存很大，
// 应该「占用 80%」来自适应。原实现里队列容量、单条文本上限、
// 分片大小全是硬编码常量 —— 在 4G 小机上合理，在 64G 机器上
// 就是白白浪费内存。
//
// ## 检测顺序（每一步都有真实的失败模式）
//
// 1. **cgroup v2**（现代 Docker/K8s）：读 `/sys/fs/cgroup/memory.max`。
//    「max」表示无限制 → 跳到下一层。
// 2. **cgroup v1**（旧内核）：读 `/sys/fs/cgroup/memory/memory.limit_in_bytes`。
//    内核用 `math.MaxInt64` 附近的一个魔法值表示无限制 → 跳到下一层。
// 3. **gopsutil 宿主机内存**（项目已有依赖，`common/system_monitor.go` 在用）。
//
// ## 为什么必须认 cgroup（这是最容易踩的坑）
//
// 在容器里 `mem.VirtualMemory()` 返回的是**宿主机**内存。
// 一个限制 512MB 的容器跑在 64G 宿主机上，若按宿主机的 80% 预算，
// 采集模块自己就能吃掉 50GB —— 直接被 OOM killer 杀掉，
// 且表现为「容器无征兆重启」，极难排查。所以 cgroup 检测**必须**在
// 宿主机检测**之前**。
//
// ## 预算的用途（自上而下分配，而不是每处自己拍脑袋）
//
//	budget = min(cgroupLimit, hostTotal) * 80%
//	├── 队列最坏占用（capacity × 单条上限）≤ budget 的 25%
//	├── 单条采集文本上限：随预算放大（64KB → 数 MB）
//	└── worker 单分片原始上限：随预算放大（2MB → 16MB）
//
// 每一项都可以用环境变量覆盖（运维显式配置 > 自动检测）。

const (
	// captureBudgetRatio 采集模块允许占用的内存比例。
	// 用户指定 80%。
	captureBudgetRatio = 0.80

	// queueWorstCaseShare 队列最坏占用占预算的最大比例。
	// 取 25%：队列是「随时可能瞬间堆满」的部分，要给它足够的突发空间，
	// 但绝不能让它成为内存大头（大头永远是缓存/连接池等主业务）。
	queueWorstCaseShare = 0.25

	// cgroupV2MaxToken cgroup v2 中「无限制」的字面值。
	cgroupV2MaxToken = "max"

	// cgroupV1Unlimited cgroup v1 中「无限制」的魔法值（近似 kernel 的
	// PAGE_COUNTER_MAX）。大于该值视为无限制。
	cgroupV1Unlimited = 1 << 60
)

// cgroup 文件路径用 var 而非 const：测试需要指向临时文件
// （不可能在 CI 里真的部署一个 cgroup）。
var (
	cgroupV2MemoryMaxPath   = "/sys/fs/cgroup/memory.max"
	cgroupV1MemoryLimitPath = "/sys/fs/cgroup/memory/memory.limit_in_bytes"
)

// Env 前缀集中定义，便于文档与部署脚本引用。
const (
	EnvCaptureMaxTextMB   = "TRAINING_CAPTURE_MAX_TEXT_MB"   // 单条采集文本上限（MB）
	EnvCaptureMaxPartMB   = "TRAINING_CAPTURE_MAX_PART_MB"   // 单分片原始上限（MB）
	EnvCaptureQueueCap    = "TRAINING_CAPTURE_QUEUE_CAP"     // 队列容量（条）
	EnvCaptureBudgetPct   = "TRAINING_CAPTURE_BUDGET_PCT"    // 预算百分比（默认 80）
	EnvCaptureMinBudgetMB = "TRAINING_CAPTURE_MIN_BUDGET_MB" // 预算下限（MB）
)

// ResourceBudget 采集模块的内存预算与派生限额。
//
// 全部字段在初始化时一次性确定（`initResourceBudget`），运行期只读 ——
// 这样热路径读它们不需要任何同步。
type ResourceBudget struct {
	// BudgetBytes 采集模块允许使用的总字节数。
	BudgetBytes int64
	// Source 预算来源（cgroupv2 / cgroupv1 / host / floor），供管理员排查。
	Source string

	// MaxTextBytes 单条采集文本的原始字节上限。
	// 超过会**截断**（不是拒绝）：截断保留开头特征，对增量与检索仍有价值；
	// 拒绝则大请求完全不被采集。诚实边界：截断的数据是残缺样本。
	MaxTextBytes int

	// MaxPartRawBytes 单个分片**压缩前**的原始字节上限。
	// 压缩后仍必须 ≤ OneDriveSimpleUploadMaxBytes(4MB)，由 worker 二次校验。
	MaxPartRawBytes int

	// QueueCapacity 采集队列容量（条）。
	QueueCapacity int
}

var resourceBudget ResourceBudget

// readCgroupLimitBytes 读 cgroup 内存上限；无限制或读不到返回 0。
func readCgroupLimitBytes() (int64, string) {
	// cgroup v2：单行，数字或 "max"。
	if raw, err := os.ReadFile(cgroupV2MemoryMaxPath); err == nil {
		s := strings.TrimSpace(string(raw))
		if s != "" && s != cgroupV2MaxToken {
			if v, err := strconv.ParseInt(s, 10, 64); err == nil && v > 0 {
				return v, "cgroupv2"
			}
		}
	}
	// cgroup v1：单行数字；无限制是一个巨大的魔法值。
	if raw, err := os.ReadFile(cgroupV1MemoryLimitPath); err == nil {
		if v, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64); err == nil {
			// 魔法值 ≈ 9.2e18（接近 MaxInt64）。> 2^60 视为无限制。
			if v > 0 && v < cgroupV1Unlimited {
				return v, "cgroupv1"
			}
		}
	}
	return 0, ""
}

// detectBudgetBytes 检测可用内存并按比例折算预算。
//
// hostTotal 注入是为了可测（生产传 gopsutil 的结果）。
func detectBudgetBytes(hostTotal int64, pct int, floorMB int64) (int64, string) {
	if floorMB <= 0 {
		floorMB = 128
	}
	limit, source := readCgroupLimitBytes()
	base := hostTotal
	if limit > 0 && (base <= 0 || limit < base) {
		// cgroup 上限存在且小于宿主机内存（或宿主机读不到）→ 用 cgroup。
		// 注意：必须取 **min**，因为容器可能同时受 cgroup 与物理内存约束。
		base = limit
	} else if base > 0 {
		source = "host"
	}
	if base <= 0 {
		// [修复防御] 全部检测失败时**不能返回 0**：预算为 0 会让队列容量、
		// 单条上限全部推导为 0，采集完全静默失效。退到安全下限 ——
		// 宁可在小预算下运行，也不要「看起来开着、实际一条都采不到」。
		return floorMB << 20, "floor"
	}
	if pct <= 0 || pct > 100 {
		pct = int(captureBudgetRatio * 100)
	}
	budget := base * int64(pct) / 100
	// 下限保护：极小预算（如 cgroup 配了 1MB）会让派生值全为 0。
	if min := floorMB << 20; budget < min {
		budget = min
		source = "floor"
	}
	return budget, source
}

// deriveLimits 从预算派生各限额。
//
// 全部可被环境变量覆盖；覆盖值同样受安全下限约束
// （比如把单条上限配成 0 会让采集完全失效，故夹到 ≥ 64KB）。
func deriveLimits(budget int64) ResourceBudget {
	// ---- 单条文本上限 ----
	// 基线 64KB（最小可用值）；预算每多 128MB 放大 2 倍，封顶 8MB。
	// 放大逻辑：64KB 在 128MB 预算下 = 队列可容纳 512 条全文 —— 足够；
	// 预算翻倍时允许更长的原文进队列，用「条数换长度」。
	maxText := int64(64) << 10
	for scale := budget; scale > 128<<20 && maxText < 8<<20; scale >>= 1 {
		maxText *= 2
	}
	if raw := os.Getenv(EnvCaptureMaxTextMB); raw != "" {
		if mb, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && mb > 0 {
			maxText = int64(mb) << 20
		}
	}
	if maxText < 64<<10 {
		maxText = 64 << 10
	}

	// ---- 单分片原始上限 ----
	// 压缩前的原始上限。JSON 文本 gzip 后约 1/5~1/10，
	// 16MB 原始 → 1.6~3.2MB 压缩，仍在 OneDrive 4MB 简单上传限制内。
	// worker 侧会对压缩后字节数**二次校验**，超限继续切分 ——
	// 因此这个值主要是「减少分片数」的性能参数，不是正确性参数。
	maxPart := int64(16) << 20
	if raw := os.Getenv(EnvCaptureMaxPartMB); raw != "" {
		if mb, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && mb > 0 {
			maxPart = int64(mb) << 20
		}
	}
	if maxPart < 1<<20 {
		maxPart = 1 << 20 // 压缩后至少要可能 ≤4MB，原始太小没意义
	}

	// ---- 队列容量 ----
	// 队列最坏占用（容量 × 单条上限）≤ 预算的 25%。
	// 上限 4096：再大也只是让积压数据在内存里放更久，没有收益
	// （采集是尽力而为的，宁可丢）。
	queueCap := int64(1024)
	if budget > 0 && maxText > 0 {
		worstCaseBudget := budget * int64(queueWorstCaseShare*100) / 100
		derived := worstCaseBudget / maxText
		if derived < queueCap {
			queueCap = derived
		}
		if queueCap > 4096 {
			queueCap = 4096
		}
	}
	if queueCap < 16 {
		queueCap = 16 // 保证小预算下仍有基本吞吐
	}
	if raw := os.Getenv(EnvCaptureQueueCap); raw != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && v > 0 {
			queueCap = int64(v)
		}
	}

	return ResourceBudget{
		BudgetBytes:     budget,
		MaxTextBytes:    int(maxText),
		MaxPartRawBytes: int(maxPart),
		QueueCapacity:   int(queueCap),
	}
}

// initResourceBudget 初始化预算（进程启动时调用一次）。
//
// hostTotal 由调用方提供（生产用 gopsutil；测试注入固定值）。
func initResourceBudget(hostTotal int64) ResourceBudget {
	pct := 0
	if raw := os.Getenv(EnvCaptureBudgetPct); raw != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && v > 0 && v <= 100 {
			pct = v
		}
	}
	floorMB := int64(0)
	if raw := os.Getenv(EnvCaptureMinBudgetMB); raw != "" {
		if v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && v > 0 {
			floorMB = v
		}
	}
	budget, source := detectBudgetBytes(hostTotal, pct, floorMB)
	resourceBudget = deriveLimits(budget)
	resourceBudget.Source = source
	return resourceBudget
}

// GetResourceBudget 返回当前预算（运行期只读，无锁）。
func GetResourceBudget() ResourceBudget {
	return resourceBudget
}

// InitResourceBudgetFromHost 用真实宿主机内存初始化预算并返回。
// 生产启动入口（controller 装配时调用）；测试用 initResourceBudget 注入固定值。
func InitResourceBudgetFromHost() ResourceBudget {
	b := initResourceBudget(hostTotalMemoryBytes())
	budgetInitialized.Store(true)
	return b
}

// hostTotalMemoryBytes 用 gopsutil 读宿主机总内存（项目已有依赖）。
// 读失败返回 0（上层会退到 floor）。
func hostTotalMemoryBytes() int64 {
	// 延迟 import 放在独立小函数里，让测试可以不触碰系统调用路径。
	return readHostMemoryTotal()
}

// 预算是否已初始化（供测试守护：忘记 init 的包级状态是最阴险的 bug 来源）。
var budgetInitialized atomic.Bool
