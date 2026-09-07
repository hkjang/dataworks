package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"dataworks/internal/store"
)

func TestContractResponseFieldsUsesContractSpelling(t *testing.T) {
	cases := []struct {
		name          string
		allowed       []string
		requested     []string
		wantFields    []string
		wantForbidden []string
	}{
		{
			name:       "request casing is normalized to the contract",
			allowed:    []string{"score", "risk_band"},
			requested:  []string{"Score", "RISK_BAND"},
			wantFields: []string{"score", "risk_band"},
		},
		{
			name:       "contract casing is preserved as written",
			allowed:    []string{"riskBand"},
			requested:  []string{"riskband"},
			wantFields: []string{"riskBand"},
		},
		{
			name:       "no requested fields returns the whole contract",
			allowed:    []string{"score", "risk_band"},
			wantFields: []string{"score", "risk_band"},
		},
		{
			name:       "wildcard contracts keep the requested fields",
			allowed:    []string{"*"},
			requested:  []string{"Score"},
			wantFields: []string{"Score"},
		},
		{
			name:          "fields outside the contract are reported as requested",
			allowed:       []string{"score"},
			requested:     []string{"Score", "salary"},
			wantForbidden: []string{"salary"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields, forbidden := contractResponseFields(tc.allowed, tc.requested)
			if len(tc.wantForbidden) > 0 {
				if !reflect.DeepEqual(forbidden, tc.wantForbidden) {
					t.Fatalf("forbidden = %v, want %v", forbidden, tc.wantForbidden)
				}
				return
			}
			if len(forbidden) > 0 {
				t.Fatalf("unexpected forbidden fields %v", forbidden)
			}
			if !reflect.DeepEqual(fields, tc.wantFields) {
				t.Fatalf("fields = %v, want %v", fields, tc.wantFields)
			}
		})
	}
}

// The published product OpenAPI document names the data properties after allowed_fields, so a
// query that spells a field differently must still get the contracted key back.
func TestDataProductQueryReturnsContractFieldSpelling(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "dw-fields.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	ctx := context.Background()
	if err := db.UpsertDataProduct(ctx, store.DataProduct{
		ID: "dprod_fields", ProductKey: "dw_field_case", NameKO: "Field Case API",
		SourceType: "api", SourceRef: "loan_history", Owner: "data",
		Sensitivity: "internal", Status: "published",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIKey(ctx, store.APIKeyRecord{
		ID: "apikey_fields", KeyHash: hashProxyKey("field-client-token"), Name: "field-client",
		Team: "team_fields", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_fields", ProductKey: "dw_field_case", CustomerKey: "cust_fields",
		AllowedFields: []string{"score", "risk_band"}, Status: "active", Purpose: "risk monitoring",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
		ID: "ent_fields", APIKeyID: "apikey_fields", APIKeyHash: hashProxyKey("field-client-token"),
		ProductKey: "dw_field_case", CustomerKey: "cust_fields", ContractKey: "ct_fields",
		Status: "active", Scope: "data_product:query",
	}); err != nil {
		t.Fatal(err)
	}

	resp := postJSON(t, srv.URL+"/v1/data-products/dw_field_case/query", "field-client-token", map[string]any{
		"fields": []string{"Score", "RISK_BAND"},
	})
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("query status = %d: %s", resp.StatusCode, raw)
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 2 {
		t.Fatalf("data = %v, want the two contracted fields", payload.Data)
	}
	for _, field := range []string{"score", "risk_band"} {
		if _, ok := payload.Data[field]; !ok {
			t.Fatalf("data is missing contracted key %q: %v", field, payload.Data)
		}
	}
}

// A contract scope without allowed_fields denies every runtime query, so the write path must
// refuse it instead of publishing a contract that can never serve data.
func TestContractScopeRequiresAllowedFields(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "dw-fields-admin.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	ctx := context.Background()
	if err := db.UpsertDataProduct(ctx, store.DataProduct{
		ID: "dprod_fields_admin", ProductKey: "dw_field_input", NameKO: "Field Input",
		SourceType: "api", SourceRef: "loan_history", Owner: "data", Status: "draft",
	}); err != nil {
		t.Fatal(err)
	}

	resp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_field_input/contract-scopes", "", map[string]any{
		"contract_key": "ct_empty", "customer_key": "cust_fields", "allowed_fields": []string{"  ", ""},
		"purpose": "risk monitoring",
	})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("contract scope without allowed_fields status = %d: %s", resp.StatusCode, body)
	}
	if code := errorCodeOf(t, body); code != "invalid_allowed_fields" {
		t.Fatalf("error code = %q: %s", code, body)
	}
	if _, ok, err := db.GetContractScope(ctx, "ct_empty"); err != nil || ok {
		t.Fatalf("rejected contract scope must not be stored (found=%v err=%v)", ok, err)
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_field_input/contract-scopes", "", map[string]any{
		"contract_key": "ct_fields", "customer_key": "cust_fields",
		"allowed_fields": []string{" score ", "Score", "", "risk_band"},
		"purpose":        "risk monitoring",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid contract scope status = %d: %s", resp.StatusCode, body)
	}
	scope, ok, err := db.GetContractScope(ctx, "ct_fields")
	if err != nil || !ok {
		t.Fatalf("contract scope lookup: found=%v err=%v", ok, err)
	}
	if want := []string{"score", "risk_band"}; !reflect.DeepEqual(scope.AllowedFields, want) {
		t.Fatalf("stored allowed_fields = %v, want %v", scope.AllowedFields, want)
	}
}
