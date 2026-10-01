package common

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// BodyStorage 请求体存储接口
type BodyStorage interface {
	io.ReadSeeker
	io.Closer
	// Bytes 获取全部内容
	Bytes() ([]byte, error)
	// Size 获取数据大小
	Size() int64
	// IsDisk 是否是磁盘存储
	IsDisk() bool
	// NewReader returns an independent reader positioned at the start of the
	// stored payload. Each call returns a reader with its own cursor, so
	// callers (e.g. http.Request.GetBody) can replay the body concurrently
	// with, or after, other readers without sharing seek state. Closing the
	// returned reader releases only that reader, never the storage itself;
	// after the storage has been closed, NewReader returns ErrStorageClosed.
	NewReader() (io.ReadCloser, error)
}

// ReplayableBody is an outbound request body that can report its byte size and
// create independent readers for transport-level retries.
type ReplayableBody interface {
	io.Reader
	Size() int64
	NewReader() (io.ReadCloser, error)
}

// ErrStorageClosed 存储已关闭错误
var ErrStorageClosed = fmt.Errorf("body storage is closed")

// memoryStorage 内存存储实现
type memoryStorage struct {
	data   []byte
	reader *bytes.Reader
	size   int64
	closed int32
	mu     sync.Mutex
}

func newMemoryStorage(data []byte) *memoryStorage {
	size := int64(len(data))
	IncrementMemoryBuffers(size)
	return &memoryStorage{
		data:   data,
		reader: bytes.NewReader(data),
		size:   size,
	}
}

func (m *memoryStorage) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return m.reader.Read(p)
}

func (m *memoryStorage) Seek(offset int64, whence int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return m.reader.Seek(offset, whence)
}

func (m *memoryStorage) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		DecrementMemoryBuffers(m.size)
	}
	return nil
}

func (m *memoryStorage) Bytes() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return nil, ErrStorageClosed
	}
	return m.data, nil
}

func (m *memoryStorage) NewReader() (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return nil, ErrStorageClosed
	}
	// A fresh bytes.Reader over the shared immutable backing array: an
	// independent cursor at zero copy cost. NopCloser keeps Close a no-op, so
	// the storage lifecycle stays owned by whoever holds the storage itself.
	return io.NopCloser(bytes.NewReader(m.data)), nil
}

func (m *memoryStorage) Size() int64 {
	return m.size
}

func (m *memoryStorage) IsDisk() bool {
	return false
}

// diskStorage 磁盘存储实现
type diskStorage struct {
	file     *os.File
	filePath string
	size     int64
	closed   int32
	mu       sync.Mutex
}

func newDiskStorage(data []byte, cachePath string) (*diskStorage, error) {
	// 使用统一的缓存目录管理
	filePath, file, err := CreateDiskCacheFile(DiskCacheTypeBody)
	if err != nil {
		return nil, err
	}

	// 写入数据
	n, err := file.Write(data)
	if err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}

	// 重置文件指针
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to seek temp file: %w", err)
	}

	size := int64(n)
	IncrementDiskFiles(size)

	return &diskStorage{
		file:     file,
		filePath: filePath,
		size:     size,
	}, nil
}

func newDiskStorageFromReader(reader io.Reader, maxBytes int64, cachePath string) (*diskStorage, error) {
	// 使用统一的缓存目录管理
	filePath, file, err := CreateDiskCacheFile(DiskCacheTypeBody)
	if err != nil {
		return nil, err
	}

	// 从 reader 读取并写入文件
	written, err := io.Copy(file, io.LimitReader(reader, maxBytes+1))
	if err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}

	if written > maxBytes {
		file.Close()
		os.Remove(filePath)
		return nil, ErrRequestBodyTooLarge
	}

	// 重置文件指针
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to seek temp file: %w", err)
	}

	IncrementDiskFiles(written)

	return &diskStorage{
		file:     file,
		filePath: filePath,
		size:     written,
	}, nil
}

