package plugins

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// ManagedCredentialReference returns the deterministic, non-secret runtime
// handle used for a Plugin bearer token. The token value never enters the
// manifest, connection record, MCP registry, or model-visible result.
func ManagedCredentialReference(pluginID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(pluginID)))
	return "CODELOCAL_PLUGIN_" + strings.ToUpper(hex.EncodeToString(digest[:8])) + "_TOKEN"
}

// IsManagedCredentialReference prevents a caller from using Plugin cleanup to
// delete an unrelated runtime secret.
func IsManagedCredentialReference(pluginID, reference string) bool {
	expected := []byte(ManagedCredentialReference(pluginID))
	actual := []byte(strings.TrimSpace(reference))
	return len(expected) == len(actual) && subtle.ConstantTimeCompare(expected, actual) == 1
}
