package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/types"
)

// 站内图床（本地磁盘产物存储）实现。
//
// 设计要点：
//  - 只存**任务产物**（生成图/视频等）与**参考素材**（图生视频的用户参考图），
//    落盘在 `TASK_ARTIFACT_STORE_DIR` 下的 `<taskID>/<artifactKey>`。
//  - **防盗刷/防穿越**：ObjectKey 严格白名单字符 + `filepath.Clean` 后必须仍在
//    根目录内；Serve 强制 `Content-Disposition: attachment` 与 `X-Content-Type-Options: nosniff`，
//    禁止目录列举；访问仍走既有 **HMAC 签名 capability URL**（`task_artifact_access.go`）。
//  - **5 分钟清理**：`CleanupExpiredArtifacts` 删除「所属任务已完成且超过保留期」的目录
//    （参考素材尤其短命），由 `StartTaskArtifactCleanup` 后台 loop 周期执行。
//
// 开关：`TASK_ARTIFACT_STORE_MODE=local` 启用；`TASK_ARTIFACT_STORE_DIR` 指定根目录
// （默认 `<工作目录>/data/task-artifacts`）。未启用时保持既有 upstream 代理行为（零变化）。

const (
	// TaskArtifactStoreModeLocal 站内磁盘图床。
	TaskArtifactStoreModeLocal = "local"

	// 单文件上限（防超大文件打爆磁盘；参考素材通常 << 该值）。
	defaultArtifactMaxFileBytes = 64 << 20 // 64 MiB

	// 参考素材保留期（任务完成 5 分钟后清理，符合需求「每五分钟自动清理」）。
	referenceArtifactRetention = 5 * time.Minute
)

var (
	ErrArtifactPathInvalid = errors.New("task artifact path is invalid")

	// taskArtifactIDPattern 任务 ID 白名单（与 model.GenerateTaskID 的 "task_"+hex 兼容，
	// 同时容忍上游 UUID/hex 形态），禁止任何路径分隔符与 .. 序列。
	taskArtifactIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,190}$`)
	// taskArtifactKeySafePattern 产物 key 白名单，与 controller 的 taskArtifactKeyPattern 一致。
	taskArtifactKeySafePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~-]{0,127}$`)
)

// localArtifactStore 本地磁盘图床实现（TaskArtifactStore）。
type localArtifactStore struct {
	root     string
	maxBytes int64
	enabled  bool
}

// objectKey 由 taskID + artifactKey 组合，二者均经白名单校验。
func artifactObjectKey(taskID, artifactKey string) (string, error) {
	if !taskArtifactIDPattern.MatchString(taskID) {
		return "", ErrArtifactPathInvalid
	}
	if !taskArtifactKeySafePattern.MatchString(artifactKey) {
		return "", ErrArtifactPathInvalid
	}
	return filepath.ToSlash(filepath.Join(taskID, artifactKey)), nil
}

func (s *localArtifactStore) Enabled() bool { return s != nil && s.enabled }

// pathFor 解析并校验落盘绝对路径，确保不逃逸出 root（防目录穿越）。
func (s *localArtifactStore) pathFor(objectKey string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(objectKey))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", ErrArtifactPathInvalid
	}
	full := filepath.Join(s.root, clean)
	// 二次确认：解析后仍须位于 root 之内。
	rel, err := filepath.Rel(s.root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", ErrArtifactPathInvalid
	}
	return full, nil
}

func (s *localArtifactStore) Resolve(task *model.Task, artifactKey string) (*StoredArtifactRef, error) {
	if !s.Enabled() || task == nil {
		return nil, nil
	}
	objectKey, err := artifactObjectKey(task.TaskID, artifactKey)
	if err != nil {
		return nil, err
	}
	full, err := s.pathFor(objectKey)
	if err != nil {
		return nil, err
	}
	info, statErr := os.Stat(full)
	if statErr != nil || info.IsDir() {
		return nil, nil // 未落盘 → 回退上游代理
	}
	mimeType := mime.TypeByExtension(filepath.Ext(full))
	return &StoredArtifactRef{
		Backend:   TaskArtifactStoreModeLocal,
		ObjectKey: objectKey,
		MimeType:  mimeType,
		Size:      info.Size(),
	}, nil
}

func (s *localArtifactStore) Persist(_ context.Context, task *model.Task, artifact types.TaskArtifact, content io.Reader) (*StoredArtifactRef, error) {
	if !s.Enabled() {
		return nil, ErrTaskArtifactStoreDisabled
	}
	if task == nil || content == nil {
		return nil, ErrArtifactPathInvalid
	}
	objectKey, err := artifactObjectKey(task.TaskID, artifact.Key)
	if err != nil {
		return nil, err
	}
	full, err := s.pathFor(objectKey)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return nil, err
	}
	// 写到临时文件再原子重命名，避免半截文件被 Serve 读到。
	tmp := full + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, err
	}
	// 上限保护：多读 1 字节用于判定越界。
	written, copyErr := io.Copy(f, io.LimitReader(content, s.maxBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return nil, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return nil, closeErr
	}
	if written > s.maxBytes {
		_ = os.Remove(tmp)
		return nil, fmt.Errorf("artifact exceeds max size %d", s.maxBytes)
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	mimeType := artifact.MimeType
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(full))
	}
	return &StoredArtifactRef{Backend: TaskArtifactStoreModeLocal, ObjectKey: objectKey, MimeType: mimeType, Size: written}, nil
}

