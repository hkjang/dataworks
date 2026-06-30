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

type DataAssetReadinessScore struct {
	AssetKey         string   `json:"asset_key"`
	QualityScore     int      `json:"quality_score"`
	FreshnessScore   int      `json:"freshness_score"`
	OwnerScore       int      `json:"owner_score"`
	MetadataScore    int      `json:"metadata_score"`
	SensitivityScore int      `json:"sensitivity_score"`
	ApprovalScore    int      `json:"approval_score"`
	SampleScore      int      `json:"sample_score"`
	OverallScore     int      `json:"overall_score"`
	Status           string   `json:"status"`
	Blockers         []string `json:"blockers"`
	CheckedBy        string   `json:"checked_by"`
	LastCheckedAt    string   `json:"last_checked_at"`
}

type ProductCanvas struct {
	ProductKey        string   `json:"product_key"`
	CustomerProblem   string   `json:"customer_problem"`
	TargetSegment     string   `json:"target_segment"`
	ValueProposition  string   `json:"value_proposition"`
	DataInputs        []string `json:"data_inputs"`
	DeliveryModel     string   `json:"delivery_model"`
	PricingHypothesis string   `json:"pricing_hypothesis"`
	RiskPosture       string   `json:"risk_posture"`
	POCSuccessMetric  string   `json:"poc_success_metric"`
	CreatedBy         string   `json:"created_by"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

type ProductEvidence struct {
	ID              string `json:"id"`
	ProductKey      string `json:"product_key"`
	EvidenceType    string `json:"evidence_type"`
	SourceRef       string `json:"source_ref"`
	Summary         string `json:"summary"`
	ConfidenceScore int    `json:"confidence_score"`
	CreatedBy       string `json:"created_by"`
	CreatedAt       string `json:"created_at"`
}

type RegulatoryTrace struct {
	ID         string `json:"id"`
	ProductKey string `json:"product_key"`
	RiskDomain string `json:"risk_domain"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
	Evidence   string `json:"evidence"`
	Decision   string `json:"decision"`
	Reviewer   string `json:"reviewer"`
	CreatedAt  string `json:"created_at"`
}

type APIContract struct {
	ID            string `json:"id"`
	ProductKey    string `json:"product_key"`
	OpenAPIJSON   string `json:"openapi_json"`
	SLAPolicy     string `json:"sla_policy"`
	RateLimit     string `json:"rate_limit"`
	MaskingPolicy string `json:"masking_policy"`
	Version       int    `json:"version"`
	CreatedBy     string `json:"created_by"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type MockAPILog struct {
	ID           string `json:"id"`
	ProductKey   string `json:"product_key"`
	CustomerType string `json:"customer_type"`
	RequestHash  string `json:"request_hash"`
	LatencyMS    int    `json:"latency_ms"`
	CreatedAt    string `json:"created_at"`
}

type ProposalFeedback struct {
	ID                 string `json:"id"`
	ProposalID         string `json:"proposal_id"`
	ProductKey         string `json:"product_key"`
	CustomerType       string `json:"customer_type"`
	CustomerNameMasked string `json:"customer_name_masked"`
	Result             string `json:"result"`
	Reason             string `json:"reason"`
	NextAction         string `json:"next_action"`
	CreatedBy          string `json:"created_by"`
	CreatedAt          string `json:"created_at"`
}

type POCOutcome struct {
	ID               string `json:"id"`
	POCID            string `json:"poc_id"`
	ProductKey       string `json:"product_key"`
	Success          bool   `json:"success"`
	MetricResult     string `json:"metric_result"`
	CustomerFeedback string `json:"customer_feedback"`
	ConversionStatus string `json:"conversion_status"`
	CreatedBy        string `json:"created_by"`
	CreatedAt        string `json:"created_at"`
}

type DataWorksFunnel struct {
	Ideas            int64 `json:"ideas"`
	Definitions      int64 `json:"definitions"`
	RiskReviews      int64 `json:"risk_reviews"`
	Proposals        int64 `json:"proposals"`
	ProposalFeedback int64 `json:"proposal_feedback"`
	POCPlans         int64 `json:"poc_plans"`
	POCOutcomes      int64 `json:"poc_outcomes"`
	Published        int64 `json:"published"`
}

func (s *SQLStore) GetDataAsset(ctx context.Context, assetKey string) (DataAsset, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, asset_key, name, domain, owner, columns_summary, sensitivity, refresh_cycle, created_at, updated_at
		FROM data_assets WHERE asset_key = ?`), assetKey)
	var a DataAsset
	err := row.Scan(&a.ID, &a.AssetKey, &a.Name, &a.Domain, &a.Owner, &a.ColumnsSummary, &a.Sensitivity, &a.RefreshCycle, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DataAsset{}, false, nil
	}
	if err != nil {
		return DataAsset{}, false, err
	}
	return a, true, nil
}

