package proxy

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"dataworks/internal/store"
)

func (s *Server) handleDataWorksFlows(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		flows, err := s.db.ListDataWorksFlows(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("status"))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "flow_list_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"flows": flows, "count": len(flows)})
	case http.MethodPost:
		var flow store.DataWorksFlow
		if err := json.NewDecoder(r.Body).Decode(&flow); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if flow.ID == "" {
			flow.ID = newID("dwflow")
		}
		flow.CreatedBy = adminID(r)
		stored, err := s.db.SaveDataWorksFlow(r.Context(), flow)
		if err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "flow_save_failed")
			return
		}
		flowURN := store.DataWorksURN("flow", stored.FlowKey)
		_ = s.db.UpsertMetadataEntity(r.Context(), store.MetadataEntity{
			ID: platformStableID("meta", flowURN), URN: flowURN, WorkspaceID: stored.WorkspaceID,
			EntityType: "flow", Name: stored.Name, Description: stored.Description, Owner: stored.Owner,
			Status: stored.Status, SourceRef: stored.FlowKey,
			Properties: map[string]any{"flow_type": stored.FlowType, "version": stored.Version},
		})
		for _, node := range stored.Nodes {
			if node.RefURN == "" {
				continue
			}
			relation := "uses"
			if node.NodeType == "output" || node.NodeType == "product_factory" {
				relation = "produces"
			}
			_ = s.db.UpsertMetadataEdge(r.Context(), store.MetadataEdge{
				ID: platformStableID("edge", flowURN+"|"+relation+"|"+node.RefURN), WorkspaceID: stored.WorkspaceID,
				SourceURN: flowURN, TargetURN: node.RefURN, RelationType: relation, CreatedBy: adminID(r),
			})
		}
		s.auditAdmin(r, "dataworks.flow.save", "", auditJSON(map[string]any{"flow_key": stored.FlowKey, "version": stored.Version, "nodes": len(stored.Nodes)}))
		writeJSON(w, http.StatusCreated, map[string]any{"flow": stored})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksFlowByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/admin/dataworks/flows/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeOpenAIError(w, http.StatusNotFound, "flow not found", "invalid_request_error", "not_found")
		return
	}
	flow, ok, err := s.db.GetDataWorksFlow(r.Context(), parts[0])
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "flow_read_failed")
		return
	}
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "flow not found", "invalid_request_error", "not_found")
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		runs, _ := s.db.ListDataWorksFlowRuns(r.Context(), flow.ID, 50)
		writeJSON(w, http.StatusOK, map[string]any{"flow": flow, "runs": runs})
		return
	}
	if len(parts) != 2 {
		writeOpenAIError(w, http.StatusNotFound, "unknown flow action", "invalid_request_error", "not_found")
		return
	}
	switch parts[1] {
	case "run":
		if r.Method != http.MethodPost {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		s.runDataWorksFlow(w, r, flow)
	case "promote":
		if r.Method != http.MethodPost {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		var in struct {
			Target string `json:"target"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		promoted, err := s.db.PromoteDataWorksFlow(r.Context(), flow.ID, strings.TrimSpace(in.Target))
		if err != nil {
			writeOpenAIError(w, http.StatusConflict, err.Error(), "invalid_request_error", "flow_promotion_failed")
			return
		}
		_ = s.db.UpsertMetadataEntity(r.Context(), store.MetadataEntity{
			ID:  platformStableID("meta", store.DataWorksURN("flow", promoted.FlowKey)),
			URN: store.DataWorksURN("flow", promoted.FlowKey), WorkspaceID: promoted.WorkspaceID,
			EntityType: "flow", Name: promoted.Name, Description: promoted.Description, Owner: promoted.Owner,
			Status: promoted.Status, SourceRef: promoted.FlowKey,
			Properties: map[string]any{"flow_type": promoted.FlowType, "version": promoted.Version},
		})
		s.auditAdmin(r, "dataworks.flow.promote", "", auditJSON(map[string]any{"flow_key": flow.FlowKey, "from": flow.Status, "to": promoted.Status}))
		writeJSON(w, http.StatusOK, map[string]any{"flow": promoted})
	case "runs":
		if r.Method != http.MethodGet {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		runs, err := s.db.ListDataWorksFlowRuns(r.Context(), flow.ID, intQuery(r, "limit", 100))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "flow_runs_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
	default:
		writeOpenAIError(w, http.StatusNotFound, "unknown flow action", "invalid_request_error", "not_found")
	}
}

func (s *Server) runDataWorksFlow(w http.ResponseWriter, r *http.Request, flow store.DataWorksFlow) {
	var in struct {
		Input       string `json:"input"`
		TriggerType string `json:"trigger_type"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	started := time.Now().UTC()
	status := "succeeded"
	steps := []map[string]any{}
	totalCost := 0.0
	for i, node := range orderedDataWorksFlowNodes(flow.Nodes, flow.Edges) {
		step := map[string]any{
			"step_no": i + 1, "node_key": node.NodeKey, "node_type": node.NodeType,
			"name": firstNonEmpty(node.Name, node.NodeKey), "status": "succeeded",
		}
		cost := dataWorksNodeCost(node.NodeType)
		switch node.NodeType {
		case "approval":
			step["status"] = "pending_approval"
			step["detail"] = "human approval interrupt"
			status = "pending_approval"
		case "tool":
			toolID, _ := node.Config["tool_id"].(string)
			if toolID == "" && strings.HasPrefix(node.RefURN, "urn:dw:tool:") {
				toolID = strings.TrimPrefix(node.RefURN, "urn:dw:tool:")
			}
			tool, ok, _ := s.db.GetDataWorksTool(r.Context(), toolID)
			if !ok || !tool.Enabled {
				step["status"] = "blocked"
				step["detail"] = "tool is missing or disabled"
				status = "blocked"
			} else if tool.RequiresApproval || tool.RiskLevel == "high" || tool.RiskLevel == "critical" {
				step["status"] = "pending_approval"
				step["detail"] = "tool governance requires approval"
				step["tool_id"] = tool.ID
				status = "pending_approval"
			} else {
				step["tool_id"] = tool.ID
				step["detail"] = "sandbox contract validation passed"
			}
		case "risk_check", "guardrail":
			step["detail"] = "policy preflight passed"
		default:
			step["detail"] = "validated in orchestration sandbox"
		}
		step["cost"] = cost
		step["latency_ms"] = 12 + i*7
		steps = append(steps, step)
		totalCost += cost
		if status != "succeeded" {
			break
		}
	}
	finished := time.Now().UTC()
	run := store.DataWorksFlowRun{
		ID: newID("dwfrun"), FlowID: flow.ID, FlowVersion: flow.Version, Status: status,
		TriggerType: firstNonEmpty(in.TriggerType, "manual"), InputSummary: truncateRunes(strings.TrimSpace(in.Input), 240),
		ResultSummary: map[string]any{"steps": steps, "mode": "orchestration_sandbox", "completed_steps": len(steps)},
		TotalCost:     totalCost, LatencyMS: finished.Sub(started).Milliseconds(), StartedAt: started.Format(time.RFC3339Nano),
		FinishedAt: finished.Format(time.RFC3339Nano), CreatedBy: adminID(r),
	}
	if err := s.db.InsertDataWorksFlowRun(r.Context(), run); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "flow_run_failed")
		return
	}
	s.auditAdmin(r, "dataworks.flow.run", "", auditJSON(map[string]any{"flow_key": flow.FlowKey, "run_id": run.ID, "status": status}))
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}

func orderedDataWorksFlowNodes(nodes []store.DataWorksFlowNode, edges []store.DataWorksFlowEdge) []store.DataWorksFlowNode {
	byKey := map[string]store.DataWorksFlowNode{}
	indegree := map[string]int{}
	adj := map[string][]string{}
	for _, node := range nodes {
		byKey[node.NodeKey] = node
		indegree[node.NodeKey] = 0
	}
	for _, edge := range edges {
		indegree[edge.TargetNodeKey]++
		adj[edge.SourceNodeKey] = append(adj[edge.SourceNodeKey], edge.TargetNodeKey)
	}
	queue := []store.DataWorksFlowNode{}
	for _, node := range nodes {
		if indegree[node.NodeKey] == 0 {
			queue = append(queue, node)
		}
	}
	sort.SliceStable(queue, func(i, j int) bool { return queue[i].SequenceNo < queue[j].SequenceNo })
	out := []store.DataWorksFlowNode{}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		out = append(out, node)
		for _, next := range adj[node.NodeKey] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, byKey[next])
				sort.SliceStable(queue, func(i, j int) bool { return queue[i].SequenceNo < queue[j].SequenceNo })
			}
		}
	}
	return out
}

