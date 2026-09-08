package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dataworks/internal/store"
)

func (s *Server) handleV1DataProductQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	productKey, ok := dataProductQueryKey(r.URL.EscapedPath())
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "data product endpoint not found", "invalid_request_error", "not_found")
		return
	}
	apiKeyID, authCtx, authOK := s.authenticateProxyContext(r)
	if !authOK {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid API key", "invalid_request_error", "invalid_api_key")
		return
	}
	if apiKeyID == "" {
		apiKeyID = "anonymous"
	}
	apiKeyHash := ""
	if token := bearerToken(r.Header.Get("Authorization")); token != "" {
		apiKeyHash = hashProxyKey(token)
	}

	product, found, err := s.db.GetDataProduct(r.Context(), productKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "product_lookup_failed")
		return
	}
	if !found {
		writeOpenAIError(w, http.StatusNotFound, "data product not found", "invalid_request_error", "product_not_found")
		return
	}
	if product.Status != "published" {
		s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "product_not_published:"+product.ProductKey)
		writeOpenAIError(w, http.StatusForbidden, "data product is not published", "invalid_request_error", "product_not_published")
		return
	}

	candidates, err := s.db.ListAPIEntitlementCandidates(r.Context(), product.ProductKey, apiKeyID, apiKeyHash)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "entitlement_lookup_failed")
		return
	}
	if len(candidates) == 0 {
		s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "missing_entitlement:"+product.ProductKey)
		writeOpenAIError(w, http.StatusForbidden, "data product entitlement is required", "invalid_request_error", "missing_entitlement")
		return
	}
	now := time.Now().UTC()
	// Prefer a grant that clears every access check. Falling straight through to the store's
	// first choice would deny a caller whose selected entitlement points at a retired contract
	// while another entitlement on the same key points at a live one. When nothing is usable
	// the store's first choice still runs the checks below so the denial names the real reason.
	ent := candidates[0]
	if usable, ok := s.usableEntitlement(r.Context(), product, candidates, now); ok {
		ent = usable
	}
	if !entitlementActive(ent, now) {
		s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "inactive_entitlement:"+ent.ID)
		writeOpenAIError(w, http.StatusForbidden, "data product entitlement is inactive or expired", "invalid_request_error", "inactive_entitlement")
		return
	}
	if !entitlementAllowsQuery(ent.Scope) {
		s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "scope_not_allowed:"+ent.Scope)
		writeOpenAIError(w, http.StatusForbidden, "entitlement scope does not allow query", "invalid_request_error", "scope_denied")
		return
	}

	var contractKey string
	var customerKey string
	var overLimit bool
	var errCode = http.StatusOK
	defer func() {
		if contractKey != "" {
			failed := errCode != http.StatusOK && !overLimit
			billingAmount := 0.0
			if errCode == http.StatusOK {
				billingAmount = 10.0
			}
			_ = s.db.IncrementUsageMetering(r.Context(), customerKey, productKey, contractKey, failed, overLimit, billingAmount)
		}
	}()

	contract, found, err := s.db.GetContractScope(r.Context(), ent.ContractKey)
	if err != nil {
		errCode = http.StatusInternalServerError
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "contract_lookup_failed")
		return
	}
	if !found || contract.ProductKey != product.ProductKey {
		errCode = http.StatusForbidden
		s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "contract_scope_missing:"+ent.ContractKey)
		writeOpenAIError(w, http.StatusForbidden, "contract scope is missing for entitlement", "invalid_request_error", "contract_scope_missing")
		return
	}
	contractKey = contract.ContractKey
	customerKey = firstNonEmpty(ent.CustomerKey, contract.CustomerKey)

	if !contractScopeActive(contract, now) {
		errCode = http.StatusForbidden
		s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "contract_scope_inactive:"+contract.ContractKey)
		writeOpenAIError(w, http.StatusForbidden, "contract scope is inactive or outside valid window", "invalid_request_error", "contract_scope_inactive")
		return
	}
	if sensitiveProduct(product) && strings.TrimSpace(contract.Purpose) == "" {
		errCode = http.StatusForbidden
		s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "missing_contract_purpose:"+contract.ContractKey)
		writeOpenAIError(w, http.StatusForbidden, "contract purpose is required for sensitive data products", "invalid_request_error", "missing_contract_purpose")
		return
	}
	if contract.RateLimit > 0 {
		allowed, used, resetAt := s.dwRateLimits.allow(contract.ContractKey, contract.RateLimit, now)
		w.Header().Set("X-DataWorks-RateLimit-Limit", strconv.Itoa(contract.RateLimit))
		w.Header().Set("X-DataWorks-RateLimit-Used", strconv.Itoa(used))
		w.Header().Set("X-DataWorks-RateLimit-Reset", resetAt.Format(time.RFC3339))
		if !allowed {
			errCode = http.StatusTooManyRequests
			overLimit = true
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(resetAt, now)))
			s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "rate_limit_exceeded:"+contract.ContractKey)
			writeOpenAIError(w, http.StatusTooManyRequests, "contract rate limit exceeded", "rate_limit_error", "contract_rate_limited")
			return
		}
	}

	requestBody, err := decodeDataProductQueryBody(r)
	if err != nil {
		errCode = http.StatusBadRequest
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	requestedFields := fieldsFromQueryBody(requestBody)
	responseFields, forbidden := contractResponseFields(contract.AllowedFields, requestedFields)
	if len(forbidden) > 0 {
		errCode = http.StatusForbidden
		s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "forbidden_fields:"+strings.Join(forbidden, ","))
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":            "requested fields exceed contract scope",
			"forbidden_fields": forbidden,
			"allowed_fields":   contract.AllowedFields,
		})
		return
	}
	if len(responseFields) == 0 {
		errCode = http.StatusForbidden
		s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "empty_contract_scope:"+contract.ContractKey)
		writeOpenAIError(w, http.StatusForbidden, "contract scope has no allowed fields", "invalid_request_error", "empty_contract_scope")
		return
	}

	data := map[string]any{}
	for _, field := range responseFields {
		val := dataWorksSampleValue(field, requestBody, product)
		if contract.MaskingPolicy != "" {
			val = applyMasking(val, contract.MaskingPolicy)
		}
		data[field] = val
	}
	s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query", product.ProductKey+":"+ent.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"product_key":    product.ProductKey,
		"customer_key":   customerKey,
		"contract_key":   contract.ContractKey,
		"entitlement_id": ent.ID,
		"purpose":        contract.Purpose,
		"rate_limit":     contract.RateLimit,
		"mock":           true,
		"as_of":          now.Format(time.RFC3339Nano),
		"data":           data,
	})
}

