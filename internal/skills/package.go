package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const SkillPackageFormatVersion = 1

type Package struct {
	FormatVersion int      `json:"formatVersion"`
	Manifest      Manifest `json:"manifest"`
	Artifact      Artifact `json:"artifact"`
	PackageHash   string   `json:"packageHash"`
}

type ImportPolicy struct {
	AllowSystem    bool
	AllowPersonal  bool
	AllowCommunity bool
	AllowVerified  bool
	AllowRuntime   bool
}

func AdminImportPolicy() ImportPolicy {
	return ImportPolicy{
		AllowSystem:    true,
		AllowPersonal:  true,
		AllowCommunity: true,
		AllowVerified:  true,
		AllowRuntime:   true,
	}
}

func UserImportPolicy() ImportPolicy {
	return ImportPolicy{
		AllowPersonal:  true,
		AllowCommunity: true,
	}
}

func BuildPackage(manifest Manifest, artifact Artifact) (Package, error) {
	if err := manifest.Validate(); err != nil {
		return Package{}, err
	}
	registry, err := NewRegistry(manifest)
	if err != nil {
		return Package{}, err
	}
	if err := ValidateArtifact(artifact, registry); err != nil {
		return Package{}, err
	}
	pkg := Package{
		FormatVersion: SkillPackageFormatVersion,
		Manifest:      manifest,
		Artifact:      artifact,
	}
	pkg.PackageHash, err = packageContentHash(pkg)
	if err != nil {
		return Package{}, err
	}
	return pkg, nil
}

func ValidatePackageForImport(pkg Package, policy ImportPolicy) error {
	if pkg.FormatVersion != SkillPackageFormatVersion {
		return fmt.Errorf("unsupported skill package format %d", pkg.FormatVersion)
	}
	if err := pkg.Manifest.Validate(); err != nil {
		return err
	}
	if pkg.Artifact.Manifest.SkillID != pkg.Manifest.ID || pkg.Artifact.Manifest.SkillVersion != pkg.Manifest.Version {
		return fmt.Errorf("skill package manifest/artifact identity mismatch")
	}
	expectedHash, err := packageContentHash(pkg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(pkg.PackageHash) == "" || pkg.PackageHash != expectedHash {
		return fmt.Errorf("skill package hash mismatch")
	}
	registry, err := NewRegistry(pkg.Manifest)
	if err != nil {
		return err
	}
	if err := ValidateArtifact(pkg.Artifact, registry); err != nil {
		return err
	}
	if err := validateImportScope(pkg.Manifest.Scope, policy); err != nil {
		return err
	}
	if pkg.Manifest.Verified && !policy.AllowVerified {
		return fmt.Errorf("import policy cannot grant verified status")
	}
	if (pkg.Manifest.Kind == KindRuntime || pkg.Manifest.Kind == KindHybrid) && !policy.AllowRuntime {
		return fmt.Errorf("import policy does not allow runtime-capable skills")
	}
	return nil
}

func packageContentHash(pkg Package) (string, error) {
	payload, err := json.Marshal(struct {
		FormatVersion int      `json:"formatVersion"`
		Manifest      Manifest `json:"manifest"`
		Artifact      Artifact `json:"artifact"`
	}{
		FormatVersion: pkg.FormatVersion,
		Manifest:      pkg.Manifest,
		Artifact:      pkg.Artifact,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validateImportScope(scope Scope, policy ImportPolicy) error {
	allowed := false
	switch scope {
	case ScopeSystem:
		allowed = policy.AllowSystem
	case ScopePersonal:
		allowed = policy.AllowPersonal
	case ScopeCommunity:
		allowed = policy.AllowCommunity
	}
	if !allowed {
		return fmt.Errorf("import policy does not allow %s skills", strings.TrimSpace(string(scope)))
	}
	return nil
}
