package service

import (
	"context"
	"errors"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/system_setting"
	"github.com/lza6/new-api-Max/types"
)

// StoredArtifactRef describes a persisted artifact object. No reference is
// produced until a concrete storage backend is implemented.
type StoredArtifactRef struct {
	Backend   string
	Bucket    string
	ObjectKey string
	MimeType  string
	Size      int64
}

// TaskArtifactStore is the persistence boundary for generated artifact bytes.
// types.TaskArtifact is re-exported by relay/channel as channel.TaskArtifact.
type TaskArtifactStore interface {
	Enabled() bool
	Resolve(task *model.Task, artifactKey string) (*StoredArtifactRef, error)
	Persist(ctx context.Context, task *model.Task, artifact types.TaskArtifact, content io.Reader) (*StoredArtifactRef, error)
	Serve(c *gin.Context, task *model.Task, ref *StoredArtifactRef) error
}

var ErrTaskArtifactStoreDisabled = errors.New("task artifact store is disabled")

type disabledArtifactStore struct{}

func (disabledArtifactStore) Enabled() bool {
	return false
}

func (disabledArtifactStore) Resolve(*model.Task, string) (*StoredArtifactRef, error) {
	return nil, nil
}

func (disabledArtifactStore) Persist(context.Context, *model.Task, types.TaskArtifact, io.Reader) (*StoredArtifactRef, error) {
	return nil, ErrTaskArtifactStoreDisabled
}

func (disabledArtifactStore) Serve(*gin.Context, *model.Task, *StoredArtifactRef) error {
	return ErrTaskArtifactStoreDisabled
}

var taskArtifactStore TaskArtifactStore = buildTaskArtifactStore()

func init() {
	_ = system_setting.LoadTaskArtifactStoreConfig()
}

// GetTaskArtifactStore returns the process-wide artifact storage backend.
// 默认（mode != local）为 disabled（既有 upstream 代理行为，零变化）；
// TASK_ARTIFACT_STORE_MODE=local 时为站内磁盘图床。
func GetTaskArtifactStore() TaskArtifactStore {
	return taskArtifactStore
}

// LocalArtifactStore 暴露本地图床的具体类型（供清理/持久化调用）；非 local 模式返回 nil。
func LocalArtifactStore() *localArtifactStore {
	if s, ok := taskArtifactStore.(*localArtifactStore); ok {
		return s
	}
	return nil
}
