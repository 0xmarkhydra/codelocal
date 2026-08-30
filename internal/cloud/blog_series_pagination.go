package cloud

import "context"

func (s *Store) ListPublicBlogSeriesPage(ctx context.Context, limit, offset int) ([]BlogSeries, error) {
	rows, err := s.DB.Query(ctx, blogSeriesSelect+` WHERE `+publicBlogSeriesPredicate+` ORDER BY s.updated_at DESC,s.series_id DESC LIMIT $1 OFFSET $2`, publicBlogPageLimit(limit), blogListOffset(offset))
	if err != nil {
		return nil, err
	}
	return scanBlogSeriesRows(rows)
}
