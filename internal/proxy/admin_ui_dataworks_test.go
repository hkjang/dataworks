package proxy

import (
	"strings"
	"testing"
)

func TestAdminUIDataWorksNavigationContract(t *testing.T) {
	required := []string{
		`href="#/dataworks/home" data-tab="dataworks-home"`,
		`href="#/dataworks/actions" data-tab="dataworks-actions"`,
		`href="#/dataworks/assets" data-tab="dataworks-assets"`,
		`href="#/factory" data-tab="factory"`,
		`href="#/dataworks/portfolio" data-tab="dataworks-portfolio"`,
		`href="#/dataworks/risk" data-tab="dataworks-risk"`,
		`href="#/dataworks/analytics" data-tab="dataworks-analytics"`,
		`href="#/dataworks/factory-runs" data-tab="dataworks-factory-runs"`,
		`href="#/dataworks/prompt-registry" data-tab="dataworks-prompt-registry"`,
		`case 'dataworks': await routeDataWorks(rest, params);`,
	}
	for _, fragment := range required {
		if !strings.Contains(adminHTML, fragment) {
			t.Errorf("admin UI missing Data Works navigation contract %q", fragment)
		}
	}
}

func TestAdminUIDataWorksAPIContractKeys(t *testing.T) {
	required := []string{
		`template_body: document.getElementById('pr-content').value`,
		`rule_expression: document.getElementById('pol-body').value`,
		`latency_target_ms: Number(document.getElementById('sla-latency').value)`,
		`success_metric: document.getElementById('poc-crit').value`,
		`/version-diff?from=`,
		`accuracy_score: numericScore`,
	}
	for _, fragment := range required {
		if !strings.Contains(adminHTML, fragment) {
			t.Errorf("admin UI missing Data Works API contract %q", fragment)
		}
	}

	for _, stale := range []string{`schema_readiness_score`, `/version-diff?v1=`, `escapeHTML(r.model_name)`} {
		if strings.Contains(adminHTML, stale) {
			t.Errorf("admin UI still contains stale Data Works API key %q", stale)
		}
	}
}
