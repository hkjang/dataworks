package proxy

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"dataworks/internal/store"
)

// meIdentity is the resolved calling user with the role/scopes that cap what keys they
// may mint for themselves.
type meIdentity struct {
	UserID           string
	TeamID           string
	Role             string
	Scopes           []string
	AllowedIPs       []string
	AllowedModels    []string
	DeniedModels     []string
	AllowedProviders []string
	DeniedProviders  []string
	BudgetLimitKRW   float64
	ExpiresAt        time.Time
}

// meKeyContext resolves the caller's identity for self-service key management, preferring
// a JWT access token, then a proxy API key. Returns false when no user can be identified.
func (s *Server) meKeyContext(r *http.Request) (meIdentity, bool) {
	if claims, ok := s.currentAccessClaims(r); ok && strings.TrimSpace(claims.Subject) != "" {
		return meIdentity{UserID: claims.Subject, TeamID: claims.TeamID, Role: claims.Role, Scopes: claims.Scopes}, true
	}
	if _, authCtx, ok := s.authenticateProxyContext(r); ok && authCtx != nil && strings.TrimSpace(authCtx.UserID) != "" {
		me := meIdentity{
			UserID: authCtx.UserID, TeamID: authCtx.TeamID, Role: authCtx.Role, Scopes: authCtx.Scopes,
			AllowedIPs: authCtx.AllowedIPs, AllowedModels: authCtx.AllowedModels, DeniedModels: authCtx.DeniedModels,
			AllowedProviders: authCtx.AllowedProviders, DeniedProviders: authCtx.DeniedProviders,
			BudgetLimitKRW: authCtx.BudgetLimitKRW,
		}
		// AuthContext intentionally contains only request-enforcement fields. Fetch the
		// authenticating record's expiry as a grant bound so a short-lived key cannot mint a
		// non-expiring or longer-lived child key.
		if authCtx.APIKeyID != "" {
			source, found, err := s.db.GetAPIKey(r.Context(), authCtx.APIKeyID)
			if err != nil || !found {
				return meIdentity{}, false
			}
			me.ExpiresAt = source.ExpiresAt
		}
		return me, true
	}
	return meIdentity{}, false
}

// scopesWithin reports whether every requested scope is allowed (subset), so a user cannot
// grant a self-issued key more than they themselves hold.
func scopesWithin(requested, allowed []string) bool {
	for _, sc := range requested {
		if !hasScope(allowed, sc) {
			return false
		}
	}
	return true
}

// handleMyKeys is self-service API key management for the calling user (opt-in via
// SELF_SERVICE_KEYS_ENABLED). GET lists the caller's own keys; POST issues a new key
// owned by the caller, capped to the caller's scopes.
// GET/POST /me/keys
func (s *Server) handleMyKeys(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Auth.SelfServiceKeys {
		writeOpenAIError(w, http.StatusNotFound, "self-service key management is disabled", "invalid_request_error", "not_found")
		return
	}
	me, ok := s.meKeyContext(r)
	if !ok {
		writeOpenAIError(w, http.StatusUnauthorized, "could not identify caller", "invalid_request_error", "invalid_api_key")
		return
	}
	switch r.Method {
	case http.MethodGet:
		all, err := s.db.ListAPIKeys(r.Context())
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "keys_failed")
			return
		}
		mine := []store.APIKeyPublic{}
		for _, k := range all {
			if k.UserID == me.UserID {
				mine = append(mine, k)
			}
		}
		// Expose the caller's role and the scopes they may grant (their own scopes, capped by
		// role) so the UI can render a role-appropriate scope picker instead of free-form text.
		grantable, _ := normalizeScopes(me.Scopes)
		writeJSON(w, http.StatusOK, map[string]any{"api_keys": mine, "role": me.Role, "grantable_scopes": grantable})
	case http.MethodPost:
		var payload struct {
			Name string `json:"name"`
			apiKeyPolicyPatch
		}
		if err := decodeStrictJSON(r.Body, &payload); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		secret, rec, errResp := s.issueSelfServiceKey(r, me, strings.TrimSpace(payload.Name), payload.apiKeyPolicyPatch)
		if errResp != nil {
			writeOpenAIError(w, errResp.status, errResp.msg, errResp.typ, errResp.code)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"api_key": apiKeyResponse(rec),
			"secret":  secret,
		})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

