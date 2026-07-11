package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"dataworks/internal/store"
)

func (s *Server) handleV1DataProductQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	productKey, ok := dataProductQueryKey(r.URL.Path)
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

	ent, found, err := s.db.FindAPIEntitlement(r.Context(), product.ProductKey, apiKeyID, apiKeyHash)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "entitlement_lookup_failed")
		return
	}
	if !found {
		s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "missing_entitlement:"+product.ProductKey)
		writeOpenAIError(w, http.StatusForbidden, "data product entitlement is required", "invalid_request_error", "missing_entitlement")
		return
	}
	now := time.Now().UTC()
	if !entitlementActive(ent, now) {
		s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "inactive_entitlement:"+ent.ID)
		writeOpenAIError(w, http.StatusForbidden, "data product entitlement is inactive or expired", "invalid_request_error", "inactive_entitlement")
		return
	}
	if ent.Scope != "" && ent.Scope != "*" && !strings.Contains(strings.ToLower(ent.Scope), "query") {
		s.auditDataProductQuery(r, authCtx, apiKeyID, "data_product_query_denied", "scope_not_allowed:"+ent.Scope)
		writeOpenAIError(w, http.StatusForbidden, "entitlement scope does not allow query", "invalid_request_error", "scope_denied")
		return
	}

	var contractKey string
	var customerKey string
	var errCode = http.StatusOK
	defer func() {
		if contractKey != "" {
			failed := errCode != http.StatusOK
			billingAmount := 0.0
			if !failed {
				billingAmount = 10.0
			}
			_ = s.db.IncrementUsageMetering(r.Context(), customerKey, productKey, contractKey, failed, billingAmount)
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
	rest := strings.Trim(strings.TrimPrefix(path, "/v1/data-products/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || parts[1] != "query" {
		return "", false
	}
	return strings.TrimSpace(parts[0]), true
}

func entitlementActive(ent store.APIEntitlement, now time.Time) bool {
	if strings.ToLower(strings.TrimSpace(ent.Status)) != "active" {
		return false
	}
	if strings.TrimSpace(ent.ExpiresAt) == "" {
		return true
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, ent.ExpiresAt)
	return err == nil && expiresAt.After(now)
}

func contractScopeActive(scope store.ContractScope, now time.Time) bool {
	if strings.ToLower(strings.TrimSpace(scope.Status)) != "active" {
		return false
	}
	if strings.TrimSpace(scope.ValidFrom) != "" {
		validFrom, err := time.Parse(time.RFC3339Nano, scope.ValidFrom)
		if err != nil || validFrom.After(now) {
			return false
		}
	}
	if strings.TrimSpace(scope.ValidTo) != "" {
		validTo, err := time.Parse(time.RFC3339Nano, scope.ValidTo)
		if err != nil || validTo.Before(now) {
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
	for _, field := range requested {
		if !containsFold(allowed, field) {
			forbidden = append(forbidden, field)
		}
	}
	if len(forbidden) > 0 {
		return nil, forbidden
	}
	return requested, nil
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