func dataProductQueryKey(path string) (string, bool) {
	parts := dataWorksPathParts(path, "/v1/data-products/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || parts[1] != "query" {
		return "", false
	}
	return strings.TrimSpace(parts[0]), true
}

// entitlementAllowsQuery reports whether an entitlement scope grants the data product
// query action. The scope is a free-form list of grants separated by commas or spaces
// (the documented value is "data_product:query"), so it is matched grant by grant: a
// substring test accepts a deny marker such as "no-query" and rejects the wildcard
// "data_product:*". An empty scope stays unrestricted for entitlements written before
// the field existed.
func entitlementAllowsQuery(scope string) bool {
	if strings.TrimSpace(scope) == "" {
		return true
	}
	grants := strings.FieldsFunc(strings.ToLower(scope), func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	for _, grant := range grants {
		switch grant {
		case "*", "query", "data_product:*", "data_product:query":
			return true
		}
	}
	return false
}

// entitlementActive is the runtime access gate. It delegates to the store so the row
// FindAPIEntitlement picks for a key is judged by the same rule that selected it.
func entitlementActive(ent store.APIEntitlement, now time.Time) bool {
	return store.EntitlementActive(ent, now)
}

// retryAfterSeconds rounds the wait until the rate limit window resets up to a whole
// second, so a client that honours Retry-After never retries inside the closed window.
func retryAfterSeconds(resetAt time.Time, now time.Time) int {
	remaining := resetAt.Sub(now)
	if remaining <= 0 {
		return 1
	}
	seconds := int(remaining / time.Second)
	if remaining%time.Second != 0 {
		seconds++
	}
	return seconds
}

// usableEntitlement returns the first candidate grant that clears the access checks a query
// cannot influence: the entitlement itself is active and allows querying, and the contract
// scope it names still exists for this product, is inside its valid window, and carries the
// purpose a sensitive product requires. One API key can hold several entitlements for a
// product, each naming its own contract, so stopping at the first one hands 403s to callers
// whose other grant is perfectly valid. Checks that depend on the request body (allowed
// fields) or that consume quota (rate limit) stay out, so selection never burns a window.
func (s *Server) usableEntitlement(ctx context.Context, product store.DataProduct, candidates []store.APIEntitlement, now time.Time) (store.APIEntitlement, bool) {
	sensitive := sensitiveProduct(product)
	for _, ent := range candidates {
		if !entitlementActive(ent, now) || !entitlementAllowsQuery(ent.Scope) {
			continue
		}
		contract, found, err := s.db.GetContractScope(ctx, ent.ContractKey)
		if err != nil || !found || contract.ProductKey != product.ProductKey {
			continue
		}
		if !contractScopeActive(contract, now) {
			continue
		}
		if sensitive && strings.TrimSpace(contract.Purpose) == "" {
			continue
		}
		return ent, true
	}
	return store.APIEntitlement{}, false
}

// contractScopeCanServe reports whether a contract scope is able to answer product queries
// now or at any later point. A revoked scope or a window that already closed never will, so
// governance readers can ignore what such a scope promises; a scope whose window opens later
// still has to hold up.
func contractScopeCanServe(scope store.ContractScope, now time.Time) bool {
	if strings.ToLower(strings.TrimSpace(scope.Status)) != "active" {
		return false
	}
	if raw := strings.TrimSpace(scope.ValidTo); raw != "" {
		validTo, err := time.Parse(time.RFC3339Nano, raw)
		// An unparseable window denies every query below, so the scope can never serve either.
		if err != nil || validTo.Before(now) {
			return false
		}
	}
	return true
}

// contractScopeStatuses lists the lifecycle states a contract scope may be stored with.
// contractScopeCanServe only serves "active", so every other value has to be a state an
// operator picked on purpose rather than a typo that quietly closes the contract.
var contractScopeStatuses = map[string]bool{"active": true, "draft": true, "suspended": true, "revoked": true}

// contractScopeStatusKnown reports whether a contract scope status is one the platform
// recognises. "" is stored as "active".
func contractScopeStatusKnown(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status == "" || contractScopeStatuses[status]
}

func contractScopeActive(scope store.ContractScope, now time.Time) bool {
	if !contractScopeCanServe(scope, now) {
		return false
	}
	if strings.TrimSpace(scope.ValidFrom) != "" {
		validFrom, err := time.Parse(time.RFC3339Nano, scope.ValidFrom)
		if err != nil || validFrom.After(now) {
			return false
		}
	}
	return true
}

func sensitiveProduct(product store.DataProduct) bool {
	value := strings.ToLower(product.Sensitivity + " " + product.SourceType)
	return strings.Contains(value, "personal") ||
		strings.Contains(value, "restricted") ||
		strings.Contains(value, "credit") ||
		strings.Contains(value, "sensitive")
}

func decodeDataProductQueryBody(r *http.Request) (map[string]any, error) {
	body := map[string]any{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return body, nil
		}
		return nil, err
	}
	return body, nil
}