// handleMyKeyByID handles rotate/revoke of one of the caller's own keys.
// POST /me/keys/{id}/rotate ; DELETE /me/keys/{id}
func (s *Server) handleMyKeyByID(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Auth.SelfServiceKeys {
		writeOpenAIError(w, http.StatusNotFound, "self-service key management is disabled", "invalid_request_error", "not_found")
		return
	}
	me, ok := s.meKeyContext(r)
	if !ok {
		writeOpenAIError(w, http.StatusUnauthorized, "could not identify caller", "invalid_request_error", "invalid_api_key")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/me/keys/")
	id, action := rest, ""
	if idx := strings.Index(rest, "/"); idx >= 0 {
		id, action = rest[:idx], rest[idx+1:]
	}
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "key id required", "invalid_request_error", "invalid_id")
		return
	}
	existing, found, err := s.db.GetAPIKey(r.Context(), id)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "key_lookup_failed")
		return
	}
	// Ownership: only the caller's own keys; respond 404 (not 403) so others' ids aren't probeable.
	if !found || existing.UserID != me.UserID {
		writeOpenAIError(w, http.StatusNotFound, "api key not found", "invalid_request_error", "api_key_not_found")
		return
	}

	switch {
	case action == "rotate" && r.Method == http.MethodPost:
		if existing.Status != "active" {
			writeOpenAIError(w, http.StatusConflict, "only an active api key can be rotated", "invalid_request_error", "key_not_active")
			return
		}
		// Rotation is not a new grant: preserve the complete identity and policy record
		// byte-for-byte (including an intentionally empty scope set), changing only secret,
		// id, lifecycle timestamps and status. Validate against the current caller before the
		// atomic store transaction so a formerly broader sibling key cannot be laundered.
		if errResp := validatePolicyWithinCaller(existing, me); errResp != nil {
			s.auditAuthEvent(r.Context(), "key_policy_denied", me.UserID, id, me.TeamID, "self-service rotate exceeds caller policy")
			writeOpenAIError(w, errResp.status, errResp.msg, errResp.typ, errResp.code)
			return
		}
		secret, err := generateAuthAPIKey(s.cfg.Auth.APIKeyPrefix)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "key_generation_failed")
			return
		}
		rec := existing
		rec.ID = "key_" + hashProxyKey(secret)[:16]
		rec.KeyHash = hashProxyKey(secret)
		rec.Status = "active"
		rec.RevokedAt = time.Time{}
		rec.CreatedAt = time.Now().UTC()
		if err := s.db.RotateAPIKeyOwned(r.Context(), id, me.UserID, rec); err != nil {
			if errors.Is(err, store.ErrInvalidTransition) {
				writeOpenAIError(w, http.StatusConflict, "api key changed while it was being rotated", "invalid_request_error", "key_rotate_conflict")
				return
			}
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "key_rotate_failed")
			return
		}
		_ = s.db.InsertAuditEvent(r.Context(), store.AuthEvent{ID: newID("ae"), EventType: "api_key_rotated", APIKeyID: id, ActorUserID: me.UserID, IP: clientIP(r), UserAgent: r.UserAgent(), Detail: "self-service → " + rec.ID, CreatedAt: time.Now().UTC()})
		writeJSON(w, http.StatusOK, map[string]any{
			"rotated_from": id,
			"api_key":      apiKeyResponse(rec),
			"secret":       secret,
		})
	case action == "" && r.Method == http.MethodPatch:
		var payload apiKeyPolicyPatch
		if err := decodeStrictJSON(r.Body, &payload); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if !payload.any() {
			writeOpenAIError(w, http.StatusBadRequest, "at least one key policy field is required", "invalid_request_error", "empty_update")
			return
		}
		if existing.Status != "active" {
			writeOpenAIError(w, http.StatusConflict, "only an active api key can be updated", "invalid_request_error", "key_not_active")
			return
		}
		updated := existing
		if policyErr := applyAPIKeyPolicyPatch(&updated, payload, time.Now().UTC()); policyErr != nil {
			writeOpenAIError(w, http.StatusBadRequest, policyErr.msg, "invalid_request_error", "invalid_"+policyErr.field)
			return
		}
		if errResp := validatePolicyWithinCaller(updated, me); errResp != nil {
			s.auditAuthEvent(r.Context(), "key_policy_denied", me.UserID, id, me.TeamID, "self-service key policy edit exceeds caller")
			writeOpenAIError(w, errResp.status, errResp.msg, errResp.typ, errResp.code)
			return
		}
		changed, err := s.db.UpdateAPIKeyPolicyOwned(r.Context(), updated, me.UserID)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "key_update_failed")
			return
		}
		if !changed {
			writeOpenAIError(w, http.StatusConflict, "api key changed while it was being updated", "invalid_request_error", "key_update_conflict")
			return
		}
		_ = s.db.InsertAuditEvent(r.Context(), store.AuthEvent{ID: newID("ae"), EventType: "api_key_policy_updated", APIKeyID: id, ActorUserID: me.UserID, TeamID: me.TeamID, IP: clientIP(r), UserAgent: r.UserAgent(), Detail: "self-service policy edit", CreatedAt: time.Now().UTC()})
		writeJSON(w, http.StatusOK, apiKeyResponse(updated))
	case action == "" && r.Method == http.MethodDelete:
		changed, err := s.db.RevokeAPIKeyOwned(r.Context(), id, me.UserID)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "key_revoke_failed")
			return
		}
		if !changed {
			writeOpenAIError(w, http.StatusConflict, "api key is not active", "invalid_request_error", "key_not_active")
			return
		}
		_ = s.db.InsertAuditEvent(r.Context(), store.AuthEvent{ID: newID("ae"), EventType: "api_key_revoked", APIKeyID: id, ActorUserID: me.UserID, IP: clientIP(r), UserAgent: r.UserAgent(), Detail: "self-service", CreatedAt: time.Now().UTC()})
		writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "revoked"})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

