package proxy

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"dataworks/internal/config"
	"dataworks/internal/store"
)

type actionCenterPayload struct {
	Summary        map[string]int   `json:"summary"`
	Actions        []map[string]any `json:"actions"`
	ExpiringWithin string           `json:"expiring_within"`
}

func getActionCenter(t *testing.T, baseURL string, query string) actionCenterPayload {
	t.Helper()
	resp, err := http.Get(baseURL + "/admin/dataworks/action-center" + query)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("action center%s status = %d: %s", query, resp.StatusCode, body)
	}
	var payload actionCenterPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func actionsOfType(payload actionCenterPayload, actionType string) []map[string]any {
	out := []map[string]any{}
	for _, action := range payload.Actions {
		if action["type"] == actionType {
			out = append(out, action)
		}
	}
	return out
}

// An entitlement that is still active but about to lapse has to be flagged before the expiry
// day: without a lookahead the customer key simply stops working and the only signal is the
// inactive_access entry that appears after access is already gone.
func TestDataWorksActionCenterWarnsBeforeEntitlementExpires(t *testing.T) {
	db, srv := newAccessWindowTestServer(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_soon", ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
		ValidTo: now.Add(60 * 24 * time.Hour).Format(time.RFC3339Nano), Status: "active",
		AllowedFields: []string{"score"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
		ID: "ent_soon", APIKeyID: "key_bank", ProductKey: "dw_credit_score", ContractKey: "ct_soon",
		ExpiresAt: now.Add(20 * 24 * time.Hour).Format(time.RFC3339Nano), Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
		ID: "ent_stable", APIKeyID: "key_insurer", ProductKey: "dw_credit_score", ContractKey: "ct_soon",
		ExpiresAt: now.Add(200 * 24 * time.Hour).Format(time.RFC3339Nano), Status: "active",
	}); err != nil {
		t.Fatal(err)
	}

	payload := getActionCenter(t, srv.URL, "")
	if payload.Summary["expiring_access"] != 1 {
		t.Fatalf("expiring_access = %d, want 1: %+v", payload.Summary["expiring_access"], payload.Actions)
	}
	if payload.Summary["inactive_access"] != 0 {
		t.Fatalf("inactive_access = %d, want 0: %+v", payload.Summary["inactive_access"], payload.Actions)
	}
	expiring := actionsOfType(payload, "entitlement_expiring")
	if len(expiring) != 1 || expiring[0]["entitlement_id"] != "ent_soon" {
		t.Fatalf("entitlement_expiring actions = %+v", expiring)
	}
	if expiring[0]["contract_key"] != "ct_soon" {
		t.Fatalf("entitlement_expiring contract_key = %v, want ct_soon", expiring[0]["contract_key"])
	}
	// The contract outlives the default window, so only the entitlement is due.
	if payload.Summary["expiring_contracts"] != 0 {
		t.Fatalf("expiring_contracts = %d, want 0: %+v", payload.Summary["expiring_contracts"], payload.Actions)
	}
	if payload.ExpiringWithin != (30 * 24 * time.Hour).String() {
		t.Fatalf("expiring_within = %q, want %q", payload.ExpiringWithin, (30 * 24 * time.Hour).String())
	}

	// A quarterly renewal cycle needs a longer lookahead than the fixed 30 days.
	wide := getActionCenter(t, srv.URL, "?expiring_within=13w")
	if wide.Summary["expiring_contracts"] != 1 {
		t.Fatalf("expiring_contracts within 13w = %d, want 1: %+v", wide.Summary["expiring_contracts"], wide.Actions)
	}
	if wide.Summary["expiring_access"] != 1 {
		t.Fatalf("expiring_access within 13w = %d, want 1: %+v", wide.Summary["expiring_access"], wide.Actions)
	}

	// A shorter lookahead only reports what lapses inside it.
	narrow := getActionCenter(t, srv.URL, "?expiring_within=7d")
	if narrow.Summary["expiring_access"] != 0 {
		t.Fatalf("expiring_access within 7d = %d, want 0: %+v", narrow.Summary["expiring_access"], narrow.Actions)
	}
	if narrow.ExpiringWithin != (7 * 24 * time.Hour).String() {
		t.Fatalf("expiring_within = %q, want %q", narrow.ExpiringWithin, (7 * 24 * time.Hour).String())
	}
}