// Serve 以安全响应头下发本地文件（防盗刷：attachment + nosniff + 无目录列举）。
// 鉴权由路由层（HMAC capability URL / token）保证，此处只负责安全下发。
func (s *localArtifactStore) Serve(c *gin.Context, task *model.Task, ref *StoredArtifactRef) error {
	if !s.Enabled() || ref == nil {
		return ErrTaskArtifactStoreDisabled
	}
	full, err := s.pathFor(ref.ObjectKey)
	if err != nil {
		return err
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		// 已被清理或不存在 → 交回上层回退（返回错误让 controller 走上游代理）。
		return os.ErrNotExist
	}
	name := filepath.Base(full)
	if ref.MimeType != "" {
		c.Header("Content-Type", ref.MimeType)
	}
	c.Header("Content-Disposition", "attachment; filename=\""+sanitizeHeaderFilename(name)+"\"")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, max-age=300")
	// http.ServeContent 处理 Range/If-Modified-Since（断点续传友好）。
	f, err := os.Open(full)
	if err != nil {
		return err
	}
	defer f.Close()
	http.ServeContent(c.Writer, c.Request, name, info.ModTime(), f)
	return nil
}

// DeleteTaskArtifacts 删除某任务的全部落盘产物（供任务失败/取消/清理调用）。
func (s *localArtifactStore) DeleteTaskArtifacts(taskID string) error {
	if !s.Enabled() || !taskArtifactIDPattern.MatchString(taskID) {
		return nil
	}
	full, err := s.pathFor(taskID)
	if err != nil {
		return err
	}
	return os.RemoveAll(full)
}

// CleanupExpiredArtifacts 删除「目录最后修改时间早于 retainBefore」的任务产物目录。
// 语义：参考素材/产物在任务完成后保留时间由调用方（后台 loop）按保留期计算 retainBefore。
// 返回删除的目录数。只删 root 下的一级子目录，绝不递归删除 root 之外。
func (s *localArtifactStore) CleanupExpiredArtifacts(retainBefore time.Time) (int, error) {
	if !s.Enabled() {
		return 0, nil
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() || !taskArtifactIDPattern.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(retainBefore) {
			if err := os.RemoveAll(filepath.Join(s.root, e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}

// sanitizeHeaderFilename 去除可能注入响应头的字符（防 CRLF 注入）。
func sanitizeHeaderFilename(name string) string {
	replacer := strings.NewReplacer("\"", "", "\\", "", "\r", "", "\n", "", ";", "")
	out := replacer.Replace(name)
	if out == "" {
		return "artifact"
	}
	return out
}

// newLocalArtifactStore 按签名模式构建；未启用返回 disabled。
func buildTaskArtifactStore() TaskArtifactStore {
	mode := common.GetEnvOrDefaultString("TASK_ARTIFACT_STORE_MODE", "upstream")
	if mode != TaskArtifactStoreModeLocal {
		return &disabledArtifactStore{}
	}
	root := strings.TrimSpace(common.GetEnvOrDefaultString("TASK_ARTIFACT_STORE_DIR", ""))
	if root == "" {
		root = filepath.Join("data", "task-artifacts")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		common.SysError("invalid TASK_ARTIFACT_STORE_DIR: " + err.Error() + "; using upstream mode")
		return &disabledArtifactStore{}
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		common.SysError("cannot create task artifact dir: " + err.Error() + "; using upstream mode")
		return &disabledArtifactStore{}
	}
	maxBytes := int64(common.GetEnvOrDefault("TASK_ARTIFACT_MAX_FILE_MB", defaultArtifactMaxFileBytes>>20)) << 20
	if maxBytes <= 0 {
		maxBytes = defaultArtifactMaxFileBytes
	}
	common.SysLog("task artifact store: local disk at " + abs)
	return &localArtifactStore{root: abs, maxBytes: maxBytes, enabled: true}
}

// artifactRetentionFromEnv 参考素材保留期（秒），默认 300（5 分钟）。
func artifactRetentionFromEnv() time.Duration {
	secs := common.GetEnvOrDefault("TASK_ARTIFACT_RETENTION_SECONDS", int(referenceArtifactRetention/time.Second))
	if secs < 0 {
		secs = int(referenceArtifactRetention / time.Second)
	}
	return time.Duration(secs) * time.Second
}
