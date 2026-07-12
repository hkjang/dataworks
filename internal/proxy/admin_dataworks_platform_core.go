package proxy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	dw "dataworks/internal/dataworks"
	"dataworks/internal/store"
)

func (s *Server) requireDataWorksAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.authorizeAdmin(r) {
		return true
	}
	writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
	return false
}

func platformStableID(prefix, value string) string {
	return prefix + "_" + fmt.Sprintf("%08x", hashString(value))
}

func dataWorksReferenceActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "archived", "retired", "blocked", "disabled", "deleted":
		return false
	default:
		return true
	}
}

func (s *Server) handleDataWorksReferenceCatalog(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	assets, err := s.db.ListDataAssets(r.Context())
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "reference_assets_failed")
		return
	}
	products, err := s.db.ListDataProducts(r.Context(), "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "reference_products_failed")
		return
	}
	workspaces, err := s.db.ListDataWorksWorkspaces(r.Context(), "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "reference_workspaces_failed")
		return
	}
	flows, err := s.db.ListDataWorksFlows(r.Context(), "", "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "reference_flows_failed")
		return
	}
	agents, err := s.db.ListDataWorksAgents(r.Context(), "", "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "reference_agents_failed")
		return
	}
	tools, err := s.db.ListDataWorksTools(r.Context(), "", "", true)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "reference_tools_failed")
		return
	}
	segments, err := s.db.ListCustomerSegments(r.Context(), "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "reference_segments_failed")
		return
	}
	policies, err := s.db.ListPolicyRules(r.Context(), "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "reference_policies_failed")
		return
	}

	activeProducts := products[:0]
	for _, product := range products {
		if dataWorksReferenceActive(product.Status) {
			activeProducts = append(activeProducts, product)
		}
	}
	activeWorkspaces := workspaces[:0]
	for _, workspace := range workspaces {
		if dataWorksReferenceActive(workspace.Status) {
			activeWorkspaces = append(activeWorkspaces, workspace)
		}
	}
	activeFlows := flows[:0]
	for _, flow := range flows {
		if dataWorksReferenceActive(flow.Status) {
			activeFlows = append(activeFlows, flow)
		}
	}
	activeAgents := agents[:0]
	for _, agent := range agents {
		if dataWorksReferenceActive(agent.Status) {
			activeAgents = append(activeAgents, agent)
		}
	}
	activePolicies := policies[:0]
	for _, policy := range policies {
		if policy.Enabled {
			activePolicies = append(activePolicies, policy)
		}
	}

	segmentSource := "registry"
	if len(segments) == 0 {
		segmentSource = "standard"
		segments = []store.CustomerSegment{
			{SegmentKey: "bank", Industry: "금융", BuyerType: "은행", PainPoints: []string{"신용위험", "수익성", "규제 대응"}, BudgetLevel: "enterprise"},
			{SegmentKey: "card", Industry: "금융", BuyerType: "카드", PainPoints: []string{"이상거래", "가맹점 성장", "고객 세분화"}, BudgetLevel: "enterprise"},
			{SegmentKey: "insurance", Industry: "금융", BuyerType: "보험", PainPoints: []string{"손해율", "사기 탐지", "상품 추천"}, BudgetLevel: "enterprise"},
			{SegmentKey: "fintech", Industry: "금융", BuyerType: "핀테크", PainPoints: []string{"대안신용", "API 연계", "빠른 PoC"}, BudgetLevel: "growth"},
			{SegmentKey: "public", Industry: "공공", BuyerType: "공공기관", PainPoints: []string{"정책 효과", "지역경제", "안전한 데이터 결합"}, BudgetLevel: "public"},
		}
	}

	ownerSet := map[string]struct{}{}
	addOwner := func(owner string) {
		owner = strings.TrimSpace(owner)
		if owner != "" {
			ownerSet[owner] = struct{}{}
		}
	}
	for _, owner := range []string{"data-platform", "product-governance", "risk-platform", "compliance"} {
		addOwner(owner)
	}
	for _, asset := range assets {
		addOwner(asset.Owner)
	}
	for _, product := range activeProducts {
		addOwner(product.Owner)
	}
	for _, workspace := range activeWorkspaces {
		addOwner(workspace.Owner)
	}
	for _, flow := range activeFlows {
		addOwner(flow.Owner)
	}
	for _, agent := range activeAgents {
		addOwner(agent.Owner)
	}
	for _, tool := range tools {
		addOwner(tool.Owner)
	}
	owners := make([]string, 0, len(ownerSet))
	for owner := range ownerSet {
		owners = append(owners, owner)
	}
	sort.Strings(owners)

	writeJSON(w, http.StatusOK, map[string]any{
		"assets": assets, "products": activeProducts, "workspaces": activeWorkspaces,
		"flows": activeFlows, "agents": activeAgents, "tools": tools,
		"policies": activePolicies, "customer_segments": segments,
		"customer_segment_source": segmentSource, "owners": owners,
	})
}

