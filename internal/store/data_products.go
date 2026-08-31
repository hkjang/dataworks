package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// DataProduct is a curated, published, team-scoped catalog entry that points at an existing
// building block (a saved Text2SQL report, a DW metric, or a golden query) by reference. It
// stores only metadata — never raw SQL/prompt — so teams can discover and request reusable data
// assets. The underlying source keeps its own (admin-only) definition.
type DataProduct struct {
	ID               string   `json:"id"`
	ProductKey       string   `json:"product_key"`
	NameKO           string   `json:"name_ko"`
	NameEN           string   `json:"name_en"`
	ShortName        string   `json:"short_name"`
	Description      string   `json:"description"`
	ExecutiveSummary string   `json:"executive_summary"`
	SalesPitch       string   `json:"sales_pitch"`
	SourceType       string   `json:"source_type"` // dataset | api | report | score | segment | model_feature | saved_report | metric | golden_query | custom
	SourceRef        string   `json:"source_ref"`  // id/key of the source (no raw SQL stored)
	Owner            string   `json:"owner"`
	AllowedTeams     []string `json:"allowed_teams"` // empty = all teams
	Sensitivity      string   `json:"sensitivity"`   // public | internal | restricted | personal_credit | pseudonymized | aggregated
	Status           string   `json:"status"`        // draft | review | risk_review | approved | published | archived
	Version          int      `json:"version"`
	TargetIndustries []string `json:"target_industries"`
	TargetCustomers  []string `json:"target_customers"`
	PricingModel     string   `json:"pricing_model"`
	APISpec          string   `json:"api_spec"`
	POCPlan          string   `json:"poc_plan"`
	RiskScore        int      `json:"risk_score"`
	RevenueScore     int      `json:"revenue_score"`
	Differentiation  string   `json:"differentiation"`
	SimilarProducts  []string `json:"similar_products"`
	UpdatedBy        string   `json:"updated_by"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

// DataProductKeyRetiredError reports an attempt to reuse a product key that was
// permanently retired. Product-scoped governance records are intentionally kept
// after catalog deletion, so reusing the key would attach that history to a new
// product.
type DataProductKeyRetiredError struct {
	ProductKey string
}

func (e *DataProductKeyRetiredError) Error() string {
	return "data product key is permanently retired: " + e.ProductKey
}

// DataProductAccessRequest is a team member's request to use a data product (mirrors the skill
// marketplace access-request model).
type DataProductAccessRequest struct {
	ID         string `json:"id"`
	ProductKey string `json:"product_key"`
	UserID     string `json:"user_id"`
	Team       string `json:"team"`
	Status     string `json:"status"` // pending | approved | denied
	Reason     string `json:"reason"`
	DecidedBy  string `json:"decided_by"`
	CreatedAt  string `json:"created_at"`
}

// UpsertDataProduct inserts or updates a data product by product_key, bumping version on update.
func (s *SQLStore) UpsertDataProduct(ctx context.Context, p DataProduct) error {
	p.ProductKey = strings.TrimSpace(p.ProductKey)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if p.Sensitivity == "" {
		p.Sensitivity = "internal"
	}
	if p.SourceType == "" {
		p.SourceType = "custom"
	}
	if p.Status == "" {
		p.Status = "draft"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Lock an existing catalog row before checking the permanent tombstone. This
	// serializes updates with DeleteDataProduct, whose lock order is identical.
	lockQuery := `SELECT product_key FROM data_products WHERE product_key = ?`
	if s.dialect == "postgres" {
		lockQuery += ` FOR UPDATE`
	}
	var existingKey string
	if err := tx.QueryRowContext(ctx, s.bind(lockQuery), p.ProductKey).Scan(&existingKey); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var retiredKey string
	err = tx.QueryRowContext(ctx, s.bind(`SELECT product_key FROM data_product_retired_keys WHERE product_key = ?`), p.ProductKey).Scan(&retiredKey)
	if err == nil {
		return &DataProductKeyRetiredError{ProductKey: retiredKey}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	_, err = tx.ExecContext(ctx, s.bind(`INSERT INTO data_products
		(id, product_key, name_ko, name_en, short_name, description, executive_summary, sales_pitch, source_type, source_ref, owner,
		 allowed_teams, sensitivity, status, version, target_industries, target_customers, pricing_model, api_spec, poc_plan,
		 risk_score, revenue_score, differentiation, similar_products, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(product_key) DO UPDATE SET
			name_ko = excluded.name_ko, name_en = excluded.name_en, short_name = excluded.short_name,
			description = excluded.description, executive_summary = excluded.executive_summary, sales_pitch = excluded.sales_pitch,
			source_type = excluded.source_type, source_ref = excluded.source_ref, owner = excluded.owner, allowed_teams = excluded.allowed_teams,
			sensitivity = excluded.sensitivity, status = excluded.status, version = data_products.version + 1,
			target_industries = excluded.target_industries, target_customers = excluded.target_customers,
			pricing_model = excluded.pricing_model, api_spec = excluded.api_spec, poc_plan = excluded.poc_plan,
			risk_score = excluded.risk_score, revenue_score = excluded.revenue_score, differentiation = excluded.differentiation,
			similar_products = excluded.similar_products, updated_by = excluded.updated_by, updated_at = excluded.updated_at`),
		p.ID, p.ProductKey, p.NameKO, p.NameEN, p.ShortName, p.Description, p.ExecutiveSummary, p.SalesPitch, p.SourceType, p.SourceRef, p.Owner,
		strings.Join(p.AllowedTeams, ","), p.Sensitivity, p.Status, strings.Join(p.TargetIndustries, ","), strings.Join(p.TargetCustomers, ","),
		p.PricingModel, p.APISpec, p.POCPlan, p.RiskScore, p.RevenueScore, p.Differentiation, strings.Join(p.SimilarProducts, ","),
		p.UpdatedBy, now, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func scanDataProduct(sc interface{ Scan(...any) error }) (DataProduct, error) {
	var p DataProduct
	var teams, industries, customers, similar string
	if err := sc.Scan(&p.ID, &p.ProductKey, &p.NameKO, &p.NameEN, &p.ShortName, &p.Description, &p.ExecutiveSummary, &p.SalesPitch,
		&p.SourceType, &p.SourceRef, &p.Owner, &teams, &p.Sensitivity, &p.Status, &p.Version, &industries, &customers,
		&p.PricingModel, &p.APISpec, &p.POCPlan, &p.RiskScore, &p.RevenueScore, &p.Differentiation, &similar,
		&p.UpdatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return DataProduct{}, err
	}
	p.AllowedTeams = splitCSVField(teams)
	p.TargetIndustries = splitCSVField(industries)
	p.TargetCustomers = splitCSVField(customers)
	p.SimilarProducts = splitCSVField(similar)
	return p, nil
}

// ListDataProducts returns products, optionally filtered by status (empty = all), newest first.
func (s *SQLStore) ListDataProducts(ctx context.Context, status string) ([]DataProduct, error) {
	q := `SELECT id, product_key, name_ko, COALESCE(name_en,''), COALESCE(short_name,''), description, COALESCE(executive_summary,''), COALESCE(sales_pitch,''),
			source_type, source_ref, owner, allowed_teams, sensitivity, status, version, COALESCE(target_industries,''), COALESCE(target_customers,''),
			COALESCE(pricing_model,''), COALESCE(api_spec,''), COALESCE(poc_plan,''), COALESCE(risk_score,0), COALESCE(revenue_score,0),
			COALESCE(differentiation,''), COALESCE(similar_products,''), updated_by, created_at, updated_at
		FROM data_products`
	args := []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataProduct{}
	for rows.Next() {
		p, err := scanDataProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetDataProduct returns a product by id or product_key.
func (s *SQLStore) GetDataProduct(ctx context.Context, idOrKey string) (DataProduct, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, product_key, name_ko, COALESCE(name_en,''), COALESCE(short_name,''), description,
			COALESCE(executive_summary,''), COALESCE(sales_pitch,''), source_type, source_ref, owner, allowed_teams, sensitivity, status, version,
			COALESCE(target_industries,''), COALESCE(target_customers,''), COALESCE(pricing_model,''), COALESCE(api_spec,''), COALESCE(poc_plan,''),
			COALESCE(risk_score,0), COALESCE(revenue_score,0), COALESCE(differentiation,''), COALESCE(similar_products,''), updated_by, created_at, updated_at
		FROM data_products WHERE id = ? OR product_key = ?`), idOrKey, idOrKey)
	p, err := scanDataProduct(row)
	if errors.Is(err, sql.ErrNoRows) {
		return DataProduct{}, false, nil
	}
	if err != nil {
		return DataProduct{}, false, err
	}
	return p, true, nil
}

