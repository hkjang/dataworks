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

type DataWorksFlowNode struct {
	ID         string         `json:"id"`
	FlowID     string         `json:"flow_id"`
	NodeKey    string         `json:"node_key"`
	NodeType   string         `json:"node_type"`
	Name       string         `json:"name"`
	RefURN     string         `json:"ref_urn"`
	Config     map[string]any `json:"config"`
	PositionX  float64        `json:"position_x"`
	PositionY  float64        `json:"position_y"`
	SequenceNo int            `json:"sequence_no"`
}

type DataWorksFlowEdge struct {
	ID                  string `json:"id"`
	FlowID              string `json:"flow_id"`
	SourceNodeKey       string `json:"source_node_key"`
	TargetNodeKey       string `json:"target_node_key"`
	ConditionExpression string `json:"condition_expression"`
}

type DataWorksFlow struct {
	ID           string              `json:"id"`
	FlowKey      string              `json:"flow_key"`
	WorkspaceID  string              `json:"workspace_id"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	FlowType     string              `json:"flow_type"`
	Status       string              `json:"status"`
	Version      int                 `json:"version"`
	Schedule     string              `json:"schedule"`
	Owner        string              `json:"owner"`
	InputSchema  string              `json:"input_schema"`
	OutputSchema string              `json:"output_schema"`
	CreatedBy    string              `json:"created_by"`
	CreatedAt    string              `json:"created_at"`
	UpdatedAt    string              `json:"updated_at"`
	Nodes        []DataWorksFlowNode `json:"nodes"`
	Edges        []DataWorksFlowEdge `json:"edges"`
}

type DataWorksFlowRun struct {
	ID            string         `json:"id"`
	FlowID        string         `json:"flow_id"`
	FlowVersion   int            `json:"flow_version"`
	Status        string         `json:"status"`
	TriggerType   string         `json:"trigger_type"`
	InputSummary  string         `json:"input_summary"`
	ResultSummary map[string]any `json:"result_summary"`
	TotalCost     float64        `json:"total_cost"`
	LatencyMS     int64          `json:"latency_ms"`
	StartedAt     string         `json:"started_at"`
	FinishedAt    string         `json:"finished_at"`
	CreatedBy     string         `json:"created_by"`
}

type DataWorksAgent struct {
	ID               string   `json:"id"`
	AgentKey         string   `json:"agent_key"`
	WorkspaceID      string   `json:"workspace_id"`
	Name             string   `json:"name"`
	Purpose          string   `json:"purpose"`
	Owner            string   `json:"owner"`
	RiskLevel        string   `json:"risk_level"`
	Status           string   `json:"status"`
	Version          int      `json:"version"`
	SystemPrompt     string   `json:"system_prompt"`
	AllowedTools     []string `json:"allowed_tools"`
	MemoryScope      string   `json:"memory_scope"`
	MaxCost          float64  `json:"max_cost"`
	MaxSteps         int      `json:"max_steps"`
	RequiresApproval bool     `json:"requires_approval"`
	CreatedBy        string   `json:"created_by"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

type DataWorksAgentSession struct {
	ID             string  `json:"id"`
	AgentID        string  `json:"agent_id"`
	Status         string  `json:"status"`
	InputSummary   string  `json:"input_summary"`
	OutputSummary  string  `json:"output_summary"`
	TotalCost      float64 `json:"total_cost"`
	LatencyMS      int64   `json:"latency_ms"`
	PolicyBlocked  bool    `json:"policy_blocked"`
	ApprovalStatus string  `json:"approval_status"`
	StartedAt      string  `json:"started_at"`
	FinishedAt     string  `json:"finished_at"`
	CreatedBy      string  `json:"created_by"`
}

type DataWorksAgentTrace struct {
	ID               string  `json:"id"`
	SessionID        string  `json:"session_id"`
	StepNo           int     `json:"step_no"`
	TraceType        string  `json:"trace_type"`
	Name             string  `json:"name"`
	Status           string  `json:"status"`
	ReasoningSummary string  `json:"reasoning_summary"`
	ToolID           string  `json:"tool_id"`
	InputSummary     string  `json:"input_summary"`
	OutputSummary    string  `json:"output_summary"`
	PolicyDecision   string  `json:"policy_decision"`
	Cost             float64 `json:"cost"`
	LatencyMS        int64   `json:"latency_ms"`
	CreatedAt        string  `json:"created_at"`
}

type DataWorksTool struct {
	ID                string         `json:"id"`
	ToolKey           string         `json:"tool_key"`
	WorkspaceID       string         `json:"workspace_id"`
	Name              string         `json:"name"`
	ToolType          string         `json:"tool_type"`
	ServerLabel       string         `json:"server_label"`
	Endpoint          string         `json:"endpoint"`
	Description       string         `json:"description"`
	Owner             string         `json:"owner"`
	RiskLevel         string         `json:"risk_level"`
	InputSchema       string         `json:"input_schema"`
	OutputSchema      string         `json:"output_schema"`
	AllowedParameters map[string]any `json:"allowed_parameters"`
	RequiresApproval  bool           `json:"requires_approval"`
	MaskingLevel      string         `json:"masking_level"`
	Enabled           bool           `json:"enabled"`
	LastTestStatus    string         `json:"last_test_status"`
	LastTestedAt      string         `json:"last_tested_at"`
	CreatedBy         string         `json:"created_by"`
	CreatedAt         string         `json:"created_at"`
	UpdatedAt         string         `json:"updated_at"`
}

type DataWorksToolPermission struct {
	ID                string         `json:"id"`
	ToolID            string         `json:"tool_id"`
	PrincipalType     string         `json:"principal_type"`
	PrincipalID       string         `json:"principal_id"`
	Allowed           bool           `json:"allowed"`
	AllowedParameters map[string]any `json:"allowed_parameters"`
	MaxCalls          int            `json:"max_calls"`
	CreatedBy         string         `json:"created_by"`
	CreatedAt         string         `json:"created_at"`
	UpdatedAt         string         `json:"updated_at"`
}

func (s *SQLStore) SaveDataWorksFlow(ctx context.Context, f DataWorksFlow) (DataWorksFlow, error) {
	f.FlowKey = strings.TrimSpace(f.FlowKey)
	f.Name = strings.TrimSpace(f.Name)
	if f.ID == "" || f.FlowKey == "" || f.Name == "" {
		return DataWorksFlow{}, errors.New("id, flow_key, and name are required")
	}
	if f.FlowType == "" {
		f.FlowType = "hybrid"
	}
	if f.Status == "" {
		f.Status = "draft"
	}
	if f.InputSchema == "" {
		f.InputSchema = "{}"
	}
	if f.OutputSchema == "" {
		f.OutputSchema = "{}"
	}
	if err := validateFlowGraph(f.Nodes, f.Edges); err != nil {
		return DataWorksFlow{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DataWorksFlow{}, err
	}
	defer tx.Rollback()
	var existingID string
	var existingVersion int
	err = tx.QueryRowContext(ctx, s.bind(`SELECT id, version FROM dw_flow_definitions WHERE flow_key = ?`), f.FlowKey).Scan(&existingID, &existingVersion)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return DataWorksFlow{}, err
	}
	if err == nil {
		f.ID = existingID
		f.Version = existingVersion + 1
	} else {
		f.Version = 1
	}
	now := formatTime(time.Now().UTC())
	_, err = tx.ExecContext(ctx, s.bind(`INSERT INTO dw_flow_definitions
		(id, flow_key, workspace_id, name, description, flow_type, status, version, schedule, owner,
		 input_schema, output_schema, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(flow_key) DO UPDATE SET workspace_id=excluded.workspace_id, name=excluded.name,
			description=excluded.description, flow_type=excluded.flow_type, status=excluded.status,
			version=excluded.version, schedule=excluded.schedule, owner=excluded.owner,
			input_schema=excluded.input_schema, output_schema=excluded.output_schema, updated_at=excluded.updated_at`),
		f.ID, f.FlowKey, f.WorkspaceID, f.Name, f.Description, f.FlowType, f.Status, f.Version,
		f.Schedule, f.Owner, f.InputSchema, f.OutputSchema, f.CreatedBy, now, now)
	if err != nil {
		return DataWorksFlow{}, err
	}
	if _, err = tx.ExecContext(ctx, s.bind(`DELETE FROM dw_flow_edges WHERE flow_id = ?`), f.ID); err != nil {
		return DataWorksFlow{}, err
	}
	if _, err = tx.ExecContext(ctx, s.bind(`DELETE FROM dw_flow_nodes WHERE flow_id = ?`), f.ID); err != nil {
		return DataWorksFlow{}, err
	}
	for i := range f.Nodes {
		n := &f.Nodes[i]
		n.FlowID = f.ID
		n.SequenceNo = i
		if n.ID == "" {
			n.ID = f.ID + ":node:" + n.NodeKey
		}
		_, err = tx.ExecContext(ctx, s.bind(`INSERT INTO dw_flow_nodes
			(id, flow_id, node_key, node_type, name, ref_urn, config_json, position_x, position_y, sequence_no)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), n.ID, f.ID, n.NodeKey, strings.ToLower(n.NodeType),
			n.Name, n.RefURN, platformJSON(n.Config, "{}"), n.PositionX, n.PositionY, n.SequenceNo)
		if err != nil {
			return DataWorksFlow{}, err
		}
	}
	for i := range f.Edges {
		e := &f.Edges[i]
		e.FlowID = f.ID
		if e.ID == "" {
			e.ID = f.ID + ":edge:" + e.SourceNodeKey + ":" + e.TargetNodeKey
		}
		_, err = tx.ExecContext(ctx, s.bind(`INSERT INTO dw_flow_edges
			(id, flow_id, source_node_key, target_node_key, condition_expression) VALUES (?, ?, ?, ?, ?)`),
			e.ID, f.ID, e.SourceNodeKey, e.TargetNodeKey, e.ConditionExpression)
		if err != nil {
			return DataWorksFlow{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return DataWorksFlow{}, err
	}
	f.CreatedAt = now
	f.UpdatedAt = now
	return f, nil
}

func validateFlowGraph(nodes []DataWorksFlowNode, edges []DataWorksFlowEdge) error {
	if len(nodes) == 0 {
		return errors.New("flow requires at least one node")
	}
	allowedTypes := map[string]bool{
		"input": true, "sql": true, "python": true, "api": true, "rag": true,
		"product_factory": true, "risk_check": true, "proposal": true, "poc": true,
		"approval": true, "prompt": true, "llm": true, "agent": true, "tool": true,
		"guardrail": true, "output": true,
	}
	keys := map[string]bool{}
	for _, n := range nodes {
		key := strings.TrimSpace(n.NodeKey)
		typ := strings.ToLower(strings.TrimSpace(n.NodeType))
		if key == "" || !allowedTypes[typ] {
			return fmt.Errorf("invalid flow node %q of type %q", key, n.NodeType)
		}
		if keys[key] {
			return fmt.Errorf("duplicate flow node_key %q", key)
		}
		keys[key] = true
	}
	indegree := map[string]int{}
	adj := map[string][]string{}
	for key := range keys {
		indegree[key] = 0
	}
	for _, e := range edges {
		if !keys[e.SourceNodeKey] || !keys[e.TargetNodeKey] {
			return fmt.Errorf("flow edge references unknown node %q -> %q", e.SourceNodeKey, e.TargetNodeKey)
		}
		if e.SourceNodeKey == e.TargetNodeKey {
			return errors.New("flow edge cannot reference itself")
		}
		indegree[e.TargetNodeKey]++
		adj[e.SourceNodeKey] = append(adj[e.SourceNodeKey], e.TargetNodeKey)
	}
	queue := []string{}
	for key, degree := range indegree {
		if degree == 0 {
			queue = append(queue, key)
		}
	}
	visited := 0
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adj[key] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if visited != len(nodes) {
		return errors.New("flow graph must be acyclic")
	}
	return nil
}

func scanDataWorksFlow(sc interface{ Scan(...any) error }) (DataWorksFlow, error) {
	var f DataWorksFlow
	err := sc.Scan(&f.ID, &f.FlowKey, &f.WorkspaceID, &f.Name, &f.Description, &f.FlowType,
		&f.Status, &f.Version, &f.Schedule, &f.Owner, &f.InputSchema, &f.OutputSchema,
		&f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

const dataWorksFlowColumns = `id, flow_key, workspace_id, name, description, flow_type, status,
	version, schedule, owner, input_schema, output_schema, created_by, created_at, updated_at`

func (s *SQLStore) GetDataWorksFlow(ctx context.Context, idOrKey string) (DataWorksFlow, bool, error) {
	f, err := scanDataWorksFlow(s.db.QueryRowContext(ctx, s.bind(`SELECT `+dataWorksFlowColumns+`
		FROM dw_flow_definitions WHERE id = ? OR flow_key = ?`), idOrKey, idOrKey))
	if errors.Is(err, sql.ErrNoRows) {
		return DataWorksFlow{}, false, nil
	}
	if err != nil {
		return DataWorksFlow{}, false, err
	}
	nodes, err := s.listDataWorksFlowNodes(ctx, f.ID)
	if err != nil {
		return DataWorksFlow{}, false, err
	}
	edges, err := s.listDataWorksFlowEdges(ctx, f.ID)
	if err != nil {
		return DataWorksFlow{}, false, err
	}
	f.Nodes, f.Edges = nodes, edges
	return f, true, nil
}

func (s *SQLStore) ListDataWorksFlows(ctx context.Context, workspaceID, status string) ([]DataWorksFlow, error) {
	q := `SELECT ` + dataWorksFlowColumns + ` FROM dw_flow_definitions WHERE 1=1`
	args := []any{}
	if workspaceID != "" {
		q += ` AND workspace_id = ?`
		args = append(args, workspaceID)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY updated_at DESC, name`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksFlow{}
	for rows.Next() {
		f, err := scanDataWorksFlow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *SQLStore) listDataWorksFlowNodes(ctx context.Context, flowID string) ([]DataWorksFlowNode, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT id, flow_id, node_key, node_type, name, ref_urn,
		config_json, position_x, position_y, sequence_no FROM dw_flow_nodes WHERE flow_id = ? ORDER BY sequence_no`), flowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksFlowNode{}
	for rows.Next() {
		var n DataWorksFlowNode
		var config string
		if err := rows.Scan(&n.ID, &n.FlowID, &n.NodeKey, &n.NodeType, &n.Name, &n.RefURN,
			&config, &n.PositionX, &n.PositionY, &n.SequenceNo); err != nil {
			return nil, err
		}
		n.Config = platformMap(config)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *SQLStore) listDataWorksFlowEdges(ctx context.Context, flowID string) ([]DataWorksFlowEdge, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT id, flow_id, source_node_key, target_node_key,
		condition_expression FROM dw_flow_edges WHERE flow_id = ? ORDER BY source_node_key, target_node_key`), flowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksFlowEdge{}
	for rows.Next() {
		var e DataWorksFlowEdge
		if err := rows.Scan(&e.ID, &e.FlowID, &e.SourceNodeKey, &e.TargetNodeKey, &e.ConditionExpression); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLStore) PromoteDataWorksFlow(ctx context.Context, id, target string) (DataWorksFlow, error) {
	f, ok, err := s.GetDataWorksFlow(ctx, id)
	if err != nil {
		return DataWorksFlow{}, err
	}
	if !ok {
		return DataWorksFlow{}, sql.ErrNoRows
	}
	next := map[string]string{"draft": "test", "test": "approved", "approved": "production"}
	if target == "" {
		target = next[f.Status]
	}
	if next[f.Status] != target {
		return DataWorksFlow{}, fmt.Errorf("invalid flow promotion %s -> %s", f.Status, target)
	}
	_, err = s.db.ExecContext(ctx, s.bind(`UPDATE dw_flow_definitions SET status = ?, updated_at = ? WHERE id = ?`),
		target, formatTime(time.Now().UTC()), f.ID)
	if err != nil {
		return DataWorksFlow{}, err
	}
	f.Status = target
	return f, nil
}

func (s *SQLStore) InsertDataWorksFlowRun(ctx context.Context, r DataWorksFlowRun) error {
	if r.ID == "" || r.FlowID == "" || r.Status == "" {
		return errors.New("id, flow_id, and status are required")
	}
	if r.TriggerType == "" {
		r.TriggerType = "manual"
	}
	if r.StartedAt == "" {
		r.StartedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_flow_runs
		(id, flow_id, flow_version, status, trigger_type, input_summary, result_summary, total_cost,
		 latency_ms, started_at, finished_at, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		r.ID, r.FlowID, r.FlowVersion, r.Status, r.TriggerType, r.InputSummary,
		platformJSON(r.ResultSummary, "{}"), r.TotalCost, r.LatencyMS, r.StartedAt, r.FinishedAt, r.CreatedBy)
	return err
}

func (s *SQLStore) ListDataWorksFlowRuns(ctx context.Context, flowID string, limit int) ([]DataWorksFlowRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, flow_id, flow_version, status, trigger_type, input_summary, result_summary,
		total_cost, latency_ms, started_at, finished_at, created_by FROM dw_flow_runs`
	args := []any{}
	if flowID != "" {
		q += ` WHERE flow_id = ?`
		args = append(args, flowID)
	}
	q += ` ORDER BY started_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksFlowRun{}
	for rows.Next() {
		var r DataWorksFlowRun
		var result string
		if err := rows.Scan(&r.ID, &r.FlowID, &r.FlowVersion, &r.Status, &r.TriggerType,
			&r.InputSummary, &result, &r.TotalCost, &r.LatencyMS, &r.StartedAt, &r.FinishedAt, &r.CreatedBy); err != nil {
			return nil, err
		}
		r.ResultSummary = platformMap(result)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLStore) SaveDataWorksAgent(ctx context.Context, a DataWorksAgent) (DataWorksAgent, error) {
	a.AgentKey = strings.TrimSpace(a.AgentKey)
	a.Name = strings.TrimSpace(a.Name)
	if a.ID == "" || a.AgentKey == "" || a.Name == "" {
		return DataWorksAgent{}, errors.New("id, agent_key, and name are required")
	}
	if a.RiskLevel == "" {
		a.RiskLevel = "medium"
	}
	if a.Status == "" {
		a.Status = "draft"
	}
	if a.MemoryScope == "" {
		a.MemoryScope = "workspace"
	}
	if a.MaxSteps <= 0 {
		a.MaxSteps = 8
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DataWorksAgent{}, err
	}
	defer tx.Rollback()
	var existingID string
	var existingVersion int
	err = tx.QueryRowContext(ctx, s.bind(`SELECT id, version FROM dw_agent_registry WHERE agent_key = ?`), a.AgentKey).Scan(&existingID, &existingVersion)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return DataWorksAgent{}, err
	}
	if err == nil {
		a.ID = existingID
		a.Version = existingVersion + 1
	} else {
		a.Version = 1
	}
	now := formatTime(time.Now().UTC())
	_, err = tx.ExecContext(ctx, s.bind(`INSERT INTO dw_agent_registry
		(id, agent_key, workspace_id, name, purpose, owner, risk_level, status, version, system_prompt,
		 allowed_tools_json, memory_scope, max_cost, max_steps, requires_approval, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_key) DO UPDATE SET workspace_id=excluded.workspace_id, name=excluded.name,
			purpose=excluded.purpose, owner=excluded.owner, risk_level=excluded.risk_level,
			status=excluded.status, version=excluded.version, system_prompt=excluded.system_prompt,
			allowed_tools_json=excluded.allowed_tools_json, memory_scope=excluded.memory_scope,
			max_cost=excluded.max_cost, max_steps=excluded.max_steps,
			requires_approval=excluded.requires_approval, updated_at=excluded.updated_at`),
		a.ID, a.AgentKey, a.WorkspaceID, a.Name, a.Purpose, a.Owner, a.RiskLevel, a.Status, a.Version,
		a.SystemPrompt, platformJSON(a.AllowedTools, "[]"), a.MemoryScope, a.MaxCost, a.MaxSteps,
		boolInt(a.RequiresApproval), a.CreatedBy, now, now)
	if err != nil {
		return DataWorksAgent{}, err
	}
	definition, _ := json.Marshal(a)
	versionID := fmt.Sprintf("%s:v%d", a.ID, a.Version)
	_, err = tx.ExecContext(ctx, s.bind(`INSERT INTO dw_agent_versions
		(id, agent_id, version, definition_json, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`),
		versionID, a.ID, a.Version, string(definition), a.CreatedBy, now)
	if err != nil {
		return DataWorksAgent{}, err
	}
	if err := tx.Commit(); err != nil {
		return DataWorksAgent{}, err
	}
	a.CreatedAt, a.UpdatedAt = now, now
	return a, nil
}

func scanDataWorksAgent(sc interface{ Scan(...any) error }) (DataWorksAgent, error) {
	var a DataWorksAgent
	var tools string
	var approval int
	err := sc.Scan(&a.ID, &a.AgentKey, &a.WorkspaceID, &a.Name, &a.Purpose, &a.Owner,
		&a.RiskLevel, &a.Status, &a.Version, &a.SystemPrompt, &tools, &a.MemoryScope,
		&a.MaxCost, &a.MaxSteps, &approval, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	a.AllowedTools = platformStringList(tools)
	a.RequiresApproval = approval != 0
	return a, err
}

const dataWorksAgentColumns = `id, agent_key, workspace_id, name, purpose, owner, risk_level,
	status, version, system_prompt, allowed_tools_json, memory_scope, max_cost, max_steps,
	requires_approval, created_by, created_at, updated_at`

func (s *SQLStore) GetDataWorksAgent(ctx context.Context, idOrKey string) (DataWorksAgent, bool, error) {
	a, err := scanDataWorksAgent(s.db.QueryRowContext(ctx, s.bind(`SELECT `+dataWorksAgentColumns+`
		FROM dw_agent_registry WHERE id = ? OR agent_key = ?`), idOrKey, idOrKey))
	if errors.Is(err, sql.ErrNoRows) {
		return DataWorksAgent{}, false, nil
	}
	if err != nil {
		return DataWorksAgent{}, false, err
	}
	return a, true, nil
}

func (s *SQLStore) ListDataWorksAgents(ctx context.Context, workspaceID, status string) ([]DataWorksAgent, error) {
	q := `SELECT ` + dataWorksAgentColumns + ` FROM dw_agent_registry WHERE 1=1`
	args := []any{}
	if workspaceID != "" {
		q += ` AND workspace_id = ?`
		args = append(args, workspaceID)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY updated_at DESC, name`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksAgent{}
	for rows.Next() {
		a, err := scanDataWorksAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLStore) InsertDataWorksAgentSession(ctx context.Context, session DataWorksAgentSession) error {
	if session.ID == "" || session.AgentID == "" || session.Status == "" {
		return errors.New("id, agent_id, and status are required")
	}
	if session.StartedAt == "" {
		session.StartedAt = formatTime(time.Now().UTC())
	}
	if session.ApprovalStatus == "" {
		session.ApprovalStatus = "not_required"
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_agent_sessions
		(id, agent_id, status, input_summary, output_summary, total_cost, latency_ms, policy_blocked,
		 approval_status, started_at, finished_at, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		session.ID, session.AgentID, session.Status, session.InputSummary, session.OutputSummary,
		session.TotalCost, session.LatencyMS, boolInt(session.PolicyBlocked), session.ApprovalStatus,
		session.StartedAt, session.FinishedAt, session.CreatedBy)
	return err
}

func (s *SQLStore) InsertDataWorksAgentTraces(ctx context.Context, traces []DataWorksAgentTrace) error {
	if len(traces) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := formatTime(time.Now().UTC())
	for i := range traces {
		t := &traces[i]
		if t.ID == "" || t.SessionID == "" || t.TraceType == "" {
			return errors.New("trace id, session_id, and trace_type are required")
		}
		if t.CreatedAt == "" {
			t.CreatedAt = now
		}
		_, err = tx.ExecContext(ctx, s.bind(`INSERT INTO dw_agent_traces
			(id, session_id, step_no, trace_type, name, status, reasoning_summary, tool_id,
			 input_summary, output_summary, policy_decision, cost, latency_ms, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			t.ID, t.SessionID, t.StepNo, t.TraceType, t.Name, t.Status, t.ReasoningSummary,
			t.ToolID, t.InputSummary, t.OutputSummary, t.PolicyDecision, t.Cost, t.LatencyMS, t.CreatedAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLStore) ListDataWorksAgentSessions(ctx context.Context, agentID string, limit int) ([]DataWorksAgentSession, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := `SELECT id, agent_id, status, input_summary, output_summary, total_cost, latency_ms,
		policy_blocked, approval_status, started_at, finished_at, created_by FROM dw_agent_sessions`
	args := []any{}
	if agentID != "" {
		q += ` WHERE agent_id = ?`
		args = append(args, agentID)
	}
	q += ` ORDER BY started_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksAgentSession{}
	for rows.Next() {
		var session DataWorksAgentSession
		var blocked int
		if err := rows.Scan(&session.ID, &session.AgentID, &session.Status, &session.InputSummary,
			&session.OutputSummary, &session.TotalCost, &session.LatencyMS, &blocked,
			&session.ApprovalStatus, &session.StartedAt, &session.FinishedAt, &session.CreatedBy); err != nil {
			return nil, err
		}
		session.PolicyBlocked = blocked != 0
		out = append(out, session)
	}
	return out, rows.Err()
}

func (s *SQLStore) ListDataWorksAgentTraces(ctx context.Context, sessionID string) ([]DataWorksAgentTrace, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT id, session_id, step_no, trace_type, name,
		status, reasoning_summary, tool_id, input_summary, output_summary, policy_decision, cost,
		latency_ms, created_at FROM dw_agent_traces WHERE session_id = ? ORDER BY step_no, created_at`), sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksAgentTrace{}
	for rows.Next() {
		var t DataWorksAgentTrace
		if err := rows.Scan(&t.ID, &t.SessionID, &t.StepNo, &t.TraceType, &t.Name, &t.Status,
			&t.ReasoningSummary, &t.ToolID, &t.InputSummary, &t.OutputSummary, &t.PolicyDecision,
			&t.Cost, &t.LatencyMS, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertDataWorksTool(ctx context.Context, t DataWorksTool) error {
	t.ID = strings.TrimSpace(t.ID)
	t.ToolKey = strings.TrimSpace(t.ToolKey)
	t.Name = strings.TrimSpace(t.Name)
	if t.ID == "" || t.ToolKey == "" || t.Name == "" {
		return errors.New("id, tool_key, and name are required")
	}
	if t.ToolType == "" {
		t.ToolType = "internal_api"
	}
	if t.RiskLevel == "" {
		t.RiskLevel = "medium"
	}
	if t.MaskingLevel == "" {
		t.MaskingLevel = "none"
	}
	if t.InputSchema == "" {
		t.InputSchema = "{}"
	}
	if t.OutputSchema == "" {
		t.OutputSchema = "{}"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_tool_registry
		(id, tool_key, workspace_id, name, tool_type, server_label, endpoint, description, owner,
		 risk_level, input_schema, output_schema, allowed_params_json, requires_approval, masking_level,
		 enabled, last_test_status, last_tested_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tool_key) DO UPDATE SET workspace_id=excluded.workspace_id, name=excluded.name,
			tool_type=excluded.tool_type, server_label=excluded.server_label, endpoint=excluded.endpoint,
			description=excluded.description, owner=excluded.owner, risk_level=excluded.risk_level,
			input_schema=excluded.input_schema, output_schema=excluded.output_schema,
			allowed_params_json=excluded.allowed_params_json, requires_approval=excluded.requires_approval,
			masking_level=excluded.masking_level, enabled=excluded.enabled, updated_at=excluded.updated_at`),
		t.ID, t.ToolKey, t.WorkspaceID, t.Name, t.ToolType, t.ServerLabel, t.Endpoint, t.Description,
		t.Owner, t.RiskLevel, t.InputSchema, t.OutputSchema, platformJSON(t.AllowedParameters, "{}"),
		boolInt(t.RequiresApproval), t.MaskingLevel, boolInt(t.Enabled), t.LastTestStatus,
		t.LastTestedAt, t.CreatedBy, now, now)
	return err
}

func scanDataWorksTool(sc interface{ Scan(...any) error }) (DataWorksTool, error) {
	var t DataWorksTool
	var params string
	var approval, enabled int
	err := sc.Scan(&t.ID, &t.ToolKey, &t.WorkspaceID, &t.Name, &t.ToolType, &t.ServerLabel,
		&t.Endpoint, &t.Description, &t.Owner, &t.RiskLevel, &t.InputSchema, &t.OutputSchema,
		&params, &approval, &t.MaskingLevel, &enabled, &t.LastTestStatus, &t.LastTestedAt,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	t.AllowedParameters = platformMap(params)
	t.RequiresApproval = approval != 0
	t.Enabled = enabled != 0
	return t, err
}

const dataWorksToolColumns = `id, tool_key, workspace_id, name, tool_type, server_label, endpoint,
	description, owner, risk_level, input_schema, output_schema, allowed_params_json,
	requires_approval, masking_level, enabled, last_test_status, last_tested_at, created_by,
	created_at, updated_at`

func (s *SQLStore) GetDataWorksTool(ctx context.Context, idOrKey string) (DataWorksTool, bool, error) {
	t, err := scanDataWorksTool(s.db.QueryRowContext(ctx, s.bind(`SELECT `+dataWorksToolColumns+`
		FROM dw_tool_registry WHERE id = ? OR tool_key = ?`), idOrKey, idOrKey))
	if errors.Is(err, sql.ErrNoRows) {
		return DataWorksTool{}, false, nil
	}
	if err != nil {
		return DataWorksTool{}, false, err
	}
	return t, true, nil
}

func (s *SQLStore) ListDataWorksTools(ctx context.Context, workspaceID, toolType string, enabledOnly bool) ([]DataWorksTool, error) {
	q := `SELECT ` + dataWorksToolColumns + ` FROM dw_tool_registry WHERE 1=1`
	args := []any{}
	if workspaceID != "" {
		q += ` AND workspace_id = ?`
		args = append(args, workspaceID)
	}
	if toolType != "" {
		q += ` AND tool_type = ?`
		args = append(args, toolType)
	}
	if enabledOnly {
		q += ` AND enabled = 1`
	}
	q += ` ORDER BY risk_level DESC, name`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksTool{}
	for rows.Next() {
		t, err := scanDataWorksTool(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpdateDataWorksToolTest(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, s.bind(`UPDATE dw_tool_registry SET last_test_status = ?,
		last_tested_at = ?, updated_at = ? WHERE id = ?`), status, formatTime(time.Now().UTC()),
		formatTime(time.Now().UTC()), id)
	return err
}

func (s *SQLStore) UpsertDataWorksToolPermission(ctx context.Context, p DataWorksToolPermission) error {
	if p.ID == "" || p.ToolID == "" || p.PrincipalType == "" || p.PrincipalID == "" {
		return errors.New("id, tool_id, principal_type, and principal_id are required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_tool_permissions
		(id, tool_id, principal_type, principal_id, allowed, allowed_params_json, max_calls,
		 created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tool_id, principal_type, principal_id) DO UPDATE SET allowed=excluded.allowed,
			allowed_params_json=excluded.allowed_params_json, max_calls=excluded.max_calls,
			updated_at=excluded.updated_at`), p.ID, p.ToolID, p.PrincipalType, p.PrincipalID,
		boolInt(p.Allowed), platformJSON(p.AllowedParameters, "{}"), p.MaxCalls, p.CreatedBy, now, now)
	return err
}

func (s *SQLStore) ListDataWorksToolPermissions(ctx context.Context, toolID string) ([]DataWorksToolPermission, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT id, tool_id, principal_type, principal_id,
		allowed, allowed_params_json, max_calls, created_by, created_at, updated_at
		FROM dw_tool_permissions WHERE tool_id = ? ORDER BY principal_type, principal_id`), toolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksToolPermission{}
	for rows.Next() {
		var p DataWorksToolPermission
		var allowed int
		var params string
		if err := rows.Scan(&p.ID, &p.ToolID, &p.PrincipalType, &p.PrincipalID, &allowed,
			&params, &p.MaxCalls, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Allowed = allowed != 0
		p.AllowedParameters = platformMap(params)
		out = append(out, p)
	}
	return out, rows.Err()
}
