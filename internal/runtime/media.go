package runtime

import (
	"context"
	"sync"

	"github.com/0xmarkhydra/codelocal/internal/mediatransport"
)

var (
	outboundMediaOnce sync.Once
	outboundMedia     *mediatransport.Publisher
)

func outboundMediaPublisher() *mediatransport.Publisher {
	outboundMediaOnce.Do(func() {
		outboundMedia = mediatransport.New(mediatransport.ConfigFromEnvironment(), nil)
	})
	return outboundMedia
}

func prepareOutboundMedia(ctx context.Context, result any) (any, error) {
	publisher := outboundMediaPublisher()
	if publisher == nil || !publisher.Enabled() {
		return result, nil
	}
	return publisher.Transform(ctx, result)
}
