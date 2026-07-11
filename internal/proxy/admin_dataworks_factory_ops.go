package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"dataworks/internal/store"
)

type factoryReplayRequest struct {
	Model          string  `json:"model"`
	TemplateKey    string  `json:"template_key"`
	PromptVersion  int     `json:"prompt_version"`
	OutputRef      string  `json:"output_ref"`
	PolicyDecision string  `json:"policy_decision"`
	TokenCost      float64 `json:"token_cost"`
	LatencyMS      int     `json:"latency_ms"`
	Reason         string  `json:"reason"`
}

type factoryEvaluateRequest struct {
	AccuracyScore      *int   `json:"accuracy_score"`
	UsefulnessScore    *int   `json:"usefulness_score"`
	RiskScore          *int   `json:"risk_score"`
	OutputQualityScore *int   `json:"output_quality_score"`
	ReviewComment      string `json:"review_comment"`
	Reviewer           string `json:"reviewer"`
}

func (s *Server) handleDataWorksPromptTemplates(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	switch r.Method {
	case http.MethodGet:
		limit := queryInt(r, "limit", 100, 500)
		templates, err := s.db.ListDataWorksPromptTemplates(r.Context(), r.URL.Query().Get("template_key"),
			r.URL.Query().Get("run_type"), r.URL.Query().Get("status"), limit)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "prompt_templates_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"prompt_templates": templates})
	case http.MethodPost:
		var template store.DataWorksPromptTemplate
		if err := json.NewDecoder(r.Body).Decode(&template); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		template.CreatedBy = adminID(r)
		created, err := s.db.InsertDataWorksPromptTemplate(r.Context(), template)
		if err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "prompt_template_invalid")
			return
		}
		s.auditAdmin(r, "dataworks.prompt_template.create", "", auditJSON(map[string]any{
			"template_key": created.TemplateKey, "run_type": created.RunType, "version": created.Version, "status": created.Status,
		}))
		writeJSON(w, http.StatusCreated, map[string]any{"prompt_template": created})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksFactoryRunAction(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	parts := dataWorksPathParts(r.URL.Path, "/admin/dataworks/factory/runs/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		writeOpenAIError(w, http.StatusNotFound, "factory run action not found", "invalid_request_error", "not_found")
		return
	}
	if parts[1] != "regression-test" && r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	run, ok, err := s.db.GetFactoryRun(r.Context(), parts[0])
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "factory_run_failed")
		return
	}
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "factory run not found", "invalid_request_error", "factory_run_not_found")
		return
	}
	switch parts[1] {
	case "replay":
		s.handleDataWorksFactoryRunReplay(w, r, run)
	case "evaluate":
		s.handleDataWorksFactoryRunEvaluate(w, r, run)
	case "regression-test":
		s.handleDataWorksFactoryRunRegressionTest(w, r, run)
	default:
		writeOpenAIError(w, http.StatusNotFound, "factory run action not found", "invalid_request_error", "not_found")
	}
}

func (s *Server) handleDataWorksFactoryRunReplay(w http.ResponseWriter, r *http.Request, source store.FactoryRun) {
	var req factoryReplayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if req.TokenCost < 0 || req.LatencyMS < 0 {
		writeOpenAIError(w, http.StatusBadRequest, "token_cost and latency_ms must be non-negative", "invalid_request_error", "invalid_replay_metrics")
		return
	}
	templateKey := strings.TrimSpace(req.TemplateKey)
	if templateKey == "" {
		templateKey = source.RunType
	}
	template, templateOK, err := s.db.GetDataWorksPromptTemplate(r.Context(), templateKey, req.PromptVersion)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "prompt_template_failed")
		return
	}
	if (strings.TrimSpace(req.TemplateKey) != "" || req.PromptVersion > 0) && !templateOK {
		writeOpenAIError(w, http.StatusNotFound, "prompt template not found", "invalid_request_error", "prompt_template_not_found")
		return
	}
	if templateOK && template.RunType != source.RunType {
		writeOpenAIError(w, http.StatusConflict, "prompt template run_type does not match source run", "invalid_request_error", "prompt_template_run_type_mismatch")
		return
	}
	promptVersion := source.PromptVersion
	if templateOK {
		promptVersion = strconv.Itoa(template.Version)
	}
	outputRef := strings.TrimSpace(req.OutputRef)
	if outputRef == "" {
		outputRef = source.OutputRef
	}
	replay := store.FactoryRun{
		ID:             newID("frun"),
		RunType:        source.RunType,
		Model:          firstNonEmpty(strings.TrimSpace(req.Model), source.Model),
		PromptVersion:  promptVersion,
		InputHash:      source.InputHash,
		OutputRef:      outputRef,
		ParentRunID:    source.ID,
		PolicyDecision: firstNonEmpty(strings.TrimSpace(req.PolicyDecision), "approved_for_replay"),
		TokenCost:      req.TokenCost,
		Status:         "replayed",
		LatencyMS:      req.LatencyMS,
		CreatedBy:      adminID(r),
	}
	if err := s.db.InsertFactoryRun(r.Context(), replay); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "factory_replay_failed")
		return
	}
	stored, _, _ := s.db.GetFactoryRun(r.Context(), replay.ID)
	s.auditAdmin(r, "dataworks.factory_run.replay", auditJSON(source), auditJSON(map[string]any{
		"replay_run_id": replay.ID, "model": replay.Model, "prompt_version": replay.PromptVersion,
		"template_key": templateKey, "reason": strings.TrimSpace(req.Reason),
	}))
	writeJSON(w, http.StatusCreated, map[string]any{
		"source_run":      source,
		"replay_run":      stored,
		"prompt_template": optional(template, templateOK),
		"reproducibility": map[string]any{
			"input_hash_reused": true,
			"output_ref_reused": strings.TrimSpace(req.OutputRef) == "",
			"parent_run_id":     source.ID,
		},
	})
}