func (s *SQLStore) UpsertDataAssetReadinessScore(ctx context.Context, score DataAssetReadinessScore) error {
	if strings.TrimSpace(score.AssetKey) == "" {
		return errors.New("asset_key is required")
	}
	now := formatTime(time.Now().UTC())
	if score.LastCheckedAt == "" {
		score.LastCheckedAt = now
	}
	blockersJSON, _ := json.Marshal(score.Blockers)
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_asset_quality_scores
		(asset_key, quality_score, freshness_score, owner_score, metadata_score, sensitivity_score, approval_score,
		 sample_score, overall_score, status, blockers_json, checked_by, last_checked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(asset_key) DO UPDATE SET
			quality_score = excluded.quality_score,
			freshness_score = excluded.freshness_score,
			owner_score = excluded.owner_score,
			metadata_score = excluded.metadata_score,
			sensitivity_score = excluded.sensitivity_score,
			approval_score = excluded.approval_score,
			sample_score = excluded.sample_score,
			overall_score = excluded.overall_score,
			status = excluded.status,
			blockers_json = excluded.blockers_json,
			checked_by = excluded.checked_by,
			last_checked_at = excluded.last_checked_at`),
		score.AssetKey, score.QualityScore, score.FreshnessScore, score.OwnerScore, score.MetadataScore,
		score.SensitivityScore, score.ApprovalScore, score.SampleScore, score.OverallScore, score.Status,
		string(blockersJSON), score.CheckedBy, score.LastCheckedAt)
	return err
}

func (s *SQLStore) LatestDataAssetReadinessScore(ctx context.Context, assetKey string) (DataAssetReadinessScore, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT asset_key, quality_score, freshness_score, owner_score, metadata_score,
			sensitivity_score, approval_score, sample_score, overall_score, status, COALESCE(blockers_json, '[]'), checked_by, last_checked_at
		FROM dw_asset_quality_scores WHERE asset_key = ?`), assetKey)
	var score DataAssetReadinessScore
	var blockersJSON string
	err := row.Scan(&score.AssetKey, &score.QualityScore, &score.FreshnessScore, &score.OwnerScore, &score.MetadataScore,
		&score.SensitivityScore, &score.ApprovalScore, &score.SampleScore, &score.OverallScore, &score.Status,
		&blockersJSON, &score.CheckedBy, &score.LastCheckedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DataAssetReadinessScore{}, false, nil
	}
	if err != nil {
		return DataAssetReadinessScore{}, false, err
	}
	_ = json.Unmarshal([]byte(blockersJSON), &score.Blockers)
	return score, true, nil
}

func (s *SQLStore) UpsertProductCanvas(ctx context.Context, c ProductCanvas) error {
	if strings.TrimSpace(c.ProductKey) == "" {
		return errors.New("product_key is required")
	}
	now := formatTime(time.Now().UTC())
	if c.CreatedAt == "" {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	inputsJSON, _ := json.Marshal(c.DataInputs)
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_product_canvas
		(product_key, customer_problem, target_segment, value_proposition, data_inputs_json, delivery_model,
		 pricing_hypothesis, risk_posture, poc_success_metric, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(product_key) DO UPDATE SET
			customer_problem = excluded.customer_problem,
			target_segment = excluded.target_segment,
			value_proposition = excluded.value_proposition,
			data_inputs_json = excluded.data_inputs_json,
			delivery_model = excluded.delivery_model,
			pricing_hypothesis = excluded.pricing_hypothesis,
			risk_posture = excluded.risk_posture,
			poc_success_metric = excluded.poc_success_metric,
			created_by = excluded.created_by,
			updated_at = excluded.updated_at`),
		c.ProductKey, c.CustomerProblem, c.TargetSegment, c.ValueProposition, string(inputsJSON), c.DeliveryModel,
		c.PricingHypothesis, c.RiskPosture, c.POCSuccessMetric, c.CreatedBy, c.CreatedAt, c.UpdatedAt)
	return err
}

func (s *SQLStore) GetProductCanvas(ctx context.Context, productKey string) (ProductCanvas, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT product_key, customer_problem, target_segment, value_proposition,
			COALESCE(data_inputs_json, '[]'), delivery_model, pricing_hypothesis, risk_posture, poc_success_metric,
			created_by, created_at, updated_at
		FROM dw_product_canvas WHERE product_key = ?`), productKey)
	var c ProductCanvas
	var inputsJSON string
	err := row.Scan(&c.ProductKey, &c.CustomerProblem, &c.TargetSegment, &c.ValueProposition, &inputsJSON,
		&c.DeliveryModel, &c.PricingHypothesis, &c.RiskPosture, &c.POCSuccessMetric, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductCanvas{}, false, nil
	}
	if err != nil {
		return ProductCanvas{}, false, err
	}
	_ = json.Unmarshal([]byte(inputsJSON), &c.DataInputs)
	return c, true, nil
}

