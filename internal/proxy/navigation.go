package proxy

import (
	"net/http"
	"strings"
)

// menuVersion is bumped whenever the menu registry or its access rules change, so the
// SPA can detect a stale navigation and refresh /me/navigation without a full reload.
const menuVersion = 23

// menuItem is one navigable destination in the admin SPA. Access is decided server-side
// from the caller's scopes + enabled feature flags — the same registry drives both the
// rendered menu (/me/navigation) and the SPA's route guard, so hiding a menu and blocking
// its route can never drift apart.
type menuItem struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Path      string   `json:"path"`            // hash route, e.g. "#/dashboard"
	Tab       string   `json:"tab"`             // SPA data-tab value
	Group     string   `json:"group"`           // me | ops | security | settings
	Scopes    []string `json:"required_scopes"` // any-of; empty = any authenticated user
	Features  []string `json:"required_features"`
	DataScope string   `json:"data_scope"` // self | team | all
}

// menuRegistry is the single source of truth for navigation. Order = display order.
var menuRegistry = []menuItem{
	// Data Works product operations (admin:read), with risk review scoped separately.
	{ID: "dataworks.home", Label: "Home", Path: "#/dataworks/home", Tab: "dataworks-home", Group: "dataworks", Scopes: []string{"admin:read"}, DataScope: "all"},
	{ID: "dataworks.actions", Label: "Action Center", Path: "#/dataworks/actions", Tab: "dataworks-actions", Group: "dataworks", Scopes: []string{"admin:read"}, DataScope: "all"},
	{ID: "dataworks.assets", Label: "데이터 자산", Path: "#/dataworks/assets", Tab: "dataworks-assets", Group: "dataworks", Scopes: []string{"admin:read"}, DataScope: "all"},
	{ID: "dataworks.factory", Label: "Product Factory", Path: "#/factory", Tab: "factory", Group: "dataworks", Scopes: []string{"admin:read"}, DataScope: "all"},
	{ID: "dataworks.portfolio", Label: "포트폴리오", Path: "#/dataworks/portfolio", Tab: "dataworks-portfolio", Group: "dataworks", Scopes: []string{"admin:read"}, DataScope: "all"},
	{ID: "dataworks.risk", Label: "리스크 검토", Path: "#/dataworks/risk", Tab: "dataworks-risk", Group: "security", Scopes: []string{"security:read"}, DataScope: "all"},
	{ID: "dataworks.analytics", Label: "성과 분석", Path: "#/dataworks/analytics", Tab: "dataworks-analytics", Group: "dataworks", Scopes: []string{"admin:read"}, DataScope: "all"},
	{ID: "dataworks.factory_runs", Label: "AI 실행 이력", Path: "#/dataworks/factory-runs", Tab: "dataworks-factory-runs", Group: "settings", Scopes: []string{"admin:read"}, DataScope: "all"},
	{ID: "dataworks.prompt_registry", Label: "프롬프트 레지스트리", Path: "#/dataworks/prompt-registry", Tab: "dataworks-prompt-registry", Group: "settings", Scopes: []string{"admin:read"}, DataScope: "all"},
	// Legacy K8s routes remain compiled but are hidden unless k8s_ops is explicitly enabled.
	{ID: "ops.k8s_home", Label: "운영 홈", Path: "#/k8s-home", Tab: "k8s-home", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s", Label: "클러스터", Path: "#/k8s", Tab: "k8s", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_collector", Label: "수집 상태", Path: "#/k8s-collector", Tab: "k8s-collector", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_pods", Label: "Pod 관리", Path: "#/k8s-pods", Tab: "k8s-pods", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_stacks", Label: "앱 배포", Path: "#/k8s-stacks", Tab: "k8s-stacks", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_timeline", Label: "변경 타임라인", Path: "#/k8s-timeline", Tab: "k8s-timeline", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_rca", Label: "장애 분석", Path: "#/k8s-rca", Tab: "k8s-rca", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_incidents", Label: "장애 워룸", Path: "#/k8s-incidents", Tab: "k8s-incidents", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_graph", Label: "리소스 그래프", Path: "#/k8s-graph", Tab: "k8s-graph", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_conn", Label: "연결성 점검", Path: "#/k8s-conn", Tab: "k8s-conn", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_actions", Label: "액션 승인함", Path: "#/k8s-actions", Tab: "k8s-actions", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_capacity", Label: "용량·자동확장", Path: "#/k8s-capacity", Tab: "k8s-capacity", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_meta", Label: "그룹·오너십", Path: "#/k8s-meta", Tab: "k8s-meta", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_ai", Label: "AI 분석", Path: "#/k8s-ai", Tab: "k8s-ai", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_reports", Label: "리포트 센터", Path: "#/k8s-reports", Tab: "k8s-reports", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "ops.k8s_slo", Label: "SLO 센터", Path: "#/k8s-slo", Tab: "k8s-slo", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "bill.k8s_cost", Label: "비용", Path: "#/k8s-cost", Tab: "k8s-cost", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "sec.k8s_security", Label: "K8s 보안", Path: "#/k8s-security", Tab: "k8s-security", Group: "legacy-k8s", Scopes: []string{"security:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "sec.k8s_policy", Label: "K8s 정책 센터", Path: "#/k8s-policy", Tab: "k8s-policy", Group: "legacy-k8s", Scopes: []string{"security:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "set.k8s_settings", Label: "운영 설정", Path: "#/k8s-settings", Tab: "k8s-settings", Group: "legacy-k8s", Scopes: []string{"admin:read"}, Features: []string{"k8s_ops"}, DataScope: "all"},
	{ID: "set.settings", Label: "설정", Path: "#/settings", Tab: "settings", Group: "settings", Scopes: []string{"admin:read"}, DataScope: "all"},
}

// childTabs maps a parent tab to the nested route tabs that share its permission. The
// route guard treats a child as accessible exactly when its parent menu is accessible.
var childTabs = map[string][]string{
	"factory":             {"data-products", "text2sql", "dataworks-factory", "dataworks-products"},
	"dataworks-risk":      {"security", "safety", "skills", "skill-studio", "modeldeprecations"},
	"dataworks-analytics": {"dwdashboard", "clickhouse", "dwmetrics"},
	"settings":            {"runtimesettings", "errors", "changesets"},
}

// featureFlags reports which optional features are enabled, for both /auth/me and menu
// gating. personal_home is always on (it is this feature); team_dashboard is reserved.
func (s *Server) featureFlags() map[string]bool {
	return map[string]bool{
		"self_service_keys": s.cfg.Auth.SelfServiceKeys,
		"personal_home":     true,
		"team_dashboard":    false,
		"k8s_ops":           false,
	}
}

// menuAccessible reports whether a caller with the given scopes/features may see an item.
func menuAccessible(item menuItem, scopes []string, features map[string]bool) bool {
	for _, f := range item.Features {
		if !features[f] {
			return false
		}
	}
	if len(item.Scopes) == 0 {
		return true // any authenticated user
	}
	for _, want := range item.Scopes {
		if hasScope(scopes, want) {
			return true
		}
	}
	return false
}

// menuDecision returns whether a menu is allowed for the caller and a human reason — the
// data behind /permissions/effective so an operator can see exactly why a menu is hidden.
func menuDecision(item menuItem, scopes []string, features map[string]bool) (bool, string) {
	for _, f := range item.Features {
		if !features[f] {
			return false, "feature '" + f + "' disabled"
		}
	}
	if len(item.Scopes) == 0 {
		return true, "any authenticated user"
	}
	for _, want := range item.Scopes {
		if hasScope(scopes, want) {
			return true, "has scope '" + want + "'"
		}
	}
	return false, "missing any of scopes: " + strings.Join(item.Scopes, ", ")
}

// accessibleMenus returns the registry filtered to what the caller may see.
func accessibleMenus(scopes []string, features map[string]bool) []menuItem {
	out := make([]menuItem, 0, len(menuRegistry))
	for _, item := range menuRegistry {
		if menuAccessible(item, scopes, features) {
			out = append(out, item)
		}
	}
	return out
}

// allowedTabs is the flat set of SPA tabs the caller may route to: each accessible menu's
// tab plus that tab's nested children. Drives the SPA route guard.
func allowedTabs(scopes []string, features map[string]bool) []string {
	tabs := []string{}
	seen := map[string]bool{}
	add := func(t string) {
		if t != "" && !seen[t] {
			seen[t] = true
			tabs = append(tabs, t)
		}
	}
	for _, item := range accessibleMenus(scopes, features) {
		add(item.Tab)
		for _, c := range childTabs[item.Tab] {
			add(c)
		}
	}
	return tabs
}

// roleHomeOverride pins specific built-in roles to a role-tailored landing that scope
// alone can't distinguish (e.g. security_admin and readonly_admin both hold admin:read +
// security:read, but the former lands on the security dashboard).
var roleHomeOverride = map[string]string{
	"security_admin": "#/dataworks/risk",
}

// resolveDefaultHome picks the landing route from scopes alone: operators (admin:read) →
// operational dashboard; team managers (team:read, no admin:read) → team dashboard; else
// the personalized home.
func resolveDefaultHome(scopes []string) string {
	if hasScope(scopes, "admin:read") {
		return "#/dataworks/home"
	}
	if hasScope(scopes, "security:read") {
		return "#/dataworks/risk"
	}
	return "#/factory"
}

// resolveHome is the role-aware landing: a per-role override wins, otherwise scope-based.
func resolveHome(role string, scopes []string) string {
	if h := roleHomeOverride[strings.TrimSpace(role)]; h != "" {
		return h
	}
	return resolveDefaultHome(scopes)
}

// navigationFor builds the full navigation payload for a caller's scopes/features.
func (s *Server) navigationFor(scopes []string, role string) map[string]any {
	features := s.featureFlags()
	return map[string]any{
		"menus":        accessibleMenus(scopes, features),
		"allowed_tabs": allowedTabs(scopes, features),
		"default_home": resolveHome(role, scopes),
		"role":         role,
		"scopes":       scopes,
		"features":     features,
		"menu_version": menuVersion,
	}
}

// handleMeNavigation returns the caller's accessible menu set, computed server-side. The
// SPA renders only these items and guards routes against allowed_tabs, so menu hiding and
// route blocking share one policy. In legacy mode (auth disabled) the full operator menu
// is returned, matching the admin-token surface.
func (s *Server) handleMeNavigation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	if !s.cfg.Auth.Enabled {
		writeJSON(w, http.StatusOK, s.navigationFor(append([]string{}, allScopes...), "admin"))
		return
	}
	claims, ok := s.currentAccessClaims(r)
	if !ok {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid access token", "invalid_request_error", "invalid_access_token")
		return
	}
	writeJSON(w, http.StatusOK, s.navigationFor(claims.Scopes, claims.Role))
}
