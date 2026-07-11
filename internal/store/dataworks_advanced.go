package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type DataQualityRule struct {
	ID             string  `json:"id"`
	AssetKey       string  `json:"asset_key"`
	ColumnName     string  `json:"column_name"`
	RuleType       string  `json:"rule_type"` // null_rate | duplicate_rate | anomaly_rate | code_values | range_condition | schema_change
	Threshold      float64 `json:"threshold"`
	ExpectedValues string  `json:"expected_values"`
	MinValue       float64 `json:"min_value"`
	MaxValue       float64 `json:"max_value"`
	Enabled        bool    `json:"enabled"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

type DataQualityResult struct {
	ID          string  `json:"id"`
	AssetKey    string  `json:"asset_key"`
	RuleID      string  `json:"rule_id"`
	RuleType    string  `json:"rule_type"`
	Passed      bool    `json:"passed"`
	ActualValue float64 `json:"actual_value"`
	Message     string  `json:"message"`
	CheckedAt   string  `json:"checked_at"`
}

type SchemaDrift struct {
	ID          string  `json:"id"`
	AssetKey    string  `json:"asset_key"`
	ColumnName  string  `json:"column_name"`
	DriftType   string  `json:"drift_type"` // added | deleted | type_changed
	OldType     string  `json:"old_type"`
	NewType     string  `json:"new_type"`
	ImpactScore float64 `json:"impact_score"`
	DetectedAt  string  `json:"detected_at"`
}

type SLAMetric struct {
	ID          string  `json:"id"`
	ProductKey  string  `json:"product_key"`
	MetricType  string  `json:"metric_type"` // freshness | latency | availability | report_delay
	ActualValue float64 `json:"actual_value"`
	TargetValue float64 `json:"target_value"`
	Status      string  `json:"status"` // normal | warning | breached
	CheckedAt   string  `json:"checked_at"`
}

type UsageMetering struct {
	ID             string  `json:"id"`
	CustomerKey    string  `json:"customer_key"`
	ProductKey     string  `json:"product_key"`
	ContractKey    string  `json:"contract_key"`
	TotalCalls     int     `json:"total_calls"`
	FailedCalls    int     `json:"failed_calls"`
	OverLimitCalls int     `json:"over_limit_calls"`
	BillingAmount  float64 `json:"billing_amount"`
	BilledDate     string  `json:"billed_date"`
}

type DataWorksPolicyRule struct {
	ID             string `json:"id"`
	PolicyType     string `json:"policy_type"` // privacy | credit_info | ai_usage | external_sharing | security
	RuleExpression string `json:"rule_expression"`
	Action         string `json:"action"` // block | warn | approve
	Enabled        bool   `json:"enabled"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type PromptRegressionTest struct {
	ID                    string  `json:"id"`
	PromptKey             string  `json:"prompt_key"`
	OldTemplateVersion    int     `json:"old_template_version"`
	NewTemplateVersion    int     `json:"new_template_version"`
	OldModel              string  `json:"old_model"`
	NewModel              string  `json:"new_model"`
	QualityDelta          float64 `json:"quality_delta"`
	CostDelta             float64 `json:"cost_delta"`
	LatencyDelta          float64 `json:"latency_delta"`
	PolicyViolationsCount int     `json:"policy_violations_count"`
	Status                string  `json:"status"`
	CreatedAt             string  `json:"created_at"`
}

type ProposalExperiment struct {
	ID              string  `json:"id"`
	ProductKey      string  `json:"product_key"`
	CustomerSegment string  `json:"customer_segment"`
	HeadlineVariant string  `json:"headline_variant"`
	PricingVariant  float64 `json:"pricing_variant"`
	PackageVariant  string  `json:"package_variant"`
	Status          string  `json:"status"` // running | completed
	ConversionRate  float64 `json:"conversion_rate"`
	ResponsesCount  int     `json:"responses_count"`
	CreatedAt       string  `json:"created_at"`
}

type MarketplaceBookmark struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	ProductKey string `json:"product_key"`
	CreatedAt  string `json:"created_at"`
}

