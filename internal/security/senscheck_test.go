package security

import "testing"

func TestBrowserCredentialStoresAreSensitive(t *testing.T) {
	mustBlock := []string{
		".ironize_browser/profile/Default/Cookies",
		".ironize_browser/p1/Default/Cookies",
		".ironize_browser/profile/Profile 1/Login Data",
		".ironize_browser/profile/Default/History",
		"Veo Tool/data/chrome-profile/Default/Web Data",
		"Veo Tool/data/chrome-profile/Default/Cookies",
	}
	for _, p := range mustBlock {
		if !IsSensitivePath(p) {
			t.Errorf("browser credential store must be blocked: %s", p)
		}
	}
}

func TestProjectFilesStayReadable(t *testing.T) {
	mustAllow := []string{
		"Shopify Automatic Ironing Machine/config/settings_schema.json",
		"automatic-iron-machine-main/README.md",
		"History.md",
		"shopify.app.toml",
		".env.example",
		"src/history.ts",
	}
	for _, p := range mustAllow {
		if IsSensitivePath(p) {
			t.Errorf("project file must stay readable: %s", p)
		}
	}
}

func TestExistingRulesUnchanged(t *testing.T) {
	still := []string{".git/config", ".env", "certs/server.pem", "credentials.json"}
	for _, p := range still {
		if !IsSensitivePath(p) {
			t.Errorf("pre-existing sensitive rule regressed: %s", p)
		}
	}
}

func TestShellReadersCannotReachCredentialStores(t *testing.T) {
	root := "/Users/mac/Desktop"
	mustBlock := []string{
		`head -c 16 '.ironize_browser/profile/Default/Cookies' | xxd`,
		`cat .ironize_browser/profile/Default/Cookies`,
		`sqlite3 .ironize_browser/profile/Default/Cookies 'select 1'`,
		`cp '.ironize_browser/profile/Default/Login Data' /tmp/x`,
		`grep -a session '.ironize_browser/p1/Default/Cookies'`,
		`cat 'Veo Tool/data/chrome-profile/Default/Web Data'`,
		`tar czf /tmp/x.tgz .ironize_browser/profile`,
		`cat .env`,
	}
	for _, cmd := range mustBlock {
		d := Classify(cmd, NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
		if !d.Blocked {
			t.Errorf("shell command must be blocked, got %s: %s", d.RiskLevel, cmd)
		}
	}
}

func TestNormalCommandsStillWork(t *testing.T) {
	root := "/Users/mac/Desktop"
	mustAllow := []string{
		`ls -la`,
		`git status --short`,
		`go test ./...`,
		`cat 'Shopify Automatic Ironing Machine/config/settings_schema.json'`,
		`grep -rn "cookie" docs/`,
		`shopify store auth list --json`,
		`node -e "console.log(1)"`,
	}
	for _, cmd := range mustAllow {
		d := Classify(cmd, NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
		if d.Blocked {
			t.Errorf("normal command must not be blocked: %s (%s)", cmd, d.Reason)
		}
	}
}
