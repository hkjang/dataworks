package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/netip"
	"strings"
	"time"
	"unicode"

	"dataworks/internal/store"
)

func decodeStrictJSON(r io.Reader, dst any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

// apiKeyPolicyPatch uses pointers so PATCH and self-service issuance can distinguish an
// omitted field (preserve/inherit) from an explicitly empty list (remove that constraint).
// Empty scopes intentionally mean no permissions; they must never be treated as "use defaults".
type apiKeyPolicyPatch struct {
	Scopes           *[]string `json:"scopes"`
	AllowedIPs       *[]string `json:"allowed_ips"`
	AllowedModels    *[]string `json:"allowed_models"`
	DeniedModels     *[]string `json:"denied_models"`
	AllowedProviders *[]string `json:"allowed_providers"`
	DeniedProviders  *[]string `json:"denied_providers"`
	BudgetLimitKRW   *float64  `json:"budget_limit_krw"`
	ExpiresAt        *string   `json:"expires_at"`
}

func (p apiKeyPolicyPatch) any() bool {
	return p.Scopes != nil || p.AllowedIPs != nil || p.AllowedModels != nil || p.DeniedModels != nil ||
		p.AllowedProviders != nil || p.DeniedProviders != nil || p.BudgetLimitKRW != nil || p.ExpiresAt != nil
}

type apiKeyPolicyError struct {
	field string
	msg   string
}

func (e *apiKeyPolicyError) Error() string { return e.msg }

func invalidPolicy(field, msg string) *apiKeyPolicyError {
	return &apiKeyPolicyError{field: field, msg: msg}
}

func applyAPIKeyPolicyPatch(rec *store.APIKeyRecord, patch apiKeyPolicyPatch, now time.Time) *apiKeyPolicyError {
	if patch.Scopes != nil {
		normalized, ok := normalizeScopes(*patch.Scopes)
		if !ok {
			return invalidPolicy("scopes", "invalid scope")
		}
		rec.Scopes = normalized
	}
	if patch.AllowedIPs != nil {
		values, err := normalizeIPConstraints(*patch.AllowedIPs)
		if err != nil {
			return invalidPolicy("allowed_ips", err.Error())
		}
		rec.AllowedIPs = values
	}
	if patch.AllowedModels != nil {
		values, err := normalizePatternConstraints(*patch.AllowedModels, "allowed_models")
		if err != nil {
			return invalidPolicy("allowed_models", err.Error())
		}
		rec.AllowedModels = values
	}
	if patch.DeniedModels != nil {
		values, err := normalizePatternConstraints(*patch.DeniedModels, "denied_models")
		if err != nil {
			return invalidPolicy("denied_models", err.Error())
		}
		rec.DeniedModels = values
	}
	if patch.AllowedProviders != nil {
		values, err := normalizePatternConstraints(*patch.AllowedProviders, "allowed_providers")
		if err != nil {
			return invalidPolicy("allowed_providers", err.Error())
		}
		rec.AllowedProviders = values
	}
	if patch.DeniedProviders != nil {
		values, err := normalizePatternConstraints(*patch.DeniedProviders, "denied_providers")
		if err != nil {
			return invalidPolicy("denied_providers", err.Error())
		}
		rec.DeniedProviders = values
	}
	if patch.BudgetLimitKRW != nil {
		if math.IsNaN(*patch.BudgetLimitKRW) || math.IsInf(*patch.BudgetLimitKRW, 0) || *patch.BudgetLimitKRW < 0 {
			return invalidPolicy("budget_limit_krw", "budget_limit_krw must be a finite non-negative number")
		}
		rec.BudgetLimitKRW = *patch.BudgetLimitKRW
	}
	if patch.ExpiresAt != nil {
		raw := strings.TrimSpace(*patch.ExpiresAt)
		if raw == "" {
			rec.ExpiresAt = time.Time{}
		} else {
			parsed, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				return invalidPolicy("expires_at", "expires_at must be an RFC3339 timestamp or an empty string")
			}
			if !now.IsZero() && !parsed.After(now) {
				return invalidPolicy("expires_at", "expires_at must be in the future")
			}
			rec.ExpiresAt = parsed.UTC()
		}
	}
	return nil
}