type MarketplaceSubscription struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	ProductKey string `json:"product_key"`
	Status     string `json:"status"` // pending | approved | rejected
	Purpose    string `json:"purpose"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// Data Quality Rules
func (s *SQLStore) InsertDataQualityRule(ctx context.Context, rule DataQualityRule) error {
	if rule.ID == "" || rule.AssetKey == "" || rule.RuleType == "" {
		return errors.New("id, asset_key, and rule_type are required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_data_quality_rules
		(id, asset_key, column_name, rule_type, threshold, expected_values, min_value, max_value, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		rule.ID, rule.AssetKey, rule.ColumnName, rule.RuleType, rule.Threshold, rule.ExpectedValues, rule.MinValue, rule.MaxValue, boolInt(rule.Enabled), now, now)
	return err
}

func (s *SQLStore) ListDataQualityRules(ctx context.Context, assetKey string) ([]DataQualityRule, error) {
	q := `SELECT id, asset_key, column_name, rule_type, threshold, expected_values, min_value, max_value, enabled, created_at, updated_at
		FROM dw_data_quality_rules WHERE asset_key = ?`
	rows, err := s.db.QueryContext(ctx, s.bind(q), assetKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []DataQualityRule
	for rows.Next() {
		var r DataQualityRule
		var enabled int
		if err := rows.Scan(&r.ID, &r.AssetKey, &r.ColumnName, &r.RuleType, &r.Threshold, &r.ExpectedValues, &r.MinValue, &r.MaxValue, &enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// Data Quality Results
func (s *SQLStore) InsertDataQualityResult(ctx context.Context, res DataQualityResult) error {
	if res.ID == "" || res.AssetKey == "" || res.RuleID == "" {
		return errors.New("id, asset_key, and rule_id are required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_data_quality_results
		(id, asset_key, rule_id, rule_type, passed, actual_value, message, checked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		res.ID, res.AssetKey, res.RuleID, res.RuleType, boolInt(res.Passed), res.ActualValue, res.Message, now)
	return err
}

func (s *SQLStore) ListDataQualityResults(ctx context.Context, assetKey string) ([]DataQualityResult, error) {
	q := `SELECT id, asset_key, rule_id, rule_type, passed, actual_value, message, checked_at
		FROM dw_data_quality_results WHERE asset_key = ? ORDER BY checked_at DESC`
	rows, err := s.db.QueryContext(ctx, s.bind(q), assetKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []DataQualityResult
	for rows.Next() {
		var r DataQualityResult
		var passed int
		if err := rows.Scan(&r.ID, &r.AssetKey, &r.RuleID, &r.RuleType, &passed, &r.ActualValue, &r.Message, &r.CheckedAt); err != nil {
			return nil, err
		}
		r.Passed = passed == 1
		results = append(results, r)
	}
	return results, rows.Err()
}

// Schema Drift
func (s *SQLStore) InsertSchemaDrift(ctx context.Context, d SchemaDrift) error {
	if d.ID == "" || d.AssetKey == "" || d.ColumnName == "" || d.DriftType == "" {
		return errors.New("id, asset_key, column_name, and drift_type are required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_schema_drifts
		(id, asset_key, column_name, drift_type, old_type, new_type, impact_score, detected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		d.ID, d.AssetKey, d.ColumnName, d.DriftType, d.OldType, d.NewType, d.ImpactScore, now)
	return err
}

func (s *SQLStore) ListSchemaDrifts(ctx context.Context, assetKey string) ([]SchemaDrift, error) {
	q := `SELECT id, asset_key, column_name, drift_type, old_type, new_type, impact_score, detected_at
		FROM dw_schema_drifts WHERE asset_key = ? ORDER BY detected_at DESC`
	rows, err := s.db.QueryContext(ctx, s.bind(q), assetKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var drifts []SchemaDrift
	for rows.Next() {
		var d SchemaDrift
		if err := rows.Scan(&d.ID, &d.AssetKey, &d.ColumnName, &d.DriftType, &d.OldType, &d.NewType, &d.ImpactScore, &d.DetectedAt); err != nil {
			return nil, err
		}
		drifts = append(drifts, d)
	}
	return drifts, rows.Err()
}

// SLA Metrics
func (s *SQLStore) InsertSLAMetric(ctx context.Context, m SLAMetric) error {
	if m.ID == "" || m.ProductKey == "" || m.MetricType == "" {
		return errors.New("id, product_key, and metric_type are required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_sla_metrics
		(id, product_key, metric_type, actual_value, target_value, status, checked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		m.ID, m.ProductKey, m.MetricType, m.ActualValue, m.TargetValue, m.Status, now)
	return err
}

func (s *SQLStore) ListSLAMetrics(ctx context.Context, productKey string) ([]SLAMetric, error) {
	q := `SELECT id, product_key, metric_type, actual_value, target_value, status, checked_at
		FROM dw_sla_metrics WHERE product_key = ? ORDER BY checked_at DESC`
	rows, err := s.db.QueryContext(ctx, s.bind(q), productKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var metrics []SLAMetric
	for rows.Next() {
		var m SLAMetric
		if err := rows.Scan(&m.ID, &m.ProductKey, &m.MetricType, &m.ActualValue, &m.TargetValue, &m.Status, &m.CheckedAt); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, rows.Err()
}

// Usage Metering
func (s *SQLStore) IncrementUsageMetering(ctx context.Context, customerKey, productKey, contractKey string, failed bool, billingAmount float64) error {
	dateStr := time.Now().UTC().Format("2006-01-02")
	id := customerKey + ":" + productKey + ":" + contractKey + ":" + dateStr
	failedVal := 0
	if failed {
		failedVal = 1
	}

	// SQLite ON CONFLICT DO UPDATE
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_usage_metering
		(id, customer_key, product_key, contract_key, total_calls, failed_calls, over_limit_calls, billing_amount, billed_date)
		VALUES (?, ?, ?, ?, 1, ?, 0, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			total_calls = total_calls + 1,
			failed_calls = failed_calls + ?,
			billing_amount = billing_amount + ?`),
		id, customerKey, productKey, contractKey, failedVal, billingAmount, dateStr, failedVal, billingAmount)
	return err
}

