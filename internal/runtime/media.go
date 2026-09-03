package runtime

import (
	"context"
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/deviceauth"
	"github.com/0xmarkhydra/codelocal/internal/mediatransport"
)

func (r *Runtime) outboundMediaPublisher() *mediatransport.Publisher {
	if r == nil {
		return nil
	}
	r.mediaOnce.Do(func() {
		authorize := func(request *http.Request, body []byte) error {
			r.headers(request)
			return deviceauth.SignRequest(request, body, r.Options.Credential.DevicePrivateKey, time.Now())
		}
		r.mediaPublisher = mediatransport.New(mediatransport.Config{
			PrepareURL:         normalizeBase(r.Options.BaseURL) + "/api/client/media/presign",
			ArtifactPrepareURL: normalizeBase(r.Options.BaseURL) + "/api/client/artifacts/presign",
			Authorize:          authorize,
			Base64Fallback:     mediatransport.Base64FallbackFromEnvironment(),
		}, nil)
	})
	return r.mediaPublisher
}

func (r *Runtime) prepareOutboundMedia(ctx context.Context, result any) (any, error) {
	publisher := r.outboundMediaPublisher()
	if publisher == nil || !publisher.Enabled() {
		return result, nil
	}
	return publisher.Transform(ctx, result)
}
