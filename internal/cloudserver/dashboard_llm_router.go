package cloudserver

import (
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

const (
	dashboardModelAuto = "auto"
	dashboardModelGLM  = "glm-5.3-flash"
	dashboardModelQwen = "qwen3.8-flash"
	dashboardModelMuse = "muse-spark-1.2-contributor-free"
)

type dashboardLLMTarget struct {
	ID        string
	BaseURL   string
	APIKey    string
	Model     string
	Community bool
}

type dashboardSelectedModelError struct {
	Err error
}

func (e *dashboardSelectedModelError) Error() string {
	return "Thánh Gióng không thể kết nối model đã chọn. CodeLocal không chuyển sang model khác; vui lòng thử lại."
}

func (e *dashboardSelectedModelError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func dashboardRouteError(selection string, err error) error {
	if dashboardNormalizeModelSelection(selection) == dashboardModelAuto {
		return err
	}
	if err == nil {
		err = errors.New("selected model route is unavailable")
	}
	return &dashboardSelectedModelError{Err: err}
}

var dashboardLLMHealth = struct {
	sync.Mutex
	cooldownUntil map[string]time.Time
}{cooldownUntil: map[string]time.Time{}}

func dashboardNormalizeModelSelection(raw string) string {
	trimmed := strings.TrimSpace(raw)
	switch strings.ToLower(trimmed) {
	case "", "auto", "thánh gióng", "thanh giong":
		return dashboardModelAuto
	case dashboardModelGLM, "glm 5.3 flash", "glm-5.3-flash-20260826":
		return dashboardModelGLM
	case dashboardModelQwen, "qwen 3.8 flash", "qwen3.8-flash-next":
		return dashboardModelQwen
	case dashboardModelMuse, "muse spark 1.2", "muse-spark-1.2":
		return dashboardModelMuse
	default:
		if dashboardModelIDSafe(trimmed) {
			return trimmed
		}
		return dashboardModelAuto
	}
}

func dashboardEmperoTarget(model string) dashboardLLMTarget {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("CODELOCAL_EMPERO_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://free.empero.org/v1"
	}
	apiKey := strings.TrimSpace(os.Getenv("CODELOCAL_EMPERO_API_KEY"))
	if apiKey == "" {
		apiKey = "free"
	}
	return dashboardLLMTarget{ID: "empero:" + model, BaseURL: baseURL, APIKey: apiKey, Model: model, Community: true}
}

func dashboardMuseTarget() (dashboardLLMTarget, bool) {
	apiKey := strings.TrimSpace(os.Getenv("OPENCODE_ZEN_API_KEY"))
	baseURL := "https://opencode.ai/zen/v1"
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CODELOCAL_LLM_PROVIDER")), "zen") {
		if configured := strings.TrimSpace(os.Getenv("CODELOCAL_LLM_API_KEY")); configured != "" {
			apiKey = configured
		}
		if configured := strings.TrimSpace(os.Getenv("CODELOCAL_LLM_BASE_URL")); configured != "" {
			baseURL = strings.TrimRight(configured, "/")
		}
	}
	if apiKey == "" {
		return dashboardLLMTarget{}, false
	}
	return dashboardLLMTarget{ID: "zen:" + dashboardModelMuse, BaseURL: baseURL, APIKey: apiKey, Model: dashboardModelMuse}, true
}

func dashboardLegacyTarget() (dashboardLLMTarget, bool) {
	apiKey, baseURL, model := dashboardLLMConfig()
	if strings.TrimSpace(apiKey) == "" {
		return dashboardLLMTarget{}, false
	}
	community := strings.Contains(strings.ToLower(baseURL), "free.empero.org")
	return dashboardLLMTarget{ID: "legacy:" + model, BaseURL: baseURL, APIKey: apiKey, Model: model, Community: community}, true
}

func dashboardLLMRoute(selection string, allowCommunity bool) []dashboardLLMTarget {
	selection = dashboardNormalizeModelSelection(selection)
	poolDefault, hasPool := dashboardAIPoolTarget("")
	ordered := make([]dashboardLLMTarget, 0, 7)
	appendTarget := func(target dashboardLLMTarget) {
		if target.Community && !allowCommunity {
			return
		}
		for _, existing := range ordered {
			if existing.BaseURL == target.BaseURL && existing.Model == target.Model {
				return
			}
		}
		ordered = append(ordered, target)
	}

	// Once CodeLocal Pool is configured it is the only chat execution plane.
	// Auto uses the Pool default model, while explicit canonical selections are
	// forwarded to Pool unchanged. Legacy providers remain available only for
	// environments that have not enabled Pool yet.
	if hasPool {
		if selection == dashboardModelAuto {
			appendTarget(poolDefault)
			return ordered
		}
		if pool, ok := dashboardAIPoolTarget(selection); ok {
			appendTarget(pool)
		}
		return ordered
	}

	glm := dashboardEmperoTarget(dashboardModelGLM)
	qwen := dashboardEmperoTarget(dashboardModelQwen)
	muse, hasMuse := dashboardMuseTarget()
	shopDefault, hasShop := dashboardShopAIKeyTarget("")
	switch selection {
	case dashboardModelGLM:
		appendTarget(glm)
	case dashboardModelQwen:
		appendTarget(qwen)
	case dashboardModelMuse:
		if hasMuse {
			appendTarget(muse)
		}
	case dashboardModelAuto:
		if hasShop {
			appendTarget(shopDefault)
		}
		appendTarget(glm)
		appendTarget(qwen)
		if hasMuse {
			appendTarget(muse)
		}
		if legacy, ok := dashboardLegacyTarget(); ok {
			appendTarget(legacy)
		}
	default:
		if shop, ok := dashboardShopAIKeyTarget(selection); ok {
			appendTarget(shop)
		}
	}
	return ordered
}

func dashboardLooksSensitive(value string) bool {
	lower := strings.ToLower(value)
	for _, pattern := range []string{"authorization: bearer", "-----begin private key-----", "api_key=", "apikey=", "access_token=", "refresh_token=", "token=", "secret=", "password=", "client_secret", "private_key", ".env"} {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func dashboardCommunityEligible(req dashboardChatRequest) bool {
	if strings.TrimSpace(req.Image) != "" || req.ImageMeta != nil || req.Workspace != nil || dashboardLooksSensitive(req.Message) {
		return false
	}
	for _, item := range req.History {
		if dashboardLooksSensitive(item.Content) {
			return false
		}
	}
	return true
}

func dashboardTargetCoolingDown(target dashboardLLMTarget) bool {
	dashboardLLMHealth.Lock()
	defer dashboardLLMHealth.Unlock()
	until := dashboardLLMHealth.cooldownUntil[target.ID]
	if until.IsZero() || time.Now().After(until) {
		delete(dashboardLLMHealth.cooldownUntil, target.ID)
		return false
	}
	return true
}

func dashboardMarkTargetFailed(target dashboardLLMTarget) {
	dashboardLLMHealth.Lock()
	dashboardLLMHealth.cooldownUntil[target.ID] = time.Now().Add(30 * time.Second)
	dashboardLLMHealth.Unlock()
}

func dashboardMarkTargetHealthy(target dashboardLLMTarget) {
	dashboardLLMHealth.Lock()
	delete(dashboardLLMHealth.cooldownUntil, target.ID)
	dashboardLLMHealth.Unlock()
}

func dashboardRetryableLLMError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *httpError
	if errors.As(err, &httpErr) {
		switch httpErr.Status {
		case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func dashboardValidateToolCalls(calls []llmToolCall) error {
	for _, call := range calls {
		arguments := strings.TrimSpace(call.Arguments)
		if strings.TrimSpace(call.Name) == "" || !strings.HasPrefix(arguments, "{") || !strings.HasSuffix(arguments, "}") {
			return errors.New("malformed tool call")
		}
	}
	return nil
}

func callDashboardLLMWithTools(selection string, allowCommunity bool, messages []map[string]any, tools []map[string]any) (dashboardLLMTarget, []llmToolCall, string, error) {
	messages = dashboardWithSkillContext(messages)
	route := dashboardLLMRoute(selection, allowCommunity)
	if len(route) == 0 {
		return dashboardLLMTarget{}, nil, "", dashboardRouteError(selection, errors.New("no configured LLM route"))
	}
	var lastErr error
	for index, target := range route {
		if dashboardTargetCoolingDown(target) && index < len(route)-1 {
			continue
		}
		for attempt := 0; attempt < 2; attempt++ {
			calls, content, err := callLLMWithTools(target.BaseURL, target.APIKey, target.Model, messages, tools)
			if err == nil {
				err = dashboardValidateToolCalls(calls)
			}
			if err == nil {
				dashboardMarkTargetHealthy(target)
				return target, calls, content, nil
			}
			lastErr = err
			if !dashboardRetryableLLMError(err) || attempt == 1 {
				break
			}
		}
		dashboardMarkTargetFailed(target)
	}
	return dashboardLLMTarget{}, nil, "", dashboardRouteError(selection, lastErr)
}

type dashboardCountingWriter struct {
	http.ResponseWriter
	written int
}

func (w *dashboardCountingWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.written += n
	return n, err
}

func proxyDashboardLLMRouteStream(w http.ResponseWriter, flusher http.Flusher, selection string, allowCommunity bool, messages []map[string]any, tools []map[string]any, r *http.Request, s *Server, userID string) (dashboardLLMTarget, error) {
	skillPlan := dashboardSkillPlanForUser(r.Context(), s, userID, messages)
	if value := dashboardSkillHeaderValue(skillPlan); value != "" {
		w.Header().Set(dashboardSkillHeader, value)
	} else {
		w.Header().Del(dashboardSkillHeader)
	}
	r = r.WithContext(cloud.WithDashboardChatSkills(r.Context(), dashboardSkillMetadata(skillPlan)))
	messages = dashboardWithSkillPlan(messages, skillPlan)
	route := dashboardLLMRoute(selection, allowCommunity)
	if len(route) == 0 {
		return dashboardLLMTarget{}, dashboardRouteError(selection, errors.New("no configured LLM route"))
	}
	var lastErr error
	for index, target := range route {
		if dashboardTargetCoolingDown(target) && index < len(route)-1 {
			continue
		}
		for attempt := 0; attempt < 2; attempt++ {
			tracked := &dashboardCountingWriter{ResponseWriter: w}
			err := proxyLLMStream(tracked, flusher, target.BaseURL, target.APIKey, target.Model, messages, tools, r, s, userID)
			if err == nil {
				dashboardMarkTargetHealthy(target)
				return target, nil
			}
			lastErr = err
			if tracked.written > 0 {
				return target, err
			}
			if !dashboardRetryableLLMError(err) || attempt == 1 {
				break
			}
		}
		dashboardMarkTargetFailed(target)
	}
	return dashboardLLMTarget{}, dashboardRouteError(selection, lastErr)
}
