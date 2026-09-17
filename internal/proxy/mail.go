package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"dataworks/internal/mail"
	"dataworks/internal/store"
)

// Mail notifications: the events people in this app actually wait for —
// a request parked for approval, a change set waiting for review, an alert
// rule that fired, and access that is about to lapse. Everything goes out in
// the background through internal/mail; a dead relay never fails a request.

// mailConfigFrom builds the mail snapshot from stored settings, decrypting
// the password the same way every other secret setting is read.
func (s *Server) mailConfigFrom(stored map[string]store.AdminSetting) mail.Config {
	values := map[string]string{}
	for _, d := range settingRegistry {
		if !strings.HasPrefix(d.Key, mail.SettingPrefix) {
			continue
		}
		value, _ := s.effectiveSettingValue(stored, d)
		values[d.Key] = value
	}
	return mail.ReadConfig(values)
}

// mailConf returns the current snapshot; a zero config (disabled) before the
// first reload.
func (s *Server) mailConf() mail.Config {
	if config := s.mailRuntime.Load(); config != nil {
		return *config
	}
	return mail.Config{}
}

// mailActor is the identity a request acts as: the signed-in user's id when a
// session is present (so the person is excluded from mail about their own
// action), otherwise the token-derived admin id used by the audit log.
func (s *Server) mailActor(r *http.Request) string {
	if claims, ok := s.currentAccessClaims(r); ok && strings.TrimSpace(claims.Subject) != "" {
		return strings.TrimSpace(claims.Subject)
	}
	return adminID(r)
}

// mailAdmins is the audience for anything that needs a decision.
func (s *Server) mailAdmins(ctx context.Context) []string {
	ids, err := s.db.AdminUserIDs(ctx)
	if err != nil {
		slog.Warn("mail: admin recipients were not listed", "error", err)
		return nil
	}
	return ids
}

// notifyMail hands one event to the mail service. It returns before anything
// is sent and is safe to call when mail is off.
func (s *Server) notifyMail(ctx context.Context, notification mail.Notification, actorID string, recipients []string) {
	if s.mailer == nil || len(recipients) == 0 {
		return
	}
	s.mailer.Notify(context.WithoutCancel(ctx), notification, actorID, recipients)
}

// ---- events -------------------------------------------------------------------

// Where each mail sends people. The admin UI routes on "#/<tab>" (parseHash in
// admin_ui.go): the approval queue and alert rules live on the safety tab,
// change sets on their own tab.
const (
	mailPathApprovals  = "/admin#/safety"
	mailPathAlerts     = "/admin#/safety"
	mailPathChangeSets = "/admin#/changesets"
)

// approvalMailWindow is how long one requester's approval mail stands in for
// the ones that follow. The gate parks every request that arrives without
// X-Governance-Approval-ID as a new approval, so a client retry loop or an
// agent calling in a loop would otherwise mail every administrator once per
// attempt; the queue the mail links to lists all of them anyway.
const approvalMailWindow = 10 * time.Minute

// mailThrottle remembers when a key last produced mail. It is per pod, which
// bounds a burst to one mail per pod per window rather than one per request.
type mailThrottle struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

// allow reports whether key may send now and, if so, starts its window.
func (t *mailThrottle) allow(key string, now time.Time, window time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.seen == nil {
		t.seen = map[string]time.Time{}
	}
	for k, at := range t.seen {
		if now.Sub(at) >= window {
			delete(t.seen, k)
		}
	}
	if at, ok := t.seen[key]; ok && now.Sub(at) < window {
		return false
	}
	t.seen[key] = now
	return true
}

// approvalRequester is the identity an approval mail is throttled on: the
// account when there is one, else the API key.
func approvalRequester(approval store.Approval) string {
	return firstNonEmpty(strings.TrimSpace(approval.UserID), strings.TrimSpace(approval.APIKeyID), "anonymous")
}

// notifyApprovalRequested tells the administrators a request is parked. One
// mail per requester per approvalMailWindow: the rest of that requester's
// approvals still land in the queue, just without another mail each.
func (s *Server) notifyApprovalRequested(ctx context.Context, approval store.Approval) {
	if s.mailer == nil || !s.mailConf().Enabled {
		return
	}
	if !s.mailApprovals.allow(approvalRequester(approval), time.Now().UTC(), approvalMailWindow) {
		return
	}
	lines := []string{
		"승인이 필요한 요청이 대기 중입니다.",
		"사유: " + firstNonEmpty(strings.TrimSpace(approval.Reason), "-"),
		"요청자: " + firstNonEmpty(strings.TrimSpace(approval.UserID), strings.TrimSpace(approval.APIKeyID), "-"),
		fmt.Sprintf("위험 점수: %d · 예상 비용: %.0f KRW", approval.RiskScore, approval.CostKRW),
		"만료: " + approval.ExpiresAt.UTC().Format(time.RFC3339) + " (그때까지 결정하지 않으면 요청은 만료됩니다)",
		fmt.Sprintf("같은 요청자의 추가 승인 요청은 %d분 동안 별도 메일 없이 대기열에만 쌓입니다.", int(approvalMailWindow/time.Minute)),
	}
	s.notifyMail(ctx, mail.Notification{Event: mail.EventApprovalRequested, Subject: "[Data Works] 승인 요청: " + shortReason(approval.Reason),
		Lines: lines, Path: mailPathApprovals, Ref: approval.ID}, approval.UserID, s.mailAdmins(ctx))
}

