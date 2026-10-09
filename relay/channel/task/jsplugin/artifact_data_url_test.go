package jsplugin

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lza6/new-api-Max/model"
	pluginruntime "github.com/lza6/new-api-Max/pkg/jsplugin"
	relaychannel "github.com/lza6/new-api-Max/relay/channel"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 纯函数型产物插件（在沙箱内生成字节而非转发上游 URL）用 data: URL 交付产物。
//
// 背景：此前 credentialless 分支强制要求绝对 http(s)，导致「生成型」插件
// 永远无法交付产物（TaskArtifactStore.Persist 在生产代码中零调用方，
// 插件又没有别的方式返回字节）。
func TestBuildContentRequestAcceptsBase64DataURL(t *testing.T) {
	source := strings.Replace(mockPlugin, `export function listArtifacts() { return []; }
export function buildContentRequest() { throw new Error("artifact_not_found"); }`, `export function listArtifacts(task) { return [{key: "deck", type: "file", mimeType: "application/vnd.openxmlformats-officedocument.presentationml.presentation"}]; }
export function buildContentRequest(ctx) {
  return {url: "data:application/vnd.openxmlformats-officedocument.presentationml.presentation;base64,UEsDBBQAAAAIAA==", method: "GET", credentialless: true};
}`, 1)
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	adaptor := New(plugin)
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://provider.example"}})

	descriptor, err := adaptor.BuildContentRequest(&model.Task{}, "deck", relaychannel.TaskArtifactClientRequest{Method: http.MethodGet})
	require.NoError(t, err, "base64 data URL must be accepted for generated artifacts")
	require.NotNil(t, descriptor)
	assert.True(t, strings.HasPrefix(descriptor.URL, "data:"), "URL must be passed through unchanged for the controller to decode")
	assert.Equal(t, http.MethodGet, descriptor.Method)
}

// 非 data: 的 credentialless URL 仍必须绝对 HTTP(S)（本次改动不得放宽这条）。
func TestBuildContentRequestStillRejectsRelativeURL(t *testing.T) {
	source := strings.Replace(mockPlugin, `export function listArtifacts() { return []; }
export function buildContentRequest() { throw new Error("artifact_not_found"); }`, `export function listArtifacts(task) { return [{key: "video", type: "video", mimeType: "video/mp4"}]; }
export function buildContentRequest(ctx) {
  return {url: "/relative/path.mp4", method: "GET", credentialless: true};
}`, 1)
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	adaptor := New(plugin)
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://provider.example"}})

	_, err = adaptor.BuildContentRequest(&model.Task{}, "video", relaychannel.TaskArtifactClientRequest{Method: http.MethodGet})
	require.Error(t, err, "relative credentialless URLs must stay rejected")
}

// data: 但仍禁止携带 headers/body（凭据无关请求不允许自定义头）。
func TestBuildContentRequestDataURLRejectsHeaders(t *testing.T) {
	source := strings.Replace(mockPlugin, `export function listArtifacts() { return []; }
export function buildContentRequest() { throw new Error("artifact_not_found"); }`, `export function listArtifacts(task) { return [{key: "deck", type: "file", mimeType: "text/plain"}]; }
export function buildContentRequest(ctx) {
  return {url: "data:text/plain;base64,aGk=", method: "GET", credentialless: true, headers: {"X-Bad": "1"}};
}`, 1)
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	adaptor := New(plugin)
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://provider.example"}})

	_, err = adaptor.BuildContentRequest(&model.Task{}, "deck", relaychannel.TaskArtifactClientRequest{Method: http.MethodGet})
	require.Error(t, err, "credentialless requests must not carry headers even for data URLs")
}

// data: 但未声明 base64 → 拒绝（writeVideoDataURL 只解 base64）。
func TestBuildContentRequestRejectsNonBase64DataURL(t *testing.T) {
	source := strings.Replace(mockPlugin, `export function listArtifacts() { return []; }
export function buildContentRequest() { throw new Error("artifact_not_found"); }`, `export function listArtifacts(task) { return [{key: "deck", type: "file", mimeType: "text/plain"}]; }
export function buildContentRequest(ctx) {
  return {url: "data:text/plain,plaintext-not-base64", method: "GET", credentialless: true};
}`, 1)
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	adaptor := New(plugin)
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://provider.example"}})

	_, err = adaptor.BuildContentRequest(&model.Task{}, "deck", relaychannel.TaskArtifactClientRequest{Method: http.MethodGet})
	require.Error(t, err, "only base64 data URLs are supported")
}

func TestIsBase64DataURL(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"data:text/plain;base64,aGk=", true},
		{"data:;base64,aGk=", true},
		{"  data:text/plain;base64,aGk=  ", true},
		{"data:text/plain,plain", false},
		{"data:text/plain;base64", false}, // 无逗号
		{"https://x/y", false},
		{"", false},
	} {
		assert.Equal(t, tc.want, isBase64DataURL(tc.in), tc.in)
	}
}
