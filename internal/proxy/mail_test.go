package proxy

import (
	"context"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"dataworks/internal/mail"
	"dataworks/internal/store"
)

// closedPort is an address nothing listens on, standing in for a dead relay.
func closedPort(t *testing.T) (string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	_ = listener.Close()
	return addr.IP.String(), addr.Port
}

type capturedMail struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (c *capturedMail) send(_ context.Context, _ mail.Config, m mail.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, m)
	return nil
}

func (c *capturedMail) recipients() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []string{}
	for _, m := range c.sent {
		out = append(out, m.To)
	}
	sort.Strings(out)
	return out
}

func mailDeliveries(t *testing.T, base, query string) (map[string]any, []map[string]any) {
	t.Helper()
	resp, body := req(t, http.MethodGet, base+"/admin/mail/deliveries"+query, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET deliveries = %d %v", resp.StatusCode, body)
	}
	items := []map[string]any{}
	for _, raw := range body["items"].([]any) {
		items = append(items, raw.(map[string]any))
	}
	return body, items
}

func seedMailUsers(t *testing.T, db *store.SQLStore) {
	t.Helper()
	for _, user := range []store.AuthUser{
		{ID: "u_admin1", Email: "admin1@example.test", Role: "admin", PasswordHash: "x"},
		{ID: "u_admin2", Email: "admin2@example.test", Role: "super_admin", PasswordHash: "x"},
		{ID: "u_gone", Email: "gone@example.test", Role: "admin", Status: "disabled", PasswordHash: "x"},
		{ID: "u_dev", Email: "dev@example.test", Role: "developer", PasswordHash: "x"},
	} {
		if err := db.CreateAuthUser(context.Background(), user); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMailIsOffByDefaultAndNeverReturnsThePassword(t *testing.T) {
	server, ts := trackingServer(t)
	resp, status := req(t, http.MethodGet, ts.URL+"/admin/mail/status", "")
	if resp.StatusCode != http.StatusOK || status["enabled"] != false || status["ready"] != false || status["smtp_port"] != float64(25) || status["security"] != "auto" {
		t.Fatalf("default status = %d %v", resp.StatusCode, status)
	}
	if _, leaked := status["password"]; leaked {
		t.Fatal("status must not carry a password field")
	}
	// Nothing goes out and nothing is recorded while mail is off.
	server.notifyChangeSetSubmitted(context.Background(), store.ChangeSet{ID: "cs1", Title: "t"}, "someone")
	server.mailer.Wait()
	if page, _ := mailDeliveries(t, ts.URL, ""); page["total"] != float64(0) {
		t.Fatalf("ledger must stay empty while mail is off: %v", page)
	}
	if resp, _ := req(t, http.MethodPost, ts.URL+"/admin/mail/test", `{"recipient":"me@example.test"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("test send while disabled = %d, want 409", resp.StatusCode)
	}

	putTrackingSetting(t, ts.URL, "mail.password", "hunter2")
	putTrackingSetting(t, ts.URL, "mail.username", "relay-user")
	_, settings := req(t, http.MethodGet, ts.URL+"/admin/settings", "")
	for _, raw := range settings["settings"].([]any) {
		item := raw.(map[string]any)
		if item["key"] != "mail.password" {
			continue
		}
		if item["is_secret"] != true || item["is_set"] != true || strings.Contains(item["value"].(string), "hunter2") {
			t.Fatalf("settings API leaked the password: %v", item)
		}
	}
	_, status = req(t, http.MethodGet, ts.URL+"/admin/mail/status", "")
	if status["password_set"] != true || status["auth"] != true {
		t.Fatalf("status must report the password as set: %v", status)
	}
	if server.mailConf().Password != "hunter2" {
		t.Fatal("runtime snapshot must decrypt the password for the transport")
	}
	// The snapshot rejects an incomplete setup and says why, even when enabled.
	putTrackingSetting(t, ts.URL, "mail.enabled", "true")
	_, status = req(t, http.MethodGet, ts.URL+"/admin/mail/status", "")
	if status["enabled"] != true || status["ready"] != false || !strings.Contains(status["problem"].(string), "mail.smtp_host") {
		t.Fatalf("incomplete status = %v", status)
	}
	for key, value := range map[string]string{"mail.smtp_port": "70000", "mail.security": "ssl", "mail.from_address": "not-an-address"} {
		if resp, _ := req(t, http.MethodPut, ts.URL+"/admin/settings/by-key/"+key, `{"value":"`+value+`"}`); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("PUT %s=%s = %d, want 400", key, value, resp.StatusCode)
		}
	}
}

func TestMailTestSendRecordsSuccessAndFailure(t *testing.T) {
	server, ts := trackingServer(t)
	host, port := closedPort(t)
	putTrackingSetting(t, ts.URL, "mail.smtp_host", host)
	putTrackingSetting(t, ts.URL, "mail.smtp_port", strconv.Itoa(port))
	putTrackingSetting(t, ts.URL, "mail.from_address", "dataworks@example.test")
	putTrackingSetting(t, ts.URL, "mail.timeout_seconds", "1")
	putTrackingSetting(t, ts.URL, "mail.enabled", "true")

	if resp, body := req(t, http.MethodPost, ts.URL+"/admin/mail/test", `{"recipient":"nobody"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad recipient = %d %v", resp.StatusCode, body)
	}
	// Real transport against a dead relay: the button reports the failure in place.
	resp, body := req(t, http.MethodPost, ts.URL+"/admin/mail/test", `{"recipient":"ops@example.test"}`)
	if resp.StatusCode != http.StatusBadGateway || !strings.Contains(body["error"].(map[string]any)["message"].(string), "SMTP") {
		t.Fatalf("dead relay = %d %v", resp.StatusCode, body)
	}
	captured := &capturedMail{}
	server.mailer.SetSender(captured.send)
	resp, body = req(t, http.MethodPost, ts.URL+"/admin/mail/test", `{"recipient":"ops@example.test"}`)
	if resp.StatusCode != http.StatusOK || body["sent"] != true {
		t.Fatalf("test send = %d %v", resp.StatusCode, body)
	}
	if got := captured.recipients(); len(got) != 1 || got[0] != "ops@example.test" {
		t.Fatalf("sent = %v", got)
	}
	page, items := mailDeliveries(t, ts.URL, "?event=test")
	if page["total"] != float64(2) || len(items) != 2 {
		t.Fatalf("ledger = %v", page)
	}
	summary := page["summary"].(map[string]any)
	if summary["sent"] != float64(1) || summary["failed"] != float64(1) {
		t.Fatalf("summary = %v (both outcomes must be recorded)", summary)
	}
	if items[0]["status"] != "sent" || items[1]["status"] != "failed" || items[1]["error_message"] == "" || items[1]["recipient"] != "ops@example.test" {
		t.Fatalf("items = %v", items)
	}
	_, failedOnly := mailDeliveries(t, ts.URL, "?status=failed")
	if len(failedOnly) != 1 {
		t.Fatalf("status filter = %v", failedOnly)
	}
}

func TestChangeSetSubmitMailsTheOtherAdminsAndNeverBlocks(t *testing.T) {
	server, ts := trackingServer(t)
	seedMailUsers(t, server.db)
	host, port := closedPort(t)
	putTrackingSetting(t, ts.URL, "mail.smtp_host", host)
	putTrackingSetting(t, ts.URL, "mail.smtp_port", strconv.Itoa(port))
	putTrackingSetting(t, ts.URL, "mail.from_address", "dataworks@example.test")
	putTrackingSetting(t, ts.URL, "mail.timeout_seconds", "1")
	putTrackingSetting(t, ts.URL, "mail.base_url", "https://dw.example.test")
	putTrackingSetting(t, ts.URL, "mail.enabled", "true")

	// Relay is dead: the submit still succeeds and both attempts are recorded as failed.
	resp, created := req(t, http.MethodPost, ts.URL+"/admin/change-sets", `{"title":"limits","items":[{"kind":"setting","key":"limits.max_output_tokens","value":"2048"}]}`)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d %v", resp.StatusCode, created)
	}
	id := created["id"].(string)
	started := time.Now()
	if resp, body := req(t, http.MethodPost, ts.URL+"/admin/change-sets/"+id+"/submit", `{}`); resp.StatusCode != http.StatusOK || body["status"] != "pending" {
		t.Fatalf("submit = %d %v", resp.StatusCode, body)
	}
	if time.Since(started) > 800*time.Millisecond {
		t.Fatal("submit waited on the relay")
	}
	server.mailer.Wait()
	_, items := mailDeliveries(t, ts.URL, "?event="+mail.EventChangeSetSubmit)
	recipients := []string{}
	for _, item := range items {
		if item["status"] != "failed" || item["ref"] != id {
			t.Fatalf("item = %v", item)
		}
		recipients = append(recipients, item["recipient"].(string))
	}
	sort.Strings(recipients)
	if len(recipients) != 2 || recipients[0] != "admin1@example.test" || recipients[1] != "admin2@example.test" {
		t.Fatalf("recipients = %v (active admins only; disabled and developer accounts excluded)", recipients)
	}

	// Actor exclusion: when admin1 submits, only admin2 hears about it — and the body links into the app.
	captured := &capturedMail{}
	server.mailer.SetSender(captured.send)
	server.notifyChangeSetSubmitted(context.Background(), store.ChangeSet{ID: "cs_self", Title: "by admin1"}, "u_admin1")
	server.mailer.Wait()
	if got := captured.recipients(); len(got) != 1 || got[0] != "admin2@example.test" {
		t.Fatalf("recipients = %v (the submitter must not be told about their own action)", got)
	}
	// The link must be a route the admin UI actually serves ("#/<tab>", see parseHash).
	if !strings.Contains(captured.sent[0].Body, "https://dw.example.test/admin#/changesets\r\n") || strings.Contains(captured.sent[0].Subject, "\n") {
		t.Fatalf("mail = %+v", captured.sent[0])
	}

	// Switching the event off silences it without touching the others.
	putTrackingSetting(t, ts.URL, "mail.notify_change_set", "false")
	server.notifyChangeSetSubmitted(context.Background(), store.ChangeSet{ID: "cs_off", Title: "muted"}, "")
	server.notifyAlertFired(context.Background(), store.AlertRule{ID: "rule1", Name: "errors", Metric: "errors", Threshold: 0.1, WindowSeconds: 60, Scope: "global"}, 0.5)
	server.mailer.Wait()
	if got := captured.recipients(); len(got) != 3 {
		t.Fatalf("after muting change sets, only the alert (2 admins) should be added: %v", got)
	}
	if !strings.Contains(captured.sent[1].Body, "https://dw.example.test/admin#/safety\r\n") {
		t.Fatalf("alert mail must link to the safety tab: %+v", captured.sent[1])
	}
	_, status := req(t, http.MethodGet, ts.URL+"/admin/mail/status", "")
	events := status["events"].(map[string]any)
	if events[mail.EventChangeSetSubmit] != false || events[mail.EventAlertFired] != true {
		t.Fatalf("events = %v", events)
	}
}

func TestApprovalDecisionMailsTheRequesterOnly(t *testing.T) {
	server, ts := trackingServer(t)
	seedMailUsers(t, server.db)
	putTrackingSetting(t, ts.URL, "mail.smtp_host", "relay.internal")
	putTrackingSetting(t, ts.URL, "mail.from_address", "dataworks@example.test")
	putTrackingSetting(t, ts.URL, "mail.base_url", "https://dw.example.test")
	putTrackingSetting(t, ts.URL, "mail.enabled", "true")
	captured := &capturedMail{}
	server.mailer.SetSender(captured.send)

	approval := store.Approval{ID: "appr_1", UserID: "u_dev", Reason: "high risk", RiskScore: 80, ExpiresAt: time.Now().Add(time.Hour)}
	server.notifyApprovalRequested(context.Background(), approval)
	server.mailer.Wait()
	if got := captured.recipients(); len(got) != 2 || got[0] != "admin1@example.test" {
		t.Fatalf("request goes to the admins: %v", got)
	}
	if !strings.Contains(captured.sent[0].Body, "/admin#/safety\r\n") {
		t.Fatalf("approval mail must link to the safety tab: %+v", captured.sent[0])
	}
	approval.Status = "approved"
	server.notifyApprovalDecided(context.Background(), approval, "u_admin1")
	server.mailer.Wait()
	got := captured.recipients()
	if len(got) != 3 || got[2] != "dev@example.test" {
		t.Fatalf("decision goes to the requester: %v", got)
	}
	if !strings.Contains(captured.sent[2].Subject, "승인되었습니다") {
		t.Fatalf("subject = %q", captured.sent[2].Subject)
	}
	// The gate only applies an approval sent back in the header, so the mail has to say so.
	if body := captured.sent[2].Body; !strings.Contains(body, "X-Governance-Approval-ID: appr_1") || strings.Contains(body, "같은 요청을 다시 보내면 이 승인으로") {
		t.Fatalf("decision body must tell the requester to resend with the header:\n%s", body)
	}
	// A requester who is not an account (bare API key) cannot be reached and is skipped quietly.
	server.notifyApprovalDecided(context.Background(), store.Approval{ID: "appr_2", UserID: "", Status: "rejected"}, "u_admin1")
	server.mailer.Wait()
	if len(captured.recipients()) != 3 {
		t.Fatal("no address, no mail")
	}
}

func TestMailThrottleAllowsOncePerWindowPerKey(t *testing.T) {
	var throttle mailThrottle
	start := time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)
	if !throttle.allow("key_a", start, time.Minute) || throttle.allow("key_a", start.Add(30*time.Second), time.Minute) {
		t.Fatal("the second call inside the window must be held")
	}
	if !throttle.allow("key_b", start, time.Minute) {
		t.Fatal("another requester is independent")
	}
	if !throttle.allow("key_a", start.Add(time.Minute), time.Minute) {
		t.Fatal("a new window opens once the old one has elapsed")
	}
	if _, kept := throttle.seen["key_b"]; kept {
		t.Fatal("expired entries are pruned so the map does not grow with every requester ever seen")
	}
}

func TestApprovalRequestsFromOneRequesterMailTheAdminsOncePerWindow(t *testing.T) {
	server, ts := trackingServer(t)
	seedMailUsers(t, server.db)
	putTrackingSetting(t, ts.URL, "mail.smtp_host", "relay.internal")
	putTrackingSetting(t, ts.URL, "mail.from_address", "dataworks@example.test")
	putTrackingSetting(t, ts.URL, "mail.enabled", "true")
	captured := &capturedMail{}
	server.mailer.SetSender(captured.send)

	resp := postJSON(t, ts.URL+"/admin/policies", "", map[string]any{
		"name":  "approval for gpt-4.1",
		"rules": []any{map[string]any{"name": "model approval", "model": "gpt-4.1", "require_approval": true}},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("policy create = %d", resp.StatusCode)
	}
	resp.Body.Close()
	// Two registered keys, so the gate sees two distinct requesters.
	for _, key := range []string{"key-a", "key-b"} {
		resp := postJSON(t, ts.URL+"/admin/api-keys", "", map[string]any{"name": key, "key": key})
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("api key %s = %d", key, resp.StatusCode)
		}
	}

	// A client retrying without X-Governance-Approval-ID parks a new approval
	// every time, but the administrators hear about the requester once.
	chat := map[string]any{"model": "gpt-4.1", "messages": []any{map[string]any{"role": "user", "content": "hello"}}}
	ids := map[string]bool{}
	for i := 0; i < 3; i++ {
		resp := postJSON(t, ts.URL+"/v1/chat/completions", "key-a", chat)
		resp.Body.Close()
		if resp.StatusCode != http.StatusLocked || resp.Header.Get("X-Governance-Approval-ID") == "" {
			t.Fatalf("attempt %d = %d", i, resp.StatusCode)
		}
		ids[resp.Header.Get("X-Governance-Approval-ID")] = true
	}
	server.mailer.Wait()
	if len(ids) != 3 {
		t.Fatalf("each bare retry still gets its own approval: %v", ids)
	}
	if got := captured.recipients(); len(got) != 2 || got[0] != "admin1@example.test" || got[1] != "admin2@example.test" {
		t.Fatalf("three retries must produce one mail per admin, got %v", got)
	}
	if !strings.Contains(captured.sent[0].Body, "추가 승인 요청은") {
		t.Fatalf("the mail must say later requests from this requester are held:\n%s", captured.sent[0].Body)
	}
	_, items := mailDeliveries(t, ts.URL, "?event="+mail.EventApprovalRequested)
	if len(items) != 2 {
		t.Fatalf("ledger = %v", items)
	}

	// Another requester opens its own window.
	resp = postJSON(t, ts.URL+"/v1/chat/completions", "key-b", chat)
	resp.Body.Close()
	if resp.StatusCode != http.StatusLocked {
		t.Fatalf("key-b = %d", resp.StatusCode)
	}
	server.mailer.Wait()
	if got := captured.recipients(); len(got) != 4 {
		t.Fatalf("a different requester is mailed on its own: %v", got)
	}
}

func TestExpiringAccessDigestBundlesAndSendsOncePerDay(t *testing.T) {
	server, ts := trackingServer(t)
	seedMailUsers(t, server.db)
	putTrackingSetting(t, ts.URL, "mail.smtp_host", "relay.internal")
	putTrackingSetting(t, ts.URL, "mail.from_address", "dataworks@example.test")
	putTrackingSetting(t, ts.URL, "mail.enabled", "true")
	captured := &capturedMail{}
	server.mailer.SetSender(captured.send)
	ctx := context.Background()

	// Nothing to say: no digest, no ledger row.
	server.runMailDigest(ctx)
	server.mailer.Wait()
	if len(captured.recipients()) != 0 {
		t.Fatal("an empty digest must not be sent")
	}
	server.mailDigestDay.Store(nil)

	if resp, out := req(t, http.MethodPost, ts.URL+"/admin/dataworks/products", `{"product_key":"orders","name_ko":"주문","owner":"ops"}`); resp.StatusCode >= 300 {
		t.Fatalf("product = %d %v", resp.StatusCode, out)
	}
	soon := time.Now().UTC().Add(5 * 24 * time.Hour).Format(time.RFC3339)
	far := time.Now().UTC().Add(400 * 24 * time.Hour).Format(time.RFC3339)
	for _, body := range []string{
		`{"contract_key":"ct_soon","customer_key":"acme","allowed_fields":["*"],"valid_to":"` + soon + `"}`,
		`{"contract_key":"ct_far","customer_key":"acme","allowed_fields":["*"],"valid_to":"` + far + `"}`,
		`{"contract_key":"ct_revoked","customer_key":"acme","allowed_fields":["*"],"status":"revoked","valid_to":"` + soon + `"}`,
	} {
		if resp, out := req(t, http.MethodPost, ts.URL+"/admin/dataworks/products/orders/contract-scopes", body); resp.StatusCode != http.StatusOK {
			t.Fatalf("contract = %d %v", resp.StatusCode, out)
		}
	}
	if resp, out := req(t, http.MethodPost, ts.URL+"/admin/dataworks/products/orders/entitlements", `{"api_key_id":"key_1","contract_key":"ct_soon","expires_at":"`+soon+`"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("entitlement = %d %v", resp.StatusCode, out)
	}

	server.runMailDigest(ctx)
	server.mailer.Wait()
	if got := captured.recipients(); len(got) != 2 {
		t.Fatalf("one digest per admin: %v", got)
	}
	body := captured.sent[0].Body
	if !strings.Contains(body, "ct_soon") || strings.Contains(body, "ct_far") || strings.Contains(body, "ct_revoked") || !strings.Contains(body, "key_1") {
		t.Fatalf("digest body:\n%s", body)
	}
	if !strings.Contains(captured.sent[0].Subject, "계약 1건 · 접근권 1건") {
		t.Fatalf("subject = %q", captured.sent[0].Subject)
	}
	// Same day again, even after this pod forgets: the ledger says it already went out.
	server.mailDigestDay.Store(nil)
	server.runMailDigest(ctx)
	server.mailer.Wait()
	if len(captured.recipients()) != 2 {
		t.Fatal("digest must go out once per day")
	}
	_, items := mailDeliveries(t, ts.URL, "?event="+mail.EventAccessExpiring)
	if len(items) != 2 || items[0]["status"] != "sent" {
		t.Fatalf("ledger = %v", items)
	}
}