// handleKeyHealth returns active keys needing attention (expiring/expired/never-used/
// idle) across all users, for admins. Read-only.
// GET /admin/keys/health?stale_days=30&expiring_days=7
func (s *Server) handleKeyHealth(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	staleDays := intQuery(r, "stale_days", 30)
	expiringDays := intQuery(r, "expiring_days", 7)
	alerts, err := s.db.KeyHealthAlerts(r.Context(), time.Now().UTC(), staleDays, expiringDays, "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "key_health_failed")
		return
	}
	high := 0
	for _, a := range alerts {
		if a.Severity == "high" {
			high++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"stale_days": staleDays, "expiring_days": expiringDays,
		"high_severity": high, "alerts": alerts,
	})
}

type meKeyError struct {
	status int
	msg    string
	typ    string
	code   string
}

// issueSelfServiceKey creates a new active key owned by the caller. Omitted policy fields
// inherit all caller constraints; explicit values may only narrow them. Returns the plaintext
// secret exactly once.
func (s *Server) issueSelfServiceKey(r *http.Request, me meIdentity, name string, patch apiKeyPolicyPatch) (string, store.APIKeyRecord, *meKeyError) {
	if name == "" {
		return "", store.APIKeyRecord{}, &meKeyError{http.StatusBadRequest, "name is required", "invalid_request_error", "missing_name"}
	}
	plainKey, err := generateAuthAPIKey(s.cfg.Auth.APIKeyPrefix)
	if err != nil {
		return "", store.APIKeyRecord{}, &meKeyError{http.StatusInternalServerError, err.Error(), "server_error", "key_generation_failed"}
	}
	rec := store.APIKeyRecord{
		ID:        "key_" + hashProxyKey(plainKey)[:16],
		Name:      name,
		KeyHash:   hashProxyKey(plainKey),
		Team:      me.TeamID,
		UserID:    me.UserID,
		Role:      me.Role,
		Status:    "active",
		CreatedAt: time.Now().UTC(),
	}
	copyKeyPolicyFromIdentity(&rec, me)
	if policyErr := applyAPIKeyPolicyPatch(&rec, patch, time.Now().UTC()); policyErr != nil {
		return "", store.APIKeyRecord{}, &meKeyError{http.StatusBadRequest, policyErr.msg, "invalid_request_error", "invalid_" + policyErr.field}
	}
	if errResp := validatePolicyWithinCaller(rec, me); errResp != nil {
		s.auditAuthEvent(r.Context(), "key_policy_denied", me.UserID, "", me.TeamID, "self-service key policy exceeds caller")
		return "", store.APIKeyRecord{}, errResp
	}
	if err := s.db.UpsertAPIKey(r.Context(), rec); err != nil {
		return "", store.APIKeyRecord{}, &meKeyError{http.StatusInternalServerError, err.Error(), "server_error", "api_key_create_failed"}
	}
	_ = s.db.InsertAuditEvent(r.Context(), store.AuthEvent{ID: newID("ae"), EventType: "api_key_created", APIKeyID: rec.ID, ActorUserID: me.UserID, TeamID: me.TeamID, IP: clientIP(r), UserAgent: r.UserAgent(), Detail: "self-service: " + name, CreatedAt: time.Now().UTC()})
	return plainKey, rec, nil
}
