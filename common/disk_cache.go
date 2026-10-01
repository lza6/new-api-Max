package common

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// DiskCacheType 磁盘缓存类型
type DiskCacheType string

const (
	DiskCacheTypeBody DiskCacheType = "body" // 请求体缓存
	DiskCacheTypeFile DiskCacheType = "file" // 文件数据缓存
)

// 统一的缓存目录名
const diskCacheDir = "new-api-body-cache"

// GetDiskCacheDir 获取统一的磁盘缓存目录
// 注意：每次调用都会重新计算，以响应配置变化
func GetDiskCacheDir() string {
	cachePath := GetDiskCachePath()
	if cachePath == "" {
		cachePath = os.TempDir()
	}
	return filepath.Join(cachePath, diskCacheDir)
}

// EnsureDiskCacheDir 确保缓存目录存在
func EnsureDiskCacheDir() error {
	dir := GetDiskCacheDir()
	return os.MkdirAll(dir, 0755)
}

// CreateDiskCacheFile 创建磁盘缓存文件
// cacheType: 缓存类型（body/file）
// 返回文件路径和文件句柄
func CreateDiskCacheFile(cacheType DiskCacheType) (string, *os.File, error) {
	if err := EnsureDiskCacheDir(); err != nil {
		return "", nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	dir := GetDiskCacheDir()
	filename := fmt.Sprintf("%s-%s-%d.tmp", cacheType, uuid.New().String()[:8], time.Now().UnixNano())
	filePath := filepath.Join(dir, filename)

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0600)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create cache file: %w", err)
	}

	return filePath, file, nil
}

// WriteDiskCacheFile 写入数据到磁盘缓存文件
// 返回文件路径
func WriteDiskCacheFile(cacheType DiskCacheType, data []byte) (string, error) {
	filePath, file, err := CreateDiskCacheFile(cacheType)
	if err != nil {
		return "", err
	}

	_, err = file.Write(data)
	if err != nil {
		file.Close()
		os.Remove(filePath)
		return "", fmt.Errorf("failed to write cache file: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(filePath)
		return "", fmt.Errorf("failed to close cache file: %w", err)
	}

	return filePath, nil
}

// WriteDiskCacheFileString 写入字符串到磁盘缓存文件
func WriteDiskCacheFileString(cacheType DiskCacheType, data string) (string, error) {
	return WriteDiskCacheFile(cacheType, []byte(data))
}

// ReadDiskCacheFile 读取磁盘缓存文件
func ReadDiskCacheFile(filePath string) ([]byte, error) {
	return os.ReadFile(filePath)
}

// ReadDiskCacheFileString 读取磁盘缓存文件为字符串
func ReadDiskCacheFileString(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RemoveDiskCacheFile 删除磁盘缓存文件
func RemoveDiskCacheFile(filePath string) error {
	return os.Remove(filePath)
}

// CleanupOldDiskCacheFiles 清理旧的缓存文件
// maxAge: 文件最大存活时间
// 注意：此函数只删除文件，不更新统计（因为无法知道每个文件的原始大小）
func CleanupOldDiskCacheFiles(maxAge time.Duration) error {
	dir := GetDiskCacheDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 目录不存在，无需清理
		}
		return err
	}

	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			// 注意：后台清理任务删除文件时，由于无法得知原始 base64Size，
			// 只能按磁盘文件大小扣减。这在目前 base64 存储模式下是准确的。
			if err := os.Remove(filepath.Join(dir, entry.Name())); err == nil {
				DecrementDiskFiles(info.Size())
			}
		}
	}
	return nil
}

// EnforceDiskCacheCapacity 按总容量上限淘汰最旧的缓存文件（LRU 近似：按
// ModTime 旧者优先）。reserveBytes 是即将写入、需要为之腾出的空间（周期清理
// 传 0）。仅回收「安全窗口」(olderThan) 之前的文件，避免误删仍在途的请求体；
// 返回删除的文件数。容量上限取自 GetDiskCacheMaxSizeBytes()；<=0 表示不限制。
func EnforceDiskCacheCapacity(olderThan time.Duration, reserveBytes int64) (int, error) {
	maxBytes := GetDiskCacheMaxSizeBytes()
	if maxBytes <= 0 {
		return 0, nil
	}
	dir := GetDiskCacheDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	type cacheFile struct {
		name    string
		size    int64
		modTime time.Time
	}
	files := make([]cacheFile, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, cacheFile{name: entry.Name(), size: info.Size(), modTime: info.ModTime()})
		total += info.Size()
	}
	if total+reserveBytes <= maxBytes {
		return 0, nil
	}

	// 越旧越靠前：优先淘汰最早写入的文件。
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.Before(files[j].modTime) })

	now := time.Now()
	removed := 0
	for _, f := range files {
		if total+reserveBytes <= maxBytes {
			break
		}
		if now.Sub(f.modTime) < olderThan {
			break // 剩余文件都在安全窗口内，停止，避免误删在途请求
		}
		if err := os.Remove(filepath.Join(dir, f.name)); err == nil {
			DecrementDiskFiles(f.size)
			total -= f.size
			removed++
		}
	}
	return removed, nil
}

// GetDiskCacheInfo 获取磁盘缓存目录信息
func GetDiskCacheInfo() (fileCount int, totalSize int64, err error) {
	dir := GetDiskCacheDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fileCount++
		totalSize += info.Size()
	}
	return fileCount, totalSize, nil
}

// ShouldUseDiskCache 判断是否应该使用磁盘缓存
func ShouldUseDiskCache(dataSize int64) bool {
	if !IsDiskCacheEnabled() {
		return false
	}
	threshold := GetDiskCacheThresholdBytes()
	if dataSize < threshold {
		return false
	}
	return IsDiskCacheAvailable(dataSize)
}
