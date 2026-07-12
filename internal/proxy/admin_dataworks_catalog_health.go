package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

type dataWorksCatalogTextField struct {
	name  string
	value string
}

type dataWorksCatalogHealthIssue struct {
	EntityType string `json:"entity_type"`
	EntityKey  string `json:"entity_key"`
	Field      string `json:"field"`
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
}

type dataWorksCatalogHealthSummary struct {
	Status        string `json:"status"`
	TotalEntities int    `json:"total_entities"`
	CheckedFields int    `json:"checked_fields"`
	IssueCount    int    `json:"issue_count"`
	CriticalCount int    `json:"critical_count"`
	WarningCount  int    `json:"warning_count"`
}

type dataWorksCatalogHealth struct {
	Summary dataWorksCatalogHealthSummary `json:"summary"`
	Issues  []dataWorksCatalogHealthIssue `json:"issues"`
}

func looksCorruptedCatalogText(value string) bool {
	compact := strings.Join(strings.Fields(value), "")
	questionMarks := strings.Count(compact, "?")
	return questionMarks >= 2 && questionMarks*2 >= utf8.RuneCountInString(compact)
}

func corruptedCatalogTextField(fields ...dataWorksCatalogTextField) string {
	for _, field := range fields {
		if looksCorruptedCatalogText(field.value) {
			return field.name
		}
	}
	return ""
}

func rejectCorruptedCatalogText(w http.ResponseWriter, fields ...dataWorksCatalogTextField) bool {
	field := corruptedCatalogTextField(fields...)
	if field == "" {
		return false
	}
	writeOpenAIError(w, http.StatusBadRequest, field+" contains corrupted text", "invalid_request_error", "invalid_text_encoding")
	return true
}

type dataWorksCatalogHealthCollector struct {
	health dataWorksCatalogHealth
}

func (c *dataWorksCatalogHealthCollector) add(entityType, entityKey string, requireOwner bool, fields ...dataWorksCatalogTextField) {
	c.health.Summary.TotalEntities++
	for _, field := range fields {
		c.health.Summary.CheckedFields++
		value := strings.TrimSpace(field.value)
		if looksCorruptedCatalogText(value) {
			c.health.Issues = append(c.health.Issues, dataWorksCatalogHealthIssue{
				EntityType: entityType, EntityKey: entityKey, Field: field.name,
				Code: "corrupted_text", Severity: "critical", Message: "문자 인코딩이 손상된 값입니다.",
			})
			continue
		}
		if field.name == "name" && value == "" {
			c.health.Issues = append(c.health.Issues, dataWorksCatalogHealthIssue{
				EntityType: entityType, EntityKey: entityKey, Field: field.name,
				Code: "missing_name", Severity: "warning", Message: "표시 이름이 없습니다.",
			})
		}
		if requireOwner && field.name == "owner" && value == "" {
			c.health.Issues = append(c.health.Issues, dataWorksCatalogHealthIssue{
				EntityType: entityType, EntityKey: entityKey, Field: field.name,
				Code: "missing_owner", Severity: "warning", Message: "책임 오너가 지정되지 않았습니다.",
			})
		}
	}
}

func (c *dataWorksCatalogHealthCollector) result() dataWorksCatalogHealth {
	c.health.Summary.Status = "healthy"
	c.health.Summary.IssueCount = len(c.health.Issues)
	for _, issue := range c.health.Issues {
		if issue.Severity == "critical" {
			c.health.Summary.CriticalCount++
		} else {
			c.health.Summary.WarningCount++
		}
	}
	if c.health.Summary.CriticalCount > 0 {
		c.health.Summary.Status = "critical"
	} else if c.health.Summary.WarningCount > 0 {
		c.health.Summary.Status = "attention"
	}
	if c.health.Issues == nil {
		c.health.Issues = []dataWorksCatalogHealthIssue{}
	}
	return c.health
}

