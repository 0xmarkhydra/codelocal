package cloud

import (
	"errors"
	"net/url"
	"strings"
)

var (
	ErrForumNotFound  = errors.New("forum topic not found")
	ErrForumForbidden = errors.New("forum action forbidden")
	ErrForumInvalid   = errors.New("invalid forum input")
)

type ForumTopic struct {
	ID                string   `json:"id"`
	AuthorUserID      string   `json:"authorUserId"`
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

type ForumTopicDraft struct {
	AuthorUserID      string
	Kind              string
	Title             string
	Body              string
	Severity          string
	Version           string
	Environment       string
	ReproductionSteps string
	ExpectedBehavior  string
	ActualBehavior    string
	Tags              []string
	AssetIDs          []string
}

type ForumComment struct {
	ID           string   `json:"id"`
	TopicID      string   `json:"topicId"`
	AuthorUserID string   `json:"authorUserId"`
	AuthorEmail  string   `json:"authorEmail"`
	Body         string   `json:"body"`
	AssetIDs     []string `json:"assetIds"`
	CreatedAt    int64    `json:"createdAt"`
	UpdatedAt    int64    `json:"updatedAt"`
}

type ForumAdminUpdate struct {
	Status            string
	Severity          string
	GitHubIssueURL    string
	GitHubIssueNumber int64
	GitHubPRURL       string
	ResolutionNote    string
}

func normalizeForumDeleteReason(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 500 {
		return "", ErrForumInvalid
	}
	return value, nil
}

func normalizeForumKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "question", "bug", "idea":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeForumStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "open", "under_review", "planned", "in_progress", "resolved", "closed":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeForumSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "low", "medium", "high", "critical":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeForumTags(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 40 {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
		if len(out) == 8 {
			break
		}
	}
	return out
}

func normalizeForumAssetIDs(values []string, limit int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = NormalizeMediaAssetID(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) == limit {
			break
		}
	}
	return out
}

func normalizeForumDraft(input ForumTopicDraft) (ForumTopicDraft, error) {
	input.AuthorUserID = strings.TrimSpace(input.AuthorUserID)
	input.Kind = normalizeForumKind(input.Kind)
	input.Title = strings.TrimSpace(input.Title)
	input.Body = strings.TrimSpace(input.Body)
	input.Severity = normalizeForumSeverity(input.Severity)
	input.Version = strings.TrimSpace(input.Version)
	input.Environment = strings.TrimSpace(input.Environment)
	input.ReproductionSteps = strings.TrimSpace(input.ReproductionSteps)
	input.ExpectedBehavior = strings.TrimSpace(input.ExpectedBehavior)
	input.ActualBehavior = strings.TrimSpace(input.ActualBehavior)
	input.Tags = normalizeForumTags(input.Tags)
	input.AssetIDs = normalizeForumAssetIDs(input.AssetIDs, 6)
	if input.AuthorUserID == "" || input.Kind == "" || input.Title == "" || input.Body == "" {
		return ForumTopicDraft{}, ErrForumInvalid
	}
	if len(input.Title) > 180 || len(input.Body) > 30000 || len(input.Version) > 120 || len(input.Environment) > 1000 || len(input.ReproductionSteps) > 12000 || len(input.ExpectedBehavior) > 6000 || len(input.ActualBehavior) > 6000 {
		return ForumTopicDraft{}, ErrForumInvalid
	}
	if input.Kind != "bug" {
		input.Severity = ""
		input.Version = ""
		input.Environment = ""
		input.ReproductionSteps = ""
		input.ExpectedBehavior = ""
		input.ActualBehavior = ""
	}
	return input, nil
}

func validGitHubLink(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "github.com" || strings.HasSuffix(host, ".github.com")
}

func normalizeForumAdminUpdate(input ForumAdminUpdate) (ForumAdminUpdate, error) {
	input.Status = normalizeForumStatus(input.Status)
	input.Severity = normalizeForumSeverity(input.Severity)
	input.GitHubIssueURL = strings.TrimSpace(input.GitHubIssueURL)
	input.GitHubPRURL = strings.TrimSpace(input.GitHubPRURL)
	input.ResolutionNote = strings.TrimSpace(input.ResolutionNote)
	if input.Status == "" || input.GitHubIssueNumber < 0 || !validGitHubLink(input.GitHubIssueURL) || !validGitHubLink(input.GitHubPRURL) || len(input.ResolutionNote) > 6000 {
		return ForumAdminUpdate{}, ErrForumInvalid
	}
	return input, nil
}
