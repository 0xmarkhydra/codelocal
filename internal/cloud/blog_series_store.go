package cloud

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type blogSeriesRowScanner interface { Scan(dest ...any) error }

const blogSeriesSelect = `
SELECT s.series_id,s.slug,s.author_user_id,COALESCE(u.email,''),s.title,s.description,s.cover_asset_id,s.status,
       (SELECT COUNT(*) FROM codelocal_blog_posts p WHERE p.series_id=s.series_id AND p.deleted_at=0 AND p.status='published' AND p.visibility='public' AND p.moderation_status='clean')::int,
       s.created_at,s.updated_at,s.deleted_at
FROM codelocal_blog_series s
LEFT JOIN codelocal_users u ON u.id=s.author_user_id`

const publicBlogSeriesPredicate = `
 s.deleted_at=0
 AND s.status IN ('active','complete')
 AND EXISTS(
   SELECT 1 FROM codelocal_blog_posts p
   WHERE p.series_id=s.series_id
     AND p.deleted_at=0
     AND p.status='published'
     AND p.visibility='public'
     AND p.moderation_status='clean'
 )`

func scanBlogSeries(row blogSeriesRowScanner) (BlogSeries, error) {
	var series BlogSeries
	err := row.Scan(&series.ID,&series.Slug,&series.AuthorUserID,&series.AuthorEmail,&series.Title,&series.Description,&series.CoverAssetID,&series.Status,&series.PostCount,&series.CreatedAt,&series.UpdatedAt,&series.DeletedAt)
	return series, err
}

