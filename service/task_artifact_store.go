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

// SetTaskArtifactStoreForTest 替换进程级产物存储后端，返回恢复函数。
//
// 仅供测试：`taskArtifactStore` 由环境变量在包初始化时构建一次，而涉及
// 「已启用 local 图床 + 文件恰好被清理」这条竞态分支的用例必须能注入一个
// 启用的 store，否则那条分支永远测不到（这也正是它曾经带着 200 空体缺陷
// 上线的原因）。生产代码没有任何调用方。
func SetTaskArtifactStoreForTest(s TaskArtifactStore) func() {
	previous := taskArtifactStore
	taskArtifactStore = s
	return func() { taskArtifactStore = previous }
}

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
