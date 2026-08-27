package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestRotateAPIKeyOwnedPreservesPolicyAndRollsBackOnOwnershipFailure(t *testing.T) {
	db := openAggTestStore(t)
	defer db.Close()
	ctx := context.Background()
	expires := time.Now().UTC().Add(8 * time.Hour)
	old := APIKeyRecord{
		ID: "key_old", Name: "personal", KeyHash: "hash-old", Owner: "Owner", Team: "team-a", UserID: "user-a",
		Role: "developer", Status: "active", Scopes: []string{}, AllowedIPs: []string{"10.0.0.0/24"},
		AllowedModels: []string{"gpt-4.1"}, DeniedModels: []string{"gpt-danger"},
		AllowedProviders: []string{"openai"}, DeniedProviders: []string{"evil"}, BudgetLimitKRW: 321, ExpiresAt: expires,
	}
	if err := db.UpsertAPIKey(ctx, old); err != nil {
		t.Fatal(err)
	}
	replacement := old
	replacement.ID = "key_new"
	replacement.KeyHash = "hash-new"
	replacement.CreatedAt = time.Now().UTC()
	if err := db.RotateAPIKeyOwned(ctx, old.ID, old.UserID, replacement); err != nil {
		t.Fatal(err)
	}
	oldAfter, found, err := db.GetAPIKey(ctx, old.ID)
	if err != nil || !found || oldAfter.Status != "revoked" || oldAfter.RevokedAt.IsZero() {
		t.Fatalf("old key not revoked found=%v key=%+v err=%v", found, oldAfter, err)
	}
	got, found, err := db.GetAPIKey(ctx, replacement.ID)
	if err != nil || !found {
		t.Fatalf("replacement missing found=%v err=%v", found, err)
	}
	if len(got.Scopes) != 0 || got.UserID != old.UserID || got.Team != old.Team || got.Role != old.Role ||
		!reflect.DeepEqual(got.AllowedIPs, old.AllowedIPs) || !reflect.DeepEqual(got.AllowedModels, old.AllowedModels) ||
		!reflect.DeepEqual(got.DeniedModels, old.DeniedModels) || !reflect.DeepEqual(got.AllowedProviders, old.AllowedProviders) ||
		!reflect.DeepEqual(got.DeniedProviders, old.DeniedProviders) || got.BudgetLimitKRW != old.BudgetLimitKRW || !got.ExpiresAt.Equal(old.ExpiresAt) {
		t.Fatalf("replacement policy mismatch old=%+v replacement=%+v", old, got)
	}

	secondOld := APIKeyRecord{ID: "key_owned", Name: "owned", KeyHash: "hash-owned", UserID: "user-a", Status: "active", Scopes: []string{"models:read"}}
	if err := db.UpsertAPIKey(ctx, secondOld); err != nil {
		t.Fatal(err)
	}
	unauthorizedReplacement := secondOld
	unauthorizedReplacement.ID = "key_must_rollback"
	unauthorizedReplacement.KeyHash = "hash-rollback"
	if err := db.RotateAPIKeyOwned(ctx, secondOld.ID, "different-user", unauthorizedReplacement); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("wrong-owner rotation error=%v, want ErrInvalidTransition", err)
	}
	if _, found, err := db.GetAPIKey(ctx, unauthorizedReplacement.ID); err != nil || found {
		t.Fatalf("failed rotation must roll replacement back found=%v err=%v", found, err)
	}
	stillActive, found, err := db.GetAPIKey(ctx, secondOld.ID)
	if err != nil || !found || stillActive.Status != "active" {
		t.Fatalf("failed rotation changed old key found=%v key=%+v err=%v", found, stillActive, err)
	}
}

func TestAPIKeyOwnedPolicyUpdateAndRevokeEnforceOwnership(t *testing.T) {
	db := openAggTestStore(t)
	defer db.Close()
	ctx := context.Background()
	rec := APIKeyRecord{ID: "key_owned", Name: "owned", KeyHash: "owned-hash", UserID: "user-a", Status: "active", Scopes: []string{"models:read"}}
	if err := db.UpsertAPIKey(ctx, rec); err != nil {
		t.Fatal(err)
	}
	rec.Scopes = []string{}
	if changed, err := db.UpdateAPIKeyPolicyOwned(ctx, rec, "user-b"); err != nil || changed {
		t.Fatalf("wrong owner update changed=%v err=%v", changed, err)
	}
	stored, found, err := db.GetAPIKey(ctx, rec.ID)
	if err != nil || !found || !reflect.DeepEqual(stored.Scopes, []string{"models:read"}) {
		t.Fatalf("wrong owner altered policy found=%v key=%+v err=%v", found, stored, err)
	}
	if changed, err := db.UpdateAPIKeyPolicyOwned(ctx, rec, "user-a"); err != nil || !changed {
		t.Fatalf("owner update changed=%v err=%v", changed, err)
	}
	stored, _, _ = db.GetAPIKey(ctx, rec.ID)
	if len(stored.Scopes) != 0 {
		t.Fatalf("explicit empty scopes not persisted: %v", stored.Scopes)
	}
	if changed, err := db.RevokeAPIKeyOwned(ctx, rec.ID, "user-b"); err != nil || changed {
		t.Fatalf("wrong owner revoke changed=%v err=%v", changed, err)
	}
	if changed, err := db.RevokeAPIKeyOwned(ctx, rec.ID, "user-a"); err != nil || !changed {
		t.Fatalf("owner revoke changed=%v err=%v", changed, err)
	}
}
