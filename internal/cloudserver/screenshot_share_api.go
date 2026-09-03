package cloudserver

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type screenshotShareDTO struct {
	ID           string `json:"id"`
	AssetID      string `json:"assetId,omitempty"`
	URL          string `json:"url"`
	ImageURL     string `json:"imageUrl"`
	PreviewURL   string `json:"previewUrl"`
	ThumbnailURL string `json:"thumbnailUrl"`
	DownloadURL  string `json:"downloadUrl"`
	ContentType  string `json:"contentType"`
	Size         int64  `json:"size"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	CreatedAt    int64  `json:"createdAt"`
}

func screenshotSharePayload(share cloud.ScreenshotShare, publicBaseURL string, includeAsset bool) screenshotShareDTO {
	base := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	sharePath := "/s/" + url.PathEscape(share.ID)
	imagePath := "/api/v1/public/shots/" + url.PathEscape(share.ID) + "/image"
	payload := screenshotShareDTO{
		ID: share.ID, URL: base + sharePath,
		ImageURL:     base + imagePath + "/original",
		PreviewURL:   base + imagePath + "/large",
		ThumbnailURL: base + imagePath + "/thumb",
		DownloadURL:  base + imagePath + "/original?download=1",
		ContentType:  share.ContentType, Size: share.Size, Width: share.Width, Height: share.Height, CreatedAt: share.CreatedAt,
	}
	if includeAsset {
		payload.AssetID = share.AssetID
	}
	return payload
}

func screenshotSharePayloads(shares []cloud.ScreenshotShare, publicBaseURL string) []screenshotShareDTO {
	payloads := make([]screenshotShareDTO, 0, len(shares))
	for _, share := range shares {
		payloads = append(payloads, screenshotSharePayload(share, publicBaseURL, true))
	}
	return payloads
}

func writeScreenshotShareAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cloud.ErrScreenshotShareNotFound):
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "screenshot_share_not_found"})
	case errors.Is(err, cloud.ErrScreenshotShareForbidden):
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "screenshot_share_forbidden"})
	case errors.Is(err, cloud.ErrScreenshotShareInvalid):
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_screenshot_share"})
	default:
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "screenshot_share_unavailable"})
	}
}

func (s *Server) screenshotSharesResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		shares, err := s.Store.ListScreenshotShares(r.Context(), identity.User.ID, 50)
		if err != nil {
			writeScreenshotShareAPIError(w, err)
			return
		}
		webutil.JSON(w, http.StatusOK, map[string]any{"shares": screenshotSharePayloads(shares, s.WebAuth.PublicBaseURL)})
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
	var input struct {
		AssetID string `json:"assetId"`
	}
	if webutil.DecodeJSON(r, 16<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	share, err := s.Store.CreateScreenshotShare(r.Context(), identity.User.ID, input.AssetID)
	if err != nil {
		writeScreenshotShareAPIError(w, err)
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "media.screenshot_shared", Detail: map[string]any{"shareId": share.ID, "assetId": share.AssetID}})
	webutil.JSON(w, http.StatusCreated, map[string]any{"share": screenshotSharePayload(share, s.WebAuth.PublicBaseURL, true)})
}

func (s *Server) screenshotShareResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if !s.WebAuth.VerifyCSRF(r) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "invalid_csrf"})
		return
	}
	shareID := r.PathValue("shareID")
	if err := s.Store.DeleteScreenshotShare(r.Context(), identity.User.ID, shareID); err != nil {
		writeScreenshotShareAPIError(w, err)
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "media.screenshot_unshared", Detail: map[string]any{"shareId": shareID}})
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) publicScreenshotShareAPI(w http.ResponseWriter, r *http.Request) {
	if s.Media == nil {
		http.NotFound(w, r)
		return
	}
	share, err := s.Store.ScreenshotShareByID(r.Context(), r.PathValue("shareID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public,no-cache,must-revalidate")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	webutil.JSON(w, http.StatusOK, map[string]any{"share": screenshotSharePayload(share, s.WebAuth.PublicBaseURL, false)})
}

func screenshotExtension(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	default:
		return ".webp"
	}
}

func (s *Server) publicScreenshotShareImageAPI(w http.ResponseWriter, r *http.Request) {
	if s.Media == nil {
		http.NotFound(w, r)
		return
	}
	shareID := r.PathValue("shareID")
	requestedVariant := r.PathValue("variant")
	variant, err := s.Store.ScreenshotShareVariant(r.Context(), shareID, requestedVariant)
	if errors.Is(err, cloud.ErrScreenshotShareNotFound) && requestedVariant == "original" {
		variant, err = s.Store.ScreenshotShareVariant(r.Context(), shareID, "large")
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public,no-cache,must-revalidate")
	w.Header().Set("ETag", `"`+variant.SHA256+`"`)
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	if strings.TrimSpace(r.URL.Query().Get("download")) == "1" {
		filename := fmt.Sprintf("codelocal-shot-%s%s", shareID, screenshotExtension(variant.ContentType))
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	}
	if match := strings.TrimSpace(r.Header.Get("If-None-Match")); match == `"`+variant.SHA256+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	object, err := s.Media.client.GetObject(r.Context(), &s3.GetObjectInput{
		Bucket: aws.String(s.Media.bucket), Key: aws.String(variant.ObjectKey),
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer object.Body.Close()
	w.Header().Set("Content-Type", variant.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(variant.Size, 10))
	_, _ = io.Copy(w, object.Body)
}
