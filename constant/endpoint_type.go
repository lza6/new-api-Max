package constant

import "github.com/lza6/new-api-Max/relaykit/types"

// EndpointType moved to types with the conversion kit; aliases keep host
// code compiling unchanged.
type EndpointType = types.EndpointType

const (
	EndpointTypeOpenAI                = types.EndpointTypeOpenAI
	EndpointTypeOpenAIResponse        = types.EndpointTypeOpenAIResponse
	EndpointTypeOpenAIResponseCompact = types.EndpointTypeOpenAIResponseCompact
	EndpointTypeOpenAIAlphaSearch     = types.EndpointTypeOpenAIAlphaSearch
	EndpointTypeAnthropic             = types.EndpointTypeAnthropic
	EndpointTypeGemini                = types.EndpointTypeGemini
	EndpointTypeJinaRerank            = types.EndpointTypeJinaRerank
	EndpointTypeImageGeneration       = types.EndpointTypeImageGeneration
	EndpointTypeEmbeddings            = types.EndpointTypeEmbeddings
	EndpointTypeOpenAIVideo           = types.EndpointTypeOpenAIVideo
)
