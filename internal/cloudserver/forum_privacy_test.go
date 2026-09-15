package cloudserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestMaskForumEmail(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "standard", in: "mong@gmail.com", want: "m***@g***.com"},
		{name: "company domain", in: "alice@engineering.example.co.uk", want: "a***@e***.e***.c***.uk"},
		{name: "single label domain", in: "a@localhost", want: "a***@l***"},
		{name: "trim spaces", in: "  User@Example.COM  ", want: "U***@E***.COM"},
		{name: "invalid", in: "not-an-email", want: forumAnonymousAuthor},
		{name: "empty", in: "", want: forumAnonymousAuthor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskForumEmail(tt.in); got != tt.want {
				t.Fatalf("maskForumEmail(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestForumTopicResponseMasksPrivateIdentity(t *testing.T) {
	payload, err := json.Marshal(toForumTopicResponse(cloud.ForumTopic{
		ID: "forum_test", AuthorUserID: "user_secret_123", AuthorEmail: "secret@example.com",
		Kind: "bug", Title: "Public bug", Body: "Details", Status: "open", Tags: []string{}, AssetIDs: []string{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	for _, forbidden := range []string{"secret@example.com", "user_secret_123", "authorUserId"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("forum topic response leaked private identity %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"authorEmail":"s***@e***.com"`) {
		t.Fatalf("forum topic response did not return masked email: %s", body)
	}
}

func TestForumCommentResponseMasksPrivateIdentity(t *testing.T) {
	payload, err := json.Marshal(toForumCommentResponse(cloud.ForumComment{
		ID: "comment_test", TopicID: "forum_test", AuthorUserID: "user_comment_secret", AuthorEmail: "reply@company.dev",
		Body: "Reply", AssetIDs: []string{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	for _, forbidden := range []string{"reply@company.dev", "user_comment_secret", "authorUserId"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("forum comment response leaked private identity %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"authorEmail":"r***@c***.dev"`) {
		t.Fatalf("forum comment response did not return masked email: %s", body)
	}
}
