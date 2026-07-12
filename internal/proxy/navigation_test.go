package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"dataworks/internal/store"
)

func tabSet(scopes []string, features map[string]bool) map[string]bool {
	set := map[string]bool{}
	for _, t := range allowedTabs(scopes, features) {
		set[t] = true
	}
	return set
}

func TestResolveDefaultHome(t *testing.T) {
	// Data Works operators land on the control tower. Roles without admin access retain the
	// legacy factory fallback, which the SPA can replace with a role-specific home.
	// A security-only role would land on the risk center; built-in security_admin has admin:read,
	// so resolveHome applies its role override separately.
	cases := []struct {
		role string
		want string
	}{
		{"admin", "#/dataworks/home"},
		{"viewer", "#/dataworks/home"},
		{"readonly_admin", "#/dataworks/home"},
		{"security_admin", "#/dataworks/home"},
		{"developer", "#/factory"},
		{"service_account", "#/factory"},
	}
	for _, c := range cases {
		if got := resolveDefaultHome(roleScopes[c.role]); got != c.want {
			t.Errorf("resolveDefaultHome(%s) = %q, want %q", c.role, got, c.want)
		}
	}
}

func TestAccessibleMenusByRole(t *testing.T) {
	features := map[string]bool{}

	// developer: no admin:read/security:read -> sees no Data Works or K8s menu.
	devTabs := tabSet(roleScopes["developer"], features)
	for _, forbidden := range []string{"dataworks-home", "dataworks-actions", "dataworks-assets", "factory", "data-products", "k8s-home", "k8s", "k8s-security", "settings"} {
		if devTabs[forbidden] {
			t.Errorf("developer must NOT see %q", forbidden)
		}
	}

	// ai_admin: admin:read but NOT security:read -> product operations + settings, but no risk center.
	aiTabs := tabSet(roleScopes["ai_admin"], features)
	for _, want := range []string{"dataworks-home", "dataworks-actions", "dataworks-assets", "factory", "dataworks-portfolio", "dataworks-analytics", "dataworks-factory-runs", "dataworks-prompt-registry", "data-products", "text2sql", "dwdashboard", "settings"} {
		if !aiTabs[want] {
			t.Errorf("ai_admin should see %q", want)
		}
	}
	if aiTabs["dataworks-risk"] || aiTabs["security"] {
		t.Error("ai_admin lacks security:read -> must NOT see risk routes")
	}

	// admin: all Data Works factory/risk areas + nested settings children; K8s remains feature-gated off.
	adminTabs := tabSet(roleScopes["admin"], features)
	for _, want := range []string{"dataworks-home", "dataworks-actions", "dataworks-assets", "factory", "dataworks-portfolio", "dataworks-risk", "dataworks-analytics", "dataworks-factory-runs", "dataworks-prompt-registry", "data-products", "text2sql", "dwdashboard", "security", "safety", "settings"} {
		if !adminTabs[want] {
			t.Errorf("admin should see %q", want)
		}
	}
	for _, forbidden := range []string{"k8s-home", "k8s-timeline", "k8s-security"} {
		if adminTabs[forbidden] {
			t.Errorf("admin should not see feature-gated legacy tab %q", forbidden)
		}
	}
	if !adminTabs["runtimesettings"] {
		t.Error("admin allowed_tabs should include nested settings children (runtimesettings)")
	}
}

func TestMeNavigationLegacyModeReturnsFullMenu(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "nav.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/me/navigation")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /me/navigation = %d", resp.StatusCode)
	}
	var nav struct {
		Menus       []menuItem `json:"menus"`
		AllowedTabs []string   `json:"allowed_tabs"`
		DefaultHome string     `json:"default_home"`
		MenuVersion int        `json:"menu_version"`
	}
	json.NewDecoder(resp.Body).Decode(&nav)
	resp.Body.Close()
	// Legacy (auth disabled) = admin-equivalent: Data Works menu with feature-gated K8s hidden.
	if nav.DefaultHome != "#/dataworks/home" {
		t.Errorf("legacy default_home = %q, want #/dataworks/home", nav.DefaultHome)
	}
	if len(nav.Menus) != 10 {
		t.Errorf("legacy mode should expose 10 Data Works menus, got %d", len(nav.Menus))
	}
	tabs := map[string]bool{}
	for _, tb := range nav.AllowedTabs {
		tabs[tb] = true
	}
	for _, want := range []string{"dataworks-home", "dataworks-actions", "dataworks-assets", "factory", "dataworks-portfolio", "dataworks-risk", "dataworks-analytics", "dataworks-factory-runs", "dataworks-prompt-registry", "data-products", "text2sql", "dwdashboard", "security", "settings", "runtimesettings"} {
		if !tabs[want] {
			t.Errorf("legacy allowed_tabs missing %q", want)
		}
	}
}