func (d *diskStorage) Read(p []byte) (n int, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.LoadInt32(&d.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return d.file.Read(p)
}

func (d *diskStorage) Seek(offset int64, whence int) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.LoadInt32(&d.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return d.file.Seek(offset, whence)
}

func (d *diskStorage) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.CompareAndSwapInt32(&d.closed, 0, 1) {
		d.file.Close()
		os.Remove(d.filePath)
		DecrementDiskFiles(d.size)
	}
	return nil
}

func (d *diskStorage) Bytes() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if atomic.LoadInt32(&d.closed) == 1 {
		return nil, ErrStorageClosed
	}

	// 保存当前位置
	currentPos, err := d.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	// 移动到开头
	if _, err := d.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	// 读取全部内容
	data := make([]byte, d.size)
	_, err = io.ReadFull(d.file, data)
	if err != nil {
		return nil, err
	}

	// 恢复位置
	if _, err := d.file.Seek(currentPos, io.SeekStart); err != nil {
		return nil, err
	}

	return data, nil
}

func (d *diskStorage) NewReader() (io.ReadCloser, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.LoadInt32(&d.closed) == 1 {
		return nil, ErrStorageClosed
	}
	// A separate file descriptor over the same cache file: an independent
	// cursor at zero copy cost. Closing the returned reader closes only that
	// descriptor; the storage keeps owning the primary descriptor and the
	// file's lifetime. Readers opened before Close stay usable even after the
	// file is unlinked, as the descriptor keeps the inode alive.
	file, err := os.Open(d.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open body cache file for replay: %w", err)
	}
	return file, nil
}

func (d *diskStorage) Size() int64 {
	return d.size
}

func (d *diskStorage) IsDisk() bool {
	return true
}

// CreateBodyStorage 根据数据大小创建合适的存储。
//
// 语义（重要）：启用磁盘缓存后，请求体 ≥ 阈值的一律走磁盘；磁盘容量不足时
// 先淘汰最旧缓存腾空间，仍不足则**明确返回错误**，绝不回退内存——把大体积
// 请求体甩回内存正是启用磁盘缓存要避免的事，会抵消其意义并放大 OOM 风险。
func CreateBodyStorage(data []byte) (BodyStorage, error) {
	size := int64(len(data))
	threshold := GetDiskCacheThresholdBytes()

	if IsDiskCacheEnabled() && size >= threshold {
		if !EnsureDiskCacheSpace(size) {
			SysError(fmt.Sprintf("disk cache capacity exhausted (%d bytes used / %d limit); refusing %d-byte body instead of falling back to memory",
				atomic.LoadInt64(&diskCacheStats.CurrentDiskUsageBytes), GetDiskCacheMaxSizeBytes(), size))
			return nil, fmt.Errorf("disk cache capacity exhausted for %d-byte request body: %w", size, ErrRequestBodyTooLarge)
		}
		storage, err := newDiskStorage(data, GetDiskCachePath())
		if err != nil {
			SysError(fmt.Sprintf("disk storage write failed for %d-byte body: %v", size, err))
			return nil, fmt.Errorf("disk storage creation failed: %w", err)
		}
		return storage, nil
	}

	// 小请求体（< 阈值）：走内存更快，磁盘缓存对小对象是负优化。
	return newMemoryStorage(data), nil
}

