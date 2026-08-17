package cloudserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/deviceauth"
)

func (s *Server) verifySignedDeviceRequest(r *http.Request, device *cloud.Device) error {
	if device == nil || device.PublicKey == "" {
		return nil
	}
	now := time.Now()
	if err := deviceauth.VerifyRequest(r, device.PublicKey, now); err != nil {
		return err
	}
	used, err := s.Store.ConsumeDeviceNonce(r.Context(), device.CredentialID, deviceauth.RequestNonce(r), now)
	if err != nil {
		return err
	}
	if !used {
		return errors.New("device signature replay detected")
	}
	return nil
}
