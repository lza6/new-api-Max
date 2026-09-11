package relay

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	kitreasoning "github.com/lza6/new-api-Max/relaykit/relayconvert/reasoning"
	"github.com/lza6/new-api-Max/relaykit/types"
)

func newConvertRequestFailedError(c *gin.Context, info *relaycommon.RelayInfo, err error) *types.NewAPIError {
	var loss *types.ConversionLossError
	if errors.As(err, &loss) {
		info.RecordConversionDiagnostics(c, loss.Diagnostics)
		return types.NewErrorWithStatusCode(err, types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if kitreasoning.IsClientError(err) {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
}
