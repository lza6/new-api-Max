package common

import (
	"os"
	"path/filepath"
)

// 磁盘写入的**统一守卫**（Batch-11）。
//
// 背景：用户的机器是「内存紧张（容器 900MB）、磁盘富余（78G）」的形态，希望把
// 进程内缓存改为磁盘承载。但磁盘缓存必须自己守住边界，否则就是把内存问题换成
// 磁盘问题。本文件提供三个原语：
//
//  1. `AvailableDiskBytes(path)` —— 查**指定目录**所在文件系统的真实剩余空间；
//  2. `HasDiskSpaceFor(path, need, minFree)` —— 写前判定「写完还剩多少」；
//  3. `ExistingAncestor(path)` —— 目录可能还没建，向上找最近的存在祖先再查。
//
// 与既有 `GetDiskSpaceInfo()` 的关系：后者只查**缓存目录**所在盘且不接参数；
// 这里参数化（缓存与图床可能在不同挂载点），且 syscall 实现完全复用同一份
// build-tag 代码（`diskSpaceInfoAt`），不重复写平台分支。

// ExistingAncestor 返回 path 或其最近的、已存在的祖先目录。
//
// 用途：目标目录可能尚未创建，此时 Statfs 会失败；向上找到第一个存在的祖先才能
// 正确判断「这个路径最终会落在哪个文件系统上」。
// 找不到任何存在的祖先时返回空串。
func ExistingAncestor(path string) string {
	if path == "" {
		return ""
	}
	current := filepath.Clean(path)
	for {
		if _, err := os.Stat(current); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// AvailableDiskBytes 返回 path 所在文件系统的**可用**字节数。
// 路径不存在时自动向上找最近的存在祖先；完全查不到时返回 (0, false)。
//
// 注意用 `Bavail`（非特权用户可用）而非 `Bfree`（含 root 保留块）——
// 缓存写入不该指望保留块。
func AvailableDiskBytes(path string) (int64, bool) {
	target := ExistingAncestor(path)
	if target == "" {
		return 0, false
	}
	info := diskSpaceInfoAt(target)
	if info.Total == 0 {
		// 总容量为 0 只有一个解释：syscall 失败（见各平台的实现）。
		return 0, false
	}
	if info.Free > uint64(1<<62) {
		return 1 << 62, true // 防御性截断，避免 uint64→int64 溢出成负数
	}
	return int64(info.Free), true
}

// HasDiskSpaceFor 判定「在 path 写入 needBytes 后，剩余空间是否仍不低于 minFreeBytes」。
//
// 这是「不要让磁盘爆满」的核心判据：只看自己的配额上限是不够的 —— 配额内的数据
// 也可能把整块盘写满（同盘的其它目录、日志、数据库都在长）。
// 查不到剩余空间时返回 false（**fail-closed**：宁可少缓存，也不要冒写满盘的风险）。
func HasDiskSpaceFor(path string, needBytes, minFreeBytes int64) bool {
	if needBytes < 0 {
		return false
	}
	free, ok := AvailableDiskBytes(path)
	if !ok {
		return false
	}
	if minFreeBytes < 0 {
		minFreeBytes = 0
	}
	return free-needBytes >= minFreeBytes
}