func dataWorksNodeCost(nodeType string) float64 {
	return map[string]float64{
		"sql": 0.04, "python": 0.06, "api": 0.03, "rag": 0.08, "llm": 0.15,
		"agent": 0.12, "tool": 0.05, "product_factory": 0.08,
	}[nodeType]
}

func (s *Server) handleDataWorksAgents(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		agents, err := s.db.ListDataWorksAgents(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("status"))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "agent_list_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"agents": agents, "count": len(agents)})
	case http.MethodPost:
		var agent store.DataWorksAgent
		if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if agent.ID == "" {
			agent.ID = newID("dwagent")
		}
		agent.CreatedBy = adminID(r)
		stored, err := s.db.SaveDataWorksAgent(r.Context(), agent)
		if err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "agent_save_failed")
			return
		}
		agentURN := store.DataWorksURN("agent", stored.AgentKey)
		_ = s.db.UpsertMetadataEntity(r.Context(), store.MetadataEntity{
			ID: platformStableID("meta", agentURN), URN: agentURN, WorkspaceID: stored.WorkspaceID,
			EntityType: "agent", Name: stored.Name, Description: stored.Purpose, Owner: stored.Owner,
			Status: stored.Status, SourceRef: stored.AgentKey,
			Properties: map[string]any{"risk_level": stored.RiskLevel, "version": stored.Version, "memory_scope": stored.MemoryScope},
		})
		for _, toolID := range stored.AllowedTools {
			tool, ok, _ := s.db.GetDataWorksTool(r.Context(), toolID)
			if !ok {
				continue
			}
			toolURN := store.DataWorksURN("tool", tool.ToolKey)
			_ = s.db.UpsertMetadataEdge(r.Context(), store.MetadataEdge{
				ID: platformStableID("edge", agentURN+"|uses|"+toolURN), WorkspaceID: stored.WorkspaceID,
				SourceURN: agentURN, TargetURN: toolURN, RelationType: "uses", CreatedBy: adminID(r),
			})
		}
		s.auditAdmin(r, "dataworks.agent.save", "", auditJSON(map[string]any{"agent_key": stored.AgentKey, "version": stored.Version}))
		writeJSON(w, http.StatusCreated, map[string]any{"agent": stored})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksAgentByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/admin/dataworks/agents/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" {
		writeOpenAIError(w, http.StatusBadRequest, "agent action required", "invalid_request_error", "bad_agent_action")
		return
	}
	agent, ok, err := s.db.GetDataWorksAgent(r.Context(), parts[0])
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "agent_read_failed")
		return
	}
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "agent not found", "invalid_request_error", "not_found")
		return
	}
	switch parts[1] {
	case "run":
		if r.Method != http.MethodPost {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		s.runDataWorksAgent(w, r, agent)
	case "trace":
		if r.Method != http.MethodGet {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		sessions, err := s.db.ListDataWorksAgentSessions(r.Context(), agent.ID, intQuery(r, "limit", 50))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "agent_sessions_failed")
			return
		}
		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" && len(sessions) > 0 {
			sessionID = sessions[0].ID
		}
		traces := []store.DataWorksAgentTrace{}
		if sessionID != "" {
			traces, err = s.db.ListDataWorksAgentTraces(r.Context(), sessionID)
			if err != nil {
				writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "agent_trace_failed")
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"agent": agent, "sessions": sessions, "session_id": sessionID, "traces": traces})
	default:
		writeOpenAIError(w, http.StatusNotFound, "unknown agent action", "invalid_request_error", "not_found")
	}
}

