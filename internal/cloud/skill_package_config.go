package cloud

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func SkillPackageStoreFromEnv(ctx context.Context) (*S3SkillPackageStore, bool, error) {
	bucket := strings.TrimSpace(os.Getenv("CODELOCAL_SKILL_STORAGE_BUCKET"))
	if bucket == "" {
		return nil, false, nil
	}
	pathStyle, err := optionalEnvBool("CODELOCAL_SKILL_STORAGE_PATH_STYLE", false)
	if err != nil {
		return nil, false, err
	}
	store, err := NewS3SkillPackageStore(ctx, S3SkillPackageStoreConfig{
		Bucket:          bucket,
		Prefix:          strings.TrimSpace(os.Getenv("CODELOCAL_SKILL_STORAGE_PREFIX")),
		Region:          strings.TrimSpace(os.Getenv("CODELOCAL_SKILL_STORAGE_REGION")),
		Endpoint:        strings.TrimSpace(os.Getenv("CODELOCAL_SKILL_STORAGE_ENDPOINT")),
		AccessKeyID:     strings.TrimSpace(os.Getenv("CODELOCAL_SKILL_STORAGE_ACCESS_KEY_ID")),
		SecretAccessKey: strings.TrimSpace(os.Getenv("CODELOCAL_SKILL_STORAGE_SECRET_ACCESS_KEY")),
		UsePathStyle:    pathStyle,
	})
	if err != nil {
		return nil, false, err
	}
	return store, true, nil
}

func optionalEnvBool(name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid %s boolean %q", name, raw)
	}
	return value, nil
}
