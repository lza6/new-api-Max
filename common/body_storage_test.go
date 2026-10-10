package common

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReplayableBodyReaderKeepsStorageLifecycleWithCaller(t *testing.T) {
	payload := []byte(`{"model":"test-model","input":"hello"}`)
	storage, err := CreateBodyStorage(payload)
	require.NoError(t, err)
	defer storage.Close()

	body := NewReplayableBodyReader(storage)
	assert.EqualValues(t, len(payload), body.Size())
	_, exposesCloser := any(body).(io.Closer)
	assert.False(t, exposesCloser, "the request body must not expose the storage closer")

	req, err := http.NewRequest(http.MethodPost, "https://example.com", body)
	require.NoError(t, err)
	require.NoError(t, req.Body.Close())

	replayBody, err := body.NewReader()
	require.NoError(t, err, "closing the HTTP request body must not close the storage")
	replay, err := io.ReadAll(replayBody)
	require.NoError(t, err)
	require.NoError(t, replayBody.Close())
	assert.Equal(t, payload, replay)

	require.NoError(t, storage.Close())
	_, err = body.NewReader()
	require.ErrorIs(t, err, ErrStorageClosed)
}

// Batch-9 / G2：出站响应读取上限的边界契约。
// 超限必须报错且**不返回部分数据** —— 静默截断的上游 JSON 会变成难排查的解析错误，
// 比直接报「响应过大」糟得多。
func TestReadAllLimited(t *testing.T) {
	const limit int64 = 16
	atLimit := strings.Repeat("a", int(limit))

	t.Run("exactly at the limit passes", func(t *testing.T) {
		got, err := ReadAllLimited(strings.NewReader(atLimit), limit)
		require.NoError(t, err)
		assert.Equal(t, atLimit, string(got))
	})
	t.Run("one byte over the limit fails without partial data", func(t *testing.T) {
		got, err := ReadAllLimited(strings.NewReader(atLimit+"b"), limit)
		require.ErrorIs(t, err, ErrReadAllLimitExceeded)
		assert.Nil(t, got, "超限不得返回部分数据")
	})
	t.Run("empty response is fine", func(t *testing.T) {
		got, err := ReadAllLimited(strings.NewReader(""), limit)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("nil reader is rejected", func(t *testing.T) {
		_, err := ReadAllLimited(nil, limit)
		require.Error(t, err)
	})
	t.Run("non-positive limit is a config error, not an unbounded read", func(t *testing.T) {
		_, err := ReadAllLimited(strings.NewReader("x"), 0)
		require.Error(t, err, "limit<=0 必须报错：静默退化成无界读取正是本次要修的问题")
	})
}