func (s *Server) handleDataWorksWorkspaces(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		workspaces, err := s.db.ListDataWorksWorkspaces(r.Context(), strings.TrimSpace(r.URL.Query().Get("status")))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "workspace_list_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"workspaces": workspaces, "count": len(workspaces)})
	case http.MethodPost:
		var in store.DataWorksWorkspace
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if rejectCorruptedCatalogText(w,
			dataWorksCatalogTextField{name: "name", value: in.Name},
			dataWorksCatalogTextField{name: "owner", value: in.Owner},
			dataWorksCatalogTextField{name: "description", value: in.Description}) {
			return
		}
		if in.ID == "" {
			in.ID = newID("dwws")
		}
		in.CreatedBy = adminID(r)
		if err := s.db.UpsertDataWorksWorkspace(r.Context(), in); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "workspace_upsert_failed")
			return
		}
		stored, _, err := s.db.GetDataWorksWorkspace(r.Context(), in.WorkspaceKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "workspace_read_failed")
			return
		}
		_ = s.db.UpsertMetadataEntity(r.Context(), store.MetadataEntity{
			ID: platformStableID("meta", "workspace:"+stored.WorkspaceKey), URN: store.DataWorksURN("workspace", stored.WorkspaceKey),
			WorkspaceID: stored.ID, EntityType: "workspace", Name: stored.Name, Description: stored.Description,
			Owner: stored.Owner, Status: stored.Status, Tags: stored.Tags, SourceRef: stored.ID,
		})
		s.auditAdmin(r, "dataworks.workspace.upsert", "", auditJSON(map[string]any{"workspace_key": stored.WorkspaceKey}))
		writeJSON(w, http.StatusCreated, map[string]any{"workspace": stored})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksWorkspaceByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/admin/dataworks/workspaces/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeOpenAIError(w, http.StatusNotFound, "workspace not found", "invalid_request_error", "not_found")
		return
	}
	workspace, ok, err := s.db.GetDataWorksWorkspace(r.Context(), parts[0])
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "workspace_read_failed")
		return
	}
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "workspace not found", "invalid_request_error", "not_found")
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		members, err := s.db.ListDataWorksWorkspaceMembers(r.Context(), workspace.ID)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "workspace_members_failed")
			return
		}
		entities, _ := s.db.SearchMetadataEntities(r.Context(), "", "", workspace.ID, 100)
		flows, _ := s.db.ListDataWorksFlows(r.Context(), workspace.ID, "")
		agents, _ := s.db.ListDataWorksAgents(r.Context(), workspace.ID, "")
		writeJSON(w, http.StatusOK, map[string]any{
			"workspace": workspace, "members": members, "recent_entities": entities, "flows": flows, "agents": agents,
		})
		return
	}
	if len(parts) == 2 && parts[1] == "members" {
		switch r.Method {
		case http.MethodGet:
			members, err := s.db.ListDataWorksWorkspaceMembers(r.Context(), workspace.ID)
			if err != nil {
				writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "workspace_members_failed")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"members": members})
		case http.MethodPost:
			var member store.DataWorksWorkspaceMember
			if err := json.NewDecoder(r.Body).Decode(&member); err != nil {
				writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
				return
			}
			member.ID = newID("dwwm")
			member.WorkspaceID = workspace.ID
			if err := s.db.UpsertDataWorksWorkspaceMember(r.Context(), member); err != nil {
				writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "workspace_member_failed")
				return
			}
			s.auditAdmin(r, "dataworks.workspace.member.upsert", "", auditJSON(map[string]any{"workspace_id": workspace.ID, "user_id": member.UserID, "role": member.Role}))
			writeJSON(w, http.StatusCreated, map[string]any{"member": member})
		default:
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		}
		return
	}
	writeOpenAIError(w, http.StatusNotFound, "unknown workspace action", "invalid_request_error", "not_found")
}

