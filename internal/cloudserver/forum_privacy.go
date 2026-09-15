package cloudserver

import (
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

const forumAnonymousAuthor = "Community member"

type forumTopicResponse struct {
	ID                string   `json:"id"`
	AuthorEmail       string   `json:"authorEmail"`
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
	VotedByViewer     bool     `json:"votedByViewer"`
	CreatedAt         int64    `json:"createdAt"`
	UpdatedAt         int64    `json:"updatedAt"`
	ResolvedAt        int64    `json:"resolvedAt,omitempty"`
}

type forumCommentResponse struct {
	ID          string   `json:"id"`
	TopicID     string   `json:"topicId"`
	AuthorEmail string   `json:"authorEmail"`
	Body        string   `json:"body"`
	AssetIDs    []string `json:"assetIds"`
	CreatedAt   int64    `json:"createdAt"`
	UpdatedAt   int64    `json:"updatedAt"`
}

func maskForumEmail(value string) string {
	value = strings.TrimSpace(value)
	at := strings.LastIndexByte(value, '@')
	if at <= 0 || at >= len(value)-1 {
		return forumAnonymousAuthor
	}

	local := []rune(value[:at])
	domain := strings.TrimSpace(value[at+1:])
	if len(local) == 0 || domain == "" {
		return forumAnonymousAuthor
	}

	labels := strings.Split(domain, ".")
	maskedLabels := make([]string, 0, len(labels))
	for i, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			return forumAnonymousAuthor
		}
		// Keep only the final TLD. Every identifying domain label is masked.
		if i == len(labels)-1 && len(labels) > 1 {
			maskedLabels = append(maskedLabels, label)
			continue
		}
		runes := []rune(label)
		if len(runes) == 0 {
			return forumAnonymousAuthor
		}
		maskedLabels = append(maskedLabels, string(runes[0])+"***")
	}

	return string(local[0]) + "***@" + strings.Join(maskedLabels, ".")
}

func toForumTopicResponse(topic cloud.ForumTopic) forumTopicResponse {
	return forumTopicResponse{
		ID:                topic.ID,
		AuthorEmail:       maskForumEmail(topic.AuthorEmail),
		Kind:              topic.Kind,
		Title:             topic.Title,
		Body:              topic.Body,
		Status:            topic.Status,
		Severity:          topic.Severity,
		Version:           topic.Version,
		Environment:       topic.Environment,
		ReproductionSteps: topic.ReproductionSteps,
		ExpectedBehavior:  topic.ExpectedBehavior,
		ActualBehavior:    topic.ActualBehavior,
		Tags:              topic.Tags,
		AssetIDs:          topic.AssetIDs,
		GitHubIssueURL:    topic.GitHubIssueURL,
		GitHubIssueNumber: topic.GitHubIssueNumber,
		GitHubPRURL:       topic.GitHubPRURL,
		ResolutionNote:    topic.ResolutionNote,
		CommentCount:      topic.CommentCount,
		VoteCount:         topic.VoteCount,
		VotedByViewer:     topic.VotedByViewer,
		CreatedAt:         topic.CreatedAt,
		UpdatedAt:         topic.UpdatedAt,
		ResolvedAt:        topic.ResolvedAt,
	}
}

func toForumCommentResponse(comment cloud.ForumComment) forumCommentResponse {
	return forumCommentResponse{
		ID:          comment.ID,
		TopicID:     comment.TopicID,
		AuthorEmail: maskForumEmail(comment.AuthorEmail),
		Body:        comment.Body,
		AssetIDs:    comment.AssetIDs,
		CreatedAt:   comment.CreatedAt,
		UpdatedAt:   comment.UpdatedAt,
	}
}

func toForumTopicResponses(topics []cloud.ForumTopic) []forumTopicResponse {
	out := make([]forumTopicResponse, 0, len(topics))
	for _, topic := range topics {
		out = append(out, toForumTopicResponse(topic))
	}
	return out
}

func toForumCommentResponses(comments []cloud.ForumComment) []forumCommentResponse {
	out := make([]forumCommentResponse, 0, len(comments))
	for _, comment := range comments {
		out = append(out, toForumCommentResponse(comment))
	}
	return out
}