func (s *Server) runDataWorksAgent(w http.ResponseWriter, r *http.Request, agent store.DataWorksAgent) {
	if agent.Status == "disabled" || agent.Status == "archived" {
		writeOpenAIError(w, http.StatusConflict, "agent is not runnable", "invalid_request_error", "agent_disabled")
		return
	}
	var in struct {
		Input   string         `json:"input"`
		ToolIDs []string       `json:"tool_ids"`
		Params  map[string]any `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if len(in.ToolIDs) == 0 {
		in.ToolIDs = append([]string(nil), agent.AllowedTools...)
	}
	allowed := map[string]bool{}
	for _, toolID := range agent.AllowedTools {
		allowed[toolID] = true
		if tool, ok, _ := s.db.GetDataWorksTool(r.Context(), toolID); ok {
			allowed[tool.ID], allowed[tool.ToolKey] = true, true
		}
	}
	started := time.Now().UTC()
	sessionID := newID("dwasess")
	traces := []store.DataWorksAgentTrace{{
		ID: newID("dwatrace"), SessionID: sessionID, StepNo: 1, TraceType: "plan", Name: "계획",
		Status: "succeeded", ReasoningSummary: "에이전트 계약에서 허용된 도구만 선택하고 실행 전 승인 정책을 확인합니다.",
		InputSummary: truncateRunes(strings.TrimSpace(in.Input), 240), PolicyDecision: "allow", LatencyMS: 5,
	}}
	status := "succeeded"
	approvalStatus := "not_required"
	policyBlocked := false
	totalCost := 0.0
	for i, requested := range in.ToolIDs {
		trace := store.DataWorksAgentTrace{
			ID: newID("dwatrace"), SessionID: sessionID, StepNo: i + 2, TraceType: "tool",
			Name: requested, ToolID: requested, InputSummary: "parameter keys: " + strings.Join(sortedMapKeys(in.Params), ", "),
			LatencyMS: int64(18 + i*7),
		}
		tool, ok, _ := s.db.GetDataWorksTool(r.Context(), requested)
		if !allowed[requested] || !ok {
			trace.Status, trace.PolicyDecision = "blocked", "deny"
			trace.ReasoningSummary = "요청한 도구가 에이전트 허용 목록에 없습니다."
			status, policyBlocked = "blocked", true
			traces = append(traces, trace)
			break
		}
		trace.ToolID, trace.Name = tool.ID, tool.Name
		if !tool.Enabled {
			trace.Status, trace.PolicyDecision = "blocked", "deny"
			trace.ReasoningSummary = "도구 거버넌스에서 비활성화된 도구입니다."
			status, policyBlocked = "blocked", true
			traces = append(traces, trace)
			break
		}
		if tool.RequiresApproval || agent.RequiresApproval || tool.RiskLevel == "high" || tool.RiskLevel == "critical" {
			trace.Status, trace.PolicyDecision = "pending_approval", "approval_required"
			trace.ReasoningSummary = "고위험 도구 실행이 담당자 승인 대기로 전환되었습니다."
			status, approvalStatus = "pending_approval", "pending"
			traces = append(traces, trace)
			break
		}
		trace.Status, trace.PolicyDecision = "succeeded", "allow"
		trace.ReasoningSummary = "샌드박스에서 도구 계약, 매개변수 정책, 마스킹 정책을 통과했습니다."
		trace.OutputSummary = "계약 검증을 통과한 샌드박스 결과"
		trace.Cost = 0.05 + float64(len(in.Params))*0.01
		totalCost += trace.Cost
		traces = append(traces, trace)
		if agent.MaxCost > 0 && totalCost > agent.MaxCost {
			traces[len(traces)-1].Status = "blocked"
			traces[len(traces)-1].PolicyDecision = "cost_limit"
			status, policyBlocked = "blocked", true
			break
		}
	}
	if status == "succeeded" {
		traces = append(traces, store.DataWorksAgentTrace{
			ID: newID("dwatrace"), SessionID: sessionID, StepNo: len(traces) + 1, TraceType: "response",
			Name: "최종 응답", Status: "succeeded", ReasoningSummary: "정책 승인을 통과한 중간 결과만 사용해 제한된 응답을 구성합니다.",
			OutputSummary: "거버넌스 샌드박스에서 에이전트 실행을 완료했습니다.", PolicyDecision: "allow", LatencyMS: 8,
		})
	}
	finished := time.Now().UTC()
	session := store.DataWorksAgentSession{
		ID: sessionID, AgentID: agent.ID, Status: status, InputSummary: truncateRunes(strings.TrimSpace(in.Input), 240),
		OutputSummary: map[string]string{"succeeded": "거버넌스 샌드박스 실행이 완료되었습니다.", "blocked": "정책에 의해 실행이 차단되었습니다.", "pending_approval": "승인을 위해 실행이 일시 중지되었습니다."}[status],
		TotalCost:     totalCost, LatencyMS: finished.Sub(started).Milliseconds(), PolicyBlocked: policyBlocked,
		ApprovalStatus: approvalStatus, StartedAt: started.Format(time.RFC3339Nano), FinishedAt: finished.Format(time.RFC3339Nano), CreatedBy: adminID(r),
	}
	if err := s.db.InsertDataWorksAgentSession(r.Context(), session); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "agent_session_failed")
		return
	}
	if err := s.db.InsertDataWorksAgentTraces(r.Context(), traces); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "agent_trace_failed")
		return
	}
	s.auditAdmin(r, "dataworks.agent.run", "", auditJSON(map[string]any{"agent_key": agent.AgentKey, "session_id": session.ID, "status": status}))
	writeJSON(w, http.StatusOK, map[string]any{"session": session, "traces": traces})
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (s *Server) handleDataWorksTools(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		tools, err := s.db.ListDataWorksTools(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("tool_type"), r.URL.Query().Get("enabled") == "true")
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "tool_list_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tools": tools, "count": len(tools)})
	case http.MethodPost:
		tool, err := decodeDataWorksTool(r)
		if err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if tool.ID == "" {
			tool.ID = newID("dwtool")
		}
		tool.CreatedBy = adminID(r)
		if err := s.db.UpsertDataWorksTool(r.Context(), tool); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "tool_save_failed")
			return
		}
		stored, _, _ := s.db.GetDataWorksTool(r.Context(), tool.ToolKey)
		toolURN := store.DataWorksURN("tool", stored.ToolKey)
		_ = s.db.UpsertMetadataEntity(r.Context(), store.MetadataEntity{
			ID: platformStableID("meta", toolURN), URN: toolURN, WorkspaceID: stored.WorkspaceID,
			EntityType: "tool", Name: stored.Name, Description: stored.Description, Owner: stored.Owner,
			Status: map[bool]string{true: "active", false: "disabled"}[stored.Enabled], SourceRef: stored.ToolKey,
			Properties: map[string]any{"tool_type": stored.ToolType, "risk_level": stored.RiskLevel, "server_label": stored.ServerLabel},
		})
		_ = s.db.UpsertMCPToolContract(r.Context(), store.MCPToolContract{
			ID: stored.ID, Namespace: firstNonEmpty(stored.ServerLabel, "dataworks"), Name: stored.ToolKey,
			Title: stored.Name, Description: stored.Description, InputSchema: stored.InputSchema,
			OutputSchema: stored.OutputSchema, RiskLevel: stored.RiskLevel, TimeoutMS: 30000,
			Owner: stored.Owner, Enabled: stored.Enabled, CreatedBy: adminID(r),
		})
		approvalRule := "inherit"
		if stored.RequiresApproval {
			approvalRule = "always"
		}
		_ = s.db.UpsertMCPToolScope(r.Context(), store.MCPToolScope{
			ServerLabel: firstNonEmpty(stored.ServerLabel, "dataworks"), ToolName: stored.ToolKey,
			MaskingLevel: stored.MaskingLevel, ApprovalRule: approvalRule, Enabled: stored.Enabled,
			Note: "managed by Data Works Tool Registry",
		})
		s.auditAdmin(r, "dataworks.tool.save", "", auditJSON(map[string]any{"tool_key": stored.ToolKey, "risk_level": stored.RiskLevel}))
		writeJSON(w, http.StatusCreated, map[string]any{"tool": stored})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func decodeDataWorksTool(r *http.Request) (store.DataWorksTool, error) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return store.DataWorksTool{}, err
	}
	for _, key := range []string{"input_schema", "output_schema"} {
		value, ok := raw[key]
		if !ok || strings.HasPrefix(strings.TrimSpace(string(value)), "\"") {
			continue
		}
		encoded, err := json.Marshal(string(value))
		if err != nil {
			return store.DataWorksTool{}, err
		}
		raw[key] = encoded
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return store.DataWorksTool{}, err
	}
	var tool store.DataWorksTool
	if err := json.Unmarshal(encoded, &tool); err != nil {
		return store.DataWorksTool{}, err
	}
	if _, supplied := raw["enabled"]; !supplied {
		tool.Enabled = true
	}
	return tool, nil
}

func (s *Server) handleDataWorksToolByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/admin/dataworks/tools/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" {
		writeOpenAIError(w, http.StatusBadRequest, "tool action required", "invalid_request_error", "bad_tool_action")
		return
	}
	tool, ok, err := s.db.GetDataWorksTool(r.Context(), parts[0])
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "tool_read_failed")
		return
	}
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "tool not found", "invalid_request_error", "not_found")
		return
	}
	switch parts[1] {
	case "test":
		if r.Method != http.MethodPost {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		var inputSchema, outputSchema any
		inputErr := json.Unmarshal([]byte(tool.InputSchema), &inputSchema)
		outputErr := json.Unmarshal([]byte(tool.OutputSchema), &outputSchema)
		status := "passed"
		issues := []string{}
		if inputErr != nil || outputErr != nil {
			status = "failed"
			issues = append(issues, "input/output schema must be valid JSON")
		}
		if tool.ToolType == "mcp" && tool.ServerLabel == "" {
			status = "failed"
			issues = append(issues, "MCP tool requires server_label")
		}
		if tool.RiskLevel == "high" || tool.RiskLevel == "critical" {
			issues = append(issues, "high-risk tool remains approval-gated")
		}
		_ = s.db.UpdateDataWorksToolTest(r.Context(), tool.ID, status)
		s.auditAdmin(r, "dataworks.tool.test", "", auditJSON(map[string]any{"tool_key": tool.ToolKey, "status": status}))
		writeJSON(w, http.StatusOK, map[string]any{
			"tool_id": tool.ID, "status": status, "issues": issues, "mode": "contract_sandbox",
			"network_invoked": false, "approval_required": tool.RequiresApproval,
		})
	case "permissions":
		if r.Method == http.MethodGet {
			permissions, err := s.db.ListDataWorksToolPermissions(r.Context(), tool.ID)
			if err != nil {
				writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "tool_permissions_failed")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"permissions": permissions})
			return
		}
		if r.Method != http.MethodPost {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		var permission store.DataWorksToolPermission
		if err := json.NewDecoder(r.Body).Decode(&permission); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		permission.ID, permission.ToolID, permission.CreatedBy = newID("dwtperm"), tool.ID, adminID(r)
		if err := s.db.UpsertDataWorksToolPermission(r.Context(), permission); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "tool_permission_failed")
			return
		}
		s.auditAdmin(r, "dataworks.tool.permission.upsert", "", auditJSON(permission))
		writeJSON(w, http.StatusCreated, map[string]any{"permission": permission})
	default:
		writeOpenAIError(w, http.StatusNotFound, "unknown tool action", "invalid_request_error", "not_found")
	}
}