func TestRoleCatalog(t *testing.T) {
	cat := roleCatalog()
	if len(cat) != len(roleScopes) {
		t.Fatalf("catalog should list all %d roles, got %d", len(roleScopes), len(cat))
	}
	byRole := map[string]roleInfo{}
	for _, c := range cat {
		byRole[c.Role] = c
	}
	if !byRole["admin"].IsAdmin || byRole["admin"].DefaultHome != "#/dataworks/home" {
		t.Errorf("admin should be is_admin with Data Works home: %+v", byRole["admin"])
	}
	if byRole["developer"].IsAdmin || byRole["developer"].DefaultHome != "#/factory" {
		t.Errorf("developer should be non-admin with factory fallback home: %+v", byRole["developer"])
	}
	// Highest rank first.
	if cat[0].Rank < cat[len(cat)-1].Rank {
		t.Errorf("catalog should be ranked high→low, got %d..%d", cat[0].Rank, cat[len(cat)-1].Rank)
	}
}

func TestPermissionsEffectiveLegacyMode(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "perm.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/permissions/effective")
	var eff struct {
		Role    string `json:"role"`
		IsAdmin bool   `json:"is_admin"`
		Menus   []struct {
			ID      string `json:"id"`
			Allowed bool   `json:"allowed"`
			Reason  string `json:"reason"`
		} `json:"menus"`
	}
	json.NewDecoder(resp.Body).Decode(&eff)
	resp.Body.Close()
	if !eff.IsAdmin {
		t.Errorf("legacy mode should be admin-equivalent, got role=%q", eff.Role)
	}
	// Every menu carries an allow/deny reason.
	for _, m := range eff.Menus {
		if m.Reason == "" {
			t.Errorf("menu %q missing decision reason", m.ID)
		}
	}
}

func TestTeamManagerNavigation(t *testing.T) {
	features := map[string]bool{}
	scopes := roleScopes["team_manager"]
	// team_manager has neither admin:read nor security:read → lands on the factory fallback
	// but sees no admin menu.
	if got := resolveDefaultHome(scopes); got != "#/factory" {
		t.Errorf("team_manager default_home = %q, want #/factory", got)
	}
	tabs := tabSet(scopes, features)
	for _, forbidden := range []string{"factory", "data-products", "k8s-home", "k8s", "settings", "security"} {
		if tabs[forbidden] {
			t.Errorf("team_manager must NOT see %q", forbidden)
		}
	}
}

func TestRoleHomeOverrides(t *testing.T) {
	features := map[string]bool{}
	// security_admin keeps a tailored landing; admin-scoped roles use the control tower.
	if got := resolveHome("security_admin", roleScopes["security_admin"]); got != "#/dataworks/risk" {
		t.Errorf("security_admin home = %q, want #/dataworks/risk", got)
	}
	for _, role := range []string{"admin", "readonly_admin", "billing_admin"} {
		if got := resolveHome(role, roleScopes[role]); got != "#/dataworks/home" {
			t.Errorf("resolveHome(%s) = %q, want #/dataworks/home", role, got)
		}
	}
	// security_admin sees the security tab; billing_admin (no security:read) does not.
	if !tabSet(roleScopes["security_admin"], features)["dataworks-risk"] {
		t.Error("security_admin should see Data Works risk review")
	}
	if tabSet(roleScopes["billing_admin"], features)["dataworks-risk"] {
		t.Error("billing_admin should not see Data Works risk review")
	}
}

func TestRedactPromptDetails(t *testing.T) {
	prompts := []store.PromptDetail{
		{Role: "user", ContentText: "secret original text", RedactedText: "[redacted]"},
		{Role: "system", ContentText: "same", RedactedText: "same"},
		{Role: "user", ContentText: "", RedactedText: "x"},
	}
	redactPromptDetails(prompts)
	if prompts[0].ContentText != "[redacted]" {
		t.Errorf("raw content should be collapsed to redacted, got %q", prompts[0].ContentText)
	}
	if prompts[1].ContentText != "same" {
		t.Errorf("already-equal content untouched, got %q", prompts[1].ContentText)
	}
	// rawPromptViewerRoles: only full admins + security_admin.
	for _, role := range []string{"admin", "super_admin", "security_admin"} {
		if !rawPromptViewerRoles[role] {
			t.Errorf("%s should be allowed to view raw prompts", role)
		}
	}
	for _, role := range []string{"viewer", "readonly_admin", "ops_admin", "ai_admin", "team_admin", "team_manager", "developer"} {
		if rawPromptViewerRoles[role] {
			t.Errorf("%s must NOT view raw prompts", role)
		}
	}
}