// notifyApprovalDecided tells the requester the outcome so they stop polling.
func (s *Server) notifyApprovalDecided(ctx context.Context, approval store.Approval, actorID string) {
	if s.mailer == nil || !s.mailConf().Enabled || strings.TrimSpace(approval.UserID) == "" {
		return
	}
	verdict := "승인되었습니다"
	if approval.Status != "approved" {
		verdict = "거절되었습니다"
	}
	lines := []string{
		"요청하신 작업이 " + verdict + ".",
		"사유: " + firstNonEmpty(strings.TrimSpace(approval.Reason), "-"),
		"승인 id: " + approval.ID,
	}
	if approval.Status == "approved" {
		// The gate only honours the approval when the id travels in the header;
		// a bare resend is parked again as a new approval.
		lines = append(lines,
			"같은 요청을 X-Governance-Approval-ID: "+approval.ID+" 헤더와 함께 다시 보내면 이 승인으로 처리됩니다.",
			"헤더 없이 다시 보내면 새 승인 요청으로 다시 대기하게 됩니다.")
	}
	s.notifyMail(ctx, mail.Notification{Event: mail.EventApprovalDecided, Subject: "[Data Works] 요청이 " + verdict,
		Lines: lines, Path: mailPathApprovals, Ref: approval.ID}, actorID, []string{approval.UserID})
}

// notifyChangeSetSubmitted asks the other administrators for a review.
func (s *Server) notifyChangeSetSubmitted(ctx context.Context, cs store.ChangeSet, actorID string) {
	if s.mailer == nil || !s.mailConf().Enabled {
		return
	}
	lines := []string{
		"설정 변경 세트가 검토를 기다립니다.",
		"제목: " + firstNonEmpty(strings.TrimSpace(cs.Title), cs.ID),
		fmt.Sprintf("항목 %d개", len(cs.Items)),
	}
	if note := strings.TrimSpace(cs.Description); note != "" {
		lines = append(lines, "설명: "+note)
	}
	s.notifyMail(ctx, mail.Notification{Event: mail.EventChangeSetSubmit, Subject: "[Data Works] 변경 세트 검토 요청: " + firstNonEmpty(strings.TrimSpace(cs.Title), cs.ID),
		Lines: lines, Path: mailPathChangeSets, Ref: cs.ID}, actorID, s.mailAdmins(ctx))
}

// notifyAlertFired is the AlertWorker hook: one mail per firing, which the
// rule's own window already debounces.
func (s *Server) notifyAlertFired(ctx context.Context, rule store.AlertRule, value float64) {
	if s.mailer == nil || !s.mailConf().Enabled {
		return
	}
	lines := []string{
		fmt.Sprintf("알림 규칙 %q 이 임계치에 도달했습니다.", rule.Name),
		fmt.Sprintf("%s = %s (임계치 %s, 윈도우 %ds)", rule.Metric, formatMetricValue(rule.Metric, value), formatMetricValue(rule.Metric, rule.Threshold), rule.WindowSeconds),
		"대상: " + rule.Scope + "/" + firstNonEmpty(rule.ScopeValue, "*"),
	}
	if note := strings.TrimSpace(rule.Note); note != "" {
		lines = append(lines, "메모: "+note)
	}
	s.notifyMail(ctx, mail.Notification{Event: mail.EventAlertFired, Subject: "[Data Works] 알림: " + rule.Name,
		Lines: lines, Path: mailPathAlerts, Ref: rule.ID}, "", s.mailAdmins(ctx))
}

// mailDigestHour is the UTC hour after which the day's expiry digest goes out
// (09:00 KST), once per day.
const mailDigestHour = 0