// A contract an operator parked or closed is not waiting on a renewal, and its valid_to only
// moves further into the past, so counting it as expiring would pin it to the screen forever
// at high severity and bury the live contracts that really are about to lapse.
func TestDataWorksActionCenterSkipsInactiveContractScopes(t *testing.T) {
	db, srv := newAccessWindowTestServer(t)
	ctx := context.Background()

	now := time.Now().UTC()
	past := now.Add(-90 * 24 * time.Hour).Format(time.RFC3339Nano)
	for _, tc := range []struct {
		contractKey string
		status      string
	}{
		{"ct_revoked", "revoked"},
		{"ct_draft", "draft"},
		{"ct_suspended", "suspended"},
	} {
		if err := db.UpsertContractScope(ctx, store.ContractScope{
			ContractKey: tc.contractKey, ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
			ValidTo: past, Status: tc.status, AllowedFields: []string{"score"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_live", ProductKey: "dw_credit_score", CustomerKey: "cust_insurer",
		ValidTo: now.Add(10 * 24 * time.Hour).Format(time.RFC3339Nano), Status: "active",
		AllowedFields: []string{"score"},
	}); err != nil {
		t.Fatal(err)
	}

	payload := getActionCenter(t, srv.URL, "")
	if payload.Summary["expiring_contracts"] != 1 {
		t.Fatalf("expiring_contracts = %d, want 1: %+v", payload.Summary["expiring_contracts"], payload.Actions)
	}
	expiring := actionsOfType(payload, "contract_expiring")
	if len(expiring) != 1 || expiring[0]["contract_key"] != "ct_live" {
		t.Fatalf("contract_expiring actions = %+v", expiring)
	}
}

// The runtime serves any grant store.EntitlementActive accepts, and that rule ignores case and
// surrounding space. Rows written before the admin path normalised the field still hold values
// like "Active", so judging them with an exact match told the operator to revoke access that
// was working — while a genuinely closed grant has to keep showing up.
func TestDataWorksActionCenterMatchesRuntimeEntitlementStatus(t *testing.T) {
	db, srv := newAccessWindowTestServer(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_legacy", ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
		ValidTo: now.Add(200 * 24 * time.Hour).Format(time.RFC3339Nano), Status: "active",
		AllowedFields: []string{"score"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id        string
		status    string
		expiresAt string
	}{
		{"ent_upper", "Active", now.Add(200 * 24 * time.Hour).Format(time.RFC3339Nano)},
		{"ent_padded", " active ", " " + now.Add(20*24*time.Hour).Format(time.RFC3339Nano) + " "},
		{"ent_revoked", "revoked", now.Add(200 * 24 * time.Hour).Format(time.RFC3339Nano)},
	} {
		if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
			ID: tc.id, APIKeyID: "key_" + tc.id, ProductKey: "dw_credit_score", ContractKey: "ct_legacy",
			ExpiresAt: tc.expiresAt, Status: tc.status,
		}); err != nil {
			t.Fatal(err)
		}
	}

	payload := getActionCenter(t, srv.URL, "")
	inactive := actionsOfType(payload, "entitlement_inactive")
	if len(inactive) != 1 || inactive[0]["entitlement_id"] != "ent_revoked" {
		t.Fatalf("entitlement_inactive actions = %+v", inactive)
	}
	if payload.Summary["inactive_access"] != 1 {
		t.Fatalf("inactive_access = %d, want 1: %+v", payload.Summary["inactive_access"], payload.Actions)
	}
	// The padded row is live and lapses inside the default window, so it belongs in the
	// lookahead instead of being written off as already gone.
	expiring := actionsOfType(payload, "entitlement_expiring")
	if len(expiring) != 1 || expiring[0]["entitlement_id"] != "ent_padded" {
		t.Fatalf("entitlement_expiring actions = %+v", expiring)
	}
	if payload.Summary["expiring_access"] != 1 {
		t.Fatalf("expiring_access = %d, want 1: %+v", payload.Summary["expiring_access"], payload.Actions)
	}
}

// Falling back to the default window for a malformed value would answer for a different
// horizon than the operator asked about, so the request is rejected instead.
func TestDataWorksActionCenterRejectsInvalidExpiringWindow(t *testing.T) {
	_, srv := newAccessWindowTestServer(t)

	for _, raw := range []string{"quarter", "0d", "-30d", "30x"} {
		resp, err := http.Get(srv.URL + "/admin/dataworks/action-center?expiring_within=" + raw)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expiring_within=%s status = %d: %s", raw, resp.StatusCode, body)
		}
		if code := errorCodeOf(t, body); code != "invalid_expiring_within" {
			t.Fatalf("expiring_within=%s error code = %q: %s", raw, code, body)
		}
	}
}

