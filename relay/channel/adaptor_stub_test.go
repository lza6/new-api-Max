package channel_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/relay/channel/mistral"
	"github.com/lza6/new-api-Max/relay/channel/zhipu"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/stretchr/testify/require"
)

type claudeConverter interface {
	ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error)
}

// Regression: these adaptors previously panicked ("implement me") instead of
// returning an error, which would surface as a 500 when a Claude-protocol
// request was routed to them.
func TestUnimplementedClaudeConvertersReturnError(t *testing.T) {
	converters := map[string]claudeConverter{
		"zhipu":   &zhipu.Adaptor{},
		"mistral": &mistral.Adaptor{},
	}
	for name, c := range converters {
		t.Run(name, func(t *testing.T) {
			_, err := c.ConvertClaudeRequest(nil, nil, nil)
			require.Error(t, err)
			require.NotPanics(t, func() {
				_, _ = c.ConvertClaudeRequest(nil, nil, nil)
			})
		})
	}
}
