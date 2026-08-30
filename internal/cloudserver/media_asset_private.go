package cloudserver

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func mediaVariantByName(asset cloud.MediaAsset, name string) (cloud.MediaVariant, bool) {
	name = strings.TrimSpace(name)
	for _, variant := range asset.Variants {
		if variant.Variant == name {
			return variant, true
		}
	}
	return cloud.MediaVariant{}, false
}

func (s *Server) mediaAssetVariantAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if s.Media == nil {
		http.NotFound(w, r)
		return
	}
	asset, err := s.Store.MediaAssetByID(r.Context(), r.PathValue("assetID"))
	if err != nil || asset.Status != "ready" {
		http.NotFound(w, r)
		return
	}
	if asset.OwnerUserID != identity.User.ID && !cloud.IsAdminEmail(identity.User.Email) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	variant, ok := mediaVariantByName(asset, r.PathValue("variant"))
	if !ok {
		http.NotFound(w, r)
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
	w.Header().Set("Cache-Control", "private,max-age=300")
	w.Header().Set("ETag", `"`+variant.SHA256+`"`)
	_, _ = io.Copy(w, object.Body)
}
