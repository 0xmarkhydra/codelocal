package skills

import "fmt"

type ImportDisposition string

const (
	ImportActive    ImportDisposition = "active"
	ImportCandidate ImportDisposition = "candidate"
)

type ImportResult struct {
	SkillID     string            `json:"skillId"`
	Version     string            `json:"version"`
	Scope       Scope             `json:"scope"`
	Disposition ImportDisposition `json:"disposition"`
}

type PackageImporter struct {
	registry *Registry
}

func NewPackageImporter(registry *Registry) (*PackageImporter, error) {
	if registry == nil {
		return nil, fmt.Errorf("skill registry is required")
	}
	return &PackageImporter{registry: registry}, nil
}

// Import validates the portable package and stores its immutable version.
// Personal skills become active immediately for that private registry. Shared
// system/community skills always enter as non-routable candidates and require
// the existing eval/canary/promotion path before they can become current.
func (i *PackageImporter) Import(pkg Package, policy ImportPolicy) (ImportResult, error) {
	if i == nil || i.registry == nil {
		return ImportResult{}, fmt.Errorf("skill importer is not configured")
	}
	if err := ValidatePackageForImport(pkg, policy); err != nil {
		return ImportResult{}, err
	}
	manifest := pkg.Manifest
	disposition := ImportCandidate
	var err error
	if manifest.Scope == ScopePersonal {
		disposition = ImportActive
		err = i.registry.Put(manifest)
	} else {
		err = i.registry.PutCandidate(manifest)
	}
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{
		SkillID:     manifest.ID,
		Version:     manifest.Version,
		Scope:       manifest.Scope,
		Disposition: disposition,
	}, nil
}
