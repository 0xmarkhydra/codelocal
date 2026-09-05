package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CanonicalJSON returns the stable wire representation used for immutable
// plugin-version identity. Manifest avoids interface{} fields so serialization
// does not depend on caller-specific map ordering or dynamic values.
func CanonicalJSON(manifest Manifest) ([]byte, error) {
	if err := ValidateManifest(manifest); err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

// ManifestHash returns the SHA-256 hash of CanonicalJSON.
func ManifestHash(manifest Manifest) (string, error) {
	raw, err := CanonicalJSON(manifest)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
