package relay

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/lza6/new-api-Max/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lza6/new-api-Max/common"
	relayconstant "github.com/lza6/new-api-Max/relay/constant"
)

func TestOpenAIOpsPathModes(t *testing.T) {
	for _, path := range []string{"/v1/files", "/v1/files/file-1/content", "/v1/fine-tunes", "/v1/batches/batch-1"} {
		assert.Equal(t, relayconstant.RelayModeOpenAIOps, relayconstant.Path2RelayMode(path), path)
	}
}

func TestOpenAIOpsIsPaid(t *testing.T) {
	assert.True(t, OpenAIOpsIsPaid(http.MethodPost, "/v1/fine-tunes"))
	assert.True(t, OpenAIOpsIsPaid(http.MethodPost, "/v1/batches"))
	assert.False(t, OpenAIOpsIsPaid(http.MethodGet, "/v1/fine-tunes"))
	assert.False(t, OpenAIOpsIsPaid(http.MethodPost, "/v1/fine-tunes/ft-1/cancel"))
	assert.False(t, OpenAIOpsIsPaid(http.MethodPost, "/v1/files"))
	assert.False(t, OpenAIOpsIsPaid(http.MethodGet, "/v1/batches/b-1"))
}

// 回归 C1：百分号编码路径必须用**解码后**的 URL.Path 判定付费。
// `POST /v1/fine%2Dtunes` 会被 Gin 路由到 fine-tunes；若用 URL.String() 做字面量
// 比较会漏判为免费，形成计费绕过。
func TestOpenAIOpsPaidUsesDecodedPath(t *testing.T) {
	u, err := url.Parse("/v1/fine%2Dtunes")
	require.NoError(t, err)
	assert.Equal(t, "/v1/fine%2Dtunes", u.String(), "URL.String() 保留编码（不能用于计费判定）")
	assert.Equal(t, "/v1/fine-tunes", u.Path, "URL.Path 是解码后的路径")
	assert.True(t, OpenAIOpsIsPaid(http.MethodPost, u.Path), "解码后必须判定为付费")
	assert.False(t, OpenAIOpsIsPaid(http.MethodPost, u.String()), "编码字符串本身不应被当作精确路径")
}

func TestOpenAIOpsPathSupported(t *testing.T) {
	for _, path := range []string{
		"/v1/files", "/v1/files/f-1", "/v1/files/f-1/content",
		"/v1/fine-tunes", "/v1/fine-tunes/ft-1", "/v1/fine-tunes/ft-1/cancel", "/v1/fine-tunes/ft-1/events",
		"/v1/batches", "/v1/batches/b-1", "/v1/batches/b-1/cancel",
	} {
		assert.Truef(t, OpenAIOpsPathSupported(path), "应允许 %s", path)
	}
	for _, path := range []string{"/v1/files/f-1/evil", "/v1/fine-tunes/ft-1/evil", "/v1/batches/b-1/evil", "/v1/other"} {
		assert.Falsef(t, OpenAIOpsPathSupported(path), "应拒绝 %s", path)
	}
}

func TestOpenAIOpsPriceData(t *testing.T) {
	price, err := OpenAIOpsPriceData(http.MethodGet, "/v1/files")
	require.NoError(t, err)
	assert.True(t, price.FreeModel, "files 元数据操作必须免费")
	assert.Zero(t, price.QuotaToPreConsume)

	t.Setenv("OPENAI_OPS_FLAT_QUOTA", "")
	_, err = OpenAIOpsPriceData(http.MethodPost, "/v1/fine-tunes")
	require.Error(t, err, "未配置按次价时必须拒绝，不能免费放行")

	t.Setenv("OPENAI_OPS_FLAT_QUOTA", "500000")
	price, err = OpenAIOpsPriceData(http.MethodPost, "/v1/fine-tunes")
	require.NoError(t, err)
	assert.False(t, price.FreeModel)
	assert.Equal(t, 500000, price.QuotaToPreConsume)
	assert.Equal(t, 500000, price.Quota)

	t.Setenv("OPENAI_OPS_FLAT_QUOTA", "999999999999")
	_, err = OpenAIOpsPriceData(http.MethodPost, "/v1/fine-tunes")
	require.Error(t, err, "超过安全上界必须拒绝")
}

func TestOpenAIOpsPriceDataRespectsMaxQuota(t *testing.T) {
	t.Setenv("OPENAI_OPS_FLAT_QUOTA", "2147483647")
	price, err := OpenAIOpsPriceData(http.MethodPost, "/v1/batches")
	require.NoError(t, err)
	assert.Equal(t, common.MaxQuota, price.QuotaToPreConsume)
}

func TestParseOpenAIOpsObjectRef(t *testing.T) {
	cases := []struct {
		path, method, kind, id, action string
	}{
		{"/v1/files", http.MethodGet, "file", "", "list"},
		{"/v1/files", http.MethodPost, "file", "", "create"},
		{"/v1/files/f-1", http.MethodGet, "file", "f-1", "get"},
		{"/v1/files/f-1", http.MethodDelete, "file", "f-1", "delete"},
		{"/v1/files/f-1/content", http.MethodGet, "file", "f-1", "content"},
		{"/v1/fine-tunes/ft-1/cancel", http.MethodPost, "fine-tune", "ft-1", "cancel"},
		{"/v1/batches/b-1/cancel", http.MethodPost, "batch", "b-1", "cancel"},
	}
	for _, tc := range cases {
		ref := parseOpenAIOpsObjectRef(tc.path, tc.method)
		assert.Equal(t, tc.kind, ref.Kind, tc.path)
		assert.Equal(t, tc.id, ref.Id, tc.path)
		assert.Equal(t, tc.action, ref.Action, tc.path)
	}
}

func TestFilterOwnedOpsListHidesUnregisteredObjects(t *testing.T) {
	previous := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.OpenAIOpsObject{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	_, err = model.RecordOpenAIOpsObject(1, "file", "owned")
	require.NoError(t, err)

	body := []byte(`{"object":"list","data":[{"id":"owned"},{"id":"someone-else"}]}`)
	out := filterOwnedOpsList(body, 1, "file")
	assert.NotContains(t, string(out), "someone-else")
	assert.Contains(t, string(out), "owned")
}
