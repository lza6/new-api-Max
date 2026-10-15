# ADR-002：采集内存预算必须 cgroup 感知

> 目录：`计划书/adr/`（编号在本目录内独立）。本次决策批次：训练数据采集 Spec-008。
> 最后核对：2026-10-16（`VERSION` = `v1.3.147`）。

## 背景

采集模块原先把队列容量、单条文本上限、分片大小**全部写成硬编码常量**（4G 小机语境）。
用户指出：容器预算锁死 900MB 不行，服务器内存很大，应按「占用 80%」自适应——在 64G 机器上
硬编码就是白白浪费内存（`service/training_capture/resources.go:12-18`）。

关键坑：**在容器里 `mem.VirtualMemory()` 返回的是宿主机内存**。一个限制 512MB 的容器跑在
64G 宿主机上，若按宿主机 80% 预算，采集模块自己就能吃掉 50GB，被 OOM killer 杀掉，
且表现为「容器无征兆重启」，极难排查（`resources.go:27-33`）。

## 决策

按检测顺序确定可用内存，预算 = `min(cgroupLimit, hostTotal) * 80%`：

1. **cgroup v2**：读 `/sys/fs/cgroup/memory.max`（`max` 表示无限制）——`resources.go:104-113`。
2. **cgroup v1**：读 `/sys/fs/cgroup/memory/memory.limit_in_bytes`（内核用 `MaxInt64` 附近的魔法值
   表示无限制，`> 2^60` 视为无限制）——`resources.go:114-122`。
3. **gopsutil 宿主机内存**（项目已有依赖）——`resources.go:139-141`。
4. 全部失败 → **安全下限**（不是 0），避免派生限额全为 0 导致采集静默失效——`resources.go:142-147`。

**cgroup 检测必须先于宿主机检测**，且取 **min**（容器可能同时受 cgroup 与物理内存约束）。
预算自上而下派生：队列最坏占用 ≤ 预算 25%、单条文本上限随预算放大、worker 分片原始上限随预算放大
（`resources.go:35-41,164-241`）。每一项都可用环境变量覆盖（运维显式配置 > 自动检测）。

## 被拒绝的替代方案

1. **写死 900MB**：用户明确否决；大机上浪费、小机上僵化。
2. **只用 `mem.VirtualMemory()`（宿主机内存）**：容器内会按宿主机算预算 → OOM killer 杀容器（见背景）。
3. **检测失败时返回 0**：预算为 0 会让队列容量、单条上限全推导为 0 → 采集完全静默失效；
   故退到安全下限 `floor`（`resources.go:142-147`）。

## 后果（含代价）

- ✅ 4G 小机与 64G 大机都能自适应；容器内存受限时不会误用宿主机内存。
- ⚠️ cgroup 文件路径用 `var` 而非 `const`，以便测试指向临时文件（CI 里无法真部署 cgroup）——`resources.go:62-67`。
- ⚠️ 队列是**包级变量**，构造时预算尚为零值 → 必须在预算初始化后**重建**队列（`resources.go:270-293`），
  否则「自适应容量」是死代码，且启动日志会报**假**容量。

## 证据

- `service/training_capture/resources.go:12-67,103-158,164-241,270-293,328-333`
- 回归用例：`service/training_capture/resources_test.go:17`（`TestDetectBudgetPrefersSmallerCgroupOverHost`，含 cgroup v2/v1/`max`/魔法值四个子例）、
  `resources_test.go:58`（`TestDetectBudgetFloorWhenNothingDetectable`）