func (s *SQLStore) ReplaceProductEvidencePack(ctx context.Context, productKey string, evidence []ProductEvidence) error {
	if strings.TrimSpace(productKey) == "" {
		return errors.New("product_key is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, s.bind(`DELETE FROM dw_product_evidence WHERE product_key = ?`), productKey); err != nil {
		return err
	}
	now := formatTime(time.Now().UTC())
	for i := range evidence {
		e := evidence[i]
		if e.ID == "" {
			e.ID = fmt.Sprintf("evid_%d_%d", time.Now().UTC().UnixNano(), i)
		}
		e.ProductKey = productKey
		if e.CreatedAt == "" {
			e.CreatedAt = now
		}
		if _, err := tx.ExecContext(ctx, s.bind(`INSERT INTO dw_product_evidence
			(id, product_key, evidence_type, source_ref, summary, confidence_score, created_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
			e.ID, e.ProductKey, e.EvidenceType, e.SourceRef, e.Summary, e.ConfidenceScore, e.CreatedBy, e.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLStore) ListProductEvidence(ctx context.Context, productKey string) ([]ProductEvidence, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT id, product_key, evidence_type, source_ref, summary, confidence_score, created_by, created_at
		FROM dw_product_evidence WHERE product_key = ? ORDER BY evidence_type, created_at DESC`), productKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProductEvidence{}
	for rows.Next() {
		var e ProductEvidence
		if err := rows.Scan(&e.ID, &e.ProductKey, &e.EvidenceType, &e.SourceRef, &e.Summary, &e.ConfidenceScore, &e.CreatedBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLStore) ReplaceRegulatoryTrace(ctx context.Context, productKey string, rowsIn []RegulatoryTrace) error {
	if strings.TrimSpace(productKey) == "" {
		return errors.New("product_key is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, s.bind(`DELETE FROM dw_regulatory_trace WHERE product_key = ?`), productKey); err != nil {
		return err
	}
	now := formatTime(time.Now().UTC())
	for i := range rowsIn {
		trace := rowsIn[i]
		if trace.ID == "" {
			trace.ID = fmt.Sprintf("rtrace_%d_%d", time.Now().UTC().UnixNano(), i)
		}
		trace.ProductKey = productKey
		if trace.CreatedAt == "" {
			trace.CreatedAt = now
		}
		if _, err := tx.ExecContext(ctx, s.bind(`INSERT INTO dw_regulatory_trace
			(id, product_key, risk_domain, question, answer, evidence, decision, reviewer, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			trace.ID, trace.ProductKey, trace.RiskDomain, trace.Question, trace.Answer, trace.Evidence, trace.Decision,
			trace.Reviewer, trace.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLStore) ListRegulatoryTrace(ctx context.Context, productKey string) ([]RegulatoryTrace, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT id, product_key, risk_domain, question, answer, evidence, decision, reviewer, created_at
		FROM dw_regulatory_trace WHERE product_key = ? ORDER BY risk_domain, created_at DESC`), productKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RegulatoryTrace{}
	for rows.Next() {
		var trace RegulatoryTrace
		if err := rows.Scan(&trace.ID, &trace.ProductKey, &trace.RiskDomain, &trace.Question, &trace.Answer,
			&trace.Evidence, &trace.Decision, &trace.Reviewer, &trace.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, trace)
	}
	return out, rows.Err()
}

func (s *SQLStore) InsertProductAPIContract(ctx context.Context, c APIContract) (int, error) {
	if strings.TrimSpace(c.ProductKey) == "" {
		return 0, errors.New("product_key is required")
	}
	if c.ID == "" {
		c.ID = fmt.Sprintf("apic_%d", time.Now().UTC().UnixNano())
	}
	if c.Version <= 0 {
		_ = s.db.QueryRowContext(ctx, s.bind(`SELECT COALESCE(MAX(version), 0) + 1 FROM dw_api_contracts WHERE product_key = ?`), c.ProductKey).Scan(&c.Version)
	}
	if c.Version <= 0 {
		c.Version = 1
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_api_contracts
		(id, product_key, openapi_json, sla_policy, rate_limit, masking_policy, version, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		c.ID, c.ProductKey, c.OpenAPIJSON, c.SLAPolicy, c.RateLimit, c.MaskingPolicy, c.Version, c.CreatedBy, now, now)
	return c.Version, err
}

func (s *SQLStore) LatestProductAPIContract(ctx context.Context, productKey string) (APIContract, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, product_key, openapi_json, sla_policy, rate_limit, masking_policy,
			version, created_by, created_at, updated_at
		FROM dw_api_contracts WHERE product_key = ? ORDER BY version DESC LIMIT 1`), productKey)
	var c APIContract
	err := row.Scan(&c.ID, &c.ProductKey, &c.OpenAPIJSON, &c.SLAPolicy, &c.RateLimit, &c.MaskingPolicy,
		&c.Version, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return APIContract{}, false, nil
	}
	if err != nil {
		return APIContract{}, false, err
	}
	return c, true, nil
}

func (s *SQLStore) InsertMockAPILog(ctx context.Context, log MockAPILog) error {
	if log.ID == "" {
		log.ID = fmt.Sprintf("mock_%d", time.Now().UTC().UnixNano())
	}
	if log.CreatedAt == "" {
		log.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_mock_api_logs
		(id, product_key, customer_type, request_hash, latency_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`),
		log.ID, log.ProductKey, log.CustomerType, log.RequestHash, log.LatencyMS, log.CreatedAt)
	return err
}

func (s *SQLStore) GetProposalPackage(ctx context.Context, id string) (ProposalPackage, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, product_key, target_customer_type, proposal_json, generated_file_ref, created_by, created_at
		FROM proposal_packages WHERE id = ?`), id)
	var p ProposalPackage
	err := row.Scan(&p.ID, &p.ProductKey, &p.TargetCustomerType, &p.ProposalJSON, &p.GeneratedFileRef, &p.CreatedBy, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProposalPackage{}, false, nil
	}
	if err != nil {
		return ProposalPackage{}, false, err
	}
	return p, true, nil
}

func (s *SQLStore) InsertProposalFeedback(ctx context.Context, fb ProposalFeedback) error {
	if fb.ID == "" {
		fb.ID = fmt.Sprintf("pfb_%d", time.Now().UTC().UnixNano())
	}
	if fb.CreatedAt == "" {
		fb.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_proposal_feedback
		(id, proposal_id, product_key, customer_type, customer_name_masked, result, reason, next_action, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		fb.ID, fb.ProposalID, fb.ProductKey, fb.CustomerType, fb.CustomerNameMasked, fb.Result, fb.Reason,
		fb.NextAction, fb.CreatedBy, fb.CreatedAt)
	return err
}

func (s *SQLStore) ListProposalFeedback(ctx context.Context, productKey string) ([]ProposalFeedback, error) {
	q := `SELECT id, proposal_id, product_key, customer_type, customer_name_masked, result, reason, next_action, created_by, created_at
		FROM dw_proposal_feedback`
	args := []any{}
	if productKey != "" {
		q += ` WHERE product_key = ?`
		args = append(args, productKey)
	}
	q += ` ORDER BY created_at DESC LIMIT 200`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProposalFeedback{}
	for rows.Next() {
		var fb ProposalFeedback
		if err := rows.Scan(&fb.ID, &fb.ProposalID, &fb.ProductKey, &fb.CustomerType, &fb.CustomerNameMasked,
			&fb.Result, &fb.Reason, &fb.NextAction, &fb.CreatedBy, &fb.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, fb)
	}
	return out, rows.Err()
}

func (s *SQLStore) GetProductPOCPlan(ctx context.Context, id string) (ProductPOCPlan, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, product_key, data_scope, success_metric, timeline, owner, approval_status,
			plan_json, created_by, created_at, updated_at
		FROM product_poc_plans WHERE id = ?`), id)
	var p ProductPOCPlan
	err := row.Scan(&p.ID, &p.ProductKey, &p.DataScope, &p.SuccessMetric, &p.Timeline, &p.Owner, &p.ApprovalStatus,
		&p.PlanJSON, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductPOCPlan{}, false, nil
	}
	if err != nil {
		return ProductPOCPlan{}, false, err
	}
	return p, true, nil
}

func (s *SQLStore) InsertPOCOutcome(ctx context.Context, outcome POCOutcome) error {
	if outcome.ID == "" {
		outcome.ID = fmt.Sprintf("pocout_%d", time.Now().UTC().UnixNano())
	}
	if outcome.CreatedAt == "" {
		outcome.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_poc_outcomes
		(id, poc_id, product_key, success_yn, metric_result, customer_feedback, conversion_status, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		outcome.ID, outcome.POCID, outcome.ProductKey, boolInt(outcome.Success), outcome.MetricResult,
		outcome.CustomerFeedback, outcome.ConversionStatus, outcome.CreatedBy, outcome.CreatedAt)
	return err
}

func (s *SQLStore) ListPOCOutcomes(ctx context.Context, productKey string) ([]POCOutcome, error) {
	q := `SELECT id, poc_id, product_key, success_yn, metric_result, customer_feedback, conversion_status, created_by, created_at
		FROM dw_poc_outcomes`
	args := []any{}
	if productKey != "" {
		q += ` WHERE product_key = ?`
		args = append(args, productKey)
	}
	q += ` ORDER BY created_at DESC LIMIT 200`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []POCOutcome{}
	for rows.Next() {
		var outcome POCOutcome
		var success int
		if err := rows.Scan(&outcome.ID, &outcome.POCID, &outcome.ProductKey, &success, &outcome.MetricResult,
			&outcome.CustomerFeedback, &outcome.ConversionStatus, &outcome.CreatedBy, &outcome.CreatedAt); err != nil {
			return nil, err
		}
		outcome.Success = success == 1
		out = append(out, outcome)
	}
	return out, rows.Err()
}

func (s *SQLStore) DataWorksFunnel(ctx context.Context, productKey string) (DataWorksFunnel, error) {
	count := func(query string, args ...any) int64 {
		var n int64
		_ = s.db.QueryRowContext(ctx, s.bind(query), args...).Scan(&n)
		return n
	}
	var f DataWorksFunnel
	if productKey == "" {
		f.Ideas = count(`SELECT COUNT(*) FROM product_ideas`)
		f.Definitions = count(`SELECT COUNT(*) FROM product_definitions`)
		f.RiskReviews = count(`SELECT COUNT(*) FROM product_risk_reviews`)
		f.Proposals = count(`SELECT COUNT(*) FROM proposal_packages`)
		f.ProposalFeedback = count(`SELECT COUNT(*) FROM dw_proposal_feedback`)
		f.POCPlans = count(`SELECT COUNT(*) FROM product_poc_plans`)
		f.POCOutcomes = count(`SELECT COUNT(*) FROM dw_poc_outcomes`)
		f.Published = count(`SELECT COUNT(*) FROM data_products WHERE status = 'published'`)
		return f, nil
	}
	f.Definitions = count(`SELECT COUNT(*) FROM product_definitions WHERE product_key = ?`, productKey)
	f.RiskReviews = count(`SELECT COUNT(*) FROM product_risk_reviews WHERE product_key = ?`, productKey)
	f.Proposals = count(`SELECT COUNT(*) FROM proposal_packages WHERE product_key = ?`, productKey)
	f.ProposalFeedback = count(`SELECT COUNT(*) FROM dw_proposal_feedback WHERE product_key = ?`, productKey)
	f.POCPlans = count(`SELECT COUNT(*) FROM product_poc_plans WHERE product_key = ?`, productKey)
	f.POCOutcomes = count(`SELECT COUNT(*) FROM dw_poc_outcomes WHERE product_key = ?`, productKey)
	f.Published = count(`SELECT COUNT(*) FROM data_products WHERE product_key = ? AND status = 'published'`, productKey)
	return f, nil
}
