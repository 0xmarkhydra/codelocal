package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/skills"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type S3SkillPackageStoreConfig struct {
	Bucket          string
	Prefix          string
	Region          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
}

type skillPackageS3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type S3SkillPackageStore struct {
	client skillPackageS3API
	bucket string
	prefix string
}

func NewS3SkillPackageStore(ctx context.Context, cfg S3SkillPackageStoreConfig) (*S3SkillPackageStore, error) {
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return nil, fmt.Errorf("skill package S3 bucket is required")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}
	loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	accessKey := strings.TrimSpace(cfg.AccessKeyID)
	secretKey := strings.TrimSpace(cfg.SecretAccessKey)
	if (accessKey == "") != (secretKey == "") {
		return nil, fmt.Errorf("skill package S3 access key and secret must be configured together")
	}
	if accessKey != "" {
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load skill package S3 config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = cfg.UsePathStyle
		if endpoint := strings.TrimSpace(cfg.Endpoint); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	return newS3SkillPackageStore(client, bucket, cfg.Prefix)
}

func newS3SkillPackageStore(client skillPackageS3API, bucket, prefix string) (*S3SkillPackageStore, error) {
	if client == nil {
		return nil, fmt.Errorf("skill package S3 client is required")
	}
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, fmt.Errorf("skill package S3 bucket is required")
	}
	return &S3SkillPackageStore{
		client: client,
		bucket: bucket,
		prefix: strings.Trim(strings.TrimSpace(prefix), "/"),
	}, nil
}

func (s *S3SkillPackageStore) Put(ctx context.Context, pkg skills.Package) error {
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
	key, err := s.objectKey(pkg.PackageHash)
	if err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(payload),
		ContentType: aws.String("application/json"),
		Metadata: map[string]string{
			"codelocal-package-hash": pkg.PackageHash,
		},
	})
	if err != nil {
		return fmt.Errorf("store skill package: %w", err)
	}
	return nil
}

func (s *S3SkillPackageStore) Get(ctx context.Context, packageHash string) (skills.Package, bool, error) {
	key, err := s.objectKey(packageHash)
	if err != nil {
		return skills.Package{}, false, err
	}
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if skillPackageObjectNotFound(err) {
			return skills.Package{}, false, nil
		}
		return skills.Package{}, false, fmt.Errorf("load skill package: %w", err)
	}
	defer output.Body.Close()
	if output.ContentLength != nil && *output.ContentLength > int64(skills.MaxStoredSkillPackageBytes) {
		return skills.Package{}, false, fmt.Errorf("stored skill package exceeds %d bytes", skills.MaxStoredSkillPackageBytes)
	}
	payload, err := io.ReadAll(io.LimitReader(output.Body, int64(skills.MaxStoredSkillPackageBytes)+1))
	if err != nil {
		return skills.Package{}, false, fmt.Errorf("read stored skill package: %w", err)
	}
	if len(payload) > skills.MaxStoredSkillPackageBytes {
		return skills.Package{}, false, fmt.Errorf("stored skill package exceeds %d bytes", skills.MaxStoredSkillPackageBytes)
	}
	var pkg skills.Package
	if err := json.Unmarshal(payload, &pkg); err != nil {
		return skills.Package{}, false, fmt.Errorf("decode stored skill package: %w", err)
	}
	if pkg.PackageHash != packageHash {
		return skills.Package{}, false, fmt.Errorf("stored skill package address mismatch")
	}
	if err := skills.ValidatePackageIntegrity(pkg); err != nil {
		return skills.Package{}, false, fmt.Errorf("validate stored skill package: %w", err)
	}
	return pkg, true, nil
}

func (s *S3SkillPackageStore) ObjectURI(packageHash string) (string, error) {
	key, err := s.objectKey(packageHash)
	if err != nil {
		return "", err
	}
	return "s3://" + s.bucket + "/" + key, nil
}

func (s *S3SkillPackageStore) objectKey(packageHash string) (string, error) {
	digest, err := skills.PackageHashDigest(packageHash)
	if err != nil {
		return "", err
	}
	key := "sha256/" + digest[:2] + "/" + digest + ".skill.json"
	if s.prefix != "" {
		key = s.prefix + "/" + key
	}
	return key, nil
}

func skillPackageObjectNotFound(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "NoSuchKey", "NotFound", "404":
		return true
	default:
		return false
	}
}