func (s *Server) handleDataWorksFactoryRunEvaluate(w http.ResponseWriter, r *http.Request, run store.FactoryRun) {
	var req factoryEvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if req.AccuracyScore == nil || req.UsefulnessScore == nil || req.RiskScore == nil {
		writeOpenAIError(w, http.StatusBadRequest, "accuracy_score, usefulness_score, and risk_score are required", "invalid_request_error", "missing_scores")
		return
	}
	quality := (*req.AccuracyScore + *req.UsefulnessScore + *req.RiskScore) / 3
	if req.OutputQualityScore != nil {
		quality = *req.OutputQualityScore
	}
	score := store.FactoryEvalScore{
		ID: newID("feval"), RunID: run.ID, AccuracyScore: *req.AccuracyScore, UsefulnessScore: *req.UsefulnessScore,
		RiskScore: *req.RiskScore, OutputQualityScore: quality, ReviewComment: strings.TrimSpace(req.ReviewComment),
		Reviewer: firstNonEmpty(strings.TrimSpace(req.Reviewer), adminID(r)),
	}
	if err := s.db.InsertFactoryEvalScore(r.Context(), score); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "factory_evaluation_invalid")
		return
	}
	evaluations, err := s.db.ListFactoryEvalScores(r.Context(), run.ID, 100)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "factory_evaluations_failed")
		return
	}
	for _, stored := range evaluations {
		if stored.ID == score.ID {
			score = stored
			break
		}
	}
	s.auditAdmin(r, "dataworks.factory_run.evaluate", "", auditJSON(map[string]any{
		"run_id": run.ID, "evaluation_id": score.ID, "output_quality_score": score.OutputQualityScore,
	}))
	writeJSON(w, http.StatusCreated, map[string]any{
		"evaluation":         score,
		"evaluation_summary": factoryEvaluationSummary(evaluations),
	})
}

func factoryEvaluationSummary(scores []store.FactoryEvalScore) map[string]any {
	if len(scores) == 0 {
		return map[string]any{"count": 0, "average_output_quality_score": 0}
	}
	accuracy, usefulness, risk, quality := 0, 0, 0, 0
	for _, score := range scores {
		accuracy += score.AccuracyScore
		usefulness += score.UsefulnessScore
		risk += score.RiskScore
		quality += score.OutputQualityScore
	}
	return map[string]any{
		"count":                        len(scores),
		"average_accuracy_score":       accuracy / len(scores),
		"average_usefulness_score":     usefulness / len(scores),
		"average_risk_score":           risk / len(scores),
		"average_output_quality_score": quality / len(scores),
	}
}

func queryInt(r *http.Request, key string, fallback, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func (s *Server) insertDataWorksFactoryRun(ctx context.Context, run store.FactoryRun) error {
	if strings.TrimSpace(run.PromptVersion) == "" {
		if template, ok, err := s.db.GetDataWorksPromptTemplate(ctx, run.RunType, 0); err == nil && ok {
			run.PromptVersion = strconv.Itoa(template.Version)
		}
	}
	if strings.TrimSpace(run.PolicyDecision) == "" {
		run.PolicyDecision = "rules_approved"
	}
	if strings.TrimSpace(run.Status) == "" {
		run.Status = "completed"
	}
	return s.db.InsertFactoryRun(ctx, run)
}
