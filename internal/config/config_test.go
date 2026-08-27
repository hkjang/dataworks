package config

import "testing"

func clearOfflineAuthEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"POSTGRES_DSN", "DATABASE_URL", "DB_DSN", "DB_DRIVER",
		"BOOTSTRAP_ADMIN", "BOOTSTRAP_ADMIN_PASSWORD", "ENCRYPTION_KEY",
		"AUTH_ADMIN_BOOTSTRAP_EMAIL", "AUTH_ADMIN_BOOTSTRAP_PASSWORD",
		"AUTH_ENABLED", "AUTH_JWT_SECRET", "GATEWAY_SECRET",
		"SELF_SERVICE_KEYS_ENABLED",
		"LIMITS_MAX_OUTPUT_TOKENS", "LIMITS_AGENT_MAX_TOKENS", "MCP_MAX_TOKENS",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadClampsAllEnvironmentTokenBudgetsToAbsoluteMaximum(t *testing.T) {
	clearOfflineAuthEnv(t)
	t.Setenv("LIMITS_MAX_OUTPUT_TOKENS", "999999")
	t.Setenv("LIMITS_AGENT_MAX_TOKENS", "999999")
	t.Setenv("MCP_MAX_TOKENS", "999999")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Limits.MaxOutputTokens != MaxSupportedOutputTokens || cfg.Limits.AgentMaxTokens != MaxSupportedOutputTokens || cfg.MCP.MaxTokens != MaxSupportedOutputTokens {
		t.Fatalf("token budgets not clamped: limits=%+v mcp=%+v", cfg.Limits, cfg.MCP)
	}
	if CapSupportedOutputTokens(0) != 0 || CapSupportedOutputTokens(4096) != 4096 {
		t.Fatal("token cap must preserve disabled and in-range values")
	}
}

func TestLoadOfflineDeploymentAliases(t *testing.T) {
	clearOfflineAuthEnv(t)
	t.Setenv("POSTGRES_DSN", "postgres://dataworks:secret@postgres:5432/dataworks?sslmode=disable")
	t.Setenv("BOOTSTRAP_ADMIN", "admin@example.local")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "offline-admin-password")
	t.Setenv("ENCRYPTION_KEY", "offline-encryption-key-with-sufficient-entropy")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Driver != "postgres" || cfg.Database.DSN != "postgres://dataworks:secret@postgres:5432/dataworks?sslmode=disable" {
		t.Fatalf("database = %+v", cfg.Database)
	}
	if !cfg.Auth.Enabled {
		t.Fatal("complete BOOTSTRAP_* credentials should enable session auth")
	}
	if cfg.Auth.BootstrapEmail != "admin@example.local" || cfg.Auth.BootstrapPassword != "offline-admin-password" {
		t.Fatalf("bootstrap credentials not loaded: email=%q password=%q", cfg.Auth.BootstrapEmail, cfg.Auth.BootstrapPassword)
	}
	if cfg.Auth.JWTSecret != "offline-encryption-key-with-sufficient-entropy" {
		t.Fatalf("JWT secret = %q, want ENCRYPTION_KEY fallback", cfg.Auth.JWTSecret)
	}
	if cfg.Secret.GatewaySecret != "offline-encryption-key-with-sufficient-entropy" {
		t.Fatalf("gateway secret = %q, want ENCRYPTION_KEY fallback", cfg.Secret.GatewaySecret)
	}
	if !cfg.Auth.SelfServiceKeys {
		t.Fatal("self-service key management should default on with session auth")
	}
	if !cfg.AI.DefaultStream {
		t.Fatal("chat streaming should default on")
	}
}

func TestLoadLegacyAuthEnvironmentStillTakesEffect(t *testing.T) {
	clearOfflineAuthEnv(t)
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_ADMIN_BOOTSTRAP_EMAIL", "legacy@example.local")
	t.Setenv("AUTH_ADMIN_BOOTSTRAP_PASSWORD", "legacy-password")
	t.Setenv("AUTH_JWT_SECRET", "legacy-jwt-secret")
	t.Setenv("GATEWAY_SECRET", "legacy-gateway-secret")
	t.Setenv("SELF_SERVICE_KEYS_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Auth.Enabled || cfg.Auth.BootstrapEmail != "legacy@example.local" || cfg.Auth.BootstrapPassword != "legacy-password" {
		t.Fatalf("legacy auth environment not preserved: %+v", cfg.Auth)
	}
	if cfg.Auth.JWTSecret != "legacy-jwt-secret" || cfg.Secret.GatewaySecret != "legacy-gateway-secret" {
		t.Fatalf("legacy secret precedence not preserved: jwt=%q gateway=%q", cfg.Auth.JWTSecret, cfg.Secret.GatewaySecret)
	}
	if cfg.Auth.SelfServiceKeys {
		t.Fatal("explicit legacy SELF_SERVICE_KEYS_ENABLED=false must be respected")
	}
}

func TestLoadIncompleteBootstrapDoesNotImplicitlyEnableAuth(t *testing.T) {
	clearOfflineAuthEnv(t)
	t.Setenv("BOOTSTRAP_ADMIN", "admin@example.local")
	t.Setenv("ENCRYPTION_KEY", "offline-encryption-key")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Enabled || cfg.Auth.SelfServiceKeys {
		t.Fatalf("incomplete bootstrap must remain disabled: enabled=%v self_service=%v", cfg.Auth.Enabled, cfg.Auth.SelfServiceKeys)
	}
}
