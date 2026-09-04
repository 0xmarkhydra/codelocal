package cloudserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const defaultVideoArtifactMaxBytes = int64(512 << 20)

var artifactOwnerPattern = regexp.MustCompile(`^[a-f0-9]{16}$`)

type videoArtifactPrepareRequest struct {
	SHA256      string `json:"sha256"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	Name        string `json:"name,omitempty"`
}

type videoArtifactPrepareResponse struct {
	ArtifactID   string           `json:"artifactId"`
	PublicURL    string           `json:"publicUrl"`
	ContentType  string           `json:"contentType"`
	Size         int64            `json:"size"`
	SHA256       string           `json:"sha256"`
	Deduplicated bool             `json:"deduplicated"`
	Upload       mediaUploadGrant `json:"upload"`
}

func videoArtifactMaxBytes() int64 {
	raw := strings.TrimSpace(os.Getenv("CODELOCAL_VIDEO_ARTIFACT_MAX_MB"))
	if raw == "" {
		return defaultVideoArtifactMaxBytes
	}
	mb, err := strconv.Atoi(raw)
	if err != nil {
		return defaultVideoArtifactMaxBytes
	}
	if mb < 1 {
		mb = 1
	}
	if mb > 2048 {
		mb = 2048
	}
	return int64(mb) << 20
}

func validateVideoArtifactPrepare(input videoArtifactPrepareRequest, maxBytes int64) error {
	input.SHA256 = strings.ToLower(strings.TrimSpace(input.SHA256))
	input.ContentType = strings.ToLower(strings.TrimSpace(input.ContentType))
	if !mediaHashPattern.MatchString(input.SHA256) {
		return errors.New("invalid video sha256")
	}
	if input.ContentType != "video/mp4" {
		return errors.New("unsupported video content type")
	}
	if input.Size <= 0 || input.Size > maxBytes {
		return fmt.Errorf("video size must be between 1 and %d bytes", maxBytes)
	}
	return nil
}

func videoArtifactObjectKey(prefix, ownerScope, hash string) string {
	return fmt.Sprintf("%s/artifacts/users/%s/sha256/%s/%s.mp4", normalizeMediaPrefix(prefix), ownerScope, hash[:2], hash)
}

func videoArtifactID(ownerScope, hash string) string {
	return "video_" + ownerScope + "_" + hash[:24]
}

func videoArtifactPublicURL(ownerScope, hash, key string) string {
	if base := strings.TrimRight(strings.TrimSpace(os.Getenv("CODELOCAL_ARTIFACT_PUBLIC_BASE_URL")), "/"); base != "" {
		return base + "/" + strings.TrimLeft(key, "/")
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/")
	if base == "" {
		base = "http://localhost:" + defaultPort()
	}
	return base + "/api/public/artifacts/" + ownerScope + "/" + hash + ".mp4"
}

func (s *Server) prepareVideoArtifact(ctx context.Context, userID string, input videoArtifactPrepareRequest) (videoArtifactPrepareResponse, error) {
	if s == nil || s.Media == nil {
		return videoArtifactPrepareResponse{}, errors.New("artifact storage is not configured")
	}
	input.SHA256 = strings.ToLower(strings.TrimSpace(input.SHA256))
	input.ContentType = strings.ToLower(strings.TrimSpace(input.ContentType))
	if err := validateVideoArtifactPrepare(input, videoArtifactMaxBytes()); err != nil {
		return videoArtifactPrepareResponse{}, err
	}
	ownerScope := mediaOwnerScope(userID)
	key := videoArtifactObjectKey(s.Media.prefix, ownerScope, input.SHA256)
	exists, err := s.Media.objectExists(ctx, key)
	if err != nil {
		return videoArtifactPrepareResponse{}, fmt.Errorf("check video artifact: %w", err)
	}
	response := videoArtifactPrepareResponse{
		ArtifactID:   videoArtifactID(ownerScope, input.SHA256),
		PublicURL:    videoArtifactPublicURL(ownerScope, input.SHA256, key),
		ContentType:  "video/mp4",
		Size:         input.Size,
		SHA256:       input.SHA256,
		Deduplicated: exists,
		Upload:       mediaUploadGrant{Required: !exists},
	}
	if exists {
		return response, nil
	}
	putRequest, err := s.Media.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.Media.bucket), Key: aws.String(key), ContentType: aws.String("video/mp4"),
		CacheControl: aws.String("public, max-age=31536000, immutable"),
		Metadata:     map[string]string{"content-sha256": input.SHA256, "source": "codelocal-video-studio"},
	}, func(options *s3.PresignOptions) { options.Expires = s.Media.urlTTL })
	if err != nil {
		return videoArtifactPrepareResponse{}, fmt.Errorf("presign video artifact upload: %w", err)
	}
	response.Upload.URL = putRequest.URL
	response.Upload.Method = putRequest.Method
	response.Upload.Headers = copySignedHeaders(putRequest.SignedHeader)
	return response, nil
}

func (s *Server) videoArtifactPresign(w http.ResponseWriter, r *http.Request) {
	device, err := s.authenticateDevice(r)
	if err != nil || device == nil {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "device_auth_failed"})
		return
	}
	if s.Media == nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "artifact_storage_not_configured"})
		return
	}
	var input videoArtifactPrepareRequest
	if webutil.DecodeJSON(r, 16<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	response, err := s.prepareVideoArtifact(r.Context(), device.UserID, input)
	if err != nil {
		if strings.Contains(err.Error(), "video sha256") || strings.Contains(err.Error(), "video content type") || strings.Contains(err.Error(), "video size") {
			webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_video_artifact", "message": err.Error()})
			return
		}
		slog.Warn("video artifact presign failed", "error", err, "deviceId", device.DeviceID)
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "artifact_storage_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, response)
}

func (s *Server) publicVideoArtifact(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.Media == nil {
		http.NotFound(w, r)
		return
	}
	ownerScope := strings.ToLower(strings.TrimSpace(r.PathValue("owner")))
	fileName := strings.ToLower(strings.TrimSpace(r.PathValue("file")))
	if !strings.HasSuffix(fileName, ".mp4") {
		http.NotFound(w, r)
		return
	}
	hash := strings.TrimSuffix(fileName, ".mp4")
	if !artifactOwnerPattern.MatchString(ownerScope) || !mediaHashPattern.MatchString(hash) {
		http.NotFound(w, r)
		return
	}
	key := videoArtifactObjectKey(s.Media.prefix, ownerScope, hash)
	input := &s3.GetObjectInput{Bucket: aws.String(s.Media.bucket), Key: aws.String(key)}
	if value := strings.TrimSpace(r.Header.Get("Range")); value != "" {
		input.Range = aws.String(value)
	}
	object, err := s.Media.client.GetObject(r.Context(), input)
	if err != nil {
		if isMissingS3Object(err) {
			http.NotFound(w, r)
			return
		}
		slog.Warn("public video artifact read failed", "error", err)
		http.Error(w, "artifact unavailable", http.StatusServiceUnavailable)
		return
	}
	defer object.Body.Close()
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if object.ContentLength != nil && *object.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(*object.ContentLength, 10))
	}
	if object.ContentRange != nil && strings.TrimSpace(*object.ContentRange) != "" {
		w.Header().Set("Content-Range", *object.ContentRange)
		w.WriteHeader(http.StatusPartialContent)
	}
	if _, err := io.Copy(w, object.Body); err != nil {
		slog.Debug("public video artifact stream interrupted", "error", err)
	}
}

func videoArtifactPresignHandler(s *Server) http.Handler {
	var handler http.Handler = http.HandlerFunc(s.videoArtifactPresign)
	handler = webutil.RateLimit(s.Store, webutil.RateLimitOptions{
		Scope: "video-artifact-presign-device", Limit: 60, Window: time.Minute,
		Subject: func(r *http.Request) string { id, _ := deviceAuth(r); return id },
	}, handler)
	return webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "video-artifact-presign-ip", Limit: 180, Window: time.Minute}, handler)
}