// GetDataProductByKey returns a product by an exact product_key match. Use this
// for key-addressed routes so a different row whose id happens to equal the key
// cannot be selected.
func (s *SQLStore) GetDataProductByKey(ctx context.Context, productKey string) (DataProduct, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, product_key, name_ko, COALESCE(name_en,''), COALESCE(short_name,''), description,
			COALESCE(executive_summary,''), COALESCE(sales_pitch,''), source_type, source_ref, owner, allowed_teams, sensitivity, status, version,
			COALESCE(target_industries,''), COALESCE(target_customers,''), COALESCE(pricing_model,''), COALESCE(api_spec,''), COALESCE(poc_plan,''),
			COALESCE(risk_score,0), COALESCE(revenue_score,0), COALESCE(differentiation,''), COALESCE(similar_products,''), updated_by, created_at, updated_at
		FROM data_products WHERE product_key = ?`), strings.TrimSpace(productKey))
	p, err := scanDataProduct(row)
	if errors.Is(err, sql.ErrNoRows) {
		return DataProduct{}, false, nil
	}
	if err != nil {
		return DataProduct{}, false, err
	}
	return p, true, nil
}

func splitCSVField(value string) []string {
	out := []string{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// DeleteDataProduct atomically retires a product key and removes its catalog
// row. The permanent tombstone prevents retained canvas, approval, contract,
// and other product-scoped history from ever being inherited by a new product.
func (s *SQLStore) DeleteDataProduct(ctx context.Context, idOrKey string) error {
	idOrKey = strings.TrimSpace(idOrKey)
	if idOrKey == "" {
		return errors.New("data product id or product_key is required")
	}
	return s.deleteDataProduct(ctx, `SELECT id, product_key FROM data_products WHERE id = ? OR product_key = ?`, idOrKey, idOrKey)
}

// DeleteDataProductByKey atomically retires and removes only the row whose
// product_key exactly matches productKey.
func (s *SQLStore) DeleteDataProductByKey(ctx context.Context, productKey string) error {
	productKey = strings.TrimSpace(productKey)
	if productKey == "" {
		return errors.New("product_key is required")
	}
	return s.deleteDataProduct(ctx, `SELECT id, product_key FROM data_products WHERE product_key = ?`, productKey)
}

func (s *SQLStore) deleteDataProduct(ctx context.Context, lookupQuery string, lookupArgs ...any) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if s.dialect == "postgres" {
		lookupQuery += ` FOR UPDATE`
	}
	var productID, productKey string
	if err := tx.QueryRowContext(ctx, s.bind(lookupQuery), lookupArgs...).Scan(&productID, &productKey); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, s.bind(`INSERT INTO data_product_retired_keys
		(product_key, product_id, retired_at) VALUES (?, ?, ?)
		ON CONFLICT(product_key) DO NOTHING`), productKey, productID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.bind(`DELETE FROM data_products WHERE id = ? AND product_key = ?`), productID, productKey); err != nil {
		return err
	}
	return tx.Commit()
}

// AddDataProductAccessRequest records a member's access request (defaults to pending).
func (s *SQLStore) AddDataProductAccessRequest(ctx context.Context, rq DataProductAccessRequest) error {
	if rq.Status == "" {
		rq.Status = "pending"
	}
	rq.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO data_product_access_requests
		(id, product_key, user_id, team, status, reason, decided_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		rq.ID, rq.ProductKey, rq.UserID, rq.Team, rq.Status, rq.Reason, rq.DecidedBy, rq.CreatedAt)
	return err
}

// ListDataProductAccessRequests returns access requests, optionally filtered by product_key.
func (s *SQLStore) ListDataProductAccessRequests(ctx context.Context, productKey string) ([]DataProductAccessRequest, error) {
	q := `SELECT id, product_key, user_id, team, status, reason, decided_by, created_at FROM data_product_access_requests`
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
	out := []DataProductAccessRequest{}
	for rows.Next() {
		var rq DataProductAccessRequest
		if err := rows.Scan(&rq.ID, &rq.ProductKey, &rq.UserID, &rq.Team, &rq.Status, &rq.Reason, &rq.DecidedBy, &rq.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rq)
	}
	return out, rows.Err()
}

// DecideDataProductAccessRequest approves or denies a request.
func (s *SQLStore) DecideDataProductAccessRequest(ctx context.Context, id string, approve bool, by string) error {
	status := "denied"
	if approve {
		status = "approved"
	}
	_, err := s.db.ExecContext(ctx, s.bind(`UPDATE data_product_access_requests SET status = ?, decided_by = ? WHERE id = ?`), status, by, id)
	return err
}
