package cloudserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/deepteams/webp"
)

const (
	durableMediaMaxPixels          = 40_000_000
	durableMediaPublicCacheControl = "public,no-cache,must-revalidate"
)

type mediaAssetPrepareRequest struct {
	SHA256           string `json:"sha256"`
	ContentType      string `json:"contentType"`
	Size             int64  `json:"size"`
	PreserveOriginal bool   `json:"preserveOriginal"`
}

type mediaAssetResponse struct {
	Asset cloud.MediaAsset `json:"asset"`
	URLs  map[string]string `json:"urls"`
}

type mediaAssetPrepareResponse struct {
	mediaAssetResponse
	Deduplicated bool             `json:"deduplicated"`
	Upload       mediaUploadGrant `json:"upload"`
}

type mediaVariantSpec struct {
	Name    string
	MaxEdge int
	Quality float32
}

var durableMediaVariantSpecs = []mediaVariantSpec{
	{Name: "thumb", MaxEdge: 320, Quality: 78},
	{Name: "medium", MaxEdge: 960, Quality: 82},
	{Name: "large", MaxEdge: 1920, Quality: 84},
}

func durableMediaContentType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/png":
		return "image/png"
	case "image/webp":
		return "image/webp"
	default:
		return ""
	}
}

func durableMediaSourceKey(m *s3MediaStore, ownerUserID string, asset cloud.MediaAsset) string {
	ext := mediaExtension(asset.SourceContentType)
	return fmt.Sprintf("%s/assets/users/%s/source/%s%s", normalizeMediaPrefix(m.prefix), mediaOwnerScope(ownerUserID), asset.SourceSHA256, ext)
}

func durableMediaVariantKey(m *s3MediaStore, ownerUserID, assetID, variant string) string {
	return fmt.Sprintf("%s/assets/users/%s/%s/%s.webp", normalizeMediaPrefix(m.prefix), mediaOwnerScope(ownerUserID), assetID, variant)
}

func mediaAssetURLs(asset cloud.MediaAsset) map[string]string {
	urls := map[string]string{}
	for _, variant := range asset.Variants {
		urls[variant.Variant] = "/api/v1/public/media/" + asset.ID + "/" + variant.Variant
	}
	return urls
}

func validateDurableMediaPrepare(input mediaAssetPrepareRequest, maxBytes int64) (mediaAssetPrepareRequest, error) {
	input.SHA256 = strings.ToLower(strings.TrimSpace(input.SHA256))
	input.ContentType = durableMediaContentType(input.ContentType)
	if !mediaHashPattern.MatchString(input.SHA256) {
		return input, errors.New("invalid image sha256")
	}
	if input.ContentType == "" {
		return input, errors.New("unsupported image content type")
	}
	if input.Size <= 0 || input.Size > maxBytes {
		return input, fmt.Errorf("image size must be between 1 and %d bytes", maxBytes)
	}
	return input, nil
}

func (s *Server) mediaAssetPrepareAPI(w http.ResponseWriter, r *http.Request) {
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
	var input mediaAssetPrepareRequest
	if webutil.DecodeJSON(r, 16<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	input, err := validateDurableMediaPrepare(input, s.Media.maxBytes)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_media", "message": err.Error()})
		return
	}
	asset, deduplicated, err := s.Store.EnsureMediaAsset(r.Context(), identity.User.ID, input.SHA256, input.ContentType, input.Size, input.PreserveOriginal)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media_asset_unavailable"})
		return
	}
	if asset.Status == "failed" {
		asset, err = s.Store.ResetMediaAssetProcessing(r.Context(), identity.User.ID, asset.ID, input.PreserveOriginal)
		if err != nil {
			webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media_asset_retry_failed"})
			return
		}
		deduplicated = false
	}
	response := mediaAssetPrepareResponse{mediaAssetResponse: mediaAssetResponse{Asset: asset, URLs: mediaAssetURLs(asset)}, Deduplicated: deduplicated}
	if asset.Status == "ready" {
		webutil.JSON(w, http.StatusOK, response)
		return
	}
	sourceKey := durableMediaSourceKey(s.Media, identity.User.ID, asset)
	exists, err := s.Media.objectExists(r.Context(), sourceKey)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media_storage_unavailable"})
		return
	}
	response.Upload.Required = !exists
	if !exists {
		request, err := s.Media.presigner.PresignPutObject(r.Context(), &s3.PutObjectInput{
			Bucket: aws.String(s.Media.bucket), Key: aws.String(sourceKey), ContentType: aws.String(input.ContentType),
			CacheControl: aws.String("private, no-store"), Metadata: map[string]string{"content-sha256": input.SHA256, "source": "codelocal-media-asset"},
		}, func(options *s3.PresignOptions) { options.Expires = s.Media.urlTTL })
		if err != nil {
			webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media_upload_unavailable"})
			return
		}
		response.Upload.URL = request.URL
		response.Upload.Method = request.Method
		response.Upload.Headers = copySignedHeaders(request.SignedHeader)
	}
	webutil.JSON(w, http.StatusOK, response)
}

