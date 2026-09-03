package cloudserver

import (
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func (s *Server) registerMediaAssetRoutes(mux *http.ServeMux) {
	prepare := dashboardJSONCSRF(http.HandlerFunc(s.mediaAssetPrepareAPI))
	upload := dashboardJSONCSRF(http.HandlerFunc(s.mediaAssetUploadAPI))
	finalize := dashboardJSONCSRF(http.HandlerFunc(s.mediaAssetFinalizeAPI))
	mux.Handle("POST /api/v1/media/assets/prepare", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "media-asset-prepare-ip", Limit: 60, Window: 10 * time.Minute}, prepare))
	mux.Handle("POST /api/v1/media/assets/{assetID}/upload", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "media-asset-upload-ip", Limit: 30, Window: 10 * time.Minute}, upload))
	mux.Handle("POST /api/v1/media/assets/{assetID}/finalize", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "media-asset-finalize-ip", Limit: 60, Window: 10 * time.Minute}, finalize))
	mux.HandleFunc("GET /api/v1/media/assets/{assetID}", s.mediaAssetResourceAPI)
	mux.HandleFunc("GET /api/v1/media/assets/{assetID}/variants/{variant}", s.mediaAssetVariantAPI)
	mux.HandleFunc("GET /api/v1/public/media/{assetID}/{variant}", s.publicMediaVariantAPI)

	shares := dashboardJSONCSRF(http.HandlerFunc(s.screenshotSharesResourceAPI))
	mux.HandleFunc("GET /api/v1/shots", s.screenshotSharesResourceAPI)
	mux.Handle("POST /api/v1/shots", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "screenshot-share-create-ip", Limit: 60, Window: 10 * time.Minute}, shares))
	mux.Handle("DELETE /api/v1/shots/{shareID}", dashboardJSONCSRF(http.HandlerFunc(s.screenshotShareResourceAPI)))
	mux.HandleFunc("GET /api/v1/public/shots/{shareID}", s.publicScreenshotShareAPI)
	mux.HandleFunc("GET /api/v1/public/shots/{shareID}/image/{variant}", s.publicScreenshotShareImageAPI)
}
