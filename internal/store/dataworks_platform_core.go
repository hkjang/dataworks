package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type DataWorksWorkspace struct {
	ID           string   `json:"id"`
	WorkspaceKey string   `json:"workspace_key"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Owner        string   `json:"owner"`
	Status       string   `json:"status"`
	Environment  string   `json:"environment"`
	Tags         []string `json:"tags"`
	CreatedBy    string   `json:"created_by"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
	MemberCount  int      `json:"member_count"`
	EntityCount  int      `json:"entity_count"`
	ProductCount int      `json:"product_count"`
	FlowCount    int      `json:"flow_count"`
	AgentCount   int      `json:"agent_count"`
	MonthlyCost  float64  `json:"monthly_cost"`
}

type DataWorksWorkspaceMember struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type MetadataEntity struct {
	ID          string         `json:"id"`
	URN         string         `json:"urn"`
	WorkspaceID string         `json:"workspace_id"`
	EntityType  string         `json:"entity_type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Owner       string         `json:"owner"`
	Domain      string         `json:"domain"`
	Status      string         `json:"status"`
	Sensitivity string         `json:"sensitivity"`
	Tags        []string       `json:"tags"`
	Properties  map[string]any `json:"properties"`
	SourceRef   string         `json:"source_ref"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

type MetadataEdge struct {
	ID           string         `json:"id"`
	WorkspaceID  string         `json:"workspace_id"`
	SourceURN    string         `json:"source_urn"`
	TargetURN    string         `json:"target_urn"`
	RelationType string         `json:"relation_type"`
	Metadata     map[string]any `json:"metadata"`
	CreatedBy    string         `json:"created_by"`
	CreatedAt    string         `json:"created_at"`
}

type MetadataGraph struct {
	RootURN string           `json:"root_urn"`
	Nodes   []MetadataEntity `json:"nodes"`
	Edges   []MetadataEdge   `json:"edges"`
}

type MetadataImpact struct {
	MetadataGraph
	RiskScore        int            `json:"risk_score"`
	AffectedByType   map[string]int `json:"affected_by_type"`
	ApprovalRequired bool           `json:"approval_required"`
}

type SemanticMetric struct {
	ID              string         `json:"id"`
	MetricKey       string         `json:"metric_key"`
	WorkspaceID     string         `json:"workspace_id"`
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	Expression      string         `json:"expression"`
	Aggregation     string         `json:"aggregation"`
	Dimensions      []string       `json:"dimensions"`
	Filters         map[string]any `json:"filters"`
	CustomerSegment string         `json:"customer_segment"`
	ProductGroup    string         `json:"product_group"`
	Owner           string         `json:"owner"`
	Status          string         `json:"status"`
	Version         int            `json:"version"`
	CreatedBy       string         `json:"created_by"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
}

type DataContractAssertion struct {
	ID            string `json:"id"`
	EntityURN     string `json:"entity_urn"`
	AssertionType string `json:"assertion_type"`
	FieldName     string `json:"field_name"`
	Operator      string `json:"operator"`
	ExpectedValue string `json:"expected_value"`
	Severity      string `json:"severity"`
	Enabled       bool   `json:"enabled"`
	CreatedBy     string `json:"created_by"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func platformJSON(v any, fallback string) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return fallback
	}
	return string(b)
}

func platformStringList(raw string) []string {
	out := []string{}
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		return []string{}
	}
	return out
}

