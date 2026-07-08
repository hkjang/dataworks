package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type AssetReadinessScore struct {
	AssetKey              string `json:"asset_key"`
	SchemaScore           int    `json:"schema_score"`
	FreshnessScore        int    `json:"freshness_score"`
	SampleScore           int    `json:"sample_score"`
	MissingnessScore      int    `json:"missingness_score"`
	SensitivityScore      int    `json:"sensitivity_score"`
	ExternalSharingScore  int    `json:"external_sharing_score"`
	APIReadinessScore     int    `json:"api_readiness_score"`
	BillingReadinessScore int    `json:"billing_readiness_score"`
	OverallScore          int    `json:"overall_score"`
	Notes                 string `json:"notes"`
	UpdatedBy             string `json:"updated_by"`
	UpdatedAt             string `json:"updated_at"`
}

type ProductCanvasV2 struct {
	ProductKey         string `json:"product_key"`
	CustomerProblem    string `json:"customer_problem"`
	Buyer              string `json:"buyer"`
	UseCases           string `json:"use_cases"`
	ProvidedData       string `json:"provided_data"`
	Differentiation    string `json:"differentiation"`
	PricingModel       string `json:"pricing_model"`
	RiskNotes          string `json:"risk_notes"`
	POCSuccessCriteria string `json:"poc_success_criteria"`
	ExpectedRevenue    string `json:"expected_revenue"`
	Owner              string `json:"owner"`
	UpdatedBy          string `json:"updated_by"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type ApprovalTrace struct {
	ID          string `json:"id"`
	ProductKey  string `json:"product_key"`
	Step        string `json:"step"`
	Status      string `json:"status"` // pending | approved | rejected | waived | expired
	Required    bool   `json:"required"`
	EvidenceRef string `json:"evidence_ref"`
	Notes       string `json:"notes"`
	DecidedBy   string `json:"decided_by"`
	ExpiresAt   string `json:"expires_at"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type EvidencePack struct {
	ProductKey  string `json:"product_key"`
	PackJSON    string `json:"pack_json"`
	ArtifactRef string `json:"artifact_ref"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type ContractVersion struct {
	ID           string `json:"id"`
	ProductKey   string `json:"product_key"`
	Version      int    `json:"version"`
	ContractJSON string `json:"contract_json"`
	Status       string `json:"status"`
	CreatedBy    string `json:"created_by"`
	CreatedAt    string `json:"created_at"`
}

func (s *SQLStore) UpsertAssetReadinessScore(ctx context.Context, score AssetReadinessScore) error {
	score.AssetKey = strings.TrimSpace(score.AssetKey)
	if score.AssetKey == "" {
		return errors.New("asset_key is required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_asset_readiness_scores
		(asset_key, schema_score, freshness_score, sample_score, missingness_score, sensitivity_score,
		 external_sharing_score, api_readiness_score, billing_readiness_score, overall_score, notes, updated_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(asset_key) DO UPDATE SET
			schema_score = excluded.schema_score,
			freshness_score = excluded.freshness_score,
			sample_score = excluded.sample_score,
			missingness_score = excluded.missingness_score,
			sensitivity_score = excluded.sensitivity_score,
			external_sharing_score = excluded.external_sharing_score,
			api_readiness_score = excluded.api_readiness_score,
			billing_readiness_score = excluded.billing_readiness_score,
			overall_score = excluded.overall_score,
			notes = excluded.notes,
			updated_by = excluded.updated_by,
			updated_at = excluded.updated_at`),
		score.AssetKey, score.SchemaScore, score.FreshnessScore, score.SampleScore, score.MissingnessScore,
		score.SensitivityScore, score.ExternalSharingScore, score.APIReadinessScore, score.BillingReadinessScore,
		score.OverallScore, score.Notes, score.UpdatedBy, now)
	return err
}