func fieldsFromQueryBody(body map[string]any) []string {
	raw, ok := body["fields"]
	if !ok {
		return nil
	}
	out := []string{}
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				out = append(out, strings.TrimSpace(item))
			}
		}
	case string:
		for _, item := range strings.Split(typed, ",") {
			if strings.TrimSpace(item) != "" {
				out = append(out, strings.TrimSpace(item))
			}
		}
	}
	return out
}

func contractResponseFields(allowed []string, requested []string) ([]string, []string) {
	allowed = normalizeFieldList(allowed)
	requested = normalizeFieldList(requested)
	wildcard := containsFold(allowed, "*")
	if len(requested) == 0 {
		if wildcard {
			return []string{"product_key", "score", "risk_band", "as_of"}, nil
		}
		return allowed, nil
	}
	if wildcard {
		return requested, nil
	}
	forbidden := []string{}
	response := []string{}
	for _, field := range requested {
		contractField, ok := matchFieldFold(allowed, field)
		if !ok {
			forbidden = append(forbidden, field)
			continue
		}
		// Answer with the contract's spelling. Allowed fields are matched case-insensitively,
		// but BuildDynamicOpenAPIDocument declares the response properties from
		// allowed_fields verbatim, so echoing the request casing would return a data key the
		// product's own published schema does not have (and drop a required one).
		response = append(response, contractField)
	}
	if len(forbidden) > 0 {
		return nil, forbidden
	}
	return response, nil
}

