package helper

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	relayconstant "github.com/lza6/new-api-Max/relay/constant"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/stretchr/testify/require"
)

// TestGetAndValidOpenAIImageRequestMultipartStream verifies multipart image
// edit parsing: the stream field is parsed and validated, and the request body
// stays replayable for the upstream request.
func TestGetAndValidOpenAIImageRequestMultipartStream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newContext := func(t *testing.T, streamValue string, withImage bool) (*gin.Context, string) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("model", "gpt-image-1"))
		require.NoError(t, writer.WriteField("prompt", "edit this image"))
		require.NoError(t, writer.WriteField("stream", streamValue))
		if withImage {
			part, err := writer.CreateFormFile("image", "input.png")
			require.NoError(t, err)
			_, err = part.Write([]byte("fake image"))
			require.NoError(t, err)
		}
		require.NoError(t, writer.Close())
		originalBody := body.String()

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return c, originalBody
	}

	t.Run("valid stream value keeps body replayable", func(t *testing.T) {
		c, originalBody := newContext(t, "true", true)

		req, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		require.NotNil(t, req.Stream)
		require.True(t, *req.Stream)
		require.True(t, req.IsStream(c.Request))

		bodyAfterValidation, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Equal(t, originalBody, string(bodyAfterValidation))

		form, err := common.ParseMultipartFormReusable(c)
		require.NoError(t, err)
		require.Equal(t, "true", url.Values(form.Value).Get("stream"))
		require.Len(t, form.File["image"], 1)
	})

	t.Run("invalid stream value is rejected", func(t *testing.T) {
		c, _ := newContext(t, "notabool", false)

		_, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid stream value")
	})
}

// TestGetAndValidOpenAIImageRequestNBounds guards the billing invariant that
// the image generation count can never reach quota calculation with a value
// large enough to overflow int64 into a negative charge.
func TestGetAndValidOpenAIImageRequestNBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newJSONContext := func(t *testing.T, body string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		return c
	}

	boundErr := fmt.Sprintf("n must be an integer between 1 and %d", dto.MaxImageN)

	tests := []struct {
		name    string
		body    string
		wantErr string
		wantN   uint
	}{
		{
			name:    "overflowed uint64 n is rejected",
			body:    `{"model":"gpt-image-1","prompt":"a cat","n":18446744073686646784}`,
			wantErr: boundErr,
		},
		{
			name:    "n above max is rejected",
			body:    fmt.Sprintf(`{"model":"gpt-image-1","prompt":"a cat","n":%d}`, dto.MaxImageN+1),
			wantErr: boundErr,
		},
		{
			name:  "n at max is accepted",
			body:  fmt.Sprintf(`{"model":"gpt-image-1","prompt":"a cat","n":%d}`, dto.MaxImageN),
			wantN: dto.MaxImageN,
		},
		{
			name:  "explicit n is accepted",
			body:  `{"model":"gpt-image-1","prompt":"a cat","n":3}`,
			wantN: 3,
		},
		{
			name:  "zero n defaults to 1",
			body:  `{"model":"gpt-image-1","prompt":"a cat","n":0}`,
			wantN: 1,
		},
		{
			name:  "absent n defaults to 1",
			body:  `{"model":"gpt-image-1","prompt":"a cat"}`,
			wantN: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newJSONContext(t, tt.body)
			req, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesGenerations)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, req.N)
			require.Equal(t, tt.wantN, *req.N)
			require.Equal(t, float64(tt.wantN), req.GetTokenCountMeta().BillingRatios["n"])
		})
	}

	t.Run("negative multipart n is rejected", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("model", "gpt-image-1"))
		require.NoError(t, writer.WriteField("prompt", "edit this image"))
		require.NoError(t, writer.WriteField("n", "-22904832"))
		require.NoError(t, writer.Close())

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		_, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.Error(t, err)
		require.Contains(t, err.Error(), boundErr)
	})
}

// TestGetAndValidImageVariationsRequest 回归：`/v1/images/variations` 的 multipart
// 校验必须与 images/edits 同族生效，尤其是 `n` 的**上界**。
//
// 为什么这条上界是计费安全问题（AGENTS.md「Billing safety invariants」）：
// `n` 直接作为计费乘数，无界时用户可以传 `n=1e9` 让单次请求的计费算术溢出，
// 而 quita 转换对非负整数是"合法"输入——必须在校验层就拒掉。
func TestGetAndValidImageVariationsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newVariationsContext := func(t *testing.T, fields map[string]string) *gin.Context {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for k, v := range fields {
			require.NoError(t, writer.WriteField(k, v))
		}
		part, err := writer.CreateFormFile("image", "source.png")
		require.NoError(t, err)
		_, err = part.Write([]byte("variation source"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/variations", &body)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return c
	}

	t.Run("parses multipart fields and defaults n to 1", func(t *testing.T) {
		c := newVariationsContext(t, map[string]string{
			"model": "dall-e-2",
			"size":  "1024x1024",
		})
		req, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesVariations)
		require.NoError(t, err)
		require.Equal(t, "dall-e-2", req.Model)
		require.Equal(t, "1024x1024", req.Size)
		require.NotNil(t, req.N)
		require.Equal(t, uint(1), *req.N, "未提供 n 时应默认 1，而不是 nil 或 0")
	})

	t.Run("accepts n at the inclusive upper bound", func(t *testing.T) {
		c := newVariationsContext(t, map[string]string{
			"model": "dall-e-2",
			"n":     fmt.Sprintf("%d", dto.MaxImageN),
		})
		req, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesVariations)
		require.NoError(t, err)
		require.Equal(t, uint(dto.MaxImageN), *req.N)
	})

	t.Run("rejects n above the bound", func(t *testing.T) {
		c := newVariationsContext(t, map[string]string{
			"model": "dall-e-2",
			"n":     fmt.Sprintf("%d", dto.MaxImageN+1),
		})
		_, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesVariations)
		require.Error(t, err, "n 超上界必须在校验层拒绝，不能进入计费算术")
	})

	t.Run("rejects absurd n that would overflow quota math", func(t *testing.T) {
		c := newVariationsContext(t, map[string]string{
			"model": "dall-e-2",
			"n":     "1000000000",
		})
		_, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesVariations)
		require.Error(t, err)
	})

	t.Run("rejects non-numeric n", func(t *testing.T) {
		c := newVariationsContext(t, map[string]string{
			"model": "dall-e-2",
			"n":     "two",
		})
		_, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesVariations)
		require.Error(t, err)
	})
}