func (s *Server) mediaAssetFinalizeAPI(w http.ResponseWriter, r *http.Request) {
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
	completed, processErr := s.processMediaAsset(r, asset)
	if processErr != nil {
		_ = s.Store.FailMediaAsset(r.Context(), identity.User.ID, asset.ID, mediaAssetErrorCode(processErr))
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "media_processing_failed", "message": processErr.Error()})
		return
	}
	webutil.JSON(w, http.StatusOK, mediaAssetResponse{Asset: completed, URLs: mediaAssetURLs(completed)})
}

func mediaAssetErrorCode(err error) string {
	value := strings.ToLower(err.Error())
	switch {
	case strings.Contains(value, "sha256"):
		return "sha256_mismatch"
	case strings.Contains(value, "dimension"), strings.Contains(value, "pixels"):
		return "dimensions_invalid"
	case strings.Contains(value, "decode"):
		return "decode_failed"
	case strings.Contains(value, "encode"):
		return "encode_failed"
	default:
		return "processing_failed"
	}
}

func (s *Server) processMediaAsset(r *http.Request, asset cloud.MediaAsset) (cloud.MediaAsset, error) {
	sourceKey := durableMediaSourceKey(s.Media, asset.OwnerUserID, asset)
	object, err := s.Media.client.GetObject(r.Context(), &s3.GetObjectInput{Bucket: aws.String(s.Media.bucket), Key: aws.String(sourceKey)})
	if err != nil {
		return cloud.MediaAsset{}, fmt.Errorf("read media source: %w", err)
	}
	defer object.Body.Close()
	data, err := io.ReadAll(io.LimitReader(object.Body, s.Media.maxBytes+1))
	if err != nil || int64(len(data)) > s.Media.maxBytes || int64(len(data)) != asset.SourceSize {
		return cloud.MediaAsset{}, errors.New("media source size mismatch")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != asset.SourceSHA256 {
		return cloud.MediaAsset{}, errors.New("media source sha256 mismatch")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return cloud.MediaAsset{}, fmt.Errorf("decode image config: %w", err)
	}
	if format != "jpeg" && format != "png" && format != "webp" {
		return cloud.MediaAsset{}, errors.New("decode image format unsupported")
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 10_000 || config.Height > 10_000 || int64(config.Width)*int64(config.Height) > durableMediaMaxPixels {
		return cloud.MediaAsset{}, errors.New("image dimensions or pixels exceed safe limits")
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return cloud.MediaAsset{}, fmt.Errorf("decode image: %w", err)
	}
	variants := make([]cloud.MediaVariant, 0, len(durableMediaVariantSpecs)+1)
	for _, spec := range durableMediaVariantSpecs {
		resized := resizeImageMaxEdge(decoded, spec.MaxEdge)
		encoded := bytes.Buffer{}
		if err := webp.Encode(&encoded, resized, &webp.EncoderOptions{Quality: spec.Quality, Method: 4, UseSharpYUV: true}); err != nil {
			return cloud.MediaAsset{}, fmt.Errorf("encode %s webp: %w", spec.Name, err)
		}
		key := durableMediaVariantKey(s.Media, asset.OwnerUserID, asset.ID, spec.Name)
		if err := s.putDurableMediaVariant(r, key, encoded.Bytes(), asset.ID, spec.Name); err != nil {
			return cloud.MediaAsset{}, err
		}
		bounds := resized.Bounds()
		sum := sha256.Sum256(encoded.Bytes())
		variants = append(variants, cloud.MediaVariant{AssetID: asset.ID, Variant: spec.Name, ObjectKey: key, ContentType: "image/webp", Size: int64(encoded.Len()), Width: bounds.Dx(), Height: bounds.Dy(), SHA256: hex.EncodeToString(sum[:])})
	}
	if asset.PreserveOriginal {
		variants = append(variants, cloud.MediaVariant{AssetID: asset.ID, Variant: "original", ObjectKey: sourceKey, ContentType: asset.SourceContentType, Size: asset.SourceSize, Width: config.Width, Height: config.Height, SHA256: asset.SourceSHA256})
	}
	result, err := s.Store.CompleteMediaAsset(r.Context(), asset.OwnerUserID, asset.ID, config.Width, config.Height, variants)
	if err != nil {
		return cloud.MediaAsset{}, err
	}
	if !asset.PreserveOriginal {
		_, _ = s.Media.client.DeleteObject(r.Context(), &s3.DeleteObjectInput{Bucket: aws.String(s.Media.bucket), Key: aws.String(sourceKey)})
	}
	return result, nil
}

func (s *Server) putDurableMediaVariant(r *http.Request, key string, data []byte, assetID, variant string) error {
	_, err := s.Media.client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket: aws.String(s.Media.bucket), Key: aws.String(key), Body: bytes.NewReader(data), ContentType: aws.String("image/webp"),
		CacheControl: aws.String(durableMediaPublicCacheControl), Metadata: map[string]string{"asset-id": assetID, "variant": variant},
	})
	if err != nil {
		return fmt.Errorf("store %s media variant: %w", variant, err)
	}
	return nil
}

