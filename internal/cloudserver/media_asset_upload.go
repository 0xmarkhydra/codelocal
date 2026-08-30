package cloudserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (s *Server) mediaAssetUploadAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if !s.WebAuth.VerifyCSRF(r) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "invalid_csrf"})
		return
	}
	if s.Media == nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media_not_configured"})
		return
	}
	asset, err := s.Store.MediaAssetByID(r.Context(), r.PathValue("assetID"))
	if err != nil {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "media_asset_not_found"})
		return
	}
	if asset.OwnerUserID != identity.User.ID {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	if asset.Status == "ready" {
		webutil.JSON(w, http.StatusOK, mediaAssetResponse{Asset: asset, URLs: mediaAssetURLs(asset)})
		return
	}
	contentType := durableMediaContentType(r.Header.Get("Content-Type"))
	if contentType == "" || contentType != asset.SourceContentType {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "media_content_type_mismatch"})
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, s.Media.maxBytes+1))
	if err != nil || int64(len(data)) != asset.SourceSize || int64(len(data)) > s.Media.maxBytes {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "media_size_mismatch"})
		return
	}
	digest := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), asset.SourceSHA256) {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "media_hash_mismatch"})
		return
	}
	sourceKey := durableMediaSourceKey(s.Media, identity.User.ID, asset)
	_, err = s.Media.client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket: aws.String(s.Media.bucket), Key: aws.String(sourceKey), Body: bytes.NewReader(data),
		ContentType: aws.String(asset.SourceContentType), CacheControl: aws.String("private, no-store"),
		Metadata: map[string]string{"content-sha256": asset.SourceSHA256, "source": "codelocal-media-asset-proxy"},
	})
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media_upload_failed"})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "assetId": asset.ID})
}