// A day or week count big enough to overflow int64 nanoseconds wraps into some other window:
// 106751992d lands on about 20 hours and 200000000d lands on a negative duration, so the screen
// silently answers for a window the operator never asked about — the exact failure the rejection
// above exists to prevent — and reports the wrapped value back as the applied window.
func TestDataWorksActionCenterRejectsOverflowingExpiringWindow(t *testing.T) {
	_, srv := newAccessWindowTestServer(t)

	for _, raw := range []string{"106751992d", "200000000d", "30000000w", "9223372036854775807d", "2562048h"} {
		resp, err := http.Get(srv.URL + "/admin/dataworks/action-center?expiring_within=" + raw)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expiring_within=%s status = %d: %s", raw, resp.StatusCode, body)
		}
		if code := errorCodeOf(t, body); code != "invalid_expiring_within" {
			t.Fatalf("expiring_within=%s error code = %q: %s", raw, code, body)
		}
	}

	// The largest windows that still fit keep working, so the guard only removes the wrapped
	// answers and not the long lookaheads a multi-year contract review uses.
	for _, tc := range []struct {
		raw  string
		want time.Duration
	}{
		{"106751d", 106751 * 24 * time.Hour},
		{"15250w", 15250 * 7 * 24 * time.Hour},
	} {
		payload := getActionCenter(t, srv.URL, "?expiring_within="+tc.raw)
		if payload.ExpiringWithin != tc.want.String() {
			t.Fatalf("expiring_within=%s applied window = %q, want %q", tc.raw, payload.ExpiringWithin, tc.want.String())
		}
	}
}

