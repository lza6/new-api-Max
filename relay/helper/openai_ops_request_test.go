package helper

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newOpsContext(t *testing.T, method, target, contentType, body string, headers map[string]string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		c.Request.Header.Set(k, v)
	}
	return c
}

func TestGetAndValidateOpenAIOpsRequestModelSources(t *testing.T) {
	t.Run("header wins", func(t *testing.T) {
		c := newOpsContext(t, "GET", "/v1/files", "", "", map[string]string{"X-New-API-Model": "gpt-4o-mini"})
		req, err := GetAndValidateOpenAIOpsRequest(c)
		require.NoError(t, err)
		assert.Equal(t, "gpt-4o-mini", req.Model)
	})

	t.Run("json body model", func(t *testing.T) {
		c := newOpsContext(t, "POST", "/v1/fine-tunes", "application/json", `{"model":"gpt-4o-mini","training_file":"f"}`, nil)
		req, err := GetAndValidateOpenAIOpsRequest(c)
		require.NoError(t, err)
		assert.Equal(t, "gpt-4o-mini", req.Model)
		assert.Equal(t, "gpt-4o-mini", req.Model)
	})

	t.Run("multipart body is passed through untouched", func(t *testing.T) {
		body := "--b\r\nContent-Disposition: form-data; name=\"purpose\"\r\n\r\nbatch\r\n--b--\r\n"
		c := newOpsContext(t, "POST", "/v1/files", "multipart/form-data; boundary=b", body, map[string]string{"X-New-API-Model": "gpt-4o-mini"})
		req, err := GetAndValidateOpenAIOpsRequest(c)
		require.NoError(t, err)
		assert.Equal(t, "gpt-4o-mini", req.Model)
		assert.Equal(t, "gpt-4o-mini", req.Model, "multipart 也必须能通过 header 选渠道")
	})

	t.Run("default model env", func(t *testing.T) {
		t.Setenv("OPENAI_OPS_DEFAULT_MODEL", "ops-default")
		c := newOpsContext(t, "GET", "/v1/batches", "", "", nil)
		req, err := GetAndValidateOpenAIOpsRequest(c)
		require.NoError(t, err)
		assert.Equal(t, "ops-default", req.Model)
	})

	t.Run("missing model is rejected with actionable message", func(t *testing.T) {
		t.Setenv("OPENAI_OPS_DEFAULT_MODEL", "")
		c := newOpsContext(t, "GET", "/v1/batches", "", "", nil)
		_, err := GetAndValidateOpenAIOpsRequest(c)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "X-New-API-Model")
	})
}
