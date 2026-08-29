package cloud

import (
	"context"
	"fmt"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/skills"
)

type SkillPackageObjectStore interface {
	skills.PackageStore
	ObjectURI(packageHash string) (string, error)
}

type SkillRegistryPersistence interface {
	CreateSkillVersion(ctx context.Context, record SkillVersionRecord) (bool, error)
	SetSkillChannel(ctx context.Context, tenantUserID, skillID, channel, version string) error
}

type SkillImportService struct {
	packages SkillPackageObjectStore
	registry SkillRegistryPersistence
}

type CloudSkillImportResult struct {
	Record      SkillVersionRecord `json:"record"`
	Created     bool               `json:"created"`
	Disposition string             `json:"disposition"`
}

func NewSkillImportService(packages SkillPackageObjectStore, registry SkillRegistryPersistence) (*SkillImportService, error) {
	if packages == nil {
		return nil, fmt.Errorf("skill package object store is required")
	}
	if registry == nil {
		return nil, fmt.Errorf("skill registry persistence is required")
	}
	return &SkillImportService{packages: packages, registry: registry}, nil
}

func (s *SkillImportService) ImportPersonal(ctx context.Context, userID string, pkg skills.Package) (CloudSkillImportResult, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return CloudSkillImportResult{}, fmt.Errorf("authenticated user is required")
	}
	if pkg.Manifest.Scope != skills.ScopePersonal {
		return CloudSkillImportResult{}, fmt.Errorf("personal import requires personal skill scope")
	}
	if err := skills.ValidatePackageForImport(pkg, skills.UserImportPolicy()); err != nil {
		return CloudSkillImportResult{}, err
	}
	result, err := s.persistPackage(ctx, userID, userID, pkg, SkillVersionActive)
	if err != nil {
		return CloudSkillImportResult{}, err
	}
	if err := s.registry.SetSkillChannel(ctx, userID, pkg.Manifest.ID, "stable", pkg.Manifest.Version); err != nil {
		return CloudSkillImportResult{}, fmt.Errorf("activate personal skill: %w", err)
	}
	result.Disposition = "active"
	return result, nil
}

func (s *SkillImportService) PublishCommunity(ctx context.Context, userID string, pkg skills.Package) (CloudSkillImportResult, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return CloudSkillImportResult{}, fmt.Errorf("authenticated user is required")
	}
	if pkg.Manifest.Scope != skills.ScopeCommunity {
		return CloudSkillImportResult{}, fmt.Errorf("community publish requires community skill scope")
	}
	if err := skills.ValidatePackageForImport(pkg, skills.UserImportPolicy()); err != nil {
		return CloudSkillImportResult{}, err
	}
	result, err := s.persistPackage(ctx, "", userID, pkg, SkillVersionCandidate)
	if err != nil {
		return CloudSkillImportResult{}, err
	}
	result.Disposition = "candidate"
	return result, nil
}

func (s *SkillImportService) ImportAdminCandidate(ctx context.Context, adminUserID string, pkg skills.Package) (CloudSkillImportResult, error) {
	adminUserID = strings.TrimSpace(adminUserID)
	if adminUserID == "" {
		return CloudSkillImportResult{}, fmt.Errorf("authenticated admin is required")
	}
	if pkg.Manifest.Scope != skills.ScopeSystem && pkg.Manifest.Scope != skills.ScopeCommunity {
		return CloudSkillImportResult{}, fmt.Errorf("admin shared import requires system or community scope")
	}
	if err := skills.ValidatePackageForImport(pkg, skills.AdminImportPolicy()); err != nil {
		return CloudSkillImportResult{}, err
	}
	result, err := s.persistPackage(ctx, "", adminUserID, pkg, SkillVersionCandidate)
	if err != nil {
		return CloudSkillImportResult{}, err
	}
	result.Disposition = "candidate"
	return result, nil
}

func (s *SkillImportService) persistPackage(ctx context.Context, tenantUserID, creatorUserID string, pkg skills.Package, state SkillVersionState) (CloudSkillImportResult, error) {
	if err := s.packages.Put(ctx, pkg); err != nil {
		return CloudSkillImportResult{}, fmt.Errorf("store skill package: %w", err)
	}
	uri, err := s.packages.ObjectURI(pkg.PackageHash)
	if err != nil {
		return CloudSkillImportResult{}, fmt.Errorf("resolve skill package URI: %w", err)
	}
	record, err := NewSkillVersionRecord(tenantUserID, creatorUserID, pkg, uri, state)
	if err != nil {
		return CloudSkillImportResult{}, err
	}
	created, err := s.registry.CreateSkillVersion(ctx, record)
	if err != nil {
		return CloudSkillImportResult{}, fmt.Errorf("register skill version: %w", err)
	}
	return CloudSkillImportResult{Record: record, Created: created}, nil
}