func (s *Store) CreateBlogSeries(ctx context.Context, input BlogSeriesDraft) (BlogSeries, error) {
	input, err := normalizeBlogSeriesDraft(input)
	if err != nil { return BlogSeries{}, err }
	if input.CoverAssetID != "" {
		ready, err := s.MediaAssetOwnedReady(ctx, input.AuthorUserID, input.CoverAssetID)
		if err != nil { return BlogSeries{}, err }
		if !ready { return BlogSeries{}, ErrBlogInvalid }
	}
	seriesID := "series_" + RandomHex(12)
	now := time.Now().UnixMilli()
	tx, err := s.DB.Begin(ctx)
	if err != nil { return BlogSeries{}, err }
	defer func(){ _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO codelocal_blog_series(series_id,slug,author_user_id,title,description,cover_asset_id,status,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,'active',$7,$7,0)`, seriesID,input.Slug,input.AuthorUserID,input.Title,input.Description,input.CoverAssetID,now)
	if err != nil { return BlogSeries{}, mapBlogWriteError(err) }
	slots := map[string]string{}
	if input.CoverAssetID != "" { slots["cover"] = input.CoverAssetID }
	if err := syncMediaAssetRefsTx(ctx, tx, input.AuthorUserID, "blog_series", seriesID, slots); err != nil { return BlogSeries{}, err }
	if err := tx.Commit(ctx); err != nil { return BlogSeries{}, err }
	return s.BlogSeriesByID(ctx, seriesID)
}

func (s *Store) BlogSeriesByID(ctx context.Context, seriesID string) (BlogSeries, error) {
	seriesID = strings.TrimSpace(seriesID)
	if seriesID == "" { return BlogSeries{}, ErrBlogNotFound }
	series, err := scanBlogSeries(s.DB.QueryRow(ctx, blogSeriesSelect+` WHERE s.series_id=$1 AND s.deleted_at=0`, seriesID))
	if errors.Is(err, pgx.ErrNoRows) { return BlogSeries{}, ErrBlogNotFound }
	return series, err
}

func (s *Store) PublicBlogSeriesBySlug(ctx context.Context, slug string) (BlogSeries, error) {
	slug = NormalizeBlogSlug(slug)
	if slug == "" { return BlogSeries{}, ErrBlogNotFound }
	query := blogSeriesSelect+` WHERE s.slug=$1 AND `+publicBlogSeriesPredicate
	series, err := scanBlogSeries(s.DB.QueryRow(ctx, query, slug))
	if err == nil { return series, nil }
	if !errors.Is(err, pgx.ErrNoRows) { return BlogSeries{}, err }
	redirectQuery := blogSeriesSelect+` JOIN codelocal_blog_series_slug_redirects r ON r.series_id=s.series_id WHERE r.old_slug=$1 AND `+publicBlogSeriesPredicate
	series, err = scanBlogSeries(s.DB.QueryRow(ctx, redirectQuery, slug))
	if errors.Is(err, pgx.ErrNoRows) { return BlogSeries{}, ErrBlogNotFound }
	return series, err
}

func scanBlogSeriesRows(rows pgx.Rows) ([]BlogSeries, error) {
	defer rows.Close()
	out := []BlogSeries{}
	for rows.Next() {
		series, err := scanBlogSeries(rows)
		if err != nil { return nil, err }
		out = append(out, series)
	}
	return out, rows.Err()
}

func (s *Store) ListBlogSeriesForUser(ctx context.Context, userID string, limit int) ([]BlogSeries, error) {
	rows, err := s.DB.Query(ctx, blogSeriesSelect+` WHERE s.author_user_id=$1 AND s.deleted_at=0 ORDER BY s.updated_at DESC LIMIT $2`, strings.TrimSpace(userID), blogListLimit(limit))
	if err != nil { return nil, err }
	return scanBlogSeriesRows(rows)
}

func (s *Store) ListAllBlogSeries(ctx context.Context, limit int) ([]BlogSeries, error) {
	rows, err := s.DB.Query(ctx, blogSeriesSelect+` WHERE s.deleted_at=0 ORDER BY s.updated_at DESC LIMIT $1`, blogListLimit(limit))
	if err != nil { return nil, err }
	return scanBlogSeriesRows(rows)
}

func (s *Store) ListPublicBlogSeries(ctx context.Context, limit int) ([]BlogSeries, error) {
	rows, err := s.DB.Query(ctx, blogSeriesSelect+` WHERE `+publicBlogSeriesPredicate+` ORDER BY s.updated_at DESC LIMIT $1`, blogListLimit(limit))
	if err != nil { return nil, err }
	return scanBlogSeriesRows(rows)
}

func (s *Store) ListPublicBlogPostsBySeries(ctx context.Context, seriesID string, limit int) ([]BlogPost, error) {
	rows, err := s.DB.Query(ctx, blogPostSelect+` WHERE p.series_id=$1 AND p.deleted_at=0 AND p.status='published' AND p.visibility='public' AND p.moderation_status='clean' ORDER BY p.series_part ASC LIMIT $2`, strings.TrimSpace(seriesID), blogListLimit(limit))
	if err != nil { return nil, err }
	return scanBlogPostRows(rows)
}

func (s *Store) UpdateBlogSeries(ctx context.Context, actorUserID string, admin bool, seriesID string, input BlogSeriesUpdate) (BlogSeries, error) {
	input, err := normalizeBlogSeriesUpdate(input)
	if err != nil { return BlogSeries{}, err }
	current, err := s.BlogSeriesByID(ctx, seriesID)
	if err != nil { return BlogSeries{}, err }
	actorUserID = strings.TrimSpace(actorUserID)
	if current.AuthorUserID != actorUserID && !admin { return BlogSeries{}, ErrBlogForbidden }
	if input.CoverAssetID != "" {
		ready, err := s.MediaAssetOwnedReady(ctx, current.AuthorUserID, input.CoverAssetID)
		if err != nil { return BlogSeries{}, err }
		if !ready { return BlogSeries{}, ErrBlogInvalid }
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil { return BlogSeries{}, err }
	defer func(){ _ = tx.Rollback(ctx) }()
	now := time.Now().UnixMilli()
	command, err := tx.Exec(ctx, `UPDATE codelocal_blog_series SET slug=$1,title=$2,description=$3,cover_asset_id=$4,status=$5,updated_at=$6 WHERE series_id=$7 AND deleted_at=0 AND (author_user_id=$8 OR $9::boolean)`, input.Slug,input.Title,input.Description,input.CoverAssetID,input.Status,now,current.ID,actorUserID,admin)
	if err != nil { return BlogSeries{}, mapBlogWriteError(err) }
	if command.RowsAffected()!=1 { return BlogSeries{}, ErrBlogForbidden }
	if current.Slug != input.Slug {
		_, err = tx.Exec(ctx, `INSERT INTO codelocal_blog_series_slug_redirects(old_slug,series_id,created_at) VALUES($1,$2,$3) ON CONFLICT(old_slug) DO UPDATE SET series_id=EXCLUDED.series_id,created_at=EXCLUDED.created_at`, current.Slug,current.ID,now)
		if err != nil { return BlogSeries{}, err }
	}
	slots := map[string]string{}
	if input.CoverAssetID != "" { slots["cover"] = input.CoverAssetID }
	if err := syncMediaAssetRefsTx(ctx, tx, current.AuthorUserID, "blog_series", current.ID, slots); err != nil { return BlogSeries{}, err }
	if err := tx.Commit(ctx); err != nil { return BlogSeries{}, err }
	return s.BlogSeriesByID(ctx, current.ID)
}

func (s *Store) DeleteBlogSeries(ctx context.Context, actorUserID string, admin bool, seriesID string) error {
	current, err := s.BlogSeriesByID(ctx, seriesID)
	if err != nil { return err }
	if current.AuthorUserID != strings.TrimSpace(actorUserID) && !admin { return ErrBlogForbidden }
	tx, err := s.DB.Begin(ctx)
	if err != nil { return err }
	defer func(){ _ = tx.Rollback(ctx) }()
	now := time.Now().UnixMilli()
	if _, err = tx.Exec(ctx, `UPDATE codelocal_blog_posts SET series_id=NULL,series_part=NULL,updated_at=$1 WHERE series_id=$2 AND deleted_at=0`, now,current.ID); err != nil { return err }
	command, err := tx.Exec(ctx, `UPDATE codelocal_blog_series SET deleted_at=$1,status='archived',updated_at=$1 WHERE series_id=$2 AND deleted_at=0`, now,current.ID)
	if err != nil { return err }
	if command.RowsAffected()!=1 { return ErrBlogNotFound }
	if err := syncMediaAssetRefsTx(ctx, tx, current.AuthorUserID, "blog_series", current.ID, map[string]string{}); err != nil { return err }
	return tx.Commit(ctx)
}
