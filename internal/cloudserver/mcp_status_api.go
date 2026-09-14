package cloudserver

import (
	"net/http"
	"regexp"

	"github.com/0xmarkhydra/codelocal/internal/mcpconfig"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func (s *Server) mcpStatusSync(w http.ResponseWriter, r *http.Request) {
	device, err := s.authenticateDevice(r)
	if err != nil || device == nil {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "device_auth_failed"})
		return
	}
	var input struct {
		Actual []mcpconfig.Actual `json:"actual"`
	}
	if webutil.DecodeJSON(r, 128<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if len(input.Actual) > 256 {
		input.Actual = input.Actual[:256]
	}
	validName := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	updated := 0
	for _, actual := range input.Actual {
		if !validName.MatchString(actual.Name) {
			continue
		}
		if err := s.Store.UpdateLocalMCPActual(r.Context(), device.UserID, device.DeviceID, actual); err == nil {
			updated++
		}
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "updated": updated})
}