func (s *Server) buildDataWorksCatalogHealth(ctx context.Context) (dataWorksCatalogHealth, error) {
	collector := &dataWorksCatalogHealthCollector{}

	assets, err := s.db.ListDataAssets(ctx)
	if err != nil {
		return dataWorksCatalogHealth{}, fmt.Errorf("list assets: %w", err)
	}
	for _, item := range assets {
		collector.add("asset", item.AssetKey, true,
			dataWorksCatalogTextField{name: "name", value: item.Name},
			dataWorksCatalogTextField{name: "owner", value: item.Owner})
	}

	products, err := s.db.ListDataProducts(ctx, "")
	if err != nil {
		return dataWorksCatalogHealth{}, fmt.Errorf("list products: %w", err)
	}
	for _, item := range products {
		collector.add("product", item.ProductKey, true,
			dataWorksCatalogTextField{name: "name", value: firstNonEmpty(item.NameKO, item.NameEN)},
			dataWorksCatalogTextField{name: "owner", value: item.Owner},
			dataWorksCatalogTextField{name: "description", value: item.Description})
	}

	workspaces, err := s.db.ListDataWorksWorkspaces(ctx, "")
	if err != nil {
		return dataWorksCatalogHealth{}, fmt.Errorf("list workspaces: %w", err)
	}
	for _, item := range workspaces {
		collector.add("workspace", item.WorkspaceKey, true,
			dataWorksCatalogTextField{name: "name", value: item.Name},
			dataWorksCatalogTextField{name: "owner", value: item.Owner},
			dataWorksCatalogTextField{name: "description", value: item.Description})
	}

	flows, err := s.db.ListDataWorksFlows(ctx, "", "")
	if err != nil {
		return dataWorksCatalogHealth{}, fmt.Errorf("list flows: %w", err)
	}
	for _, item := range flows {
		collector.add("flow", item.FlowKey, true,
			dataWorksCatalogTextField{name: "name", value: item.Name},
			dataWorksCatalogTextField{name: "owner", value: item.Owner},
			dataWorksCatalogTextField{name: "description", value: item.Description})
	}

	agents, err := s.db.ListDataWorksAgents(ctx, "", "")
	if err != nil {
		return dataWorksCatalogHealth{}, fmt.Errorf("list agents: %w", err)
	}
	for _, item := range agents {
		collector.add("agent", item.AgentKey, true,
			dataWorksCatalogTextField{name: "name", value: item.Name},
			dataWorksCatalogTextField{name: "owner", value: item.Owner},
			dataWorksCatalogTextField{name: "description", value: item.Purpose})
	}

	tools, err := s.db.ListDataWorksTools(ctx, "", "", false)
	if err != nil {
		return dataWorksCatalogHealth{}, fmt.Errorf("list tools: %w", err)
	}
	for _, item := range tools {
		collector.add("tool", item.ToolKey, true,
			dataWorksCatalogTextField{name: "name", value: item.Name},
			dataWorksCatalogTextField{name: "owner", value: item.Owner},
			dataWorksCatalogTextField{name: "description", value: item.Description})
	}

	segments, err := s.db.ListCustomerSegments(ctx, "")
	if err != nil {
		return dataWorksCatalogHealth{}, fmt.Errorf("list customer segments: %w", err)
	}
	for _, item := range segments {
		collector.add("customer_segment", item.SegmentKey, false,
			dataWorksCatalogTextField{name: "name", value: item.BuyerType},
			dataWorksCatalogTextField{name: "industry", value: item.Industry})
	}

	metrics, err := s.db.ListSemanticMetrics(ctx, "", "")
	if err != nil {
		return dataWorksCatalogHealth{}, fmt.Errorf("list semantic metrics: %w", err)
	}
	for _, item := range metrics {
		collector.add("metric", item.MetricKey, true,
			dataWorksCatalogTextField{name: "name", value: item.Name},
			dataWorksCatalogTextField{name: "owner", value: item.Owner},
			dataWorksCatalogTextField{name: "description", value: item.Description})
	}

	return collector.result(), nil
}

func (s *Server) handleDataWorksCatalogHealth(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	health, err := s.buildDataWorksCatalogHealth(r.Context())
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "catalog_health_failed")
		return
	}
	writeJSON(w, http.StatusOK, health)
}