func (s *Server) mediaAssetResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	asset, err := s.Store.MediaAssetByID(r.Context(), r.PathValue("assetID"))
	if err != nil {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "media_asset_not_found"})
		return
	}
	if asset.OwnerUserID != identity.User.ID && !cloud.IsAdminEmail(identity.User.Email) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	webutil.JSON(w, http.StatusOK, mediaAssetResponse{Asset: asset, URLs: mediaAssetURLs(asset)})
}

func (s *Server) publicMediaVariantAPI(w http.ResponseWriter, r *http.Request) {
	if s.Media == nil {
		http.NotFound(w, r)
		return
	}
	variant, err := s.Store.PublicBlogMediaVariant(r.Context(), r.PathValue("assetID"), r.PathValue("variant"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", durableMediaPublicCacheControl)
	w.Header().Set("ETag", `"`+variant.SHA256+`"`)
	if match := strings.TrimSpace(r.Header.Get("If-None-Match")); match == `"`+variant.SHA256+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	object, err := s.Media.client.GetObject(r.Context(), &s3.GetObjectInput{Bucket: aws.String(s.Media.bucket), Key: aws.String(variant.ObjectKey)})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer object.Body.Close()
	w.Header().Set("Content-Type", variant.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(variant.Size, 10))
	_, _ = io.Copy(w, object.Body)
}

func resizeImageMaxEdge(source image.Image, maxEdge int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= maxEdge && height <= maxEdge {
		return source
	}
	scale := math.Min(float64(maxEdge)/float64(width), float64(maxEdge)/float64(height))
	targetWidth := int(math.Max(1, math.Round(float64(width)*scale)))
	targetHeight := int(math.Max(1, math.Round(float64(height)*scale)))
	target := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := 0; y < targetHeight; y++ {
		sourceY := (float64(y)+0.5)*float64(height)/float64(targetHeight) - 0.5
		y0 := clampInt(int(math.Floor(sourceY)), 0, height-1)
		y1 := clampInt(y0+1, 0, height-1)
		fy := sourceY - math.Floor(sourceY)
		for x := 0; x < targetWidth; x++ {
			sourceX := (float64(x)+0.5)*float64(width)/float64(targetWidth) - 0.5
			x0 := clampInt(int(math.Floor(sourceX)), 0, width-1)
			x1 := clampInt(x0+1, 0, width-1)
			fx := sourceX - math.Floor(sourceX)
			c00 := color.NRGBAModel.Convert(source.At(bounds.Min.X+x0, bounds.Min.Y+y0)).(color.NRGBA)
			c10 := color.NRGBAModel.Convert(source.At(bounds.Min.X+x1, bounds.Min.Y+y0)).(color.NRGBA)
			c01 := color.NRGBAModel.Convert(source.At(bounds.Min.X+x0, bounds.Min.Y+y1)).(color.NRGBA)
			c11 := color.NRGBAModel.Convert(source.At(bounds.Min.X+x1, bounds.Min.Y+y1)).(color.NRGBA)
			target.SetNRGBA(x, y, bilinearNRGBA(c00, c10, c01, c11, fx, fy))
		}
	}
	return target
}

func bilinearNRGBA(c00, c10, c01, c11 color.NRGBA, fx, fy float64) color.NRGBA {
	blend := func(v00, v10, v01, v11 uint8) uint8 {
		top := float64(v00)*(1-fx) + float64(v10)*fx
		bottom := float64(v01)*(1-fx) + float64(v11)*fx
		value := top*(1-fy) + bottom*fy
		return uint8(math.Round(math.Max(0, math.Min(255, value))))
	}
	return color.NRGBA{R: blend(c00.R, c10.R, c01.R, c11.R, fx, fy), G: blend(c00.G, c10.G, c01.G, c11.G, fx, fy), B: blend(c00.B, c10.B, c01.B, c11.B, fx, fy), A: blend(c00.A, c10.A, c01.A, c11.A, fx, fy)}
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