func normalizeIPConstraints(values []string) ([]string, error) {
	if len(values) > 128 {
		return nil, fmt.Errorf("allowed_ips supports at most 128 entries")
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			return nil, fmt.Errorf("allowed_ips cannot contain an empty entry")
		}
		var normalized string
		if strings.Contains(value, "/") {
			prefix, err := netip.ParsePrefix(value)
			if err != nil {
				return nil, fmt.Errorf("invalid allowed_ips entry %q", value)
			}
			normalized = prefix.Masked().String()
		} else {
			addr, err := netip.ParseAddr(value)
			if err != nil {
				return nil, fmt.Errorf("invalid allowed_ips entry %q", value)
			}
			normalized = addr.String()
		}
		if !seen[normalized] {
			seen[normalized] = true
			out = append(out, normalized)
		}
	}
	return out, nil
}

func normalizePatternConstraints(values []string, field string) ([]string, error) {
	if len(values) > 128 {
		return nil, fmt.Errorf("%s supports at most 128 entries", field)
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			return nil, fmt.Errorf("%s cannot contain an empty entry", field)
		}
		if len(value) > 256 || strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
			return nil, fmt.Errorf("invalid %s entry %q", field, value)
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out, nil
}

func copyKeyPolicyFromIdentity(rec *store.APIKeyRecord, me meIdentity) {
	rec.Scopes = append([]string(nil), me.Scopes...)
	rec.AllowedIPs = append([]string(nil), me.AllowedIPs...)
	rec.AllowedModels = append([]string(nil), me.AllowedModels...)
	rec.DeniedModels = append([]string(nil), me.DeniedModels...)
	rec.AllowedProviders = append([]string(nil), me.AllowedProviders...)
	rec.DeniedProviders = append([]string(nil), me.DeniedProviders...)
	rec.BudgetLimitKRW = me.BudgetLimitKRW
	rec.ExpiresAt = me.ExpiresAt
}

// validatePolicyWithinCaller rejects every form of privilege laundering available to an API
// key caller: empty allow-lists cannot remove a parent allow-list, parent deny rules cannot be
// dropped, a finite parent budget/expiry cannot be removed or enlarged, and scopes stay a subset.
func validatePolicyWithinCaller(rec store.APIKeyRecord, me meIdentity) *meKeyError {
	targetRole := strings.TrimSpace(rec.Role)
	if targetRole == "" {
		if rec.ServiceAccountID != "" {
			targetRole = "service_account"
		} else {
			targetRole = "developer"
		}
	}
	callerRole := strings.TrimSpace(me.Role)
	if callerRole == "" {
		callerRole = "developer"
	}
	targetRank, callerRank := roleRank(targetRole), roleRank(callerRole)
	if targetRank == 0 || callerRank == 0 || targetRank > callerRank || (targetRank == callerRank && targetRole != callerRole) {
		return &meKeyError{status: 403, msg: "cannot manage a key with a role beyond your own", typ: "permission_error", code: "role_escalation_denied"}
	}
	if strings.TrimSpace(rec.Team) != strings.TrimSpace(me.TeamID) {
		return &meKeyError{status: 403, msg: "cannot manage a key outside your current team", typ: "permission_error", code: "team_scope_denied"}
	}
	if !scopesWithin(rec.Scopes, me.Scopes) {
		return &meKeyError{status: 403, msg: "cannot grant scopes beyond your own", typ: "permission_error", code: "scope_denied"}
	}
	if !ipAllowListWithin(rec.AllowedIPs, me.AllowedIPs) {
		return &meKeyError{status: 403, msg: "allowed_ips would exceed your own key policy", typ: "permission_error", code: "key_policy_denied"}
	}
	if !patternAllowListWithin(rec.AllowedModels, me.AllowedModels) {
		return &meKeyError{status: 403, msg: "allowed_models would exceed your own key policy", typ: "permission_error", code: "key_policy_denied"}
	}
	if !patternAllowListWithin(rec.AllowedProviders, me.AllowedProviders) {
		return &meKeyError{status: 403, msg: "allowed_providers would exceed your own key policy", typ: "permission_error", code: "key_policy_denied"}
	}
	if !denyListPreserved(rec.DeniedModels, me.DeniedModels) {
		return &meKeyError{status: 403, msg: "denied_models must preserve your own deny policy", typ: "permission_error", code: "key_policy_denied"}
	}
	if !denyListPreserved(rec.DeniedProviders, me.DeniedProviders) {
		return &meKeyError{status: 403, msg: "denied_providers must preserve your own deny policy", typ: "permission_error", code: "key_policy_denied"}
	}
	if me.BudgetLimitKRW > 0 && (rec.BudgetLimitKRW <= 0 || rec.BudgetLimitKRW > me.BudgetLimitKRW) {
		return &meKeyError{status: 403, msg: "budget_limit_krw cannot exceed or remove your own budget", typ: "permission_error", code: "key_policy_denied"}
	}
	if !me.ExpiresAt.IsZero() && (rec.ExpiresAt.IsZero() || rec.ExpiresAt.After(me.ExpiresAt)) {
		return &meKeyError{status: 403, msg: "expires_at cannot outlive or remove your own key expiry", typ: "permission_error", code: "key_policy_denied"}
	}
	return nil
}

