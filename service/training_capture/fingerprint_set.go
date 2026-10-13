package training_capture

import (
	"container/list"
	"sync"
)

// 有界指纹集合（LRU 淘汰）。
//
// ## 为什么必须有界
//
// `GlobalDedup` 需要「已经上传过哪些内容」这个信息。若用无界 map，
// 每个新消息都会往里面塞一条 64 字符的十六进制指纹——按每天 10 万条消息算，
// 一年就是 3650 万条 ≈ 2.3GB，**直接把容器（限 900MB）撑爆**。
//
// 因此：固定容量 + LRU 淘汰。代价是「很久以前传过的内容再出现时会被重传一次」，
// 但这个代价远小于 OOM，且真正的去重主路径是 `SelectIncrement` 的水位机制
// （O(1) 状态、精确），本集合只是**拿不到会话 id 时的退化兜底**。
//
// ## 并发
//
// 采集 worker 目前是单 goroutine，但接口预置互斥锁：
// 未来横向扩展 worker 时不必回头改数据结构的语义（加锁比事后发现
// map 并发读写要便宜）。

// BoundedFingerprintSet 固定容量的指纹集合，超出容量按 LRU 淘汰。
type BoundedFingerprintSet struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List // 前端 = 最近使用
}

// FingerprintSetDefaultCapacity 默认容量（约 6.4MB 内存：10 万条 × 64B 指纹 + 开销）。
const FingerprintSetDefaultCapacity = 100_000

// NewBoundedFingerprintSet 创建有界集合。capacity <= 0 时用默认值。
func NewBoundedFingerprintSet(capacity int) *BoundedFingerprintSet {
	if capacity <= 0 {
		capacity = FingerprintSetDefaultCapacity
	}
	return &BoundedFingerprintSet{
		capacity: capacity,
		items:    make(map[string]*list.Element, min(capacity, 1024)),
		order:    list.New(),
	}
}

// Has 报告指纹是否已存在。命中会**提升其新鲜度**（LRU touch），
// 因为「刚刚见过的内容」比「很久以前见过的」更可能再次出现。
func (s *BoundedFingerprintSet) Has(fingerprint string) bool {
	if s == nil || fingerprint == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.items[fingerprint]
	if !ok {
		return false
	}
	s.order.MoveToFront(el)
	return true
}

// Add 记录一个指纹；超出容量时淘汰最久未使用的。
func (s *BoundedFingerprintSet) Add(fingerprint string) {
	if s == nil || fingerprint == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if el, ok := s.items[fingerprint]; ok {
		s.order.MoveToFront(el)
		return
	}
	el := s.order.PushFront(fingerprint)
	s.items[fingerprint] = el
	for s.order.Len() > s.capacity {
		oldest := s.order.Back()
		if oldest == nil {
			break
		}
		s.order.Remove(oldest)
		delete(s.items, oldest.Value.(string))
	}
}

// Len 返回当前元素数（供测试与可观测）。
func (s *BoundedFingerprintSet) Len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.order.Len()
}

// Capacity 返回容量上限。
func (s *BoundedFingerprintSet) Capacity() int {
	if s == nil {
		return 0
	}
	return s.capacity
}