func platformMap(raw string) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func (s *SQLStore) UpsertDataWorksWorkspace(ctx context.Context, w DataWorksWorkspace) error {
	w.ID = strings.TrimSpace(w.ID)
	w.WorkspaceKey = strings.TrimSpace(w.WorkspaceKey)
	w.Name = strings.TrimSpace(w.Name)
	if w.ID == "" || w.WorkspaceKey == "" || w.Name == "" {
		return errors.New("id, workspace_key, and name are required")
	}
	if w.Status == "" {
		w.Status = "active"
	}
	if w.Environment == "" {
		w.Environment = "development"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_workspaces
		(id, workspace_key, name, description, owner, status, environment, tags_json, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_key) DO UPDATE SET
			name=excluded.name, description=excluded.description, owner=excluded.owner,
			status=excluded.status, environment=excluded.environment, tags_json=excluded.tags_json,
			updated_at=excluded.updated_at`),
		w.ID, w.WorkspaceKey, w.Name, w.Description, w.Owner, w.Status, w.Environment,
		platformJSON(w.Tags, "[]"), w.CreatedBy, now, now)
	return err
}

func scanDataWorksWorkspace(sc interface{ Scan(...any) error }) (DataWorksWorkspace, error) {
	var w DataWorksWorkspace
	var tags string
	err := sc.Scan(&w.ID, &w.WorkspaceKey, &w.Name, &w.Description, &w.Owner, &w.Status,
		&w.Environment, &tags, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt)
	w.Tags = platformStringList(tags)
	return w, err
}

func (s *SQLStore) GetDataWorksWorkspace(ctx context.Context, idOrKey string) (DataWorksWorkspace, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, workspace_key, name, description, owner, status,
		environment, tags_json, created_by, created_at, updated_at
		FROM dw_workspaces WHERE id = ? OR workspace_key = ?`), idOrKey, idOrKey)
	w, err := scanDataWorksWorkspace(row)
	if errors.Is(err, sql.ErrNoRows) {
		return DataWorksWorkspace{}, false, nil
	}
	if err != nil {
		return DataWorksWorkspace{}, false, err
	}
	if err := s.populateWorkspaceSummary(ctx, &w); err != nil {
		return DataWorksWorkspace{}, false, err
	}
	return w, true, nil
}

