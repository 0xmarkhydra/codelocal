package cloudserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRuntimeTransportDispatchesBootstrapBeforeFrontend(t *testing.T) {
	server := &Server{}
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := server.runtimeTransportMiddleware(downstream)

	req := httptest.NewRequest(http.MethodPost, "/api/client/runtime/bootstrap/exchange", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("bootstrap transport was not dispatched to Go handler: status=%d", res.Code)
	}
}

func TestRuntimeTransportIsExactMethodAndPath(t *testing.T) {
	server := &Server{}
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := server.runtimeTransportMiddleware(downstream)

	for _, testCase := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/client/runtime/bootstrap/exchange"},
		{method: http.MethodPost, path: "/api/client/runtime/bootstrap/exchange/extra"},
		{method: http.MethodPost, path: "/api/client/runtime/other"},
	} {
		req := httptest.NewRequest(testCase.method, testCase.path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusTeapot {
			t.Fatalf("unexpected runtime transport interception for %s %s: status=%d", testCase.method, testCase.path, res.Code)
		}
	}
}
