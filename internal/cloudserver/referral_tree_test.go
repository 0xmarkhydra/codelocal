package cloudserver

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestRenderReferralTreeUsesRelationshipNodes(t *testing.T) {
	users := []adminUserState{
		{AdminUser: cloud.AdminUser{ID: "root", Email: "root@example.com", ReferralCode: "MMON", InviteCount: 1}},
		{AdminUser: cloud.AdminUser{ID: "child", Email: "child@example.com", ReferralCode: "ABC123", ReferredByCode: "MMON"}},
	}

	html := renderReferralTree(users)
	for _, want := range []string{
		`class="referral-branch"`,
		`class="referral-children"`,
		`data-referral-node`,
		`data-referral-toggle`,
		`root@example.com`,
		`child@example.com`,
		`data-search="child@example.com abc123 mmon"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("referral graph missing %q", want)
		}
	}
	if strings.Contains(html, `<details class="tree-branch"`) {
		t.Fatal("referral graph must not fall back to the legacy accordion tree")
	}
}
