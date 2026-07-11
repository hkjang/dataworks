package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type DataAsset struct {
	ID             string `json:"id"`
	AssetKey       string `json:"asset_key"`
	Name           string `json:"name"`
	Domain         string `json:"domain"`
	Owner          string `json:"owner"`
	ColumnsSummary string `json:"columns_summary"`
	Sensitivity    string `json:"sensitivity"`
	RefreshCycle   string `json:"refresh_cycle"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type ProductIdea struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	TargetIndustry  string   `json:"target_industry"`
	TargetCustomers []string `json:"target_customers"`
	CustomerNeed    string   `json:"customer_need"`
	DataAssets      []string `json:"data_assets"`
	DeliveryMethod  string   `json:"delivery_method"`
	ExpectedImpact  string   `json:"expected_impact"`
	DifficultyScore int      `json:"difficulty_score"`
	RiskScore       int      `json:"risk_score"`
	RevenueScore    int      `json:"revenue_score"`
	Differentiation string   `json:"differentiation"`
	SourcePrompt    string   `json:"source_prompt"`
	CreatedBy       string   `json:"created_by"`
	Status          string   `json:"status"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

type ProductDefinition struct {
	ID             string `json:"id"`
	IdeaID         string `json:"idea_id"`
	ProductKey     string `json:"product_key"`
	DefinitionJSON string `json:"definition_json"`
	Version        int    `json:"version"`
	Status         string `json:"status"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type ProductRiskReview struct {
	ID            string `json:"id"`
	ProductKey    string `json:"product_key"`
	PrivacyScore  int    `json:"privacy_score"`
	CreditScore   int    `json:"credit_score"`
	AIScore       int    `json:"ai_score"`
	SecurityScore int    `json:"security_score"`
	OverallScore  int    `json:"overall_score"`
	ChecklistJSON string `json:"checklist_json"`
	ReviewNotes   string `json:"review_notes"`
	CreatedBy     string `json:"created_by"`
	CreatedAt     string `json:"created_at"`
}

type ProductPOCPlan struct {
	ID             string `json:"id"`
	ProductKey     string `json:"product_key"`
	DataScope      string `json:"data_scope"`
	SuccessMetric  string `json:"success_metric"`
	Timeline       string `json:"timeline"`
	Owner          string `json:"owner"`
	ApprovalStatus string `json:"approval_status"`
	PlanJSON       string `json:"plan_json"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type ProposalPackage struct {
	ID                 string `json:"id"`
	ProductKey         string `json:"product_key"`
	TargetCustomerType string `json:"target_customer_type"`
	ProposalJSON       string `json:"proposal_json"`
	GeneratedFileRef   string `json:"generated_file_ref"`
	CreatedBy          string `json:"created_by"`
	CreatedAt          string `json:"created_at"`
}

type FactoryRun struct {
	ID             string  `json:"id"`
	RunType        string  `json:"run_type"`
	Model          string  `json:"model"`
	PromptVersion  string  `json:"prompt_version"`
	InputHash      string  `json:"input_hash"`
	OutputRef      string  `json:"output_ref"`
	ParentRunID    string  `json:"parent_run_id"`
	PolicyDecision string  `json:"policy_decision"`
	TokenCost      float64 `json:"token_cost"`
	Status         string  `json:"status"`
	LatencyMS      int     `json:"latency_ms"`
	CreatedBy      string  `json:"created_by"`
	CreatedAt      string  `json:"created_at"`
}

type FactoryDashboard struct {
	IdeasTotal         int64 `json:"ideas_total"`
	DraftProducts      int64 `json:"draft_products"`
	ReviewProducts     int64 `json:"review_products"`
	RiskReviewProducts int64 `json:"risk_review_products"`
	ApprovedProducts   int64 `json:"approved_products"`
	PublishedProducts  int64 `json:"published_products"`
	ArchivedProducts   int64 `json:"archived_products"`
	HighRiskReviews    int64 `json:"high_risk_reviews"`
	PendingPOCPlans    int64 `json:"pending_poc_plans"`
	AverageRevenue     int64 `json:"average_revenue_score"`
}

func (s *SQLStore) UpsertDataAsset(ctx context.Context, a DataAsset) error {
	now := formatTime(time.Now().UTC())
	if a.Sensitivity == "" {
		a.Sensitivity = "internal"
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO data_assets
		(id, asset_key, name, domain, owner, columns_summary, sensitivity, refresh_cycle, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(asset_key) DO UPDATE SET
			name = excluded.name, domain = excluded.domain, owner = excluded.owner,
			columns_summary = excluded.columns_summary, sensitivity = excluded.sensitivity,
			refresh_cycle = excluded.refresh_cycle, updated_at = excluded.updated_at`),
		a.ID, strings.TrimSpace(a.AssetKey), a.Name, a.Domain, a.Owner, a.ColumnsSummary, a.Sensitivity, a.RefreshCycle, now, now)
	return err
}

func (s *SQLStore) ListDataAssets(ctx context.Context) ([]DataAsset, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, asset_key, name, domain, owner, columns_summary, sensitivity, refresh_cycle, created_at, updated_at
		FROM data_assets ORDER BY domain, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataAsset{}
	for rows.Next() {
		var a DataAsset
		if err := rows.Scan(&a.ID, &a.AssetKey, &a.Name, &a.Domain, &a.Owner, &a.ColumnsSummary, &a.Sensitivity, &a.RefreshCycle, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLStore) InsertProductIdea(ctx context.Context, i ProductIdea) error {
	now := formatTime(time.Now().UTC())
	if i.Status == "" {
		i.Status = "draft"
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO product_ideas
		(id, title, target_industry, target_customers, customer_need, data_assets, delivery_method, expected_impact,
		 difficulty_score, risk_score, revenue_score, differentiation, source_prompt, created_by, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		i.ID, i.Title, i.TargetIndustry, strings.Join(i.TargetCustomers, ","), i.CustomerNeed, strings.Join(i.DataAssets, ","),
		i.DeliveryMethod, i.ExpectedImpact, i.DifficultyScore, i.RiskScore, i.RevenueScore, i.Differentiation,
		i.SourcePrompt, i.CreatedBy, i.Status, now, now)
	return err
}

func scanProductIdea(sc interface{ Scan(...any) error }) (ProductIdea, error) {
	var i ProductIdea
	var customers, assets string
	if err := sc.Scan(&i.ID, &i.Title, &i.TargetIndustry, &customers, &i.CustomerNeed, &assets, &i.DeliveryMethod,
		&i.ExpectedImpact, &i.DifficultyScore, &i.RiskScore, &i.RevenueScore, &i.Differentiation,
		&i.SourcePrompt, &i.CreatedBy, &i.Status, &i.CreatedAt, &i.UpdatedAt); err != nil {
		return ProductIdea{}, err
	}
	i.TargetCustomers = splitCSVField(customers)
	i.DataAssets = splitCSVField(assets)
	return i, nil
}

func (s *SQLStore) ListProductIdeas(ctx context.Context, status string, limit int) ([]ProductIdea, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	q := `SELECT id, title, target_industry, target_customers, customer_need, data_assets, delivery_method, expected_impact,
			difficulty_score, risk_score, revenue_score, differentiation, source_prompt, created_by, status, created_at, updated_at
		FROM product_ideas`
	args := []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProductIdea{}
	for rows.Next() {
		i, err := scanProductIdea(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *SQLStore) GetProductIdea(ctx context.Context, id string) (ProductIdea, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, title, target_industry, target_customers, customer_need, data_assets, delivery_method, expected_impact,
			difficulty_score, risk_score, revenue_score, differentiation, source_prompt, created_by, status, created_at, updated_at
		FROM product_ideas WHERE id = ?`), id)
	i, err := scanProductIdea(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductIdea{}, false, nil
	}
	if err != nil {
		return ProductIdea{}, false, err
	}
	return i, true, nil
}

func (s *SQLStore) InsertProductDefinition(ctx context.Context, d ProductDefinition) (int, error) {
	now := formatTime(time.Now().UTC())
	if d.Status == "" {
		d.Status = "review"
	}
	var version int
	_ = s.db.QueryRowContext(ctx, s.bind(`SELECT COALESCE(MAX(version), 0) + 1 FROM product_definitions WHERE product_key = ?`), d.ProductKey).Scan(&version)
	if version <= 0 {
		version = 1
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO product_definitions
		(id, idea_id, product_key, definition_json, version, status, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		d.ID, d.IdeaID, d.ProductKey, d.DefinitionJSON, version, d.Status, d.CreatedBy, now, now)
	return version, err
}

func (s *SQLStore) LatestProductDefinition(ctx context.Context, productKey string) (ProductDefinition, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, idea_id, product_key, definition_json, version, status, created_by, created_at, updated_at
		FROM product_definitions WHERE product_key = ? ORDER BY version DESC LIMIT 1`), productKey)
	var d ProductDefinition
	err := row.Scan(&d.ID, &d.IdeaID, &d.ProductKey, &d.DefinitionJSON, &d.Version, &d.Status, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductDefinition{}, false, nil
	}
	if err != nil {
		return ProductDefinition{}, false, err
	}
	return d, true, nil
}

func (s *SQLStore) InsertProductRiskReview(ctx context.Context, rr ProductRiskReview) error {
	if rr.OverallScore <= 0 {
		rr.OverallScore = (rr.PrivacyScore + rr.CreditScore + rr.AIScore + rr.SecurityScore) / 4
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO product_risk_reviews
		(id, product_key, privacy_score, credit_score, ai_score, security_score, overall_score, checklist_json, review_notes, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		rr.ID, rr.ProductKey, rr.PrivacyScore, rr.CreditScore, rr.AIScore, rr.SecurityScore, rr.OverallScore,
		rr.ChecklistJSON, rr.ReviewNotes, rr.CreatedBy, formatTime(time.Now().UTC()))
	return err
}

func (s *SQLStore) LatestProductRiskReview(ctx context.Context, productKey string) (ProductRiskReview, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, product_key, privacy_score, credit_score, ai_score, security_score,
			overall_score, checklist_json, review_notes, created_by, created_at
		FROM product_risk_reviews WHERE product_key = ? ORDER BY created_at DESC LIMIT 1`), productKey)
	var rr ProductRiskReview
	err := row.Scan(&rr.ID, &rr.ProductKey, &rr.PrivacyScore, &rr.CreditScore, &rr.AIScore, &rr.SecurityScore,
		&rr.OverallScore, &rr.ChecklistJSON, &rr.ReviewNotes, &rr.CreatedBy, &rr.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductRiskReview{}, false, nil
	}
	if err != nil {
		return ProductRiskReview{}, false, err
	}
	return rr, true, nil
}

func (s *SQLStore) InsertProductPOCPlan(ctx context.Context, p ProductPOCPlan) error {
	now := formatTime(time.Now().UTC())
	if p.ApprovalStatus == "" {
		p.ApprovalStatus = "pending"
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO product_poc_plans
		(id, product_key, data_scope, success_metric, timeline, owner, approval_status, plan_json, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		p.ID, p.ProductKey, p.DataScope, p.SuccessMetric, p.Timeline, p.Owner, p.ApprovalStatus, p.PlanJSON, p.CreatedBy, now, now)
	return err
}

func (s *SQLStore) LatestProductPOCPlan(ctx context.Context, productKey string) (ProductPOCPlan, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, product_key, data_scope, success_metric, timeline, owner, approval_status, plan_json, created_by, created_at, updated_at
		FROM product_poc_plans WHERE product_key = ? ORDER BY created_at DESC LIMIT 1`), productKey)
	var p ProductPOCPlan
	err := row.Scan(&p.ID, &p.ProductKey, &p.DataScope, &p.SuccessMetric, &p.Timeline, &p.Owner, &p.ApprovalStatus, &p.PlanJSON, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductPOCPlan{}, false, nil
	}
	if err != nil {
		return ProductPOCPlan{}, false, err
	}
	return p, true, nil
}

func (s *SQLStore) InsertProposalPackage(ctx context.Context, p ProposalPackage) error {
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO proposal_packages
		(id, product_key, target_customer_type, proposal_json, generated_file_ref, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		p.ID, p.ProductKey, p.TargetCustomerType, p.ProposalJSON, p.GeneratedFileRef, p.CreatedBy, formatTime(time.Now().UTC()))
	return err
}

func (s *SQLStore) InsertFactoryRun(ctx context.Context, run FactoryRun) error {
	if run.TokenCost < 0 || run.LatencyMS < 0 {
		return errors.New("token_cost and latency_ms must be non-negative")
	}
	if strings.TrimSpace(run.Status) == "" {
		run.Status = "completed"
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO factory_runs
		(id, run_type, model, prompt_version, input_hash, output_ref, parent_run_id, policy_decision,
		 token_cost, status, latency_ms, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		run.ID, run.RunType, run.Model, run.PromptVersion, run.InputHash, run.OutputRef, run.ParentRunID,
		run.PolicyDecision, run.TokenCost, run.Status, run.LatencyMS, run.CreatedBy, formatTime(time.Now().UTC()))
	return err
}

func (s *SQLStore) GetFactoryRun(ctx context.Context, id string) (FactoryRun, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, run_type, model, prompt_version, input_hash, output_ref,
		parent_run_id, policy_decision, token_cost, status, latency_ms, created_by, created_at
		FROM factory_runs WHERE id = ?`), strings.TrimSpace(id))
	var run FactoryRun
	err := row.Scan(&run.ID, &run.RunType, &run.Model, &run.PromptVersion, &run.InputHash, &run.OutputRef,
		&run.ParentRunID, &run.PolicyDecision, &run.TokenCost, &run.Status, &run.LatencyMS, &run.CreatedBy, &run.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return FactoryRun{}, false, nil
	}
	if err != nil {
		return FactoryRun{}, false, err
	}
	return run, true, nil
}