func (s *Server) handleDataWorksMetadataEntities(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		entities, err := s.db.SearchMetadataEntities(r.Context(), r.URL.Query().Get("q"),
			r.URL.Query().Get("type"), r.URL.Query().Get("workspace_id"), intQuery(r, "limit", 100))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "metadata_list_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"entities": entities, "count": len(entities)})
	case http.MethodPost:
		var entity store.MetadataEntity
		if err := json.NewDecoder(r.Body).Decode(&entity); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if rejectCorruptedCatalogText(w,
			dataWorksCatalogTextField{name: "name", value: entity.Name},
			dataWorksCatalogTextField{name: "owner", value: entity.Owner},
			dataWorksCatalogTextField{name: "description", value: entity.Description}) {
			return
		}
		if entity.ID == "" {
			entity.ID = newID("dwmeta")
		}
		if entity.URN == "" && entity.EntityType != "" && entity.SourceRef != "" {
			entity.URN = store.DataWorksURN(entity.EntityType, entity.SourceRef)
		}
		if err := s.db.UpsertMetadataEntity(r.Context(), entity); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "metadata_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.metadata.entity.upsert", "", auditJSON(map[string]any{"urn": entity.URN, "entity_type": entity.EntityType}))
		writeJSON(w, http.StatusCreated, map[string]any{"entity": entity})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksMetadataEdges(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var edge store.MetadataEdge
	if err := json.NewDecoder(r.Body).Decode(&edge); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if edge.ID == "" {
		edge.ID = newID("dwedge")
	}
	edge.CreatedBy = adminID(r)
	if err := s.db.UpsertMetadataEdge(r.Context(), edge); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "metadata_edge_failed")
		return
	}
	s.auditAdmin(r, "dataworks.metadata.edge.upsert", "", auditJSON(edge))
	writeJSON(w, http.StatusCreated, map[string]any{"edge": edge})
}

func (s *Server) handleDataWorksMetadataSearch(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	if err := s.syncDataWorksMetadata(r.Context()); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "metadata_sync_failed")
		return
	}
	entities, err := s.db.SearchMetadataEntities(r.Context(), r.URL.Query().Get("q"),
		r.URL.Query().Get("type"), r.URL.Query().Get("workspace_id"), intQuery(r, "limit", 100))
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "metadata_search_failed")
		return
	}
	facets := map[string]int{}
	for _, entity := range entities {
		facets[entity.EntityType]++
	}
	writeJSON(w, http.StatusOK, map[string]any{"entities": entities, "facets": facets, "count": len(entities)})
}

func (s *Server) handleDataWorksMetadataByURN(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/admin/dataworks/metadata/"), "/")
	idx := strings.LastIndex(rest, "/")
	if idx < 0 {
		writeOpenAIError(w, http.StatusBadRequest, "lineage or impact action required", "invalid_request_error", "bad_metadata_action")
		return
	}
	urn, err := url.PathUnescape(rest[:idx])
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid metadata URN", "invalid_request_error", "bad_urn")
		return
	}
	action := rest[idx+1:]
	if err := s.syncDataWorksMetadata(r.Context()); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "metadata_sync_failed")
		return
	}
	switch action {
	case "lineage":
		graph, err := s.db.MetadataLineage(r.Context(), urn, intQuery(r, "depth", 4))
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, sql.ErrNoRows) {
				status = http.StatusNotFound
			}
			writeOpenAIError(w, status, err.Error(), "invalid_request_error", "lineage_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"lineage": graph})
	case "impact":
		impact, err := s.db.MetadataImpactAnalysis(r.Context(), urn, intQuery(r, "depth", 4))
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, sql.ErrNoRows) {
				status = http.StatusNotFound
			}
			writeOpenAIError(w, status, err.Error(), "invalid_request_error", "impact_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"impact": impact})
	default:
		writeOpenAIError(w, http.StatusNotFound, "unknown metadata action", "invalid_request_error", "not_found")
	}
}

