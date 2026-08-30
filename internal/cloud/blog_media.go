package cloud

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

func blogMediaSlots(coverAssetID string, content json.RawMessage) (map[string]string, error) {
	slots := map[string]string{}
	coverAssetID = strings.TrimSpace(coverAssetID)
	if coverAssetID != "" {
		coverAssetID = NormalizeMediaAssetID(coverAssetID)
		if coverAssetID == "" {
			return nil, ErrBlogInvalid
		}
		slots["cover"] = coverAssetID
	}
	if len(content) == 0 {
		return slots, nil
	}
	var blocks []struct {
		Type    string `json:"type"`
		AssetID string `json:"assetId"`
	}
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, ErrBlogInvalid
	}
	for index, block := range blocks {
		if strings.TrimSpace(block.Type) != "image" {
			continue
		}
		assetID := NormalizeMediaAssetID(block.AssetID)
		if assetID == "" {
			return nil, ErrBlogInvalid
		}
		slots["image:"+strconv.Itoa(index)] = assetID
	}
	return slots, nil
}

func (s *Store) validateBlogMediaAssets(ctx context.Context, ownerUserID, coverAssetID string, content json.RawMessage) (map[string]string, error) {
	slots, err := blogMediaSlots(coverAssetID, content)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, assetID := range slots {
		if _, duplicate := seen[assetID]; duplicate {
			continue
		}
		seen[assetID] = struct{}{}
		ready, err := s.MediaAssetOwnedReady(ctx, ownerUserID, assetID)
		if err != nil {
			return nil, err
		}
		if !ready {
			return nil, ErrBlogInvalid
		}
	}
	return slots, nil
}

func (s *Store) syncBlogMediaRefs(ctx context.Context, ownerUserID, postID string, slots map[string]string) error {
	return s.SyncMediaAssetRefs(ctx, strings.TrimSpace(ownerUserID), "blog_post", strings.TrimSpace(postID), slots)
}
