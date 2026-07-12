package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type DataWorksPolicyFinding struct {
	Policy      string `json:"policy"`
	Severity    string `json:"severity"`
	Decision    string `json:"decision"`
	Reason      string `json:"reason"`
	Remediation string `json:"remediation"`
}

type DataWorksApprovalRequirement struct {
	Role     string `json:"role"`
	Reason   string `json:"reason"`
	Required bool   `json:"required"`
}

type DataWorksPolicySimulation struct {
	ID             string                         `json:"id"`
	WorkspaceID    string                         `json:"workspace_id"`
	TargetType     string                         `json:"target_type"`
	TargetID       string                         `json:"target_id"`
	Context        map[string]any                 `json:"context"`
	Decision       string                         `json:"decision"`
	RiskScore      int                            `json:"risk_score"`
	Findings       []DataWorksPolicyFinding       `json:"findings"`
	ApprovalMatrix []DataWorksApprovalRequirement `json:"approval_matrix"`
	CreatedBy      string                         `json:"created_by"`
	CreatedAt      string                         `json:"created_at"`
}

type SyntheticField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Sensitivity string `json:"sensitivity"`
	Format      string `json:"format"`
}

type SyntheticDataset struct {
	ID           string           `json:"id"`
	WorkspaceID  string           `json:"workspace_id"`
	Name         string           `json:"name"`
	Purpose      string           `json:"purpose"`
	Strategy     string           `json:"strategy"`
	Schema       []SyntheticField `json:"schema"`
	RowCount     int              `json:"row_count"`
	Sample       []map[string]any `json:"sample"`
	SafetyReport map[string]any   `json:"safety_report"`
	Status       string           `json:"status"`
	CreatedBy    string           `json:"created_by"`
	CreatedAt    string           `json:"created_at"`
}

type DataWorksUnitEconomics struct {
	ID                 string  `json:"id"`
	ProductKey         string  `json:"product_key"`
	ScenarioKey        string  `json:"scenario_key"`
	CustomerSegment    string  `json:"customer_segment"`
	ExpectedCalls      int     `json:"expected_calls"`
	UnitPrice          float64 `json:"unit_price"`
	ExpectedRevenue    float64 `json:"expected_revenue"`
	QueryCost          float64 `json:"query_cost"`
	LLMCost            float64 `json:"llm_cost"`
	DataProcessingCost float64 `json:"data_processing_cost"`
	OpsCost            float64 `json:"ops_cost"`
	TotalCost          float64 `json:"total_cost"`
	Margin             float64 `json:"margin"`
	MarginRate         float64 `json:"margin_rate"`
	BreakEvenCalls     int     `json:"break_even_calls"`
	Currency           string  `json:"currency"`
	UpdatedAt          string  `json:"updated_at"`
}

type DataWorksMarketplaceItem struct {
	ID                string   `json:"id"`
	ItemKey           string   `json:"item_key"`
	WorkspaceID       string   `json:"workspace_id"`
	ItemType          string   `json:"item_type"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Owner             string   `json:"owner"`
	Status            string   `json:"status"`
	RiskLevel         string   `json:"risk_level"`
	Tags              []string `json:"tags"`
	SourceRef         string   `json:"source_ref"`
	Version           int      `json:"version"`
	SubscriptionCount int      `json:"subscription_count"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

type DataWorksMarketplaceSubscription struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	ItemID    string `json:"item_id"`
	ItemKey   string `json:"item_key"`
	ItemType  string `json:"item_type"`
	Status    string `json:"status"`
	Purpose   string `json:"purpose"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type DataWorksApprovalTask struct {
	ID          string `json:"id"`
	TargetType  string `json:"target_type"`
	TargetID    string `json:"target_id"`
	Step        string `json:"step"`
	Status      string `json:"status"`
	RequestedBy string `json:"requested_by"`
	DecidedBy   string `json:"decided_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func (s *SQLStore) InsertDataWorksPolicySimulation(ctx context.Context, sim DataWorksPolicySimulation) error {
	if sim.ID == "" || sim.TargetType == "" || sim.Decision == "" {
		return errors.New("id, target_type, and decision are required")
	}
	if sim.CreatedAt == "" {
		sim.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_policy_simulations
		(id, workspace_id, target_type, target_id, context_json, decision, risk_score,
		 findings_json, approval_matrix_json, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), sim.ID, sim.WorkspaceID, sim.TargetType,
		sim.TargetID, platformJSON(sim.Context, "{}"), sim.Decision, sim.RiskScore,
		platformJSON(sim.Findings, "[]"), platformJSON(sim.ApprovalMatrix, "[]"), sim.CreatedBy, sim.CreatedAt)
	return err
}

