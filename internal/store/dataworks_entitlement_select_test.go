package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"dataworks/internal/config"
)

func TestEntitlementActive(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		ent  APIEntitlement
		want bool
	}{
		{"active without expiry", APIEntitlement{Status: "active"}, true},
		{"active mixed case", APIEntitlement{Status: "Active"}, true},
		{"active until tomorrow", APIEntitlement{Status: "active", ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339Nano)}, true},
		{"expired yesterday", APIEntitlement{Status: "active", ExpiresAt: now.Add(-24 * time.Hour).Format(time.RFC3339Nano)}, false},
		{"revoked", APIEntitlement{Status: "revoked"}, false},
		{"empty status", APIEntitlement{}, false},
		// A timestamp the gateway cannot parse must not keep access open.
		{"unparseable expiry", APIEntitlement{Status: "active", ExpiresAt: "2026-12-31"}, false},
	}
	for _, tc := range cases {
		if got := EntitlementActive(tc.ent, now); got != tc.want {
			t.Errorf("%s: EntitlementActive = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestFindAPIEntitlementPrefersActive covers a key that holds more than one entitlement row
// for the same product: the active grant must win even when a revoked or expired row was
// written later.
func TestFindAPIEntitlementPrefersActive(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "dw.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	past := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)
	rows := []APIEntitlement{
		{ID: "ent_active", APIKeyID: "key_bank", ProductKey: "dw_credit_score", ContractKey: "ct_active", Status: "active"},
		{ID: "ent_expired", APIKeyID: "key_bank", ProductKey: "dw_credit_score", ContractKey: "ct_expired", Status: "active", ExpiresAt: past},
		{ID: "ent_revoked", APIKeyID: "key_bank", ProductKey: "dw_credit_score", ContractKey: "ct_revoked", Status: "revoked"},
	}
	for _, ent := range rows {
		if err := db.UpsertAPIEntitlement(ctx, ent); err != nil {
			t.Fatal(err)
		}
	}

	got, ok, err := db.FindAPIEntitlement(ctx, "dw_credit_score", "key_bank", "")
	if err != nil || !ok {
		t.Fatalf("find failed: ok=%v err=%v", ok, err)
	}
	if got.ID != "ent_active" {
		t.Fatalf("expected the active entitlement, got %q (contract %q)", got.ID, got.ContractKey)
	}

	// With nothing active the newest row is still returned, so the runtime gate reports an
	// inactive entitlement instead of a missing one.
	rows[0].Status = "revoked"
	if err := db.UpsertAPIEntitlement(ctx, rows[0]); err != nil {
		t.Fatal(err)
	}
	got, ok, err = db.FindAPIEntitlement(ctx, "dw_credit_score", "key_bank", "")
	if err != nil || !ok {
		t.Fatalf("expected an inactive entitlement to still be found: ok=%v err=%v", ok, err)
	}
	if EntitlementActive(got, time.Now().UTC()) {
		t.Fatalf("expected an inactive entitlement, got %+v", got)
	}
}