func TestParseExpiryHorizon(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
		ok   bool
	}{
		{"", 30 * 24 * time.Hour, true},
		{"  ", 30 * 24 * time.Hour, true},
		{"45d", 45 * 24 * time.Hour, true},
		{"13W", 13 * 7 * 24 * time.Hour, true},
		{"72h", 72 * time.Hour, true},
		{"90m", 90 * time.Minute, true},
		{"0", 0, false},
		{"0d", 0, false},
		{"-7d", 0, false},
		{"-24h", 0, false},
		{"90", 0, false},
		{"quarter", 0, false},
		// Boundaries of the int64 nanosecond range: one more day or week wraps.
		{"106751d", 106751 * 24 * time.Hour, true},
		{"106752d", 0, false},
		{"15250w", 15250 * 7 * 24 * time.Hour, true},
		{"15251w", 0, false},
		{"106751992d", 0, false},
		{"200000000d", 0, false},
		{"9223372036854775808d", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseExpiryHorizon(tc.raw)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Fatalf("parseExpiryHorizon(%q) = (%v, %t), want (%v, %t)", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}

// Legacy rows preserve surrounding whitespace even though admin writes now trim it.
// The same persisted scope must serve queries and produce the right renewal warning.
func TestDataWorksActionCenterMatchesRuntimeContractExpiry(t *testing.T) {
	now := time.Now().UTC()
	paddedDate := func(days int) string {
		return " \t" + now.Add(time.Duration(days)*24*time.Hour).Format(time.RFC3339Nano) + "\r\n "
	}
	for _, tc := range []struct {
		name, validTo, status string
		serves                bool
		counts                [3]int
		severity              string
	}{
		{"twenty_days", paddedDate(20), "active", true, [3]int{1, 0, 1}, "medium"},
		{"sixty_days", paddedDate(60), "active", true, [3]int{0, 0, 1}, "medium"},
		{"empty", "", "active", true, [3]int{}, ""},
		{"whitespace", " \t\r\n ", "active", true, [3]int{}, ""},
		{"expired", paddedDate(-20), "active", false, [3]int{1, 1, 1}, "high"},
		{"invalid", " not-a-date ", "active", false, [3]int{1, 1, 1}, "high"},
		{"draft", paddedDate(-20), "draft", false, [3]int{}, ""},
		{"suspended", paddedDate(-20), "suspended", false, [3]int{}, ""},
		{"revoked", paddedDate(-20), "revoked", false, [3]int{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, srv := newAccessWindowTestServer(t)
			ctx := context.Background()
			if err := db.UpsertContractScope(ctx, store.ContractScope{
				ContractKey: "ct_legacy", ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
				ValidTo: tc.validTo, Status: tc.status, AllowedFields: []string{"score"},
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.UpsertAPIKey(ctx, store.APIKeyRecord{
				ID: "key_bank", Name: "Bank API", KeyHash: hashProxyKey("bank-secret"), Status: "active",
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
				ID: "ent_bank", APIKeyID: "key_bank", ProductKey: "dw_credit_score", ContractKey: "ct_legacy",
				Scope: "data_product:query", Status: "active",
			}); err != nil {
				t.Fatal(err)
			}
			resp := postJSON(t, srv.URL+"/v1/data-products/dw_credit_score/query", "bank-secret", map[string]any{"fields": []string{"score"}})
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := http.StatusForbidden
			if tc.serves {
				wantStatus = http.StatusOK
			}
			if resp.StatusCode != wantStatus {
				t.Fatalf("runtime query status = %d, want %d: %s", resp.StatusCode, wantStatus, body)
			}
			for i, query := range []string{"", "?expiring_within=7d", "?expiring_within=13w"} {
				payload := getActionCenter(t, srv.URL, query)
				expiring := actionsOfType(payload, "contract_expiring")
				if count, ok := payload.Summary["expiring_contracts"]; !ok || count != tc.counts[i] {
					t.Errorf("window %q expiring_contracts = %d (present=%t), want %d", query, count, ok, tc.counts[i])
				}
				if len(expiring) != tc.counts[i] {
					t.Errorf("window %q contract_expiring actions = %+v, want %d", query, expiring, tc.counts[i])
					continue
				}
				if len(expiring) == 1 {
					action := expiring[0]
					if action["severity"] != tc.severity || action["contract_key"] != "ct_legacy" ||
						action["product_key"] != "dw_credit_score" || action["customer_key"] != "cust_bank" || action["valid_to"] != tc.validTo {
						t.Errorf("window %q contract_expiring = %+v, want severity %q and original contract fields", query, action, tc.severity)
					}
				}
			}
			saved, found, err := db.GetContractScope(ctx, "ct_legacy")
			if err != nil || !found || saved.ValidTo != tc.validTo {
				t.Fatalf("stored valid_to changed: %+v, found=%t err=%v", saved, found, err)
			}
		})
	}
}

// contractScopeActive denies every query on a valid_from it cannot parse, exactly as it does on
// an unparseable valid_to, so the contract is dead until someone fixes the row. The action center
// only looked at valid_to, so such a contract was invisible on the operations screen and the first
// signal was a customer reporting 403s.
func TestDataWorksActionCenterReportsUnparseableContractStart(t *testing.T) {
	now := time.Now().UTC()
	plainDate := func(days int) string {
		return now.Add(time.Duration(days) * 24 * time.Hour).Format(time.RFC3339Nano)
	}
	for _, tc := range []struct {
		name, validFrom, validTo, status string
		serves                           bool
		wantActions                      int
		severity                         string
	}{
		// A date-only valid_from parses nowhere in this platform, and with no valid_to the old
		// loop skipped the row outright.
		{"unparseable_from_no_end", "2026-01-01", "", "active", false, 1, "high"},
		// The valid_to branch would have stopped at its own continue: the window closes far
		// beyond any lookahead, yet no query gets through today.
		{"unparseable_from_future_end", "2026-01-01", plainDate(3650), "active", false, 1, "high"},
		// Both halves unreadable is still one broken contract, not two renewal items.
		{"unparseable_both", "2026-01-01", "2026-12-31", "active", false, 1, "high"},
		// Nothing binds the runtime to a closed contract, so a broken window in it is not work.
		{"revoked", "2026-01-01", "", "revoked", false, 0, ""},
		{"draft", "2026-01-01", plainDate(3650), "draft", false, 0, ""},
		// A legacy row that only carries whitespace around a readable start serves queries and
		// must stay off the screen, the same as before.
		{"padded_readable_from", " 2020-01-01T00:00:00Z ", plainDate(3650), "active", true, 0, ""},
		{"empty_from", "", plainDate(3650), "active", true, 0, ""},
		{"whitespace_only_from", " \t\r\n ", plainDate(3650), "active", true, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, srv := newAccessWindowTestServer(t)
			ctx := context.Background()
			if err := db.UpsertContractScope(ctx, store.ContractScope{
				ContractKey: "ct_legacy", ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
				ValidFrom: tc.validFrom, ValidTo: tc.validTo, Status: tc.status,
				AllowedFields: []string{"score"},
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.UpsertAPIKey(ctx, store.APIKeyRecord{
				ID: "key_bank", Name: "Bank API", KeyHash: hashProxyKey("bank-secret"), Status: "active",
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
				ID: "ent_bank", APIKeyID: "key_bank", ProductKey: "dw_credit_score", ContractKey: "ct_legacy",
				Scope: "data_product:query", Status: "active",
			}); err != nil {
				t.Fatal(err)
			}

			resp := postJSON(t, srv.URL+"/v1/data-products/dw_credit_score/query", "bank-secret", map[string]any{"fields": []string{"score"}})
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := http.StatusForbidden
			if tc.serves {
				wantStatus = http.StatusOK
			}
			if resp.StatusCode != wantStatus {
				t.Fatalf("runtime query status = %d, want %d: %s", resp.StatusCode, wantStatus, body)
			}
			if !tc.serves {
				if code := errorCodeOf(t, body); code != "contract_scope_inactive" {
					t.Fatalf("runtime query error code = %q, want contract_scope_inactive: %s", code, body)
				}
			}

			// The finding does not depend on the lookahead: the contract is already dead.
			for _, query := range []string{"", "?expiring_within=7d", "?expiring_within=13w"} {
				payload := getActionCenter(t, srv.URL, query)
				if count := payload.Summary["expiring_contracts"]; count != tc.wantActions {
					t.Errorf("window %q expiring_contracts = %d, want %d: %+v", query, count, tc.wantActions, payload.Actions)
				}
				expiring := actionsOfType(payload, "contract_expiring")
				if len(expiring) != tc.wantActions {
					t.Errorf("window %q contract_expiring actions = %+v, want %d", query, expiring, tc.wantActions)
					continue
				}
				if len(expiring) == 1 {
					action := expiring[0]
					if action["severity"] != tc.severity || action["contract_key"] != "ct_legacy" ||
						action["product_key"] != "dw_credit_score" || action["customer_key"] != "cust_bank" ||
						action["valid_to"] != tc.validTo || action["valid_from"] != tc.validFrom {
						t.Errorf("window %q contract_expiring = %+v, want severity %q and original contract fields", query, action, tc.severity)
					}
				}
			}

			// Trimming decides the report only; the stored row and the admin view keep the original.
			saved, found, err := db.GetContractScope(ctx, "ct_legacy")
			if err != nil || !found || saved.ValidFrom != tc.validFrom || saved.ValidTo != tc.validTo {
				t.Fatalf("stored window changed: %+v, found=%t err=%v", saved, found, err)
			}
			listed := listContractScopes(t, srv.URL, "dw_credit_score")
			if len(listed) != 1 || listed[0].ValidFrom != tc.validFrom || listed[0].ValidTo != tc.validTo {
				t.Fatalf("admin contract-scopes window = %+v, want original %q..%q", listed, tc.validFrom, tc.validTo)
			}
		})
	}
}

// The admin write path refuses a window that closes before it opens (dataWorksAccessWindowOrdered)
// because contractScopeActive needs now to sit inside the window: an inverted one denies every
// query for the whole life of the contract. Rows stored before that check still exist, and the
// action center only looked at valid_to, so an inverted window whose end lies beyond the lookahead
// was permanently dead and invisible to operators.
func TestDataWorksActionCenterReportsInvertedContractWindow(t *testing.T) {
	now := time.Now().UTC()
	plainDate := func(days int) string {
		return now.Add(time.Duration(days) * 24 * time.Hour).Format(time.RFC3339Nano)
	}
	sameInstant := plainDate(3650)
	for _, tc := range []struct {
		name, validFrom, validTo, status string
		serves                           bool
		wantActions                      int
		severity                         string
	}{
		// Both ends parse and the end sits far beyond any lookahead, so none of the valid_to
		// branches ever fired — yet the window opens after it closes, so the contract is dead.
		{"inverted_far_future", plainDate(3650), plainDate(3600), "active", false, 1, "high"},
		// An inverted window whose end has already passed is one broken contract, not two: the
		// expired-end branch and the ordering branch must still produce a single entry.
		{"inverted_past_end", plainDate(1), plainDate(-1), "active", false, 1, "high"},
		// A window that opens before it closes serves queries and stays off the screen.
		{"ordered_window", plainDate(-3650), plainDate(3650), "active", true, 0, ""},
		// The write path rejects only valid_from strictly after valid_to, so the same instant on
		// both ends is a scheduled contract, not a fault.
		{"equal_bounds", sameInstant, sameInstant, "active", false, 0, ""},
		{"empty_from", "", plainDate(3650), "active", true, 0, ""},
		{"empty_to", plainDate(-3650), "", "active", true, 0, ""},
		// Nothing binds the runtime to a parked or closed contract, so a broken window in it is
		// not renewal work.
		{"draft_inverted", plainDate(3650), plainDate(3600), "draft", false, 0, ""},
		{"revoked_inverted", plainDate(3650), plainDate(3600), "revoked", false, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, srv := newAccessWindowTestServer(t)
			ctx := context.Background()
			if err := db.UpsertContractScope(ctx, store.ContractScope{
				ContractKey: "ct_legacy", ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
				ValidFrom: tc.validFrom, ValidTo: tc.validTo, Status: tc.status,
				AllowedFields: []string{"score"},
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.UpsertAPIKey(ctx, store.APIKeyRecord{
				ID: "key_bank", Name: "Bank API", KeyHash: hashProxyKey("bank-secret"), Status: "active",
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
				ID: "ent_bank", APIKeyID: "key_bank", ProductKey: "dw_credit_score", ContractKey: "ct_legacy",
				Scope: "data_product:query", Status: "active",
			}); err != nil {
				t.Fatal(err)
			}

			resp := postJSON(t, srv.URL+"/v1/data-products/dw_credit_score/query", "bank-secret", map[string]any{"fields": []string{"score"}})
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := http.StatusForbidden
			if tc.serves {
				wantStatus = http.StatusOK
			}
			if resp.StatusCode != wantStatus {
				t.Fatalf("runtime query status = %d, want %d: %s", resp.StatusCode, wantStatus, body)
			}
			if !tc.serves {
				if code := errorCodeOf(t, body); code != "contract_scope_inactive" {
					t.Fatalf("runtime query error code = %q, want contract_scope_inactive: %s", code, body)
				}
			}

			// An inverted window is dead regardless of how far ahead the operator looks.
			for _, query := range []string{"", "?expiring_within=7d", "?expiring_within=13w"} {
				payload := getActionCenter(t, srv.URL, query)
				if count := payload.Summary["expiring_contracts"]; count != tc.wantActions {
					t.Errorf("window %q expiring_contracts = %d, want %d: %+v", query, count, tc.wantActions, payload.Actions)
				}
				expiring := actionsOfType(payload, "contract_expiring")
				if len(expiring) != tc.wantActions {
					t.Errorf("window %q contract_expiring actions = %+v, want %d", query, expiring, tc.wantActions)
					continue
				}
				if len(expiring) == 1 {
					action := expiring[0]
					if action["severity"] != tc.severity || action["contract_key"] != "ct_legacy" ||
						action["product_key"] != "dw_credit_score" || action["customer_key"] != "cust_bank" ||
						action["valid_to"] != tc.validTo || action["valid_from"] != tc.validFrom {
						t.Errorf("window %q contract_expiring = %+v, want severity %q and original contract fields", query, action, tc.severity)
					}
				}
			}

			// Trimming decides the report only; the stored row and the admin view keep the original.
			saved, found, err := db.GetContractScope(ctx, "ct_legacy")
			if err != nil || !found || saved.ValidFrom != tc.validFrom || saved.ValidTo != tc.validTo {
				t.Fatalf("stored window changed: %+v, found=%t err=%v", saved, found, err)
			}
			listed := listContractScopes(t, srv.URL, "dw_credit_score")
			if len(listed) != 1 || listed[0].ValidFrom != tc.validFrom || listed[0].ValidTo != tc.validTo {
				t.Fatalf("admin contract-scopes window = %+v, want original %q..%q", listed, tc.validFrom, tc.validTo)
			}
		})
	}
}

// A failed inventory read is not evidence that no operational work remains. Exercise
// real SQLite failures through the production store and router, then restore each
// table to distinguish an unavailable inventory from a successfully empty one.
func TestDataWorksActionCenterRejectsUnavailableInventories(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "action-center.db")
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "audit.ndjson"))
	logger.Start()
	t.Cleanup(func() { logger.Stop(ctx) })
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	t.Cleanup(srv.Close)
	schema, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { schema.Close() })

	assertEmptyInventory := func(t *testing.T) {
		t.Helper()
		payload := getActionCenter(t, srv.URL, "")
		if len(payload.Actions) != 0 {
			t.Fatalf("empty inventory actions = %+v", payload.Actions)
		}
		if len(payload.Summary) != 9 {
			t.Fatalf("summary = %+v, want all 9 counters", payload.Summary)
		}
		for key, count := range payload.Summary {
			if count != 0 {
				t.Errorf("empty inventory %s = %d, want 0", key, count)
			}
		}
	}
	assertEmptyInventory(t)
	for _, tc := range []struct{ table, code string }{
		{"dw_product_fit_scores", "fit_scores_failed"},
		{"dw_contract_scopes", "contract_scopes_failed"},
		{"dw_api_entitlements", "entitlements_failed"},
		{"dw_data_watermarks", "watermarks_failed"},
		{"dw_product_costs", "costs_failed"},
		{"dw_retirement_candidates", "retirement_candidates_failed"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			// Table names come exclusively from the fixed cases above.
			if _, err := schema.ExecContext(ctx, "ALTER TABLE "+tc.table+" RENAME TO unavailable_inventory"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := schema.ExecContext(ctx, "ALTER TABLE unavailable_inventory RENAME TO "+tc.table); err != nil {
					t.Fatal(err)
				}
				assertEmptyInventory(t)
			})
			resp, err := http.Get(srv.URL + "/admin/dataworks/action-center")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusInternalServerError {
				t.Fatalf("unavailable %s: status = %d, want 500: %s", tc.table, resp.StatusCode, body)
			}
			if code := errorCodeOf(t, body); code != tc.code {
				t.Errorf("error code = %q, want %q", code, tc.code)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"summary", "actions"} {
				if _, ok := payload[key]; ok {
					t.Errorf("failed inventory returned a misleading %s: %s", key, body)
				}
			}
		})
	}
}

