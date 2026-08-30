package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/skills"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSkillPackageStore is the durable fallback for immutable Skill
// packages when an external S3-compatible object store is not configured.
// Packages stay content-addressed and pass the same integrity checks as S3.
type PostgresSkillPackageStore struct {
	db *pgxpool.Pool
}

func NewPostgresSkillPackageStore(db *pgxpool.Pool) (*PostgresSkillPackageStore, error) {
	if db == nil {
		return nil, fmt.Errorf("skill package postgres database is required")
	}
	return &PostgresSkillPackageStore{db: db}, nil
}

func (s *PostgresSkillPackageStore) Put(ctx context.Context, pkg skills.Package) error {
	if err := skills.ValidatePackageIntegrity(pkg); err != nil {
		return err
	}
	payload, err := json.Marshal(pkg)
	if err != nil {
		return err
	}
	if len(payload) > skills.MaxStoredSkillPackageBytes {
		return fmt.Errorf("skill package exceeds %d stored bytes", skills.MaxStoredSkillPackageBytes)
	}
	_, err = s.db.Exec(ctx, `
INSERT INTO codelocal_skill_packages(package_hash, payload, size_bytes, created_at)
VALUES($1, $2, $3, $4)
ON CONFLICT (package_hash) DO NOTHING
`, pkg.PackageHash, payload, len(payload), time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("store postgres skill package: %w", err)
	}
	return nil
}

func (s *PostgresSkillPackageStore) Get(ctx context.Context, packageHash string) (skills.Package, bool, error) {
	packageHash = strings.TrimSpace(packageHash)
	if _, err := skills.PackageHashDigest(packageHash); err != nil {
		return skills.Package{}, false, err
	}
	var payload []byte
	var sizeBytes int64
	err := s.db.QueryRow(ctx, `
SELECT payload, size_bytes
FROM codelocal_skill_packages
WHERE package_hash = $1
`, packageHash).Scan(&payload, &sizeBytes)
	if err != nil {
		if err == pgx.ErrNoRows {
			return skills.Package{}, false, nil
		}
		return skills.Package{}, false, fmt.Errorf("load postgres skill package: %w", err)
	}
	if sizeBytes < 0 || sizeBytes > int64(skills.MaxStoredSkillPackageBytes) || len(payload) > skills.MaxStoredSkillPackageBytes {
		return skills.Package{}, false, fmt.Errorf("stored skill package exceeds %d bytes", skills.MaxStoredSkillPackageBytes)
	}
	var pkg skills.Package
	if err := json.Unmarshal(payload, &pkg); err != nil {
		return skills.Package{}, false, fmt.Errorf("decode postgres skill package: %w", err)
	}
	if pkg.PackageHash != packageHash {
		return skills.Package{}, false, fmt.Errorf("stored skill package address mismatch")
	}
	if err := skills.ValidatePackageIntegrity(pkg); err != nil {
		return skills.Package{}, false, fmt.Errorf("validate postgres skill package: %w", err)
	}
	return pkg, true, nil
}

func (s *PostgresSkillPackageStore) ObjectURI(packageHash string) (string, error) {
	digest, err := skills.PackageHashDigest(packageHash)
	if err != nil {
		return "", err
	}
	return "postgres://codelocal_skill_packages/sha256/" + digest, nil
}