func (s *SQLStore) ListUsageMetering(ctx context.Context, productKey string) ([]UsageMetering, error) {
	q := `SELECT id, customer_key, product_key, contract_key, total_calls, failed_calls, over_limit_calls, billing_amount, billed_date
		FROM dw_usage_metering WHERE product_key = ? ORDER BY billed_date DESC`
	rows, err := s.db.QueryContext(ctx, s.bind(q), productKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []UsageMetering
	for rows.Next() {
		var u UsageMetering
		if err := rows.Scan(&u.ID, &u.CustomerKey, &u.ProductKey, &u.ContractKey, &u.TotalCalls, &u.FailedCalls, &u.OverLimitCalls, &u.BillingAmount, &u.BilledDate); err != nil {
			return nil, err
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

// Policy Rules
func (s *SQLStore) InsertPolicyRule(ctx context.Context, rule DataWorksPolicyRule) error {
	if rule.ID == "" || rule.PolicyType == "" || rule.RuleExpression == "" {
		return errors.New("id, policy_type, and rule_expression are required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_policy_rules
		(id, policy_type, rule_expression, action, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		rule.ID, rule.PolicyType, rule.RuleExpression, rule.Action, boolInt(rule.Enabled), now, now)
	return err
}

func (s *SQLStore) ListPolicyRules(ctx context.Context, policyType string) ([]DataWorksPolicyRule, error) {
	q := `SELECT id, policy_type, rule_expression, action, enabled, created_at, updated_at
		FROM dw_policy_rules`
	var args []any
	if policyType != "" {
		q += ` WHERE policy_type = ?`
		args = append(args, policyType)
	}
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []DataWorksPolicyRule
	for rows.Next() {
		var r DataWorksPolicyRule
		var enabled int
		if err := rows.Scan(&r.ID, &r.PolicyType, &r.RuleExpression, &r.Action, &enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// Prompt Regression
func (s *SQLStore) InsertPromptRegressionTest(ctx context.Context, t PromptRegressionTest) error {
	if t.ID == "" || t.PromptKey == "" {
		return errors.New("id and prompt_key are required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_prompt_regression_tests
		(id, prompt_key, old_template_version, new_template_version, old_model, new_model, quality_delta, cost_delta, latency_delta, policy_violations_count, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		t.ID, t.PromptKey, t.OldTemplateVersion, t.NewTemplateVersion, t.OldModel, t.NewModel, t.QualityDelta, t.CostDelta, t.LatencyDelta, t.PolicyViolationsCount, t.Status, now)
	return err
}

func (s *SQLStore) ListPromptRegressionTests(ctx context.Context, promptKey string) ([]PromptRegressionTest, error) {
	q := `SELECT id, prompt_key, old_template_version, new_template_version, old_model, new_model, quality_delta, cost_delta, latency_delta, policy_violations_count, status, created_at
		FROM dw_prompt_regression_tests WHERE prompt_key = ? ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, s.bind(q), promptKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tests []PromptRegressionTest
	for rows.Next() {
		var t PromptRegressionTest
		if err := rows.Scan(&t.ID, &t.PromptKey, &t.OldTemplateVersion, &t.NewTemplateVersion, &t.OldModel, &t.NewModel, &t.QualityDelta, &t.CostDelta, &t.LatencyDelta, &t.PolicyViolationsCount, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		tests = append(tests, t)
	}
	return tests, rows.Err()
}

// Proposal Experiments
func (s *SQLStore) InsertProposalExperiment(ctx context.Context, exp ProposalExperiment) error {
	if exp.ID == "" || exp.ProductKey == "" || exp.CustomerSegment == "" {
		return errors.New("id, product_key, and customer_segment are required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_proposal_experiments
		(id, product_key, customer_segment, headline_variant, pricing_variant, package_variant, status, conversion_rate, responses_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		exp.ID, exp.ProductKey, exp.CustomerSegment, exp.HeadlineVariant, exp.PricingVariant, exp.PackageVariant, exp.Status, exp.ConversionRate, exp.ResponsesCount, now)
	return err
}

func (s *SQLStore) ListProposalExperiments(ctx context.Context, productKey string) ([]ProposalExperiment, error) {
	q := `SELECT id, product_key, customer_segment, headline_variant, pricing_variant, package_variant, status, conversion_rate, responses_count, created_at
		FROM dw_proposal_experiments`
	var args []any
	if productKey != "" {
		q += ` WHERE product_key = ?`
		args = append(args, productKey)
	}
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var exps []ProposalExperiment
	for rows.Next() {
		var e ProposalExperiment
		if err := rows.Scan(&e.ID, &e.ProductKey, &e.CustomerSegment, &e.HeadlineVariant, &e.PricingVariant, &e.PackageVariant, &e.Status, &e.ConversionRate, &e.ResponsesCount, &e.CreatedAt); err != nil {
			return nil, err
		}
		exps = append(exps, e)
	}
	return exps, rows.Err()
}

func (s *SQLStore) RecordProposalExperimentResponse(ctx context.Context, id string, positive bool) error {
	posVal := 0
	if positive {
		posVal = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var count int
	var currentRate float64
	err = tx.QueryRowContext(ctx, s.bind(`SELECT responses_count, conversion_rate FROM dw_proposal_experiments WHERE id = ?`), id).Scan(&count, &currentRate)
	if err != nil {
		return err
	}

	newCount := count + 1
	var newRate float64
	if positive {
		currentPositive := currentRate * float64(count)
		newRate = (currentPositive + float64(posVal)) / float64(newCount)
	} else {
		newRate = (currentRate * float64(count)) / float64(newCount)
	}

	_, err = tx.ExecContext(ctx, s.bind(`UPDATE dw_proposal_experiments SET responses_count = ?, conversion_rate = ? WHERE id = ?`), newCount, newRate, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Marketplace Bookmark
func (s *SQLStore) ToggleMarketplaceBookmark(ctx context.Context, userID, productKey string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var id string
	err = tx.QueryRowContext(ctx, s.bind(`SELECT id FROM dw_marketplace_bookmarks WHERE user_id = ? AND product_key = ?`), userID, productKey).Scan(&id)
	if err == sql.ErrNoRows {
		// Insert
		newID := userID + ":" + productKey
		now := formatTime(time.Now().UTC())
		_, err = tx.ExecContext(ctx, s.bind(`INSERT INTO dw_marketplace_bookmarks (id, user_id, product_key, created_at) VALUES (?, ?, ?, ?)`), newID, userID, productKey, now)
		if err != nil {
			return false, err
		}
		err = tx.Commit()
		return true, err
	} else if err != nil {
		return false, err
	}

	// Delete
	_, err = tx.ExecContext(ctx, s.bind(`DELETE FROM dw_marketplace_bookmarks WHERE id = ?`), id)
	if err != nil {
		return false, err
	}
	err = tx.Commit()
	return false, nil
}

func (s *SQLStore) ListMarketplaceBookmarks(ctx context.Context, userID string) ([]MarketplaceBookmark, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT id, user_id, product_key, created_at FROM dw_marketplace_bookmarks WHERE user_id = ?`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []MarketplaceBookmark
	for rows.Next() {
		var b MarketplaceBookmark
		if err := rows.Scan(&b.ID, &b.UserID, &b.ProductKey, &b.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

// Marketplace Subscription
func (s *SQLStore) InsertMarketplaceSubscription(ctx context.Context, sub MarketplaceSubscription) error {
	if sub.ID == "" || sub.UserID == "" || sub.ProductKey == "" {
		return errors.New("id, user_id, and product_key are required")
	}
	now := formatTime(time.Now().UTC())
	if sub.Status == "" {
		sub.Status = "pending"
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_marketplace_subscriptions
		(id, user_id, product_key, status, purpose, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		sub.ID, sub.UserID, sub.ProductKey, sub.Status, sub.Purpose, now, now)
	return err
}

func (s *SQLStore) ListMarketplaceSubscriptions(ctx context.Context, userID, status string) ([]MarketplaceSubscription, error) {
	q := `SELECT id, user_id, product_key, status, purpose, created_at, updated_at FROM dw_marketplace_subscriptions WHERE 1=1`
	var args []any
	if userID != "" {
		q += ` AND user_id = ?`
		args = append(args, userID)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []MarketplaceSubscription
	for rows.Next() {
		var sub MarketplaceSubscription
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.ProductKey, &sub.Status, &sub.Purpose, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, sub)
	}
	return list, rows.Err()
}
