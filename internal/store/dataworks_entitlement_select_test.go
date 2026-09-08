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

// TestListAPIEntitlementCandidatesOrder checks that the runtime gets every row a key holds for
// a product, active grants first, so it can skip a grant whose contract scope is unusable.
func TestListAPIEntitlementCandidatesOrder(t *testing.T) {
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
		{ID: "ent_a_active", APIKeyID: "key_bank", ProductKey: "dw_credit_score", ContractKey: "ct_a", Status: "active"},
		{ID: "ent_z_active", APIKeyID: "key_bank", ProductKey: "dw_credit_score", ContractKey: "ct_z", Status: "active"},
		{ID: "ent_expired", APIKeyID: "key_bank", ProductKey: "dw_credit_score", ContractKey: "ct_expired", Status: "active", ExpiresAt: past},
		{ID: "ent_revoked", APIKeyID: "key_bank", ProductKey: "dw_credit_score", ContractKey: "ct_revoked", Status: "revoked"},
		{ID: "ent_other", APIKeyID: "key_bank", ProductKey: "dw_other", ContractKey: "ct_other", Status: "active"},
	}
	for _, ent := range rows {
		if err := db.UpsertAPIEntitlement(ctx, ent); err != nil {
			t.Fatal(err)
		}
	}

	got, err := db.ListAPIEntitlementCandidates(ctx, "dw_credit_score", "key_bank", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("expected the four rows for this product, got %d: %+v", len(got), got)
	}
	now := time.Now().UTC()
	if !EntitlementActive(got[0], now) || !EntitlementActive(got[1], now) {
		t.Fatalf("expected both active grants first, got %+v", got)
	}
	if EntitlementActive(got[2], now) || EntitlementActive(got[3], now) {
		t.Fatalf("expected the inactive rows last, got %+v", got)
	}
	if got[0].ID != "ent_z_active" {
		t.Fatalf("expected the newest active row first, got %q", got[0].ID)
	}

	if candidates, err := db.ListAPIEntitlementCandidates(ctx, "dw_credit_score", "", ""); err != nil || len(candidates) != 0 {
		t.Fatalf("expected no candidates without a key identity: %d rows, err=%v", len(candidates), err)
	}
}
