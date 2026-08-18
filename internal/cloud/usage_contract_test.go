package cloud

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMCPUsageContractIsExplicitlyTelemetryOnly(t *testing.T) {
	if MCPUsageAuthority != "telemetry-only" || MCPUsageDurable || MCPUsageAuthoritative() {
		t.Fatalf("bounded MCP usage queue must remain non-authoritative: authority=%q durable=%v authoritative=%v", MCPUsageAuthority, MCPUsageDurable, MCPUsageAuthoritative())
	}
}

func TestMCPUsageSummaryDoesNotLeakIntoDecisionPackages(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate cloud package")
	}
	internalRoot := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	allowed := map[string]bool{"cloud": true, "cloudserver": true}
	err := filepath.WalkDir(internalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || filepath.Ext(path) != ".go" {
			return walkErr
		}
		rel, err := filepath.Rel(internalRoot, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) == 0 || allowed[parts[0]] {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), "MCPUsageSummary") {
			t.Errorf("%s references MCPUsageSummary outside analytics packages; telemetry is non-authoritative and must not drive billing/entitlement/access decisions", filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
