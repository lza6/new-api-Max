package relayconvert

import relaymedia "github.com/lza6/new-api-Max/relaykit/relayconvert/internal/media"

type MediaResolver = relaymedia.MediaResolver

func SetMediaResolver(resolver MediaResolver) {
	relaymedia.SetMediaResolver(resolver)
}