// runMailDigest is the AlertWorker tick hook: once per UTC day, bundle every
// contract and entitlement that lapses within the default horizon into one
// mail per administrator. The ledger, not memory, decides whether today's
// digest already went out, so restarts and extra pods do not repeat it.
func (s *Server) runMailDigest(ctx context.Context) {
	if s.mailer == nil {
		return
	}
	config := s.mailConf()
	if !config.Enabled || !config.Allows(mail.EventAccessExpiring) {
		return
	}
	now := time.Now().UTC()
	if now.Hour() < mailDigestHour {
		return
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if last := s.mailDigestDay.Load(); last != nil && *last == dayStart.Format("2006-01-02") {
		return
	}
	sent, err := s.db.CountMailDeliveriesSince(ctx, mail.EventAccessExpiring, dayStart)
	if err != nil {
		slog.Warn("mail: digest state was not read", "error", err)
		return
	}
	day := dayStart.Format("2006-01-02")
	s.mailDigestDay.Store(&day)
	if sent > 0 {
		return
	}
	notification, ok := s.expiringAccessDigest(ctx, now, defaultExpiryHorizon)
	if !ok {
		return
	}
	s.notifyMail(ctx, notification, "", s.mailAdmins(ctx))
}

// expiringAccessDigest builds the bundled summary. It applies the same active
// and lookahead rules as the action center so mail and screen agree.
func (s *Server) expiringAccessDigest(ctx context.Context, now time.Time, horizon time.Duration) (mail.Notification, bool) {
	deadline := now.Add(horizon)
	scopes, _ := s.db.ListContractScopes(ctx, "", "")
	entitlements, _ := s.db.ListAPIEntitlements(ctx, "", "")
	lines := []string{}
	contracts := 0
	for _, scope := range scopes {
		if !contractScopeStatusActive(scope.Status) || strings.TrimSpace(scope.ValidTo) == "" {
			continue
		}
		expiresAt, err := time.Parse(time.RFC3339Nano, scope.ValidTo)
		if err == nil && !expiresAt.Before(deadline) {
			continue
		}
		contracts++
		state := "만료 예정"
		if err != nil || expiresAt.Before(now) {
			state = "이미 만료"
		}
		lines = append(lines, fmt.Sprintf("- 계약 %s (상품 %s, 고객 %s): %s %s", scope.ContractKey, scope.ProductKey, scope.CustomerKey, scope.ValidTo, state))
	}
	access := 0
	for _, ent := range entitlements {
		if !entitlementActive(ent, now) || !entitlementExpiresWithin(ent.ExpiresAt, deadline) {
			continue
		}
		access++
		lines = append(lines, fmt.Sprintf("- 접근권 %s (상품 %s, API 키 %s): %s 만료 예정", ent.ID, ent.ProductKey, ent.APIKeyID, ent.ExpiresAt))
	}
	if contracts+access == 0 {
		return mail.Notification{}, false
	}
	head := []string{fmt.Sprintf("%d일 안에 만료되는 계약 %d건, 접근권 %d건이 있습니다. 갱신하지 않으면 고객 API 키가 상품 조회를 잃습니다.", int(horizon.Hours()/24), contracts, access), ""}
	return mail.Notification{
		Event:   mail.EventAccessExpiring,
		Subject: fmt.Sprintf("[Data Works] 만료 임박: 계약 %d건 · 접근권 %d건", contracts, access),
		Lines:   append(head, lines...),
		Path:    "/dataworks/operations",
		Ref:     now.Format("2006-01-02"),
	}, true
}

func shortReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "결정이 필요합니다"
	}
	if runes := []rune(reason); len(runes) > 60 {
		return string(runes[:60]) + "…"
	}
	return reason
}

// ---- admin API ----------------------------------------------------------------

// handleMailStatus reports the effective configuration without the password.
// GET /admin/mail/status
func (s *Server) handleMailStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuthorization(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	config := s.mailConf()
	events := map[string]bool{}
	for event := range mail.EventSettings {
		events[event] = config.Allows(event)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": config.Enabled, "ready": config.Enabled && config.Validate() == nil, "problem": config.Problem(),
		"smtp_host": config.Host, "smtp_port": config.Port, "security": config.Security, "skip_tls_verify": config.SkipVerify,
		"auth": strings.TrimSpace(config.Username) != "", "password_set": config.Password != "",
		"from": config.Address(), "base_url": config.BaseURL, "timeout_seconds": int(config.Timeout / time.Second),
		"events": events,
	})
}

// handleMailDeliveries lists what went out (and what did not).
// GET /admin/mail/deliveries?status=&event=&limit=
func (s *Server) handleMailDeliveries(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuthorization(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := s.db.ListMailDeliveries(r.Context(), r.URL.Query().Get("status"), r.URL.Query().Get("event"), limit)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "mail_deliveries_failed")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleMailTest sends one real message with the saved settings and reports
// the outcome in place. POST /admin/mail/test {"recipient": "..."}
func (s *Server) handleMailTest(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuthorization(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var input struct {
		Recipient string `json:"recipient"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	recipient := strings.TrimSpace(input.Recipient)
	if recipient == "" {
		if claims, ok := s.currentAccessClaims(r); ok {
			recipient = strings.TrimSpace(claims.Email)
		}
	}
	if !strings.Contains(recipient, "@") {
		writeOpenAIError(w, http.StatusBadRequest, "recipient must be an email address", "invalid_request_error", "invalid_recipient")
		return
	}
	if s.mailer == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "mail service is not configured", "server_error", "mail_unavailable")
		return
	}
	err := s.mailer.SendNow(r.Context(), mail.TestMessage(), s.mailActor(r), recipient)
	s.auditAdmin(r, "mail.test", "", auditJSON(map[string]any{"recipient": recipient, "sent": err == nil}))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"sent": true, "recipient": recipient})
	case err == mail.ErrDisabled:
		writeOpenAIError(w, http.StatusConflict, "mail.enabled is off; save the relay settings and turn it on first", "invalid_request_error", "mail_disabled")
	default:
		writeOpenAIError(w, http.StatusBadGateway, err.Error(), "server_error", "mail_send_failed")
	}
}