func (s *SQLStore) FactoryDashboard(ctx context.Context) (FactoryDashboard, error) {
	var d FactoryDashboard
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_ideas`).Scan(&d.IdeasTotal); err != nil {
		return d, err
	}
	countStatus := func(status string) int64 {
		var n int64
		_ = s.db.QueryRowContext(ctx, s.bind(`SELECT COUNT(*) FROM data_products WHERE status = ?`), status).Scan(&n)
		return n
	}
	d.DraftProducts = countStatus("draft")
	d.ReviewProducts = countStatus("review")
	d.RiskReviewProducts = countStatus("risk_review")
	d.ApprovedProducts = countStatus("approved")
	d.PublishedProducts = countStatus("published")
	d.ArchivedProducts = countStatus("archived")
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_risk_reviews WHERE overall_score >= 70`).Scan(&d.HighRiskReviews)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_poc_plans WHERE approval_status = 'pending'`).Scan(&d.PendingPOCPlans)
	var avgRevenue float64
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(AVG(revenue_score), 0) FROM data_products WHERE revenue_score > 0`).Scan(&avgRevenue)
	d.AverageRevenue = int64(avgRevenue + 0.5)
	return d, nil
}

// ListFactoryRuns returns the execution history of factory runs.
func (s *SQLStore) ListFactoryRuns(ctx context.Context, since time.Time, limit int) ([]FactoryRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT id, run_type, model, prompt_version, input_hash, output_ref,
		parent_run_id, policy_decision, token_cost, status, latency_ms, created_by, created_at
		FROM factory_runs
		WHERE created_at >= ?
		ORDER BY created_at DESC
		LIMIT ?`), formatTime(since), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []FactoryRun
	for rows.Next() {
		var run FactoryRun
		if err := rows.Scan(&run.ID, &run.RunType, &run.Model, &run.PromptVersion, &run.InputHash, &run.OutputRef,
			&run.ParentRunID, &run.PolicyDecision, &run.TokenCost, &run.Status, &run.LatencyMS, &run.CreatedBy, &run.CreatedAt); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}