// CreateBodyStorageFromReader 从 Reader 创建存储（用于大请求的流式处理）。
// 与 CreateBodyStorage 相同的容量语义：磁盘腾不出空间时明确失败，不回退内存。
func CreateBodyStorageFromReader(reader io.Reader, contentLength int64, maxBytes int64) (BodyStorage, error) {
	threshold := GetDiskCacheThresholdBytes()

	// 已启用磁盘缓存且内容长度超过阈值：必须落盘。
	if IsDiskCacheEnabled() &&
		contentLength > 0 &&
		contentLength >= threshold {
		if !EnsureDiskCacheSpace(contentLength) {
			SysError(fmt.Sprintf("disk cache capacity exhausted (%d bytes used / %d limit); refusing %d-byte streamed body instead of falling back to memory",
				atomic.LoadInt64(&diskCacheStats.CurrentDiskUsageBytes), GetDiskCacheMaxSizeBytes(), contentLength))
			return nil, fmt.Errorf("disk cache capacity exhausted for %d-byte request body: %w", contentLength, ErrRequestBodyTooLarge)
		}
		storage, err := newDiskStorageFromReader(reader, maxBytes, GetDiskCachePath())
		if err != nil {
			if IsRequestBodyTooLargeError(err) {
				return nil, err
			}
			// 磁盘存储失败，reader 已被消费，无法安全回退
			return nil, fmt.Errorf("disk storage creation failed: %w", err)
		}
		IncrementDiskCacheHits()
		return storage, nil
	}

	// 长度未知（chunked）：小于阈值走内存，超过阈值溢写落盘，避免大请求体整体进内存。
	if IsDiskCacheEnabled() && contentLength <= 0 {
		return createBodyStorageSpill(reader, maxBytes, threshold)
	}

	// 使用内存读取
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrRequestBodyTooLarge
	}

	storage, err := CreateBodyStorage(data)
	if err != nil {
		return nil, err
	}
	// 如果最终使用内存存储，记录内存缓存命中
	if !storage.IsDisk() {
		IncrementMemoryCacheHits()
	} else {
		IncrementDiskCacheHits()
	}
	return storage, nil
}

// createBodyStorageSpill handles a request body whose total length is unknown
// (chunked transfer) while the disk cache is enabled. It buffers at most the
// disk-cache threshold in memory; anything larger is spilled to disk so a large
// body is never held fully in memory. Returns ErrRequestBodyTooLarge if the
// stream (or its serialized size) exceeds maxBytes, or a capacity error when
// the disk cache cannot make room.
func createBodyStorageSpill(reader io.Reader, maxBytes, threshold int64) (BodyStorage, error) {
	prefix, err := io.ReadAll(io.LimitReader(reader, threshold+1))
	if err != nil {
		return nil, err
	}
	if int64(len(prefix)) <= threshold {
		// Still under the threshold: a small body, memory is the right home.
		return newMemoryStorage(prefix), nil
	}

	// Larger than the threshold: spill to disk. Make room first (evicting the
	// oldest cache files) rather than falling back to memory.
	if !EnsureDiskCacheSpace(int64(len(prefix))) {
		return nil, fmt.Errorf("disk cache capacity exhausted for a chunked request body: %w", ErrRequestBodyTooLarge)
	}
	filePath, file, err := CreateDiskCacheFile(DiskCacheTypeBody)
	if err != nil {
		return nil, fmt.Errorf("disk storage creation failed: %w", err)
	}
	written, err := file.Write(prefix)
	if err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("disk storage write failed: %w", err)
	}
	rest, err := io.Copy(file, io.LimitReader(reader, maxBytes-int64(written)+1))
	if err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("disk storage write failed: %w", err)
	}
	written += int(rest)
	if int64(written) > maxBytes {
		file.Close()
		os.Remove(filePath)
		return nil, ErrRequestBodyTooLarge
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to seek temp file: %w", err)
	}
	IncrementDiskFiles(int64(written))
	IncrementDiskCacheHits()
	return &diskStorage{file: file, filePath: filePath, size: int64(written)}, nil
}

type replayableBodyReader struct {
	storage BodyStorage
}

func (r replayableBodyReader) Read(p []byte) (int, error) {
	return r.storage.Read(p)
}

func (r replayableBodyReader) Size() int64 {
	return r.storage.Size()
}

func (r replayableBodyReader) NewReader() (io.ReadCloser, error) {
	return r.storage.NewReader()
}

// NewReplayableBodyReader exposes the replay capabilities of storage without
// exposing io.Closer. This keeps ownership of the storage lifecycle with the
// caller instead of allowing net/http to close it as the request body.
func NewReplayableBodyReader(storage BodyStorage) ReplayableBody {
	return replayableBodyReader{storage: storage}
}

// CleanupOldCacheFiles 清理旧的缓存文件（用于启动时清理残留）
func CleanupOldCacheFiles() {
	// 使用统一的缓存管理
	CleanupOldDiskCacheFiles(5 * time.Minute)
}
