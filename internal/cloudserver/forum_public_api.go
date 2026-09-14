package cloudserver

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type publicForumTopic struct {
	ID                string   `json:"id"`
	Author            string   `json:"author"`
	Kind              string   `json:"kind"`
	Title             string   `json:"title"`
	Body              string   `json:"body"`
	Status            string   `json:"status"`
	Severity          string   `json:"severity,omitempty"`
	Version           string   `json:"version,omitempty"`
	Environment       string   `json:"environment,omitempty"`
	ReproductionSteps string   `json:"reproductionSteps,omitempty"`
	ExpectedBehavior  string   `json:"expectedBehavior,omitempty"`
	ActualBehavior    string   `json:"actualBehavior,omitempty"`
	Tags              []string `json:"tags"`
	AssetIDs          []string `json:"assetIds"`
	GitHubIssueURL    string   `json:"githubIssueUrl,omitempty"`
	GitHubIssueNumber int64    `json:"githubIssueNumber,omitempty"`
	GitHubPRURL       string   `json:"githubPrUrl,omitempty"`
	ResolutionNote    string   `json:"resolutionNote,omitempty"`
	CommentCount      int      `json:"commentCount"`
	VoteCount         int      `json:"voteCount"`
	CreatedAt         int64    `json:"createdAt"`
	UpdatedAt         int64    `json:"updatedAt"`
	ResolvedAt        int64    `json:"resolvedAt,omitempty"`
}

type publicForumComment struct {
	ID        string   `json:"id"`
	Author    string   `json:"author"`
	Body      string   `json:"body"`
	AssetIDs  []string `json:"assetIds"`
	CreatedAt int64    `json:"createdAt"`
	UpdatedAt int64    `json:"updatedAt"`
}

func toPublicForumTopic(topic cloud.ForumTopic) publicForumTopic {
	return publicForumTopic{
		ID: topic.ID, Author: "Community member", Kind: topic.Kind, Title: topic.Title, Body: topic.Body, Status: topic.Status,
		Severity: topic.Severity, Version: topic.Version, Environment: topic.Environment, ReproductionSteps: topic.ReproductionSteps,
		ExpectedBehavior: topic.ExpectedBehavior, ActualBehavior: topic.ActualBehavior, Tags: topic.Tags, AssetIDs: topic.AssetIDs,
		GitHubIssueURL: topic.GitHubIssueURL, GitHubIssueNumber: topic.GitHubIssueNumber, GitHubPRURL: topic.GitHubPRURL,
		ResolutionNote: topic.ResolutionNote, CommentCount: topic.CommentCount, VoteCount: topic.VoteCount,
		CreatedAt: topic.CreatedAt, UpdatedAt: topic.UpdatedAt, ResolvedAt: topic.ResolvedAt,
	}
}

func toPublicForumComment(comment cloud.ForumComment) publicForumComment {
	return publicForumComment{ID: comment.ID, Author: "Community member", Body: comment.Body, AssetIDs: comment.AssetIDs, CreatedAt: comment.CreatedAt, UpdatedAt: comment.UpdatedAt}
}

func (s *Server) publicForumTopicsAPI(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if parsed, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit"))); err == nil && parsed > 0 {
		limit = parsed
	}
	topics, err := s.Store.ListForumTopics(r.Context(), r.URL.Query().Get("kind"), r.URL.Query().Get("status"), r.URL.Query().Get("q"), limit)
	if err != nil {
		writeForumAPIError(w, err)
		return
	}
	out := make([]publicForumTopic, 0, len(topics))
	for _, topic := range topics {
		out = append(out, toPublicForumTopic(topic))
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"topics": out})
}

func (s *Server) publicForumTopicAPI(w http.ResponseWriter, r *http.Request) {
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
	publicComments := make([]publicForumComment, 0, len(comments))
	for _, comment := range comments {
		publicComments = append(publicComments, toPublicForumComment(comment))
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"topic": toPublicForumTopic(topic), "comments": publicComments})
}
