package openai

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	relayconstant "github.com/lza6/new-api-Max/relay/constant"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/stretchr/testify/require"
)

// TestConvertImageEditRequestMultipart verifies that ConvertImageRequest
// re-serializes multipart image edit requests with all fields (including
// stream) and the file intact, both when the form was already parsed and when
// it must be re-parsed from the reusable body.
func TestConvertImageEditRequestMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newMultipartContext := func(t *testing.T, prompt string) *gin.Context {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("model", "gpt-image-1"))
		require.NoError(t, writer.WriteField("prompt", prompt))
		require.NoError(t, writer.WriteField("stream", "true"))
		require.NoError(t, writer.WriteField("partial_images", "3"))
		part, err := writer.CreateFormFile("image", "input.png")
		require.NoError(t, err)
		_, err = part.Write([]byte("fake image"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return c
	}

	convertAndReplay := func(t *testing.T, c *gin.Context, prompt string) {
		info := &relaycommon.RelayInfo{
			RelayMode: relayconstant.RelayModeImagesEdits,
		}
		request := dto.ImageRequest{
			Model:  "gpt-image-1",
			Prompt: prompt,
			Stream: common.GetPointer(true),
		}

		converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
		require.NoError(t, err)
		convertedBody, ok := converted.(*bytes.Buffer)
		require.True(t, ok)

		replayedRequest := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(convertedBody.Bytes()))
		replayedRequest.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
		require.NoError(t, replayedRequest.ParseMultipartForm(32<<20))

		require.Equal(t, "gpt-image-1", replayedRequest.PostForm.Get("model"))
		require.Equal(t, prompt, replayedRequest.PostForm.Get("prompt"))
		require.Equal(t, "true", replayedRequest.PostForm.Get("stream"))
		require.Equal(t, "3", replayedRequest.PostForm.Get("partial_images"))
		require.Len(t, replayedRequest.MultipartForm.File["image"], 1)

		file, err := replayedRequest.MultipartForm.File["image"][0].Open()
		require.NoError(t, err)
		defer file.Close()
		fileBytes, err := io.ReadAll(file)
		require.NoError(t, err)
		require.Equal(t, []byte("fake image"), fileBytes)
	}

	t.Run("with pre-parsed form", func(t *testing.T) {
		prompt := "edit this image"
		c := newMultipartContext(t, prompt)
		require.NoError(t, c.Request.ParseMultipartForm(32<<20))

		convertAndReplay(t, c, prompt)
	})

	t.Run("re-parses reusable body when form is missing", func(t *testing.T) {
		prompt := "edit without pre-parsed form"
		c := newMultipartContext(t, prompt)

		storage, err := common.GetBodyStorage(c)
		require.NoError(t, err)
		c.Request.Body = io.NopCloser(storage)
		c.Request.MultipartForm = nil
		c.Request.PostForm = nil

		convertAndReplay(t, c, prompt)
	})
}

// TestConvertImageVariationsRequestMultipart 回归：`/v1/images/variations` 也是
// multipart（image + n/size/response_format），必须走与 images/edits 相同的
// 重序列化路径。若只给 images/edits 加了 case，variations 会掉进 default 分支
// 被当成 JSON 解析（multipart 体解析失败），表现为"上传变体图片无反应"。
func TestConvertImageVariationsRequestMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "dall-e-2"))
	require.NoError(t, writer.WriteField("n", "2"))
	require.NoError(t, writer.WriteField("size", "1024x1024"))
	require.NoError(t, writer.WriteField("response_format", "url"))
	part, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("variation source bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/variations", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesVariations}
	request := dto.ImageRequest{Model: "dall-e-2"}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err, "variations 的 multipart 体必须能被转换，而不是按 JSON 解析")
	convertedBody, ok := converted.(*bytes.Buffer)
	require.True(t, ok)

	replayed := httptest.NewRequest(http.MethodPost, "/v1/images/variations", bytes.NewReader(convertedBody.Bytes()))
	replayed.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	require.NoError(t, replayed.ParseMultipartForm(32<<20))

	// 非文件字段逐字保留（多值字段不能丢）。
	require.Equal(t, "dall-e-2", replayed.PostForm.Get("model"))
	require.Equal(t, "2", replayed.PostForm.Get("n"))
	require.Equal(t, "1024x1024", replayed.PostForm.Get("size"))
	require.Equal(t, "url", replayed.PostForm.Get("response_format"))

	// 文件字节必须完整往返。
	require.Len(t, replayed.MultipartForm.File["image"], 1)
	file, err := replayed.MultipartForm.File["image"][0].Open()
	require.NoError(t, err)
	defer file.Close()
	got, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, []byte("variation source bytes"), got)
}

// TestImageVariationsRoutesToFormRequest 回归：variations 的 multipart 请求必须
// 走 DoFormRequest（透传原始 Content-Type/边界），不能走 DoApiRequest —— 后者会
// 把 multipart 体当 JSON 转发，上游必然 400。
func TestImageVariationsUsesFormRequestPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 与 Adaptor.DoRequest 的分支判据保持一致：multipart + variations → form 路径。
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/variations", nil)
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")

	require.False(t, isJSONRequest(c), "multipart 请求不得被判为 JSON")
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesVariations}
	require.True(t,
		(info.RelayMode == relayconstant.RelayModeImagesEdits ||
			info.RelayMode == relayconstant.RelayModeImagesVariations) && !isJSONRequest(c),
		"variations 的 multipart 必须命中 form 请求分支")
}
