package cloud

import (
	"context"
	"strings"
)

func (s *Store) ListPublicBlogSeriesPage(ctx context.Context, limit, offset int) ([]BlogSeries, error) {
	rows, err := s.DB.Query(ctx, blogSeriesSelect+` WHERE `+publicBlogSeriesPredicate+` ORDER BY s.updated_at DESC,s.series_id DESC LIMIT $1 OFFSET $2`, publicBlogPageLimit(limit), blogListOffset(offset))
	if err != nil {
		return nil, err
	}
	return scanBlogSeriesRows(rows)
}

func (s *Store) ListPublicBlogPostsBySeriesPage(ctx context.Context, seriesID string, limit, offset int) ([]BlogPost, error) {
	rows, err := s.DB.Query(ctx, blogPostSelect+` WHERE p.series_id=$1 AND p.deleted_at=0 AND p.status='published' AND p.visibility='public' AND p.moderation_status='clean' ORDER BY p.series_part ASC,p.published_at ASC,p.post_id ASC LIMIT $2 OFFSET $3`, strings.TrimSpace(seriesID), publicBlogPageLimit(limit), blogListOffset(offset))
	if err != nil {
		return nil, err
	}
	return scanBlogPostRows(rows)
}
