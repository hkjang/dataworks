package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"dataworks/internal/config"
)

func TestOIDCFlowStateRoundtrip(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "oidc.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	if err := db.SaveOIDCFlowState(ctx, "state1", OIDCFlowState{Nonce: "nonce1", Verifier: "verifier1", Silent: true, ReturnTo: "/dataworks/products/x?tab=1"}, now); err != nil {
		t.Fatal(err)
	}
	fs, found, err := db.TakeOIDCFlowState(ctx, "state1")
	if err != nil || !found || fs.Nonce != "nonce1" || fs.Verifier != "verifier1" || !fs.Silent || fs.ReturnTo != "/dataworks/products/x?tab=1" {
		t.Fatalf("take = (%+v,%v,%v)", fs, found, err)
	}
	// Single-use: a second take must miss.
	if _, found, _ := db.TakeOIDCFlowState(ctx, "state1"); found {
		t.Fatal("flow state should be single-use (consumed on first take)")
	}
	// Unknown state.
	if _, found, _ := db.TakeOIDCFlowState(ctx, "nope"); found {
		t.Fatal("unknown state should not be found")
	}
	// A plain (non-silent) login round-trips with the zero values.
	if err := db.SaveOIDCFlowState(ctx, "plain", OIDCFlowState{Nonce: "n", Verifier: "v"}, now); err != nil {
		t.Fatal(err)
	}
	if fs, found, _ := db.TakeOIDCFlowState(ctx, "plain"); !found || fs.Silent || fs.ReturnTo != "" {
		t.Fatalf("plain take = (%+v,%v)", fs, found)
	}
	// Expired (created 11m ago) → not found.
	if err := db.SaveOIDCFlowState(ctx, "old", OIDCFlowState{Nonce: "n", Verifier: "v"}, now.Add(-11*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := db.TakeOIDCFlowState(ctx, "old"); found {
		t.Fatal("expired flow state should not be found")
	}
}

// Silent SSO must be off unless an administrator turned it on; the stored override must
// round-trip the flag so the runtime overlay sees what the screen saved.
func TestSSOProviderConfigAutoLoginRoundtrip(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "sso.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSSOProviderConfig(ctx, SSOProviderConfig{Provider: "keycloak", Enabled: true, IssuerURL: "https://kc/realms/x", ClientID: "c"}); err != nil {
		t.Fatal(err)
	}
	rec, found, err := db.GetSSOProviderConfig(ctx, "keycloak")
	if err != nil || !found || rec.AutoLogin {
		t.Fatalf("default auto_login must be off: rec=%+v found=%v err=%v", rec, found, err)
	}
	if err := db.SaveSSOProviderConfig(ctx, SSOProviderConfig{Provider: "keycloak", Enabled: true, IssuerURL: "https://kc/realms/x", ClientID: "c", AutoLogin: true}); err != nil {
		t.Fatal(err)
	}
	if rec, _, _ := db.GetSSOProviderConfig(ctx, "keycloak"); !rec.AutoLogin {
		t.Fatalf("auto_login=true did not persist: %+v", rec)
	}
}
