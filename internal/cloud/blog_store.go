package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type blogRowScanner interface {
	Scan(dest ...any) error
}

const blogPostSelect = `
SELECT p.post_id,p.slug,p.author_user_id,COALESCE(u.email,''),p.title,p.excerpt,p.content,
       p.cover_asset_id,p.category,p.tags,COALESCE(p.series_id,''),COALESCE(p.series_part,0),
       p.status,p.visibility,p.moderation_status,p.featured,p.show_on_landing,
       p.published_at,p.scheduled_at,p.created_at,p.updated_at,p.deleted_at
FROM codelocal_blog_posts p
LEFT JOIN codelocal_users u ON u.id=p.author_user_id`

func scanBlogPost(row blogRowScanner) (BlogPost, error) {
	var post BlogPost
	var content, tagsJSON []byte
	if err := row.Scan(
		&post.ID, &post.Slug, &post.AuthorUserID, &post.AuthorEmail, &post.Title, &post.Excerpt, &content,
		&post.CoverAssetID, &post.Category, &tagsJSON, &post.SeriesID, &post.SeriesPart,
		&post.Status, &post.Visibility, &post.ModerationStatus, &post.Featured, &post.ShowOnLanding,
		&post.PublishedAt, &post.ScheduledAt, &post.CreatedAt, &post.UpdatedAt, &post.DeletedAt,
	); err != nil {
		return BlogPost{}, err
	}
	post.Content = append(json.RawMessage(nil), content...)
	if len(tagsJSON) > 0 {
		if err := json.Unmarshal(tagsJSON, &post.Tags); err != nil {
			return BlogPost{}, err
		}
	}
	if post.Tags == nil {
		post.Tags = []string{}
	}
	return post, nil
}

func mapBlogWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrBlogSlugConflict
		case "23503", "23514", "22P02":
			return ErrBlogInvalid
		}
	}
	return err
}

func (s *Store) validateBlogSeriesOwner(ctx context.Context, authorUserID, seriesID string) error {
	seriesID = strings.TrimSpace(seriesID)
	if seriesID == "" {
		return nil
	}
	var owner string
	err := s.DB.QueryRow(ctx, `SELECT author_user_id FROM codelocal_blog_series WHERE series_id=$1 AND deleted_at=0`, seriesID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) || strings.TrimSpace(owner) != strings.TrimSpace(authorUserID) {
		return ErrBlogInvalid
	}
	return err
}