// A publish gate the server could not evaluate is not evidence that a product is clear to
// launch: GET /publish-gate and POST /publish both answer 500 on the same read failure, so
// the action center must not be the one screen that reports the product as unblocked.
// Exercise real SQLite failures through the production store and router, restoring each
// table to show the same product is reported as blocked whenever the gate can be evaluated.
func TestDataWorksActionCenterRejectsUnavailablePublishGate(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "action-center-gate.db")
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "audit.ndjson"))
	logger.Start()
	t.Cleanup(func() { logger.Stop(ctx) })
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	t.Cleanup(srv.Close)
	if err := db.UpsertDataProduct(ctx, store.DataProduct{
		// restricted sensitivity puts the product on the strict gate, which is the path that
		// reads asset readiness, approval traces, and the evidence pack.
		ID: "dprod_gate", ProductKey: "dw_gate_blocked", NameKO: "Gate Blocked API",
		SourceType: "api", SourceRef: "loan_history", Sensitivity: "restricted", Status: "approved",
	}); err != nil {
		t.Fatal(err)
	}
	schema, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { schema.Close() })

	// The product has no approvals or evidence pack, so an evaluated gate always blocks it.
	assertBlockedLaunchReported := func(t *testing.T) {
		t.Helper()
		payload := getActionCenter(t, srv.URL, "")
		if payload.Summary["blocked_launches"] != 1 {
			t.Fatalf("blocked_launches = %d, want 1: %+v", payload.Summary["blocked_launches"], payload.Actions)
		}
		blocked := actionsOfType(payload, "launch_blocked")
		if len(blocked) != 1 || blocked[0]["product_key"] != "dw_gate_blocked" {
			t.Fatalf("launch_blocked actions = %+v, want 1 for dw_gate_blocked", blocked)
		}
	}
	assertBlockedLaunchReported(t)
	for _, table := range []string{"dw_asset_readiness_scores", "dw_approval_traces", "dw_evidence_packs"} {
		t.Run(table, func(t *testing.T) {
			// Table names come exclusively from the fixed list above.
			if _, err := schema.ExecContext(ctx, "ALTER TABLE "+table+" RENAME TO unavailable_gate_input"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := schema.ExecContext(ctx, "ALTER TABLE unavailable_gate_input RENAME TO "+table); err != nil {
					t.Fatal(err)
				}
				assertBlockedLaunchReported(t)
			})
			resp, err := http.Get(srv.URL + "/admin/dataworks/action-center")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusInternalServerError {
				t.Fatalf("unavailable %s: status = %d, want 500: %s", table, resp.StatusCode, body)
			}
			if code := errorCodeOf(t, body); code != "publish_gate_failed" {
				t.Errorf("error code = %q, want %q", code, "publish_gate_failed")
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"summary", "actions"} {
				if _, ok := payload[key]; ok {
					t.Errorf("unevaluated publish gate returned a misleading %s: %s", key, body)
				}
			}
		})
	}
}
