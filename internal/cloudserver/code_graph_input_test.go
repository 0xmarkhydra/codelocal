package cloudserver

import (
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCodeGraphQueryParamsAreRuneBounded(t *testing.T) {
	symbol := strings.Repeat("đ", 200)
	repository := strings.Repeat("仓", 300)
	req := httptest.NewRequest("GET", "/dashboard/code-graph?symbol="+symbol+"&repository="+repository, nil)
	gotSymbol, gotRepository := codeGraphSymbol(req), codeGraphRepository(req)
	if !utf8.ValidString(gotSymbol) || !utf8.ValidString(gotRepository) {
		t.Fatal("bounded graph parameters must preserve valid UTF-8")
	}
	if utf8.RuneCountInString(gotSymbol) != 160 || utf8.RuneCountInString(gotRepository) != 240 {
		t.Fatalf("unexpected rune bounds symbol=%d repository=%d", utf8.RuneCountInString(gotSymbol), utf8.RuneCountInString(gotRepository))
	}
}
