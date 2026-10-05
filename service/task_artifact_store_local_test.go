package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLocalStore(t *testing.T) (*localArtifactStore, string) {
	t.Helper()
	root := t.TempDir()
	s := &localArtifactStore{root: root, maxBytes: 1 << 20, enabled: true}
	return s, root
}

func testTask(id string) *model.Task { return &model.Task{TaskID: id} }

// TestLocalArtifactStorePersistResolveServe 落盘 → 解析 → 下发闭环。
func TestLocalArtifactStorePersistResolveServe(t *testing.T) {
	s, _ := newTestLocalStore(t)
	task := testTask("task_abc123")
	payload := []byte("hello-artifact")

	ref, err := s.Persist(t.Context(), task, types.TaskArtifact{Key: "video", Type: "video"}, bytes.NewReader(payload))
	require.NoError(t, err)
	require.NotNil(t, ref)
	assert.Equal(t, TaskArtifactStoreModeLocal, ref.Backend)
	assert.EqualValues(t, len(payload), ref.Size)

	got, err := s.Resolve(task, "video")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.EqualValues(t, len(payload), got.Size)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	require.NoError(t, s.Serve(c, task, got))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, payload, rec.Body.Bytes())
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")
}

// TestLocalArtifactStoreRejectsPathTraversal 防目录穿越：任务 id / key 含 .. / 分隔符一律拒绝。
func TestLocalArtifactStoreRejectsPathTraversal(t *testing.T) {
	s, _ := newTestLocalStore(t)
	for _, bad := range []string{"../etc", "..\\win", "a/b", "a\\b", ".", "..", ""} {
		_, err := s.Persist(t.Context(), &model.Task{TaskID: bad}, types.TaskArtifact{Key: "v"}, bytes.NewReader([]byte("x")))
		assert.Error(t, err, "taskID %q must be rejected", bad)
		_, err = s.Persist(t.Context(), testTask("task_x"), types.TaskArtifact{Key: bad}, bytes.NewReader([]byte("x")))
		assert.Error(t, err, "key %q must be rejected", bad)
	}
	// Resolve 对非法 key 也拒绝。
	_, err := s.Resolve(testTask("task_x"), "../secret")
	assert.Error(t, err)
}

// TestLocalArtifactStoreOversizeRejected 超限文件被拒且不留半截文件。
func TestLocalArtifactStoreOversizeRejected(t *testing.T) {
	s, root := newTestLocalStore(t)
	s.maxBytes = 8
	_, err := s.Persist(t.Context(), testTask("task_big"), types.TaskArtifact{Key: "v"}, bytes.NewReader(bytes.Repeat([]byte("A"), 100)))
	assert.Error(t, err)
	// 目录应为空（.tmp 已清）。
	entries, _ := os.ReadDir(filepath.Join(root, "task_big"))
	assert.Empty(t, entries)
}

// TestLocalArtifactStoreCleanup 5 分钟保留期清理：过期目录删、未过期保留。
func TestLocalArtifactStoreCleanup(t *testing.T) {
	s, root := newTestLocalStore(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "task_old"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "task_new"), 0o750))
	// 把 task_old 的 mtime 拨到 10 分钟前。
	old := time.Now().Add(-10 * time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(root, "task_old"), old, old))

	removed, err := s.CleanupExpiredArtifacts(time.Now().Add(-5 * time.Minute))
	require.NoError(t, err)
	assert.Equal(t, 1, removed)
	_, err = os.Stat(filepath.Join(root, "task_old"))
	assert.True(t, os.IsNotExist(err), "expired dir must be removed")
	_, err = os.Stat(filepath.Join(root, "task_new"))
	assert.NoError(t, err, "fresh dir must be kept")
}

// TestSanitizeHeaderFilename 防 CRLF 头注入。
func TestSanitizeHeaderFilename(t *testing.T) {
	assert.Equal(t, "artifact", sanitizeHeaderFilename("\r\n"))
	assert.NotContains(t, sanitizeHeaderFilename(`a"b\c`), "\"")
	assert.NotContains(t, sanitizeHeaderFilename("x;y"), ";")
}

// TestArtifactObjectKeyRejectsUnsafe 组合 key 的白名单校验。
func TestArtifactObjectKeyRejectsUnsafe(t *testing.T) {
	_, err := artifactObjectKey("task_ok", "../../etc/passwd")
	assert.Error(t, err)
	k, err := artifactObjectKey("task_ok", "video")
	require.NoError(t, err)
	assert.Equal(t, "task_ok/video", k)
	assert.True(t, strings.Contains(k, "/"))
}