// matchFieldFold returns the contract spelling of field when the contract allows it,
// matching case-insensitively like containsFold.
func matchFieldFold(allowed []string, field string) (string, bool) {
	for _, candidate := range allowed {
		if strings.EqualFold(candidate, field) {
			return candidate, true
		}
	}
	return "", false
}

func normalizeFieldList(fields []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || seen[strings.ToLower(field)] {
			continue
		}
		seen[strings.ToLower(field)] = true
		out = append(out, field)
	}
	return out
}

func dataWorksSampleValue(field string, body map[string]any, product store.DataProduct) any {
	lower := strings.ToLower(field)
	switch {
	case lower == "product_key":
		return product.ProductKey
	case lower == "as_of" || strings.Contains(lower, "date") || strings.Contains(lower, "time"):
		return time.Now().UTC().Format(time.RFC3339)
	case strings.Contains(lower, "score") || strings.Contains(lower, "rate") || strings.Contains(lower, "probability"):
		return 82
	case strings.Contains(lower, "risk") || strings.Contains(lower, "band"):
		return "medium"
	case strings.Contains(lower, "amount") || strings.Contains(lower, "revenue") || strings.Contains(lower, "price"):
		return 1250000
	case strings.Contains(lower, "customer"):
		if v, ok := body["customer_key"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return "sample_customer"
	default:
		return "sample_" + strings.ReplaceAll(lower, " ", "_")
	}
}

func (s *Server) auditDataProductQuery(r *http.Request, authCtx *store.AuthContext, apiKeyID string, eventType string, detail string) {
	event := store.AuthEvent{
		ID:        newID("ae"),
		EventType: eventType,
		APIKeyID:  apiKeyID,
		IP:        clientIP(r),
		UserAgent: r.UserAgent(),
		Detail:    detail,
		CreatedAt: time.Now().UTC(),
	}
	if authCtx != nil {
		event.ActorUserID = authCtx.UserID
		event.TeamID = authCtx.TeamID
	}
	_ = s.db.InsertAuditEvent(r.Context(), event)
}

// maskingPolicies lists the contract masking policies applyMasking actually implements.
// Any other value falls through unmasked, so the admin write path refuses to store one and
// the publish gate must not count it as configured masking.
var maskingPolicies = map[string]bool{"redact": true, "hash": true}

// maskingPolicyKnown reports whether a contract masking policy is one the runtime
// understands. "" and "none" are the explicit no-masking values.
func maskingPolicyKnown(policy string) bool {
	policy = strings.ToLower(strings.TrimSpace(policy))
	return policy == "" || policy == "none" || maskingPolicies[policy]
}

// maskingPolicyMasks reports whether a contract masking policy makes applyMasking rewrite
// response values, which is the only sense in which masking is configured for a contract.
func maskingPolicyMasks(policy string) bool {
	return maskingPolicies[strings.ToLower(strings.TrimSpace(policy))]
}

func applyMasking(value any, policy string) any {
	policy = strings.ToLower(strings.TrimSpace(policy))
	if policy == "" || policy == "none" {
		return value
	}
	switch policy {
	case "redact":
		if s, ok := value.(string); ok {
			return strings.Repeat("*", len(s))
		}
		if _, ok := value.(int); ok {
			return 0
		}
		if _, ok := value.(float64); ok {
			return 0.0
		}
		return "****"
	case "hash":
		if s, ok := value.(string); ok {
			return fmt.Sprintf("hash_%d", hashString(s))
		}
		return 9999
	default:
		return value
	}
}

func hashString(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h = (h ^ uint32(s[i])) * 16777619
	}
	return h
}