func ipAllowListWithin(child, parent []string) bool {
	if len(parent) == 0 {
		return true
	}
	if len(child) == 0 {
		return false
	}
	parents := make([]netip.Prefix, 0, len(parent))
	for _, raw := range parent {
		p, ok := constraintPrefix(raw)
		if !ok {
			return false
		}
		parents = append(parents, p)
	}
	for _, raw := range child {
		candidate, ok := constraintPrefix(raw)
		if !ok {
			return false
		}
		covered := false
		for _, allowed := range parents {
			if allowed.Addr().BitLen() == candidate.Addr().BitLen() && allowed.Bits() <= candidate.Bits() && allowed.Contains(candidate.Addr()) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func constraintPrefix(raw string) (netip.Prefix, bool) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "/") {
		p, err := netip.ParsePrefix(raw)
		return p.Masked(), err == nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(addr, addr.BitLen()), true
}

func patternAllowListWithin(child, parent []string) bool {
	if len(parent) == 0 {
		return true
	}
	if len(child) == 0 {
		return false
	}
	for _, requested := range child {
		requested = strings.ToLower(strings.TrimSpace(requested))
		allowed := false
		for _, bound := range parent {
			bound = strings.ToLower(strings.TrimSpace(bound))
			// A literal child matching a parent glob is a provable narrowing.  A child glob
			// is accepted only under '*' or when exactly equal; arbitrary glob containment
			// is intentionally not guessed.
			if bound == "*" || bound == requested || (!strings.Contains(requested, "*") && matchGlob(bound, requested)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return true
}

func denyListPreserved(child, parent []string) bool {
	for _, required := range parent {
		required = strings.ToLower(strings.TrimSpace(required))
		covered := false
		for _, denied := range child {
			denied = strings.ToLower(strings.TrimSpace(denied))
			if denied == "*" || denied == required || (!strings.Contains(required, "*") && matchGlob(denied, required)) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func apiKeyResponse(rec store.APIKeyRecord) map[string]any {
	createdAt := ""
	if !rec.CreatedAt.IsZero() {
		createdAt = rec.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	expiresAt := ""
	if !rec.ExpiresAt.IsZero() {
		expiresAt = rec.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	revokedAt := ""
	if !rec.RevokedAt.IsZero() {
		revokedAt = rec.RevokedAt.UTC().Format(time.RFC3339Nano)
	}
	return map[string]any{
		"id": rec.ID, "name": rec.Name, "owner": rec.Owner, "team": rec.Team,
		"user_id": rec.UserID, "service_account_id": rec.ServiceAccountID, "role": rec.Role,
		"status": rec.Status, "scopes": rec.Scopes, "allowed_ips": rec.AllowedIPs,
		"allowed_models": rec.AllowedModels, "denied_models": rec.DeniedModels,
		"allowed_providers": rec.AllowedProviders, "denied_providers": rec.DeniedProviders,
		"budget_limit_krw": rec.BudgetLimitKRW, "expires_at": expiresAt, "revoked_at": revokedAt, "created_at": createdAt,
	}
}
