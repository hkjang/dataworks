package proxy

import (
	"encoding/json"
	"net/http"
	"strconv"

	"dataworks/internal/config"
)

// clampMaxOutputTokens enforces an output-token ceiling on a Chat Completions body. Every
// supported field present in the request is clamped, including requests that send both
// max_tokens and max_completion_tokens. If neither field is present, max_tokens is injected.
// Returns (newBody, from, to, changed). from is -1 when the field was absent (injected), and
// is the largest original value when multiple fields were clamped. Safe no-op on parse failure
// or cap <= 0.
func clampMaxOutputTokens(body []byte, maxOut int) ([]byte, int, int, bool) {
	return clampTokenFields(body, maxOut, true, "max_tokens", "max_tokens", "max_completion_tokens")
}

// clampExplicitMaxOutputTokens applies an absolute safety ceiling only when the client sent
// a token field. It deliberately does not inject a field, preserving the historical meaning
// of limits.max_output_tokens=0 while still preventing values above the service maximum.
func clampExplicitMaxOutputTokens(body []byte, maxOut int) ([]byte, int, int, bool) {
	return clampTokenFields(body, maxOut, false, "max_tokens", "max_tokens", "max_completion_tokens")
}

// clampResponsesMaxOutputTokens applies the configured Responses API ceiling and injects the
// native max_output_tokens field when it is absent.
func clampResponsesMaxOutputTokens(body []byte, maxOut int) ([]byte, int, int, bool) {
	return clampTokenFields(body, maxOut, true, "max_output_tokens", "max_output_tokens")
}

// clampExplicitResponsesMaxOutputTokens applies only the service absolute ceiling to an
// explicitly supplied Responses API max_output_tokens value.
func clampExplicitResponsesMaxOutputTokens(body []byte, maxOut int) ([]byte, int, int, bool) {
	return clampTokenFields(body, maxOut, false, "max_output_tokens", "max_output_tokens")
}

func clampTokenFields(body []byte, maxOut int, inject bool, injectField string, fields ...string) ([]byte, int, int, bool) {
	if maxOut <= 0 || len(body) == 0 {
		return body, 0, 0, false
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return body, 0, 0, false
	}
	present := false
	changed := false
	from := 0
	for _, field := range fields {
		cur, ok := numField(root, field)
		if !ok {
			continue
		}
		present = true
		if cur > maxOut {
			root[field] = maxOut
			changed = true
			if cur > from {
				from = cur
			}
		}
	}
	if !present {
		if !inject {
			return body, 0, 0, false
		}
		root[injectField] = maxOut
		from = -1
		changed = true
	}
	if !changed {
		return body, 0, 0, false
	}
	out, err := json.Marshal(root)
	if err != nil {
		return body, 0, 0, false
	}
	return out, from, maxOut, true
}

// countMessages returns the length of the chat request's messages array (0 on parse failure
// or when absent).
func countMessages(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	var req struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return 0
	}
	return len(req.Messages)
}

// numField reads an integer-valued JSON field (numbers decode as float64).
func numField(root map[string]any, key string) (int, bool) {
	v, ok := root[key]
	if !ok {
		return 0, false
	}
	if f, ok := v.(float64); ok {
		return int(f), true
	}
	return 0, false
}

// stepLimits clamps supported output-token fields to limits.max_output_tokens (when set), while
// always enforcing the service absolute maximum. Token mutation is endpoint-specific: Chat
// Completions uses max_tokens/max_completion_tokens and Responses uses max_output_tokens.
// Other POST endpoints still receive the body-size guard but never get a token field injected.
// Runs after deprecation, before governance, so the clamped body flows downstream.
func (rc *requestPipeline) stepLimits() bool {
	s, r, w := rc.s, rc.r, rc.w
	if r.Method != http.MethodPost {
		return true
	}
	lim := s.limitsConf()

	// Input guard: reject oversized request bodies before any upstream work.
	if lim.MaxRequestBytes > 0 && len(rc.body) > lim.MaxRequestBytes {
		s.metrics.IncLimitsRejected()
		w.Header().Set("X-Request-Bytes", strconv.Itoa(len(rc.body)))
		writeOpenAIError(w, http.StatusRequestEntityTooLarge,
			"request body exceeds the configured limit ("+strconv.Itoa(len(rc.body))+" > "+strconv.Itoa(lim.MaxRequestBytes)+" bytes)",
			"invalid_request_error", "payload_too_large")
		return false
	}

	// Message-count guard: reject context-stuffed message arrays.
	if lim.MaxMessages > 0 {
		if n := countMessages(rc.body); n > lim.MaxMessages {
			s.metrics.IncLimitsRejected()
			w.Header().Set("X-Message-Count", strconv.Itoa(n))
			writeOpenAIError(w, http.StatusBadRequest,
				"too many messages ("+strconv.Itoa(n)+" > "+strconv.Itoa(lim.MaxMessages)+")",
				"invalid_request_error", "too_many_messages")
			return false
		}
	}

	configuredMax := lim.MaxOutputTokens
	inject := configuredMax > 0
	maxOut := config.MaxSupportedOutputTokens
	if configuredMax > 0 && configuredMax < maxOut {
		maxOut = configuredMax
	}
	var newBody []byte
	var from, to int
	var changed bool
	switch r.URL.Path {
	case "/v1/chat/completions":
		if inject {
			newBody, from, to, changed = clampMaxOutputTokens(rc.body, maxOut)
		} else {
			newBody, from, to, changed = clampExplicitMaxOutputTokens(rc.body, maxOut)
		}
	case "/v1/responses":
		if inject {
			newBody, from, to, changed = clampResponsesMaxOutputTokens(rc.body, maxOut)
		} else {
			newBody, from, to, changed = clampExplicitResponsesMaxOutputTokens(rc.body, maxOut)
		}
	default:
		return true
	}
	if !changed {
		return true
	}
	rc.body = newBody
	s.metrics.IncLimitsClamped()
	if from < 0 {
		w.Header().Set("X-Max-Tokens-Clamped", "injected:"+strconv.Itoa(to))
	} else {
		w.Header().Set("X-Max-Tokens-Clamped", strconv.Itoa(from)+"->"+strconv.Itoa(to))
	}
	return true
}
