package service

import (
	"testing"

	"github.com/lza6/new-api-Max/model"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/stretchr/testify/assert"
)

// §4.6.2 产物质检闸门：taskArtifactLooksUsable 的边界（保守 fail-open）。
func TestTaskArtifactLooksUsable(t *testing.T) {
	cases := []struct {
		name string
		task *model.Task
		info *relaycommon.TaskInfo
		want bool
	}{
		{"no url at all -> fail-open", &model.Task{}, nil, true},
		{"valid https url", &model.Task{}, &relaycommon.TaskInfo{Url: "https://cdn.example.com/v.mp4"}, true},
		{"valid http url", &model.Task{}, &relaycommon.TaskInfo{Url: "http://cdn.example.com/v.mp4"}, true},
		{"站点相对路径", &model.Task{}, &relaycommon.TaskInfo{Url: "/v1/tasks/task_x/artifacts/video/content"}, true},
		{"scheme-relative //host tolerated (fail-open)", &model.Task{}, &relaycommon.TaskInfo{Url: "//cdn.example.com/v"}, true},
		{"javascript scheme invalid", &model.Task{}, &relaycommon.TaskInfo{Url: "javascript:alert(1)"}, false},
		{"garbage invalid", &model.Task{}, &relaycommon.TaskInfo{Url: "not-a-url"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, taskArtifactLooksUsable(tc.task, tc.info))
		})
	}
}
