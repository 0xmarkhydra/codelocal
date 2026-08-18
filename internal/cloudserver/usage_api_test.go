package cloudserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestUsageResourceDTOKeepsEstimatedMCPPayloadSemantics(t *testing.T) {
	payload := buildUsageResourceDTO(
		cloud.MCPUsageSummary{Calls: 7, InputTokensEst: 100, OutputTokensEst: 50, TotalTokensEst: 150},
		cloud.MCPUsageSummary{Calls: 20, InputTokensEst: 600, OutputTokensEst: 300, TotalTokensEst: 900},
		cloud.MCPUsageSummary{Calls: 30, InputTokensEst: 900, OutputTokensEst: 500, TotalTokensEst: 1400},
	)

	if !payload.Estimated || payload.Scope != dashboardUsageScope {
		t.Fatalf("unexpected usage metadata: %#v", payload)
	}
	if payload.Last24h.Calls != 7 || payload.Last24h.InputTokensEstimated != 100 || payload.Last24h.OutputTokensEstimated != 50 || payload.Last24h.TotalTokensEstimated != 150 {
		t.Fatalf("unexpected last24h payload: %#v", payload.Last24h)
	}
	if payload.Last30d.TotalTokensEstimated != 900 || payload.AllTime.TotalTokensEstimated != 1400 {
		t.Fatalf("unexpected longer usage windows: %#v", payload)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{`"usd"`, `"cost"`, `"price"`, `"reference"`} {
		if strings.Contains(strings.ToLower(serialized), forbidden) {
			t.Fatalf("usage API must not invent billing/cost field %q: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, "not full AI model/provider billing tokens") {
		t.Fatalf("usage API lost explicit scope disclaimer: %s", serialized)
	}
}
