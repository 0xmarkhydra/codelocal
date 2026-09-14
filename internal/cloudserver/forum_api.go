package cloudserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type forumTopicCreateRequest struct {
	Kind              string   `json:"kind"`
	Title             string   `json:"title"`
	Body              string   `json:"body"`
	Severity          string   `json:"severity"`
	Version           string   `json:"version"`
	Environment       string   `json:"environment"`
	ReproductionSteps string   `json:"reproductionSteps"`
	ExpectedBehavior  string   `json:"expectedBehavior"`
	ActualBehavior    string   `json:"actualBehavior"`
	Tags              []string `json:"tags"`
	AssetIDs          []string `json:"assetIds"`
}

type forumCommentCreateRequest struct {
	Body     string   `json:"body"`
	AssetIDs []string `json:"assetIds"`
}

type forumAdminUpdateRequest struct {
	Status            string `json:"status"`
	Severity          string `json:"severity"`
	GitHubIssueURL    string `json:"githubIssueUrl"`
	GitHubIssueNumber int64  `json:"githubIssueNumber"`
	GitHubPRURL       string `json:"githubPrUrl"`
	ResolutionNote    string `json:"resolutionNote"`
}

func (s *Server) forumIdentity(w http.ResponseWriter, r *http.Request, mutation bool) (*webauth.Identity, bool) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return nil, false
	}
	if mutation && !s.WebAuth.VerifyCSRF(r) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "invalid_csrf"})
		return nil, false
	}
	return identity, true
}

func writeForumAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cloud.ErrForumNotFound):
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "forum_topic_not_found"})
	case errors.Is(err, cloud.ErrForumForbidden):
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "forum_forbidden"})
	case errors.Is(err, cloud.ErrForumInvalid):
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_forum_input"})
	default:
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "forum_unavailable"})
	}
}

func (s *Server) forumTopicsAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.forumIdentity(w, r, r.Method != http.MethodGet)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		limit := 100
		if parsed, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit"))); err == nil && parsed > 0 {
			limit = parsed
		}
		topics, err := s.Store.ListForumTopics(r.Context(), r.URL.Query().Get("kind"), r.URL.Query().Get("status"), r.URL.Query().Get("q"), limit)
		if err != nil {
			writeForumAPIError(w, err)
			return
		}
		webutil.JSON(w, http.StatusOK, map[string]any{"topics": topics, "isAdmin": cloud.IsAdminEmail(identity.User.Email)})
		return
	}

	var input forumTopicCreateRequest
	if webutil.DecodeJSON(r, 80<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	topic, err := s.Store.CreateForumTopic(r.Context(), cloud.ForumTopicDraft{
		AuthorUserID: identity.User.ID,
		Kind:         input.Kind, Title: input.Title, Body: input.Body, Severity: input.Severity, Version: input.Version,
		Environment: input.Environment, ReproductionSteps: input.ReproductionSteps, ExpectedBehavior: input.ExpectedBehavior,
		ActualBehavior: input.ActualBehavior, Tags: input.Tags, AssetIDs: input.AssetIDs,
	})
	if err != nil {
		writeForumAPIError(w, err)
		return
	}
	webutil.JSON(w, http.StatusCreated, map[string]any{"topic": topic})
}

func (s *Server) forumTopicAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.forumIdentity(w, r, false)
	if !ok {
		return
	}
	topic, err := s.Store.ForumTopicByID(r.Context(), r.PathValue("topicID"))
	if err != nil {
		writeForumAPIError(w, err)
		return
	}
	comments, err := s.Store.ListForumComments(r.Context(), topic.ID)
	if err != nil {
		writeForumAPIError(w, err)
		return
	}
	voted, err := s.Store.ForumViewerVoted(r.Context(), topic.ID, identity.User.ID)
	if err != nil {
		writeForumAPIError(w, err)
		return
	}
	topic.VotedByViewer = voted
	webutil.JSON(w, http.StatusOK, map[string]any{
		"topic":    topic,
		"comments": comments,
		"isAdmin":  cloud.IsAdminEmail(identity.User.Email),
	})
}

func (s *Server) forumCommentAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.forumIdentity(w, r, true)
	if !ok {
		return
	}
	var input forumCommentCreateRequest
	if webutil.DecodeJSON(r, 24<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	comment, err := s.Store.CreateForumComment(r.Context(), r.PathValue("topicID"), identity.User.ID, input.Body, input.AssetIDs)
	if err != nil {
		writeForumAPIError(w, err)
		return
	}
	webutil.JSON(w, http.StatusCreated, map[string]any{"comment": comment})
}

func (s *Server) forumVoteAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.forumIdentity(w, r, true)
	if !ok {
		return
	}
	voted, count, err := s.Store.ToggleForumVote(r.Context(), r.PathValue("topicID"), identity.User.ID)
	if err != nil {
		writeForumAPIError(w, err)
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"voted": voted, "voteCount": count})
}

func (s *Server) adminForumTopicAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.forumIdentity(w, r, true)
	if !ok {
		return
	}
	if !cloud.IsAdminEmail(identity.User.Email) {
		writeForumAPIError(w, cloud.ErrForumForbidden)
		return
	}
	var input forumAdminUpdateRequest
	if webutil.DecodeJSON(r, 32<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	topic, err := s.Store.AdminUpdateForumTopic(r.Context(), r.PathValue("topicID"), cloud.ForumAdminUpdate{
		Status: input.Status, Severity: input.Severity, GitHubIssueURL: input.GitHubIssueURL,
		GitHubIssueNumber: input.GitHubIssueNumber, GitHubPRURL: input.GitHubPRURL, ResolutionNote: input.ResolutionNote,
	})
	if err != nil {
		writeForumAPIError(w, err)
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"topic": topic})
}
