package types

type RelayFormat string

const (
	RelayFormatOpenAI                    RelayFormat = "openai"
	RelayFormatClaude                                = "claude"
	RelayFormatGemini                                = "gemini"
	RelayFormatOpenAIResponses                       = "openai_responses"
	RelayFormatOpenAIResponsesCompaction             = "openai_responses_compaction"
	RelayFormatOpenAIAlphaSearch                     = "openai_alpha_search"
	RelayFormatOpenAIAudio                           = "openai_audio"
	RelayFormatOpenAIImage                           = "openai_image"
	RelayFormatOpenAIRealtime                        = "openai_realtime"
	RelayFormatRerank                                = "rerank"
	RelayFormatEmbedding                             = "embedding"
	// RelayFormatOpenAIOps 覆盖 /v1/files、/v1/fine-tunes、/v1/batches：
	// 原始 body 透传、不做 chat 协议解析，计费按「免费元数据 / 显式配置的按次价」处理。
	RelayFormatOpenAIOps = "openai_ops"

	RelayFormatTask    = "task"
	RelayFormatMjProxy = "mj_proxy"
)