func (s *SQLStore) ListDataWorksPolicySimulations(ctx context.Context, workspaceID, targetType string, limit int) ([]DataWorksPolicySimulation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, workspace_id, target_type, target_id, context_json, decision, risk_score,
		findings_json, approval_matrix_json, created_by, created_at FROM dw_policy_simulations WHERE 1=1`
	args := []any{}
	if workspaceID != "" {
		q += ` AND workspace_id = ?`
		args = append(args, workspaceID)
	}
	if targetType != "" {
		q += ` AND target_type = ?`
		args = append(args, targetType)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksPolicySimulation{}
	for rows.Next() {
		var sim DataWorksPolicySimulation
		var contextJSON, findingsJSON, approvalsJSON string
		if err := rows.Scan(&sim.ID, &sim.WorkspaceID, &sim.TargetType, &sim.TargetID, &contextJSON,
			&sim.Decision, &sim.RiskScore, &findingsJSON, &approvalsJSON, &sim.CreatedBy, &sim.CreatedAt); err != nil {
			return nil, err
		}
		sim.Context = platformMap(contextJSON)
		_ = json.Unmarshal([]byte(findingsJSON), &sim.Findings)
		_ = json.Unmarshal([]byte(approvalsJSON), &sim.ApprovalMatrix)
		out = append(out, sim)
	}
	return out, rows.Err()
}

func (s *SQLStore) InsertSyntheticDataset(ctx context.Context, d SyntheticDataset) error {
	if d.ID == "" || strings.TrimSpace(d.Name) == "" || len(d.Schema) == 0 {
		return errors.New("id, name, and schema are required")
	}
	if d.Strategy == "" {
		d.Strategy = "synthetic"
	}
	if d.Status == "" {
		d.Status = "ready"
	}
	if d.CreatedAt == "" {
		d.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_synthetic_datasets
		(id, workspace_id, name, purpose, strategy, schema_json, row_count, sample_json,
		 safety_report_json, status, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		d.ID, d.WorkspaceID, d.Name, d.Purpose, d.Strategy, platformJSON(d.Schema, "[]"),
		d.RowCount, platformJSON(d.Sample, "[]"), platformJSON(d.SafetyReport, "{}"), d.Status,
		d.CreatedBy, d.CreatedAt)
	return err
}

func (s *SQLStore) ListSyntheticDatasets(ctx context.Context, workspaceID string, limit int) ([]SyntheticDataset, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, workspace_id, name, purpose, strategy, schema_json, row_count, sample_json,
		safety_report_json, status, created_by, created_at FROM dw_synthetic_datasets`
	args := []any{}
	if workspaceID != "" {
		q += ` WHERE workspace_id = ?`
		args = append(args, workspaceID)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SyntheticDataset{}
	for rows.Next() {
		var d SyntheticDataset
		var schemaJSON, sampleJSON, safetyJSON string
		if err := rows.Scan(&d.ID, &d.WorkspaceID, &d.Name, &d.Purpose, &d.Strategy, &schemaJSON,
			&d.RowCount, &sampleJSON, &safetyJSON, &d.Status, &d.CreatedBy, &d.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(schemaJSON), &d.Schema)
		_ = json.Unmarshal([]byte(sampleJSON), &d.Sample)
		d.SafetyReport = platformMap(safetyJSON)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertDataWorksUnitEconomics(ctx context.Context, e DataWorksUnitEconomics) error {
	if e.ID == "" || strings.TrimSpace(e.ProductKey) == "" {
		return errors.New("id and product_key are required")
	}
	if e.ScenarioKey == "" {
		e.ScenarioKey = "base"
	}
	if e.Currency == "" {
		e.Currency = "KRW"
	}
	e.UpdatedAt = formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_unit_economics
		(id, product_key, scenario_key, customer_segment, expected_calls, unit_price, expected_revenue,
		 query_cost, llm_cost, data_processing_cost, ops_cost, total_cost, margin, margin_rate,
		 break_even_calls, currency, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(product_key, scenario_key, customer_segment) DO UPDATE SET
			expected_calls=excluded.expected_calls, unit_price=excluded.unit_price,
			expected_revenue=excluded.expected_revenue, query_cost=excluded.query_cost,
			llm_cost=excluded.llm_cost, data_processing_cost=excluded.data_processing_cost,
			ops_cost=excluded.ops_cost, total_cost=excluded.total_cost, margin=excluded.margin,
			margin_rate=excluded.margin_rate, break_even_calls=excluded.break_even_calls,
			currency=excluded.currency, updated_at=excluded.updated_at`), e.ID, e.ProductKey,
		e.ScenarioKey, e.CustomerSegment, e.ExpectedCalls, e.UnitPrice, e.ExpectedRevenue, e.QueryCost,
		e.LLMCost, e.DataProcessingCost, e.OpsCost, e.TotalCost, e.Margin, e.MarginRate,
		e.BreakEvenCalls, e.Currency, e.UpdatedAt)
	return err
}

func (s *SQLStore) ListDataWorksUnitEconomics(ctx context.Context, productKey string) ([]DataWorksUnitEconomics, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT id, product_key, scenario_key, customer_segment,
		expected_calls, unit_price, expected_revenue, query_cost, llm_cost, data_processing_cost,
		ops_cost, total_cost, margin, margin_rate, break_even_calls, currency, updated_at
		FROM dw_unit_economics WHERE product_key = ? ORDER BY scenario_key, customer_segment`), productKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksUnitEconomics{}
	for rows.Next() {
		var e DataWorksUnitEconomics
		if err := rows.Scan(&e.ID, &e.ProductKey, &e.ScenarioKey, &e.CustomerSegment,
			&e.ExpectedCalls, &e.UnitPrice, &e.ExpectedRevenue, &e.QueryCost, &e.LLMCost,
			&e.DataProcessingCost, &e.OpsCost, &e.TotalCost, &e.Margin, &e.MarginRate,
			&e.BreakEvenCalls, &e.Currency, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertDataWorksMarketplaceItem(ctx context.Context, item DataWorksMarketplaceItem) error {
	if item.ID == "" || item.ItemKey == "" || item.ItemType == "" || item.Name == "" {
		return errors.New("id, item_key, item_type, and name are required")
	}
	if item.Status == "" {
		item.Status = "published"
	}
	if item.RiskLevel == "" {
		item.RiskLevel = "low"
	}
	if item.Version <= 0 {
		item.Version = 1
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_marketplace_items
		(id, item_key, workspace_id, item_type, name, description, owner, status, risk_level,
		 tags_json, source_ref, version, subscription_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(item_key) DO UPDATE SET workspace_id=excluded.workspace_id, item_type=excluded.item_type,
			name=excluded.name, description=excluded.description, owner=excluded.owner,
			status=excluded.status, risk_level=excluded.risk_level, tags_json=excluded.tags_json,
			source_ref=excluded.source_ref, version=excluded.version, updated_at=excluded.updated_at`),
		item.ID, item.ItemKey, item.WorkspaceID, item.ItemType, item.Name, item.Description, item.Owner,
		item.Status, item.RiskLevel, platformJSON(item.Tags, "[]"), item.SourceRef, item.Version,
		item.SubscriptionCount, now, now)
	return err
}

func scanDataWorksMarketplaceItem(sc interface{ Scan(...any) error }) (DataWorksMarketplaceItem, error) {
	var item DataWorksMarketplaceItem
	var tags string
	err := sc.Scan(&item.ID, &item.ItemKey, &item.WorkspaceID, &item.ItemType, &item.Name,
		&item.Description, &item.Owner, &item.Status, &item.RiskLevel, &tags, &item.SourceRef,
		&item.Version, &item.SubscriptionCount, &item.CreatedAt, &item.UpdatedAt)
	item.Tags = platformStringList(tags)
	return item, err
}

func (s *SQLStore) GetDataWorksMarketplaceItem(ctx context.Context, idOrKey string) (DataWorksMarketplaceItem, bool, error) {
	item, err := scanDataWorksMarketplaceItem(s.db.QueryRowContext(ctx, s.bind(`SELECT id, item_key,
		workspace_id, item_type, name, description, owner, status, risk_level, tags_json, source_ref,
		version, subscription_count, created_at, updated_at FROM dw_marketplace_items
		WHERE id = ? OR item_key = ?`), idOrKey, idOrKey))
	if errors.Is(err, sql.ErrNoRows) {
		return DataWorksMarketplaceItem{}, false, nil
	}
	if err != nil {
		return DataWorksMarketplaceItem{}, false, err
	}
	return item, true, nil
}

func (s *SQLStore) ListDataWorksMarketplaceItems(ctx context.Context, query, itemType, workspaceID string) ([]DataWorksMarketplaceItem, error) {
	q := `SELECT id, item_key, workspace_id, item_type, name, description, owner, status,
		risk_level, tags_json, source_ref, version, subscription_count, created_at, updated_at
		FROM dw_marketplace_items WHERE status = 'published'`
	args := []any{}
	if query = strings.ToLower(strings.TrimSpace(query)); query != "" {
		like := "%" + query + "%"
		q += ` AND (LOWER(name) LIKE ? OR LOWER(description) LIKE ? OR LOWER(tags_json) LIKE ?)`
		args = append(args, like, like, like)
	}
	if itemType != "" {
		q += ` AND item_type = ?`
		args = append(args, itemType)
	}
	if workspaceID != "" {
		q += ` AND workspace_id = ?`
		args = append(args, workspaceID)
	}
	q += ` ORDER BY subscription_count DESC, updated_at DESC, name`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksMarketplaceItem{}
	for rows.Next() {
		item, err := scanDataWorksMarketplaceItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLStore) InsertDataWorksMarketplaceSubscription(ctx context.Context, sub DataWorksMarketplaceSubscription) error {
	if sub.ID == "" || sub.UserID == "" || sub.ItemID == "" || sub.ItemKey == "" || sub.ItemType == "" {
		return errors.New("id, user_id, item_id, item_key, and item_type are required")
	}
	if strings.TrimSpace(sub.Purpose) == "" {
		return errors.New("purpose is required")
	}
	if sub.Status == "" {
		sub.Status = "pending"
	}
	now := formatTime(time.Now().UTC())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, s.bind(`INSERT INTO dw_marketplace_subscriptions
		(id, user_id, product_key, status, purpose, created_at, updated_at, item_type, item_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), sub.ID, sub.UserID, sub.ItemKey, sub.Status,
		sub.Purpose, now, now, sub.ItemType, sub.ItemID)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.bind(`UPDATE dw_marketplace_items SET subscription_count = subscription_count + 1,
		updated_at = ? WHERE id = ?`), now, sub.ItemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLStore) ListDataWorksMarketplaceSubscriptions(ctx context.Context, userID string) ([]DataWorksMarketplaceSubscription, error) {
	q := `SELECT id, user_id, item_id, product_key, item_type, status, purpose, created_at, updated_at
		FROM dw_marketplace_subscriptions`
	args := []any{}
	if userID != "" {
		q += ` WHERE user_id = ?`
		args = append(args, userID)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksMarketplaceSubscription{}
	for rows.Next() {
		var sub DataWorksMarketplaceSubscription
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.ItemID, &sub.ItemKey, &sub.ItemType,
			&sub.Status, &sub.Purpose, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *SQLStore) InsertDataWorksApprovalTask(ctx context.Context, task DataWorksApprovalTask) error {
	if task.ID == "" || task.TargetType == "" || task.TargetID == "" || task.Step == "" {
		return errors.New("id, target_type, target_id, and step are required")
	}
	if task.Status == "" {
		task.Status = "pending"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_approval_tasks
		(id, target_type, target_id, step, status, requested_by, decided_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), task.ID, task.TargetType, task.TargetID, task.Step,
		task.Status, task.RequestedBy, task.DecidedBy, now, now)
	return err
}
