package cloudserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestPublicForumTopicDoesNotExposeAccountIdentity(t *testing.T) {
	payload, err := json.Marshal(toPublicForumTopic(cloud.ForumTopic{
		ID: "forum_test", AuthorUserID: "user_secret", AuthorEmail: "secret@example.com", Kind: "bug", Title: "Public bug", Body: "Details", Status: "open", Tags: []string{}, AssetIDs: []string{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	if strings.Contains(body, "secret@example.com") || strings.Contains(body, "user_secret") {
		t.Fatalf("public forum payload leaked private account identity: %s", body)
	}
	if !strings.Contains(body, "Community member") {
		t.Fatalf("public forum payload missing anonymous author label: %s", body)
	}
}