func (s *SQLStore) ListAssetReadinessScores(ctx context.Context, assetKey string) ([]AssetReadinessScore, error) {
	q := `SELECT asset_key, schema_score, freshness_score, sample_score, missingness_score, sensitivity_score,
			external_sharing_score, api_readiness_score, billing_readiness_score, overall_score, notes, updated_by, updated_at
		FROM dw_asset_readiness_scores`
	args := []any{}
	if strings.TrimSpace(assetKey) != "" {
		q += ` WHERE asset_key = ?`
		args = append(args, strings.TrimSpace(assetKey))
	}
	q += ` ORDER BY overall_score ASC, asset_key`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AssetReadinessScore{}
	for rows.Next() {
		var score AssetReadinessScore
		if err := rows.Scan(&score.AssetKey, &score.SchemaScore, &score.FreshnessScore, &score.SampleScore,
			&score.MissingnessScore, &score.SensitivityScore, &score.ExternalSharingScore, &score.APIReadinessScore,
			&score.BillingReadinessScore, &score.OverallScore, &score.Notes, &score.UpdatedBy, &score.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, score)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertProductCanvasV2(ctx context.Context, canvas ProductCanvasV2) error {
	canvas.ProductKey = strings.TrimSpace(canvas.ProductKey)
	if canvas.ProductKey == "" {
		return errors.New("product_key is required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_product_canvases
		(product_key, customer_problem, buyer, use_cases, provided_data, differentiation, pricing_model,
		 risk_notes, poc_success_criteria, expected_revenue, owner, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(product_key) DO UPDATE SET
			customer_problem = excluded.customer_problem,
			buyer = excluded.buyer,
			use_cases = excluded.use_cases,
			provided_data = excluded.provided_data,
			differentiation = excluded.differentiation,
			pricing_model = excluded.pricing_model,
			risk_notes = excluded.risk_notes,
			poc_success_criteria = excluded.poc_success_criteria,
			expected_revenue = excluded.expected_revenue,
			owner = excluded.owner,
			updated_by = excluded.updated_by,
			updated_at = excluded.updated_at`),
		canvas.ProductKey, canvas.CustomerProblem, canvas.Buyer, canvas.UseCases, canvas.ProvidedData,
		canvas.Differentiation, canvas.PricingModel, canvas.RiskNotes, canvas.POCSuccessCriteria,
		canvas.ExpectedRevenue, canvas.Owner, canvas.UpdatedBy, now, now)
	return err
}

func (s *SQLStore) GetProductCanvasV2(ctx context.Context, productKey string) (ProductCanvasV2, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT product_key, customer_problem, buyer, use_cases, provided_data,
			differentiation, pricing_model, risk_notes, poc_success_criteria, expected_revenue, owner, updated_by, created_at, updated_at
		FROM dw_product_canvases WHERE product_key = ?`), strings.TrimSpace(productKey))
	var canvas ProductCanvasV2
	err := row.Scan(&canvas.ProductKey, &canvas.CustomerProblem, &canvas.Buyer, &canvas.UseCases, &canvas.ProvidedData,
		&canvas.Differentiation, &canvas.PricingModel, &canvas.RiskNotes, &canvas.POCSuccessCriteria,
		&canvas.ExpectedRevenue, &canvas.Owner, &canvas.UpdatedBy, &canvas.CreatedAt, &canvas.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductCanvasV2{}, false, nil
	}
	if err != nil {
		return ProductCanvasV2{}, false, err
	}
	return canvas, true, nil
}

func (s *SQLStore) UpsertApprovalTrace(ctx context.Context, trace ApprovalTrace) error {
	trace.ID = strings.TrimSpace(trace.ID)
	trace.ProductKey = strings.TrimSpace(trace.ProductKey)
	trace.Step = strings.TrimSpace(trace.Step)
	if trace.ID == "" || trace.ProductKey == "" || trace.Step == "" {
		return errors.New("id, product_key, and step are required")
	}
	if trace.Status == "" {
		trace.Status = "pending"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_approval_traces
		(id, product_key, step, status, required, evidence_ref, notes, decided_by, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			product_key = excluded.product_key,
			step = excluded.step,
			status = excluded.status,
			required = excluded.required,
			evidence_ref = excluded.evidence_ref,
			notes = excluded.notes,
			decided_by = excluded.decided_by,
			expires_at = excluded.expires_at,
			updated_at = excluded.updated_at`),
		trace.ID, trace.ProductKey, trace.Step, trace.Status, boolInt(trace.Required), trace.EvidenceRef,
		trace.Notes, trace.DecidedBy, trace.ExpiresAt, now, now)
	return err
}

func (s *SQLStore) ListApprovalTraces(ctx context.Context, productKey string) ([]ApprovalTrace, error) {
	q := `SELECT id, product_key, step, status, required, evidence_ref, notes, decided_by, expires_at, created_at, updated_at
		FROM dw_approval_traces`
	args := []any{}
	if strings.TrimSpace(productKey) != "" {
		q += ` WHERE product_key = ?`
		args = append(args, strings.TrimSpace(productKey))
	}
	q += ` ORDER BY product_key, required DESC, step, updated_at DESC`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ApprovalTrace{}
	for rows.Next() {
		var trace ApprovalTrace
		var required int
		if err := rows.Scan(&trace.ID, &trace.ProductKey, &trace.Step, &trace.Status, &required,
			&trace.EvidenceRef, &trace.Notes, &trace.DecidedBy, &trace.ExpiresAt, &trace.CreatedAt, &trace.UpdatedAt); err != nil {
			return nil, err
		}
		trace.Required = required == 1
		out = append(out, trace)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertEvidencePack(ctx context.Context, pack EvidencePack) error {
	pack.ProductKey = strings.TrimSpace(pack.ProductKey)
	if pack.ProductKey == "" {
		return errors.New("product_key is required")
	}
	if strings.TrimSpace(pack.PackJSON) == "" {
		pack.PackJSON = "{}"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_evidence_packs
		(product_key, pack_json, artifact_ref, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(product_key) DO UPDATE SET
			pack_json = excluded.pack_json,
			artifact_ref = excluded.artifact_ref,
			created_by = excluded.created_by,
			updated_at = excluded.updated_at`),
		pack.ProductKey, pack.PackJSON, pack.ArtifactRef, pack.CreatedBy, now, now)
	return err
}

func (s *SQLStore) GetEvidencePack(ctx context.Context, productKey string) (EvidencePack, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT product_key, pack_json, artifact_ref, created_by, created_at, updated_at
		FROM dw_evidence_packs WHERE product_key = ?`), strings.TrimSpace(productKey))
	var pack EvidencePack
	err := row.Scan(&pack.ProductKey, &pack.PackJSON, &pack.ArtifactRef, &pack.CreatedBy, &pack.CreatedAt, &pack.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EvidencePack{}, false, nil
	}
	if err != nil {
		return EvidencePack{}, false, err
	}
	return pack, true, nil
}

func (s *SQLStore) InsertContractVersion(ctx context.Context, version ContractVersion) (int, error) {
	version.ProductKey = strings.TrimSpace(version.ProductKey)
	if version.ID == "" || version.ProductKey == "" {
		return 0, errors.New("id and product_key are required")
	}
	if version.Status == "" {
		version.Status = "draft"
	}
	if strings.TrimSpace(version.ContractJSON) == "" {
		version.ContractJSON = "{}"
	}
	next := version.Version
	if next <= 0 {
		_ = s.db.QueryRowContext(ctx, s.bind(`SELECT COALESCE(MAX(version), 0) + 1 FROM dw_contract_versions WHERE product_key = ?`), version.ProductKey).Scan(&next)
	}
	if next <= 0 {
		next = 1
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_contract_versions
		(id, product_key, version, contract_json, status, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		version.ID, version.ProductKey, next, version.ContractJSON, version.Status, version.CreatedBy, formatTime(time.Now().UTC()))
	return next, err
}

func (s *SQLStore) LatestContractVersion(ctx context.Context, productKey string) (ContractVersion, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT id, product_key, version, contract_json, status, created_by, created_at
		FROM dw_contract_versions WHERE product_key = ? ORDER BY version DESC LIMIT 1`), strings.TrimSpace(productKey))
	var version ContractVersion
	err := row.Scan(&version.ID, &version.ProductKey, &version.Version, &version.ContractJSON, &version.Status, &version.CreatedBy, &version.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ContractVersion{}, false, nil
	}
	if err != nil {
		return ContractVersion{}, false, err
	}
	return version, true, nil
}