func (s *Server) handleDataWorksSemanticMetrics(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		metrics, err := s.db.ListSemanticMetrics(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("status"))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "semantic_metric_list_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"metrics": metrics, "count": len(metrics)})
	case http.MethodPost:
		var in struct {
			store.SemanticMetric
			SourceURN string `json:"source_urn"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if rejectCorruptedCatalogText(w,
			dataWorksCatalogTextField{name: "name", value: in.Name},
			dataWorksCatalogTextField{name: "owner", value: in.Owner},
			dataWorksCatalogTextField{name: "description", value: in.Description}) {
			return
		}
		if in.ID == "" {
			in.ID = newID("dwmetric")
		}
		in.CreatedBy = adminID(r)
		if err := s.db.UpsertSemanticMetric(r.Context(), in.SemanticMetric); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "semantic_metric_failed")
			return
		}
		metrics, _ := s.db.ListSemanticMetrics(r.Context(), in.WorkspaceID, "")
		stored := in.SemanticMetric
		for _, metric := range metrics {
			if metric.MetricKey == in.MetricKey {
				stored = metric
				break
			}
		}
		metricURN := store.DataWorksURN("metric", stored.MetricKey)
		_ = s.db.UpsertMetadataEntity(r.Context(), store.MetadataEntity{
			ID: platformStableID("meta", metricURN), URN: metricURN, WorkspaceID: stored.WorkspaceID,
			EntityType: "metric", Name: stored.Name, Description: stored.Description, Owner: stored.Owner,
			Status: stored.Status, SourceRef: stored.MetricKey,
			Properties: map[string]any{"expression": stored.Expression, "aggregation": stored.Aggregation, "version": stored.Version},
		})
		if in.SourceURN != "" {
			_ = s.db.UpsertMetadataEdge(r.Context(), store.MetadataEdge{
				ID: platformStableID("edge", metricURN+"|uses|"+in.SourceURN), WorkspaceID: stored.WorkspaceID,
				SourceURN: metricURN, TargetURN: in.SourceURN, RelationType: "uses", CreatedBy: adminID(r),
			})
		}
		s.auditAdmin(r, "dataworks.semantic.metric.upsert", "", auditJSON(map[string]any{"metric_key": stored.MetricKey, "version": stored.Version}))
		writeJSON(w, http.StatusCreated, map[string]any{"metric": stored})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksContractAssertions(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		assertions, err := s.db.ListDataContractAssertions(r.Context(), r.URL.Query().Get("entity_urn"))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "assertion_list_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"assertions": assertions})
	case http.MethodPost:
		var assertion store.DataContractAssertion
		if err := json.NewDecoder(r.Body).Decode(&assertion); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if assertion.ID == "" {
			assertion.ID = newID("dwassert")
		}
		assertion.CreatedBy = adminID(r)
		if err := s.db.UpsertDataContractAssertion(r.Context(), assertion); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "assertion_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.contract.assertion.upsert", "", auditJSON(assertion))
		writeJSON(w, http.StatusCreated, map[string]any{"assertion": assertion})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksSemanticGlossary(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		terms, err := s.db.ListText2SQLBusinessTerms(r.Context(), r.URL.Query().Get("schema_name"))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "glossary_list_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"terms": terms})
	case http.MethodPost:
		var term store.Text2SQLBusinessTerm
		if err := json.NewDecoder(r.Body).Decode(&term); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if rejectCorruptedCatalogText(w,
			dataWorksCatalogTextField{name: "term", value: term.Term},
			dataWorksCatalogTextField{name: "mapping", value: term.Mapping},
			dataWorksCatalogTextField{name: "description", value: term.Description}) {
			return
		}
		if term.ID == "" {
			term.ID = newID("dwterm")
		}
		if strings.TrimSpace(term.Term) == "" || strings.TrimSpace(term.Mapping) == "" {
			writeOpenAIError(w, http.StatusBadRequest, "term and mapping are required", "invalid_request_error", "glossary_invalid")
			return
		}
		if err := s.db.UpsertText2SQLBusinessTerm(r.Context(), term); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "glossary_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.semantic.glossary.upsert", "", auditJSON(map[string]any{"term": term.Term, "schema_name": term.SchemaName}))
		writeJSON(w, http.StatusCreated, map[string]any{"term": term})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) syncDataWorksMetadata(ctx context.Context) error {
	assets, err := s.db.ListDataAssets(ctx)
	if err != nil {
		return err
	}
	for _, asset := range assets {
		urn := store.DataWorksURN("dataset", asset.AssetKey)
		if err := s.db.UpsertMetadataEntity(ctx, store.MetadataEntity{
			ID: platformStableID("meta", urn), URN: urn, EntityType: "dataset", Name: asset.Name,
			Owner: asset.Owner, Domain: asset.Domain, Sensitivity: asset.Sensitivity,
			Status: "active", SourceRef: asset.AssetKey,
			Properties: map[string]any{"refresh_cycle": asset.RefreshCycle, "columns_summary": asset.ColumnsSummary},
		}); err != nil {
			return err
		}
	}
	products, err := s.db.ListDataProducts(ctx, "")
	if err != nil {
		return err
	}
	for _, product := range products {
		productURN := store.DataWorksURN("product", product.ProductKey)
		if err := s.db.UpsertMetadataEntity(ctx, store.MetadataEntity{
			ID: platformStableID("meta", productURN), URN: productURN, EntityType: "product",
			Name: firstNonEmpty(product.NameKO, product.NameEN, product.ProductKey), Description: product.Description,
			Owner: product.Owner, Status: product.Status, Sensitivity: product.Sensitivity,
			SourceRef: product.ProductKey, Properties: map[string]any{"source_type": product.SourceType, "risk_score": product.RiskScore, "revenue_score": product.RevenueScore},
		}); err != nil {
			return err
		}
		for _, assetKey := range dw.ProductAssetKeys(product) {
			assetURN := store.DataWorksURN("dataset", assetKey)
			_ = s.db.UpsertMetadataEdge(ctx, store.MetadataEdge{
				ID: platformStableID("edge", productURN+"|uses|"+assetURN), SourceURN: productURN,
				TargetURN: assetURN, RelationType: "uses", CreatedBy: "system",
			})
		}
		apiURN := store.DataWorksURN("api", product.ProductKey)
		_ = s.db.UpsertMetadataEntity(ctx, store.MetadataEntity{
			ID: platformStableID("meta", apiURN), URN: apiURN, EntityType: "api",
			Name: firstNonEmpty(product.NameKO, product.ProductKey) + " API", Description: product.Description,
			Owner: product.Owner, Status: product.Status, Sensitivity: product.Sensitivity,
			SourceRef: product.ProductKey, Properties: map[string]any{"endpoint": "/v1/data-products/" + product.ProductKey + "/query"},
		})
		_ = s.db.UpsertMetadataEdge(ctx, store.MetadataEdge{
			ID: platformStableID("edge", productURN+"|exposes|"+apiURN), SourceURN: productURN,
			TargetURN: apiURN, RelationType: "exposes", CreatedBy: "system",
		})
	}
	metrics, _ := s.db.ListSemanticMetrics(ctx, "", "")
	for _, metric := range metrics {
		urn := store.DataWorksURN("metric", metric.MetricKey)
		_ = s.db.UpsertMetadataEntity(ctx, store.MetadataEntity{
			ID: platformStableID("meta", urn), URN: urn, WorkspaceID: metric.WorkspaceID, EntityType: "metric",
			Name: metric.Name, Description: metric.Description, Owner: metric.Owner, Status: metric.Status,
			SourceRef: metric.MetricKey, Properties: map[string]any{"expression": metric.Expression, "version": metric.Version},
		})
	}
	flows, _ := s.db.ListDataWorksFlows(ctx, "", "")
	for _, summary := range flows {
		flow, ok, _ := s.db.GetDataWorksFlow(ctx, summary.ID)
		if !ok {
			continue
		}
		flowURN := store.DataWorksURN("flow", flow.FlowKey)
		_ = s.db.UpsertMetadataEntity(ctx, store.MetadataEntity{
			ID: platformStableID("meta", flowURN), URN: flowURN, WorkspaceID: flow.WorkspaceID,
			EntityType: "flow", Name: flow.Name, Description: flow.Description, Owner: flow.Owner,
			Status: flow.Status, SourceRef: flow.FlowKey, Properties: map[string]any{"flow_type": flow.FlowType, "version": flow.Version},
		})
		for _, node := range flow.Nodes {
			if node.RefURN == "" {
				continue
			}
			relation := "uses"
			if node.NodeType == "output" || node.NodeType == "product_factory" {
				relation = "produces"
			}
			_ = s.db.UpsertMetadataEdge(ctx, store.MetadataEdge{
				ID: platformStableID("edge", flowURN+"|"+relation+"|"+node.RefURN), WorkspaceID: flow.WorkspaceID,
				SourceURN: flowURN, TargetURN: node.RefURN, RelationType: relation, CreatedBy: "system",
			})
		}
	}
	tools, _ := s.db.ListDataWorksTools(ctx, "", "", false)
	for _, tool := range tools {
		toolURN := store.DataWorksURN("tool", tool.ToolKey)
		_ = s.db.UpsertMetadataEntity(ctx, store.MetadataEntity{
			ID: platformStableID("meta", toolURN), URN: toolURN, WorkspaceID: tool.WorkspaceID,
			EntityType: "tool", Name: tool.Name, Description: tool.Description, Owner: tool.Owner,
			Status: map[bool]string{true: "active", false: "disabled"}[tool.Enabled], SourceRef: tool.ToolKey,
			Properties: map[string]any{"tool_type": tool.ToolType, "risk_level": tool.RiskLevel, "server_label": tool.ServerLabel},
		})
	}
	agents, _ := s.db.ListDataWorksAgents(ctx, "", "")
	for _, agent := range agents {
		agentURN := store.DataWorksURN("agent", agent.AgentKey)
		_ = s.db.UpsertMetadataEntity(ctx, store.MetadataEntity{
			ID: platformStableID("meta", agentURN), URN: agentURN, WorkspaceID: agent.WorkspaceID,
			EntityType: "agent", Name: agent.Name, Description: agent.Purpose, Owner: agent.Owner,
			Status: agent.Status, SourceRef: agent.AgentKey, Properties: map[string]any{"risk_level": agent.RiskLevel, "version": agent.Version},
		})
		for _, toolID := range agent.AllowedTools {
			tool, ok, _ := s.db.GetDataWorksTool(ctx, toolID)
			if !ok {
				continue
			}
			toolURN := store.DataWorksURN("tool", tool.ToolKey)
			_ = s.db.UpsertMetadataEdge(ctx, store.MetadataEdge{
				ID: platformStableID("edge", agentURN+"|uses|"+toolURN), WorkspaceID: agent.WorkspaceID,
				SourceURN: agentURN, TargetURN: toolURN, RelationType: "uses", CreatedBy: "system",
			})
		}
	}
	templates, _ := s.db.ListDataWorksPromptTemplates(ctx, "", "", "active", 500)
	seenPrompt := map[string]bool{}
	for _, template := range templates {
		if seenPrompt[template.TemplateKey] {
			continue
		}
		seenPrompt[template.TemplateKey] = true
		urn := store.DataWorksURN("prompt", template.TemplateKey)
		_ = s.db.UpsertMetadataEntity(ctx, store.MetadataEntity{
			ID: platformStableID("meta", urn), URN: urn, EntityType: "prompt", Name: template.TemplateKey,
			Status: template.Status, SourceRef: template.TemplateKey,
			Properties: map[string]any{"run_type": template.RunType, "version": template.Version},
		})
	}
	return nil
}
