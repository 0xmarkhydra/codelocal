package cloudserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

const (
	defaultMediaURLTTL    = 3 * time.Minute
	defaultMediaRetention = 24 * time.Hour
	defaultMediaMaxBytes  = int64(12 << 20)
	mediaCleanupInterval  = time.Hour
	mediaCleanupMaxPages  = 5
)

var mediaHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type mediaPrepareRequest struct {
	SHA256      string `json:"sha256"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

type mediaUploadGrant struct {
	Required bool                `json:"required"`
	URL      string              `json:"url,omitempty"`
	Method   string              `json:"method,omitempty"`
	Headers  map[string][]string `json:"headers,omitempty"`
}

type mediaPrepareResponse struct {
	ImageRef     string           `json:"imageRef"`
	Key          string           `json:"key"`
	URL          string           `json:"url"`
	ContentType  string           `json:"contentType"`
	Size         int64            `json:"size"`
	SHA256       string           `json:"sha256"`
	Deduplicated bool             `json:"deduplicated"`
	ExpiresAt    int64            `json:"expiresAt"`
	ExpiresIn    int64            `json:"expiresIn"`
	Visibility   string           `json:"visibility"`
	Upload       mediaUploadGrant `json:"upload"`
}

type s3MediaStore struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
	prefix    string
	urlTTL    time.Duration
	retention time.Duration
	maxBytes  int64
}

func envFirst(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func mediaDurationSeconds(name string, fallback, min, max time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	value := time.Duration(seconds) * time.Second
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func mediaMaxBytes() int64 {
	raw := strings.TrimSpace(os.Getenv("CODELOCAL_MEDIA_MAX_MB"))
	if raw == "" {
		return defaultMediaMaxBytes
	}
	mb, err := strconv.Atoi(raw)
	if err != nil {
		return defaultMediaMaxBytes
	}
	if mb < 1 {
		mb = 1
	}
	if mb > 25 {
		mb = 25
	}
	return int64(mb) << 20
}

func normalizeMediaPrefix(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return "codelocal"
	}
	var out strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '/', r == '_', r == '-':
			out.WriteRune(r)
		default:
			out.WriteByte('-')
		}
	}
	cleaned := strings.Trim(out.String(), "/")
	if cleaned == "" {
		return "codelocal"
	}
	return cleaned
}

func newS3MediaStoreFromEnvironment(ctx context.Context) (*s3MediaStore, error) {
	endpoint := envFirst("CODELOCAL_MEDIA_S3_ENDPOINT", "S3_ENDPOINT")
	bucket := envFirst("CODELOCAL_MEDIA_S3_BUCKET", "S3_BUCKET")
	accessKey := envFirst("CODELOCAL_MEDIA_S3_ACCESS_KEY_ID", "S3_ACCESS_KEY_ID")
	secretKey := envFirst("CODELOCAL_MEDIA_S3_SECRET_ACCESS_KEY", "S3_SECRET_ACCESS_KEY")
	region := envFirst("CODELOCAL_MEDIA_S3_REGION", "S3_REGION")
	if region == "" {
		region = "us-east-1"
	}
	configured := endpoint != "" || bucket != "" || accessKey != "" || secretKey != ""
	if !configured {
		return nil, nil
	}
	missing := []string{}
	for name, value := range map[string]string{
		"endpoint": endpoint, "bucket": bucket, "accessKey": accessKey, "secretKey": secretKey,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("CodeLocal media S3 configuration is incomplete: missing %s", strings.Join(missing, ", "))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load CodeLocal media S3 config: %w", err)
	}
	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(strings.TrimRight(endpoint, "/"))
		options.UsePathStyle = true
	})
	retention := mediaDurationSeconds("CODELOCAL_MEDIA_RETENTION_SECONDS", defaultMediaRetention, time.Hour, 7*24*time.Hour)
	return &s3MediaStore{
		client:    client,
		presigner: s3.NewPresignClient(client),
		bucket:    bucket,
		prefix:    normalizeMediaPrefix(envFirst("CODELOCAL_MEDIA_PREFIX")),
		urlTTL:    mediaDurationSeconds("CODELOCAL_MEDIA_URL_TTL_SECONDS", defaultMediaURLTTL, time.Minute, 5*time.Minute),
		retention: retention,
		maxBytes:  mediaMaxBytes(),
	}, nil
}

func mediaExtension(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ""
	}
}

func mediaOwnerScope(userID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(userID)))
	return hex.EncodeToString(digest[:8])
}

func mediaObjectKey(prefix, userID, hash, contentType string) string {
	return fmt.Sprintf("%s/users/%s/sha256/%s/%s%s", normalizeMediaPrefix(prefix), mediaOwnerScope(userID), hash[:2], hash, mediaExtension(contentType))
}

func validateMediaPrepare(input mediaPrepareRequest, maxBytes int64) error {
	input.SHA256 = strings.ToLower(strings.TrimSpace(input.SHA256))
	if !mediaHashPattern.MatchString(input.SHA256) {
		return errors.New("invalid image sha256")
	}
	if mediaExtension(input.ContentType) == "" {
		return errors.New("unsupported image content type")
	}
	if input.Size <= 0 || input.Size > maxBytes {
		return fmt.Errorf("image size must be between 1 and %d bytes", maxBytes)
	}
	return nil
}

func isMissingS3Object(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchObject":
			return true
		}
	}
	var responseErr *awshttp.ResponseError
	return errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == http.StatusNotFound
}

func (m *s3MediaStore) objectExists(ctx context.Context, key string) (bool, error) {
	_, err := m.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(m.bucket), Key: aws.String(key)})
	if err == nil {
		return true, nil
	}
	if isMissingS3Object(err) {
		return false, nil
	}
	return false, err
}

func copySignedHeaders(input http.Header) map[string][]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string][]string, len(input))
	for key, values := range input {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func (m *s3MediaStore) prepare(ctx context.Context, userID string, input mediaPrepareRequest) (mediaPrepareResponse, error) {
	input.SHA256 = strings.ToLower(strings.TrimSpace(input.SHA256))
	input.ContentType = strings.ToLower(strings.TrimSpace(input.ContentType))
	if err := validateMediaPrepare(input, m.maxBytes); err != nil {
		return mediaPrepareResponse{}, err
	}
	key := mediaObjectKey(m.prefix, userID, input.SHA256, input.ContentType)
	exists, err := m.objectExists(ctx, key)
	if err != nil {
		return mediaPrepareResponse{}, fmt.Errorf("check visual object: %w", err)
	}
	getRequest, err := m.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(m.bucket), Key: aws.String(key), ResponseContentType: aws.String(input.ContentType),
	}, func(options *s3.PresignOptions) { options.Expires = m.urlTTL })
	if err != nil {
		return mediaPrepareResponse{}, fmt.Errorf("presign visual read: %w", err)
	}
	upload := mediaUploadGrant{Required: !exists}
	if !exists {
		putRequest, presignErr := m.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(m.bucket), Key: aws.String(key), ContentType: aws.String(input.ContentType),
			CacheControl: aws.String("private, no-store"), Metadata: map[string]string{"content-sha256": input.SHA256, "source": "codelocal-computer"},
		}, func(options *s3.PresignOptions) { options.Expires = m.urlTTL })
		if presignErr != nil {
			return mediaPrepareResponse{}, fmt.Errorf("presign visual upload: %w", presignErr)
		}
		upload.URL = putRequest.URL
		upload.Method = putRequest.Method
		upload.Headers = copySignedHeaders(putRequest.SignedHeader)
	}
	return mediaPrepareResponse{
		ImageRef: "sha256:" + input.SHA256, Key: key, URL: getRequest.URL, ContentType: input.ContentType, Size: input.Size,
		SHA256: input.SHA256, Deduplicated: exists, ExpiresAt: time.Now().Add(m.urlTTL).UnixMilli(), ExpiresIn: int64(m.urlTTL.Seconds()),
		Visibility: "private-signed-url", Upload: upload,
	}, nil
}

func (m *s3MediaStore) cleanup(ctx context.Context) error {
	cutoff := time.Now().Add(-m.retention)
	paginator := s3.NewListObjectsV2Paginator(m.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(m.bucket), Prefix: aws.String(normalizeMediaPrefix(m.prefix) + "/users/"),
	})
	pages := 0
	deleted := 0
	for paginator.HasMorePages() && pages < mediaCleanupMaxPages {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		pages++
		for _, object := range page.Contents {
			if object.Key == nil || object.LastModified == nil || !object.LastModified.Before(cutoff) {
				continue
			}
			if _, err := m.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(m.bucket), Key: object.Key}); err != nil {
				return err
			}
			deleted++
		}
	}
	if deleted > 0 {
		slog.Info("expired CodeLocal visual objects removed", "count", deleted)
	}
	return nil
}

func (m *s3MediaStore) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(mediaCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			if err := m.cleanup(cleanupCtx); err != nil && cleanupCtx.Err() == nil {
				slog.Warn("CodeLocal visual cleanup failed", "error", err)
			}
			cancel()
		}
	}
}

func (s *Server) mediaPresign(w http.ResponseWriter, r *http.Request) {
	device, err := s.authenticateDevice(r)
	if err != nil || device == nil {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "device_auth_failed"})
		return
	}
	if s.Media == nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "media_not_configured"})
		return
	}
	var input mediaPrepareRequest
	if webutil.DecodeJSON(r, 16<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	response, err := s.Media.prepare(r.Context(), device.UserID, input)
	if err != nil {
		if strings.Contains(err.Error(), "invalid image") || strings.Contains(err.Error(), "unsupported image") || strings.Contains(err.Error(), "image size") {
			webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_media", "message": err.Error()})
			return
		}
		slog.Warn("CodeLocal visual presign failed", "error", err, "deviceId", device.DeviceID)
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "media_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, response)
}
