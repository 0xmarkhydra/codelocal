package cloud

import (
	"errors"
	"strings"
)

var (
	ErrMediaAssetNotFound  = errors.New("media asset not found")
	ErrMediaAssetForbidden = errors.New("media asset forbidden")
	ErrMediaAssetInvalid   = errors.New("invalid media asset")
)

type MediaAsset struct {
	ID                string         `json:"id"`
	OwnerUserID       string         `json:"ownerUserId"`
	SourceSHA256      string         `json:"sourceSha256"`
	SourceContentType string         `json:"sourceContentType"`
	SourceSize        int64          `json:"sourceSize"`
	Width             int            `json:"width"`
	Height            int            `json:"height"`
	Status            string         `json:"status"`
	PreserveOriginal  bool           `json:"preserveOriginal"`
	ErrorCode         string         `json:"errorCode,omitempty"`
	CreatedAt         int64          `json:"createdAt"`
	UpdatedAt         int64          `json:"updatedAt"`
	DeletedAt         int64          `json:"deletedAt,omitempty"`
	Variants          []MediaVariant `json:"variants"`
}

type MediaVariant struct {
	AssetID     string `json:"assetId"`
	Variant     string `json:"variant"`
	ObjectKey   string `json:"objectKey,omitempty"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	SHA256      string `json:"sha256"`
	CreatedAt   int64  `json:"createdAt"`
}

type MediaUnderstanding struct {
	AssetID       string   `json:"assetId"`
	Summary       string   `json:"summary"`
	AltText       string   `json:"altText"`
	ExtractedText string   `json:"extractedText"`
	Tags          []string `json:"tags"`
	Language      string   `json:"language"`
	Provider      string   `json:"provider"`
	Model         string   `json:"model"`
	AnalyzedAt    int64    `json:"analyzedAt"`
	UpdatedAt     int64    `json:"updatedAt"`
}

func NormalizeMediaAssetID(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "media_") || len(value) > 80 {
		return ""
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return ""
	}
	return value
}