func validateBlogSeriesOwnerTx(ctx context.Context, tx pgx.Tx, authorUserID, seriesID string) error {
	seriesID = strings.TrimSpace(seriesID)
	if seriesID == "" {
		return nil
	}
	var owner string
	err := tx.QueryRow(ctx, `SELECT author_user_id FROM codelocal_blog_series WHERE series_id=$1 AND deleted_at=0 FOR SHARE`, seriesID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrBlogInvalid
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(owner) != strings.TrimSpace(authorUserID) {
		return ErrBlogInvalid
	}
	return nil
}

func (s *Store) CreateBlogPost(ctx context.Context, input BlogPostDraft) (BlogPost, error) {
	input, err := normalizeBlogDraft(input)
	if err != nil {
		return BlogPost{}, err
	}
	if err := s.validateBlogSeriesOwner(ctx, input.AuthorUserID, input.SeriesID); err != nil {
		return BlogPost{}, err
	}
	mediaSlots, err := s.validateBlogMediaAssets(ctx, input.AuthorUserID, input.CoverAssetID, input.Content)
	if err != nil {
		return BlogPost{}, err
	}
	tagsJSON, err := json.Marshal(input.Tags)
	if err != nil {
		return BlogPost{}, err
	}
	postID := "blog_" + RandomHex(12)
	now := time.Now().UnixMilli()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return BlogPost{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := validateBlogSeriesOwnerTx(ctx, tx, input.AuthorUserID, input.SeriesID); err != nil {
		return BlogPost{}, err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO codelocal_blog_posts(
 post_id,slug,author_user_id,title,excerpt,content,cover_asset_id,category,tags,series_id,series_part,
 status,visibility,moderation_status,featured,show_on_landing,published_at,scheduled_at,created_at,updated_at,deleted_at
) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9::jsonb,NULLIF($10,''),NULLIF($11,0),
 'draft',$12,'clean',FALSE,FALSE,0,0,$13,$13,0)`,
		postID, input.Slug, input.AuthorUserID, input.Title, input.Excerpt, string(input.Content), input.CoverAssetID,
		input.Category, string(tagsJSON), input.SeriesID, input.SeriesPart, input.Visibility, now,
	)
	if err != nil {
		return BlogPost{}, mapBlogWriteError(err)
	}
	if err := syncMediaAssetRefsTx(ctx, tx, input.AuthorUserID, "blog_post", postID, mediaSlots); err != nil {
		return BlogPost{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BlogPost{}, err
	}
	return s.BlogPostByID(ctx, postID)
}

func (s *Store) BlogPostByID(ctx context.Context, postID string) (BlogPost, error) {
	postID = strings.TrimSpace(postID)
	if postID == "" {
		return BlogPost{}, ErrBlogNotFound
	}
	post, err := scanBlogPost(s.DB.QueryRow(ctx, blogPostSelect+` WHERE p.post_id=$1 AND p.deleted_at=0`, postID))
	if errors.Is(err, pgx.ErrNoRows) {
		return BlogPost{}, ErrBlogNotFound
	}
	return post, err
}

func (s *Store) PublicBlogPostBySlug(ctx context.Context, slug string) (BlogPost, error) {
	slug = NormalizeBlogSlug(slug)
	if slug == "" {
		return BlogPost{}, ErrBlogNotFound
	}
	query := blogPostSelect + ` WHERE p.slug=$1 AND p.deleted_at=0 AND p.status='published' AND p.visibility='public' AND p.moderation_status='clean'`
	post, err := scanBlogPost(s.DB.QueryRow(ctx, query, slug))
	if err == nil {
		return post, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return BlogPost{}, err
	}
	redirectQuery := blogPostSelect + `
 JOIN codelocal_blog_slug_redirects r ON r.post_id=p.post_id
 WHERE r.old_slug=$1 AND p.deleted_at=0 AND p.status='published' AND p.visibility='public' AND p.moderation_status='clean'`
	post, err = scanBlogPost(s.DB.QueryRow(ctx, redirectQuery, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return BlogPost{}, ErrBlogNotFound
	}
	return post, err
}

func scanBlogPostRows(rows pgx.Rows) ([]BlogPost, error) {
	defer rows.Close()
	out := []BlogPost{}
	for rows.Next() {
		post, err := scanBlogPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, post)
	}
	return out, rows.Err()
}

func blogListLimit(limit int) int {
	if limit < 1 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func publicBlogPageLimit(limit int) int {
	if limit < 1 {
		return 100
	}
	if limit > 201 {
		return 201
	}
	return limit
}

func blogListOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func (s *Store) ListBlogPostsForUser(ctx context.Context, userID string, limit int) ([]BlogPost, error) {
	rows, err := s.DB.Query(ctx, blogPostSelect+` WHERE p.author_user_id=$1 AND p.deleted_at=0 ORDER BY p.updated_at DESC LIMIT $2`, strings.TrimSpace(userID), blogListLimit(limit))
	if err != nil {
		return nil, err
	}
	return scanBlogPostRows(rows)
}

func (s *Store) ListAllBlogPosts(ctx context.Context, limit int) ([]BlogPost, error) {
	rows, err := s.DB.Query(ctx, blogPostSelect+` WHERE p.deleted_at=0 ORDER BY p.updated_at DESC LIMIT $1`, blogListLimit(limit))
	if err != nil {
		return nil, err
	}
	return scanBlogPostRows(rows)
}

func (s *Store) ListPublicBlogPosts(ctx context.Context, limit int) ([]BlogPost, error) {
	return s.ListPublicBlogPostsPage(ctx, blogListLimit(limit), 0)
}

func (s *Store) ListPublicBlogPostsPage(ctx context.Context, limit, offset int) ([]BlogPost, error) {
	rows, err := s.DB.Query(ctx, blogPostSelect+` WHERE p.deleted_at=0 AND p.status='published' AND p.visibility='public' AND p.moderation_status='clean' ORDER BY p.published_at DESC,p.updated_at DESC LIMIT $1 OFFSET $2`, publicBlogPageLimit(limit), blogListOffset(offset))
	if err != nil {
		return nil, err
	}
	return scanBlogPostRows(rows)
}

func (s *Store) UpdateBlogPost(ctx context.Context, actorUserID string, admin bool, postID string, input BlogPostUpdate) (BlogPost, error) {
	input, err := normalizeBlogUpdate(input)
	if err != nil {
		return BlogPost{}, err
	}
	current, err := s.BlogPostByID(ctx, postID)
	if err != nil {
		return BlogPost{}, err
	}
	actorUserID = strings.TrimSpace(actorUserID)
	if current.AuthorUserID != actorUserID && !admin {
		return BlogPost{}, ErrBlogForbidden
	}
	if err := s.validateBlogSeriesOwner(ctx, current.AuthorUserID, input.SeriesID); err != nil {
		return BlogPost{}, err
	}
	mediaSlots, err := s.validateBlogMediaAssets(ctx, current.AuthorUserID, input.CoverAssetID, input.Content)
	if err != nil {
		return BlogPost{}, err
	}
	tagsJSON, err := json.Marshal(input.Tags)
	if err != nil {
		return BlogPost{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return BlogPost{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := validateBlogSeriesOwnerTx(ctx, tx, current.AuthorUserID, input.SeriesID); err != nil {
		return BlogPost{}, err
	}
	now := time.Now().UnixMilli()
	if _, err = tx.Exec(ctx, `
INSERT INTO codelocal_blog_post_revisions(revision_id,post_id,editor_user_id,snapshot,created_at)
SELECT $1,p.post_id,$2,to_jsonb(p),$3 FROM codelocal_blog_posts p WHERE p.post_id=$4`,
		"blogrev_"+RandomHex(12), actorUserID, now, postID); err != nil {
		return BlogPost{}, err
	}
	command, err := tx.Exec(ctx, `
UPDATE codelocal_blog_posts SET
 slug=$1,title=$2,excerpt=$3,content=$4::jsonb,cover_asset_id=$5,category=$6,tags=$7::jsonb,
 visibility=$8,series_id=NULLIF($9,''),series_part=NULLIF($10,0),updated_at=$11
WHERE post_id=$12 AND deleted_at=0 AND (author_user_id=$13 OR $14::boolean)`,
		input.Slug, input.Title, input.Excerpt, string(input.Content), input.CoverAssetID, input.Category, string(tagsJSON),
		input.Visibility, input.SeriesID, input.SeriesPart, now, postID, actorUserID, admin,
	)
	if err != nil {
		return BlogPost{}, mapBlogWriteError(err)
	}
	if command.RowsAffected() != 1 {
		return BlogPost{}, ErrBlogForbidden
	}
	if current.Slug != input.Slug {
		if _, err = tx.Exec(ctx, `
INSERT INTO codelocal_blog_slug_redirects(old_slug,post_id,created_at) VALUES($1,$2,$3)
ON CONFLICT(old_slug) DO UPDATE SET post_id=EXCLUDED.post_id,created_at=EXCLUDED.created_at`, current.Slug, postID, now); err != nil {
			return BlogPost{}, err
		}
	}
	if err := syncMediaAssetRefsTx(ctx, tx, current.AuthorUserID, "blog_post", postID, mediaSlots); err != nil {
		return BlogPost{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BlogPost{}, err
	}
	return s.BlogPostByID(ctx, postID)
}

func (s *Store) SetBlogPostPublished(ctx context.Context, actorUserID string, admin bool, postID string, published bool) (BlogPost, error) {
	post, err := s.BlogPostByID(ctx, postID)
	if err != nil {
		return BlogPost{}, err
	}
	if post.AuthorUserID != strings.TrimSpace(actorUserID) && !admin {
		return BlogPost{}, ErrBlogForbidden
	}
	if published && post.ModerationStatus == "hidden" {
		return BlogPost{}, fmt.Errorf("%w: hidden posts cannot be published", ErrBlogForbidden)
	}
	if published {
		mediaSlots, err := s.validateBlogMediaAssets(ctx, post.AuthorUserID, post.CoverAssetID, post.Content)
		if err != nil {
			return BlogPost{}, err
		}
		if err := s.syncBlogMediaRefs(ctx, post.AuthorUserID, post.ID, mediaSlots); err != nil {
			return BlogPost{}, err
		}
	}
	status := "draft"
	publishedAt := post.PublishedAt
	if published {
		status = "published"
		if publishedAt == 0 {
			publishedAt = time.Now().UnixMilli()
		}
	}
	_, err = s.DB.Exec(ctx, `UPDATE codelocal_blog_posts SET status=$1,published_at=$2,scheduled_at=0,updated_at=$3 WHERE post_id=$4 AND deleted_at=0`, status, publishedAt, time.Now().UnixMilli(), postID)
	if err != nil {
		return BlogPost{}, err
	}
	return s.BlogPostByID(ctx, postID)
}

func (s *Store) SetBlogPostDistribution(ctx context.Context, postID string, featured, showOnLanding bool, moderationStatus string) (BlogPost, error) {
	switch moderationStatus {
	case "clean", "pending", "hidden":
	default:
		return BlogPost{}, ErrBlogInvalid
	}
	command, err := s.DB.Exec(ctx, `
UPDATE codelocal_blog_posts SET featured=$1,show_on_landing=$2,moderation_status=$3,updated_at=$4
WHERE post_id=$5 AND deleted_at=0`, featured, showOnLanding, moderationStatus, time.Now().UnixMilli(), strings.TrimSpace(postID))
	if err != nil {
		return BlogPost{}, err
	}
	if command.RowsAffected() != 1 {
		return BlogPost{}, ErrBlogNotFound
	}
	return s.BlogPostByID(ctx, postID)
}

func (s *Store) DeleteBlogPost(ctx context.Context, actorUserID string, admin bool, postID string) error {
	post, err := s.BlogPostByID(ctx, postID)
	if err != nil {
		return err
	}
	if post.AuthorUserID != strings.TrimSpace(actorUserID) && !admin {
		return ErrBlogForbidden
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := time.Now().UnixMilli()
	command, err := tx.Exec(ctx, `UPDATE codelocal_blog_posts SET deleted_at=$1,status='archived',updated_at=$1 WHERE post_id=$2 AND deleted_at=0`, now, postID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrBlogNotFound
	}
	if err := syncMediaAssetRefsTx(ctx, tx, post.AuthorUserID, "blog_post", post.ID, map[string]string{}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
