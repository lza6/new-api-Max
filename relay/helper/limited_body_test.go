package helper

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadLimitedUpstreamBody(t *testing.T) {
	t.Run("normal small body", func(t *testing.T) {
		data, err := ReadLimitedUpstreamBody(strings.NewReader("hello upstream"))
		require.NoError(t, err)
		require.Equal(t, "hello upstream", string(data))
	})

	t.Run("empty body", func(t *testing.T) {
		data, err := ReadLimitedUpstreamBody(strings.NewReader(""))
		require.NoError(t, err)
		require.Empty(t, data)
	})

	t.Run("nil body errors", func(t *testing.T) {
		_, err := ReadLimitedUpstreamBody(nil)
		require.Error(t, err)
	})

	t.Run("oversized body rejected", func(t *testing.T) {
		limit := maxUpstreamResponseBytes()
		r := &lazyZeroReader{remaining: limit + 1}
		_, err := ReadLimitedUpstreamBody(r)
		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeds limit")
	})

	t.Run("body exactly at limit accepted", func(t *testing.T) {
		limit := maxUpstreamResponseBytes()
		r := &lazyZeroReader{remaining: limit}
		data, err := ReadLimitedUpstreamBody(r)
		require.NoError(t, err)
		require.Len(t, data, int(limit))
	})
}

// lazyZeroReader 惰性产生 0x00 字节，直到 remaining 耗尽，避免测试实际分配大块内存。
type lazyZeroReader struct {
	remaining int64
}

func (r *lazyZeroReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > r.remaining {
		n = r.remaining
	}
	clear(p[:n])
	r.remaining -= n
	return int(n), nil
}