func (s *SQLStore) ListDataWorksWorkspaces(ctx context.Context, status string) ([]DataWorksWorkspace, error) {
	q := `SELECT id, workspace_key, name, description, owner, status, environment, tags_json,
		created_by, created_at, updated_at FROM dw_workspaces`
	args := []any{}
	if strings.TrimSpace(status) != "" {
		q += ` WHERE status = ?`
		args = append(args, strings.TrimSpace(status))
	}
	q += ` ORDER BY updated_at DESC, name`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksWorkspace{}
	for rows.Next() {
		w, err := scanDataWorksWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.populateWorkspaceSummary(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *SQLStore) populateWorkspaceSummary(ctx context.Context, w *DataWorksWorkspace) error {
	queries := []struct {
		q    string
		dest *int
	}{
		{`SELECT COUNT(*) FROM dw_workspace_members WHERE workspace_id = ?`, &w.MemberCount},
		{`SELECT COUNT(*) FROM dw_metadata_entities WHERE workspace_id = ?`, &w.EntityCount},
		{`SELECT COUNT(*) FROM dw_metadata_entities WHERE workspace_id = ? AND entity_type = 'product'`, &w.ProductCount},
		{`SELECT COUNT(*) FROM dw_flow_definitions WHERE workspace_id = ?`, &w.FlowCount},
		{`SELECT COUNT(*) FROM dw_agent_registry WHERE workspace_id = ?`, &w.AgentCount},
	}
	for _, item := range queries {
		if err := s.db.QueryRowContext(ctx, s.bind(item.q), w.ID).Scan(item.dest); err != nil {
			return err
		}
	}
	return s.db.QueryRowContext(ctx, s.bind(`SELECT
		COALESCE((SELECT SUM(r.total_cost) FROM dw_flow_runs r JOIN dw_flow_definitions f ON f.id=r.flow_id WHERE f.workspace_id=?), 0) +
		COALESCE((SELECT SUM(x.total_cost) FROM dw_agent_sessions x JOIN dw_agent_registry a ON a.id=x.agent_id WHERE a.workspace_id=?), 0)`),
		w.ID, w.ID).Scan(&w.MonthlyCost)
}

func (s *SQLStore) UpsertDataWorksWorkspaceMember(ctx context.Context, m DataWorksWorkspaceMember) error {
	if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.WorkspaceID) == "" || strings.TrimSpace(m.UserID) == "" {
		return errors.New("id, workspace_id, and user_id are required")
	}
	if m.Role == "" {
		m.Role = "viewer"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_workspace_members
		(id, workspace_id, user_id, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, user_id) DO UPDATE SET role=excluded.role, updated_at=excluded.updated_at`),
		m.ID, m.WorkspaceID, m.UserID, m.Role, now, now)
	return err
}

func (s *SQLStore) ListDataWorksWorkspaceMembers(ctx context.Context, workspaceID string) ([]DataWorksWorkspaceMember, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT id, workspace_id, user_id, role, created_at, updated_at
		FROM dw_workspace_members WHERE workspace_id = ? ORDER BY role, user_id`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksWorkspaceMember{}
	for rows.Next() {
		var m DataWorksWorkspaceMember
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertMetadataEntity(ctx context.Context, e MetadataEntity) error {
	e.ID = strings.TrimSpace(e.ID)
	e.URN = strings.TrimSpace(e.URN)
	e.EntityType = strings.ToLower(strings.TrimSpace(e.EntityType))
	e.Name = strings.TrimSpace(e.Name)
	if e.ID == "" || e.URN == "" || e.EntityType == "" || e.Name == "" {
		return errors.New("id, urn, entity_type, and name are required")
	}
	if e.Status == "" {
		e.Status = "active"
	}
	if e.Sensitivity == "" {
		e.Sensitivity = "internal"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_metadata_entities
		(id, urn, workspace_id, entity_type, name, description, owner, domain, status, sensitivity,
		 tags_json, properties_json, source_ref, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(urn) DO UPDATE SET workspace_id=excluded.workspace_id, entity_type=excluded.entity_type,
			name=excluded.name, description=excluded.description, owner=excluded.owner, domain=excluded.domain,
			status=excluded.status, sensitivity=excluded.sensitivity, tags_json=excluded.tags_json,
			properties_json=excluded.properties_json, source_ref=excluded.source_ref, updated_at=excluded.updated_at`),
		e.ID, e.URN, e.WorkspaceID, e.EntityType, e.Name, e.Description, e.Owner, e.Domain, e.Status,
		e.Sensitivity, platformJSON(e.Tags, "[]"), platformJSON(e.Properties, "{}"), e.SourceRef, now, now)
	return err
}

func scanMetadataEntity(sc interface{ Scan(...any) error }) (MetadataEntity, error) {
	var e MetadataEntity
	var tags, properties string
	err := sc.Scan(&e.ID, &e.URN, &e.WorkspaceID, &e.EntityType, &e.Name, &e.Description, &e.Owner,
		&e.Domain, &e.Status, &e.Sensitivity, &tags, &properties, &e.SourceRef, &e.CreatedAt, &e.UpdatedAt)
	e.Tags = platformStringList(tags)
	e.Properties = platformMap(properties)
	return e, err
}

const metadataEntityColumns = `id, urn, workspace_id, entity_type, name, description, owner, domain,
	status, sensitivity, tags_json, properties_json, source_ref, created_at, updated_at`

func (s *SQLStore) GetMetadataEntity(ctx context.Context, urn string) (MetadataEntity, bool, error) {
	e, err := scanMetadataEntity(s.db.QueryRowContext(ctx, s.bind(`SELECT `+metadataEntityColumns+`
		FROM dw_metadata_entities WHERE urn = ?`), urn))
	if errors.Is(err, sql.ErrNoRows) {
		return MetadataEntity{}, false, nil
	}
	if err != nil {
		return MetadataEntity{}, false, err
	}
	return e, true, nil
}

func (s *SQLStore) SearchMetadataEntities(ctx context.Context, query, entityType, workspaceID string, limit int) ([]MetadataEntity, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT ` + metadataEntityColumns + ` FROM dw_metadata_entities WHERE 1=1`
	args := []any{}
	if query = strings.ToLower(strings.TrimSpace(query)); query != "" {
		like := "%" + query + "%"
		q += ` AND (LOWER(name) LIKE ? OR LOWER(description) LIKE ? OR LOWER(urn) LIKE ? OR LOWER(tags_json) LIKE ?)`
		args = append(args, like, like, like, like)
	}
	if entityType = strings.ToLower(strings.TrimSpace(entityType)); entityType != "" {
		q += ` AND entity_type = ?`
		args = append(args, entityType)
	}
	if workspaceID = strings.TrimSpace(workspaceID); workspaceID != "" {
		q += ` AND workspace_id = ?`
		args = append(args, workspaceID)
	}
	q += ` ORDER BY updated_at DESC, name LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MetadataEntity{}
	for rows.Next() {
		e, err := scanMetadataEntity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertMetadataEdge(ctx context.Context, e MetadataEdge) error {
	if strings.TrimSpace(e.ID) == "" || strings.TrimSpace(e.SourceURN) == "" ||
		strings.TrimSpace(e.TargetURN) == "" || strings.TrimSpace(e.RelationType) == "" {
		return errors.New("id, source_urn, target_urn, and relation_type are required")
	}
	if e.SourceURN == e.TargetURN {
		return errors.New("metadata edge cannot reference itself")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_metadata_edges
		(id, workspace_id, source_urn, target_urn, relation_type, metadata_json, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_urn, target_urn, relation_type) DO UPDATE SET
			workspace_id=excluded.workspace_id, metadata_json=excluded.metadata_json, created_by=excluded.created_by`),
		e.ID, e.WorkspaceID, e.SourceURN, e.TargetURN, strings.ToLower(e.RelationType),
		platformJSON(e.Metadata, "{}"), e.CreatedBy, now)
	return err
}

func scanMetadataEdge(sc interface{ Scan(...any) error }) (MetadataEdge, error) {
	var e MetadataEdge
	var metadata string
	err := sc.Scan(&e.ID, &e.WorkspaceID, &e.SourceURN, &e.TargetURN, &e.RelationType,
		&metadata, &e.CreatedBy, &e.CreatedAt)
	e.Metadata = platformMap(metadata)
	return e, err
}

func (s *SQLStore) metadataNeighbors(ctx context.Context, urn string) ([]MetadataEdge, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT id, workspace_id, source_urn, target_urn,
		relation_type, metadata_json, created_by, created_at FROM dw_metadata_edges
		WHERE source_urn = ? OR target_urn = ? ORDER BY created_at`), urn, urn)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MetadataEdge{}
	for rows.Next() {
		e, err := scanMetadataEdge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLStore) MetadataLineage(ctx context.Context, urn string, maxDepth int) (MetadataGraph, error) {
	if maxDepth <= 0 || maxDepth > 8 {
		maxDepth = 4
	}
	if _, ok, err := s.GetMetadataEntity(ctx, urn); err != nil {
		return MetadataGraph{}, err
	} else if !ok {
		return MetadataGraph{}, sql.ErrNoRows
	}
	seenURN := map[string]bool{urn: true}
	seenEdge := map[string]bool{}
	frontier := []string{urn}
	edges := []MetadataEdge{}
	for depth := 0; depth < maxDepth && len(frontier) > 0 && len(seenURN) < 200; depth++ {
		next := []string{}
		for _, current := range frontier {
			neighbors, err := s.metadataNeighbors(ctx, current)
			if err != nil {
				return MetadataGraph{}, err
			}
			for _, edge := range neighbors {
				if !seenEdge[edge.ID] {
					seenEdge[edge.ID] = true
					edges = append(edges, edge)
				}
				other := edge.SourceURN
				if other == current {
					other = edge.TargetURN
				}
				if !seenURN[other] {
					seenURN[other] = true
					next = append(next, other)
				}
			}
		}
		frontier = next
	}
	nodes, err := s.metadataEntitiesByURN(ctx, seenURN)
	return MetadataGraph{RootURN: urn, Nodes: nodes, Edges: edges}, err
}

func (s *SQLStore) MetadataImpactAnalysis(ctx context.Context, urn string, maxDepth int) (MetadataImpact, error) {
	if maxDepth <= 0 || maxDepth > 8 {
		maxDepth = 4
	}
	root, ok, err := s.GetMetadataEntity(ctx, urn)
	if err != nil {
		return MetadataImpact{}, err
	}
	if !ok {
		return MetadataImpact{}, sql.ErrNoRows
	}
	seenURN := map[string]bool{urn: true}
	seenEdge := map[string]bool{}
	frontier := []string{urn}
	edges := []MetadataEdge{}
	dependencyRelations := map[string]bool{"uses": true, "depends_on": true, "governed_by": true, "owned_by": true, "approved_by": true}
	outputRelations := map[string]bool{"produces": true, "exposes": true}
	for depth := 0; depth < maxDepth && len(frontier) > 0 && len(seenURN) < 200; depth++ {
		next := []string{}
		for _, current := range frontier {
			neighbors, err := s.metadataNeighbors(ctx, current)
			if err != nil {
				return MetadataImpact{}, err
			}
			for _, edge := range neighbors {
				affected := ""
				if edge.TargetURN == current && dependencyRelations[edge.RelationType] {
					affected = edge.SourceURN
				} else if edge.SourceURN == current && outputRelations[edge.RelationType] {
					affected = edge.TargetURN
				}
				if affected == "" || seenURN[affected] {
					continue
				}
				seenURN[affected] = true
				next = append(next, affected)
				if !seenEdge[edge.ID] {
					seenEdge[edge.ID] = true
					edges = append(edges, edge)
				}
			}
		}
		frontier = next
	}
	nodes, err := s.metadataEntitiesByURN(ctx, seenURN)
	if err != nil {
		return MetadataImpact{}, err
	}
	byType := map[string]int{}
	riskScore := 0
	for _, node := range nodes {
		if node.URN == urn {
			continue
		}
		byType[node.EntityType]++
		riskScore += map[string]int{"api": 18, "product": 16, "agent": 14, "flow": 12, "report": 8}[node.EntityType]
		if node.Sensitivity == "personal_credit" || node.Sensitivity == "restricted" {
			riskScore += 12
		}
	}
	if root.Sensitivity == "personal_credit" || root.Sensitivity == "restricted" {
		riskScore += 20
	}
	if riskScore > 100 {
		riskScore = 100
	}
	return MetadataImpact{
		MetadataGraph: MetadataGraph{RootURN: urn, Nodes: nodes, Edges: edges},
		RiskScore:     riskScore, AffectedByType: byType, ApprovalRequired: riskScore >= 50,
	}, nil
}

func (s *SQLStore) metadataEntitiesByURN(ctx context.Context, urns map[string]bool) ([]MetadataEntity, error) {
	out := []MetadataEntity{}
	for urn := range urns {
		e, ok, err := s.GetMetadataEntity(ctx, urn)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *SQLStore) UpsertSemanticMetric(ctx context.Context, m SemanticMetric) error {
	m.ID = strings.TrimSpace(m.ID)
	m.MetricKey = strings.TrimSpace(m.MetricKey)
	m.Name = strings.TrimSpace(m.Name)
	m.Expression = strings.TrimSpace(m.Expression)
	if m.ID == "" || m.MetricKey == "" || m.Name == "" || m.Expression == "" {
		return errors.New("id, metric_key, name, and expression are required")
	}
	if m.Aggregation == "" {
		m.Aggregation = "sum"
	}
	if m.Status == "" {
		m.Status = "draft"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_semantic_metrics
		(id, metric_key, workspace_id, name, description, expression, aggregation, dimensions_json,
		 filters_json, customer_segment, product_group, owner, status, version, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
		ON CONFLICT(metric_key) DO UPDATE SET workspace_id=excluded.workspace_id, name=excluded.name,
			description=excluded.description, expression=excluded.expression, aggregation=excluded.aggregation,
			dimensions_json=excluded.dimensions_json, filters_json=excluded.filters_json,
			customer_segment=excluded.customer_segment, product_group=excluded.product_group,
			owner=excluded.owner, status=excluded.status, version=dw_semantic_metrics.version + 1,
			updated_at=excluded.updated_at`),
		m.ID, m.MetricKey, m.WorkspaceID, m.Name, m.Description, m.Expression, m.Aggregation,
		platformJSON(m.Dimensions, "[]"), platformJSON(m.Filters, "{}"), m.CustomerSegment,
		m.ProductGroup, m.Owner, m.Status, m.CreatedBy, now, now)
	return err
}

func scanSemanticMetric(sc interface{ Scan(...any) error }) (SemanticMetric, error) {
	var m SemanticMetric
	var dimensions, filters string
	err := sc.Scan(&m.ID, &m.MetricKey, &m.WorkspaceID, &m.Name, &m.Description, &m.Expression,
		&m.Aggregation, &dimensions, &filters, &m.CustomerSegment, &m.ProductGroup, &m.Owner,
		&m.Status, &m.Version, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt)
	m.Dimensions = platformStringList(dimensions)
	m.Filters = platformMap(filters)
	return m, err
}

func (s *SQLStore) ListSemanticMetrics(ctx context.Context, workspaceID, status string) ([]SemanticMetric, error) {
	q := `SELECT id, metric_key, workspace_id, name, description, expression, aggregation,
		dimensions_json, filters_json, customer_segment, product_group, owner, status, version,
		created_by, created_at, updated_at FROM dw_semantic_metrics WHERE 1=1`
	args := []any{}
	if workspaceID != "" {
		q += ` AND workspace_id = ?`
		args = append(args, workspaceID)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY updated_at DESC, metric_key`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SemanticMetric{}
	for rows.Next() {
		m, err := scanSemanticMetric(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertDataContractAssertion(ctx context.Context, a DataContractAssertion) error {
	if a.ID == "" || a.EntityURN == "" || a.AssertionType == "" {
		return errors.New("id, entity_urn, and assertion_type are required")
	}
	if a.Severity == "" {
		a.Severity = "medium"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_data_contract_assertions
		(id, entity_urn, assertion_type, field_name, operator, expected_value, severity, enabled,
		 created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET entity_urn=excluded.entity_urn, assertion_type=excluded.assertion_type,
			field_name=excluded.field_name, operator=excluded.operator, expected_value=excluded.expected_value,
			severity=excluded.severity, enabled=excluded.enabled, updated_at=excluded.updated_at`),
		a.ID, a.EntityURN, a.AssertionType, a.FieldName, a.Operator, a.ExpectedValue, a.Severity,
		boolInt(a.Enabled), a.CreatedBy, now, now)
	return err
}

func (s *SQLStore) ListDataContractAssertions(ctx context.Context, entityURN string) ([]DataContractAssertion, error) {
	q := `SELECT id, entity_urn, assertion_type, field_name, operator, expected_value, severity,
		enabled, created_by, created_at, updated_at FROM dw_data_contract_assertions`
	args := []any{}
	if entityURN != "" {
		q += ` WHERE entity_urn = ?`
		args = append(args, entityURN)
	}
	q += ` ORDER BY severity DESC, updated_at DESC`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataContractAssertion{}
	for rows.Next() {
		var a DataContractAssertion
		var enabled int
		if err := rows.Scan(&a.ID, &a.EntityURN, &a.AssertionType, &a.FieldName, &a.Operator,
			&a.ExpectedValue, &a.Severity, &enabled, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.Enabled = enabled != 0
		out = append(out, a)
	}
	return out, rows.Err()
}

func DataWorksURN(entityType, key string) string {
	return fmt.Sprintf("urn:dw:%s:%s", strings.ToLower(strings.TrimSpace(entityType)), strings.TrimSpace(key))
}
