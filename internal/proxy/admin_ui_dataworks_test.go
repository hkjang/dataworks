package proxy

import (
	"strings"
	"testing"
)

func TestAdminUIDataWorksNavigationContract(t *testing.T) {
	required := []string{
		`href="#/dataworks/home" data-tab="dataworks-home"`,
		`href="#/dataworks/workspaces" data-tab="dataworks-workspaces"`,
		`href="#/dataworks/metadata" data-tab="dataworks-metadata"`,
		`href="#/dataworks/flows" data-tab="dataworks-flows"`,
		`href="#/dataworks/agents" data-tab="dataworks-agents"`,
		`href="#/dataworks/tools" data-tab="dataworks-tools"`,
		`href="#/dataworks/policy-simulator" data-tab="dataworks-policy-simulator"`,
		`href="#/dataworks/synthetic" data-tab="dataworks-synthetic"`,
		`href="#/dataworks/marketplace" data-tab="dataworks-marketplace"`,
		`href="#/dataworks/agentops" data-tab="dataworks-agentops"`,
		`href="#/dataworks/executive" data-tab="dataworks-executive"`,
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
		`/admin/dataworks/platform/overview`,
		`/admin/dataworks/reference-catalog`,
		`/admin/dataworks/metadata/search`,
		`/admin/dataworks/semantic/metrics`,
		`/admin/dataworks/flows`,
		`/admin/dataworks/agents`,
		`/admin/dataworks/tools`,
		`/admin/dataworks/policies/simulate`,
		`/admin/dataworks/synthetic/generate`,
		`/admin/dataworks/marketplace/items`,
		`/admin/dataworks/executive/simulate`,
		`template_body: document.getElementById('pr-content').value`,
		`rule_expression: document.getElementById('pol-body').value`,
		`latency_target_ms: Number(document.getElementById('sla-latency').value)`,
		`success_metric: document.getElementById('poc-crit').value`,
		`/version-diff?from=`,
		`accuracy_score: numericScore`,
		`class="reference-preview"`,
		`id="factory-form"`,
		`class="studio-shell"`,
		`id="dw-flow-canvas"`,
		`id="dw-agent-console-select"`,
		`정책 시뮬레이션 센터`,
		`수익성 및 경영 시뮬레이터`,
		`AI 상품 아이디어 미리보기`,
	}
	for _, fragment := range required {
		if !strings.Contains(adminHTML, fragment) {
			t.Errorf("admin UI missing Data Works API contract %q", fragment)
		}
	}

	for _, stale := range []string{
		`schema_readiness_score`, `/version-diff?v1=`, `escapeHTML(r.model_name)`,
		`id="dwp-id" placeholder=`, `id="dwex-products" placeholder=`, `prompt('Usage purpose')`,
	} {
		if strings.Contains(adminHTML, stale) {
			t.Errorf("admin UI still contains stale Data Works API key %q", stale)
		}
	}
}

func TestAdminUIDataWorksFieldComponentContract(t *testing.T) {
	required := []string{
		`--control-height: 38px;`,
		`--form-row-gap: 18px;`,
		`class="field-label-text"`,
		`class="field-control"`,
		`class="field-error" aria-live="polite"`,
		`function standardizeFormComponents(root)`,
		`standardizeFormComponents(document.getElementById('view'))`,
		`standardizeFormComponents(document.getElementById('modal-body'))`,
	}
	for _, fragment := range required {
		if !strings.Contains(adminHTML, fragment) {
			t.Errorf("admin UI missing field component contract %q", fragment)
		}
	}

	for _, stale := range []string{
		`<div><label>Template Key</label>`,
		`<div><label>Rule ID</label>`,
		`<div class="canvas-block"><label>`,
	} {
		if strings.Contains(adminHTML, stale) {
			t.Errorf("admin UI still contains unstructured field markup %q", stale)
		}
	}
}

func TestAdminUIDataWorksTextComponentContract(t *testing.T) {
	required := []string{
		`class="section-title-text"`,
		`class="section-intro-text"`,
		`class="kpi-label-text"`,
		`class="kpi-value-text"`,
		`class="kv-label-text"`,
		`class="kv-value-text"`,
		`class="entity-title-text"`,
		`class="entity-summary-text"`,
		`class="message-title-text"`,
		`class="message-text"`,
		`function sectionIntro(text)`,
		`function standardizeTextComponents(root)`,
		`.section-intro {`,
		`padding:10px 14px`,
		`.kv .k, .kv .v {`,
		`min-height:40px`,
		`class="twin-detail-body"`,
		`운영자가 즉시 후속 조치해야 할 상품화 경고 및 대기 작업`,
	}
	for _, fragment := range required {
		if !strings.Contains(adminHTML, fragment) {
			t.Errorf("admin UI missing text component contract %q", fragment)
		}
	}

	for _, stale := range []string{
		`<p class="muted" style="margin:-4px 0 12px">운영자가 즉시 후속 조치해야 할 상품화 경고 및 대기 작업</p>`,
		`style="margin:-4px 0 12px"`,
		`return '<div class="k">' + escapeHTML(k) + '</div><div class="v">' + v + '</div>';`,
		`<h2 style="margin:0;font-size:20px">' + escapeHTML(product.name_ko`,
	} {
		if strings.Contains(adminHTML, stale) {
			t.Errorf("admin UI still contains unstructured text markup %q", stale)
		}
	}
}
