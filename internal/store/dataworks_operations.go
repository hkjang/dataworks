package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"
)

type CustomerSegment struct {
	SegmentKey  string   `json:"segment_key"`
	Industry    string   `json:"industry"`
	BuyerType   string   `json:"buyer_type"`
	PainPoints  []string `json:"pain_points"`
	BudgetLevel string   `json:"budget_level"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type ProductFitScore struct {
	ProductKey      string   `json:"product_key"`
	CustomerSegment string   `json:"customer_segment"`
	FitScore        int      `json:"fit_score"`
	Reason          string   `json:"reason"`
	EvidenceRefs    []string `json:"evidence_refs"`
	UpdatedAt       string   `json:"updated_at"`
}

type ProductVersion struct {
	ProductKey   string `json:"product_key"`
	Version      int    `json:"version"`
	SnapshotJSON string `json:"snapshot_json"`
	DiffSummary  string `json:"diff_summary"`
	ChangedBy    string `json:"changed_by"`
	ChangedAt    string `json:"changed_at"`
}

type ContractScope struct {
	ContractKey   string   `json:"contract_key"`
	ProductKey    string   `json:"product_key"`
	CustomerKey   string   `json:"customer_key"`
	AllowedFields []string `json:"allowed_fields"`
	RateLimit     int      `json:"rate_limit"`
	ValidFrom     string   `json:"valid_from"`
	ValidTo       string   `json:"valid_to"`
	Purpose       string   `json:"purpose"`
	Restrictions  string   `json:"restrictions"`
	Status        string   `json:"status"`
	CreatedBy     string   `json:"created_by"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	MaskingPolicy string   `json:"masking_policy"`
}

type APIEntitlement struct {
	ID          string `json:"id"`
	APIKeyID    string `json:"api_key_id"`
	APIKeyHash  string `json:"api_key_hash"`
	CustomerKey string `json:"customer_key"`
	ProductKey  string `json:"product_key"`
	ContractKey string `json:"contract_key"`
	Scope       string `json:"scope"`
	ExpiresAt   string `json:"expires_at"`
	Status      string `json:"status"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type ProductSLA struct {
	ProductKey         string  `json:"product_key"`
	RefreshCycle       string  `json:"refresh_cycle"`
	LatencyTargetMS    int     `json:"latency_target_ms"`
	AvailabilityTarget float64 `json:"availability_target"`
	SupportLevel       string  `json:"support_level"`
	UpdatedBy          string  `json:"updated_by"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

type DataWatermark struct {
	AssetKey     string `json:"asset_key"`
	ProductKey   string `json:"product_key"`
	LastLoadedAt string `json:"last_loaded_at"`
	DataAsOf     string `json:"data_as_of"`
	DelayStatus  string `json:"delay_status"`
	Notes        string `json:"notes"`
	UpdatedBy    string `json:"updated_by"`
	UpdatedAt    string `json:"updated_at"`
}

type ProductCost struct {
	ProductKey         string  `json:"product_key"`
	QueryCost          float64 `json:"query_cost"`
	LLMCost            float64 `json:"llm_cost"`
	OpsCost            float64 `json:"ops_cost"`
	DataProcessingCost float64 `json:"data_processing_cost"`
	EstimatedMargin    float64 `json:"estimated_margin"`
	Currency           string  `json:"currency"`
	UpdatedBy          string  `json:"updated_by"`
	UpdatedAt          string  `json:"updated_at"`
	ExpectedRevenue    float64 `json:"expected_revenue"`
}

type CustomerProposalEvent struct {
	ID           string `json:"id"`
	CustomerKey  string `json:"customer_key"`
	ProductKey   string `json:"product_key"`
	ProposalID   string `json:"proposal_id"`
	Variant      string `json:"variant"`
	EventType    string `json:"event_type"`
	Feedback     string `json:"feedback"`
	NextActionAt string `json:"next_action_at"`
	CreatedBy    string `json:"created_by"`
	CreatedAt    string `json:"created_at"`
}

type RetirementCandidate struct {
	ProductKey     string `json:"product_key"`
	Reason         string `json:"reason"`
	RiskScore      int    `json:"risk_score"`
	UsageCount     int    `json:"usage_count"`
	LastUsedAt     string `json:"last_used_at"`
	Recommendation string `json:"recommendation"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func (s *SQLStore) UpsertCustomerSegment(ctx context.Context, seg CustomerSegment) error {
	seg.SegmentKey = strings.TrimSpace(seg.SegmentKey)
	if seg.SegmentKey == "" {
		return errors.New("segment_key is required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_customer_segments
		(segment_key, industry, buyer_type, pain_points, budget_level, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(segment_key) DO UPDATE SET
			industry = excluded.industry,
			buyer_type = excluded.buyer_type,
			pain_points = excluded.pain_points,
			budget_level = excluded.budget_level,
			updated_at = excluded.updated_at`),
		seg.SegmentKey, seg.Industry, seg.BuyerType, strings.Join(seg.PainPoints, ","), seg.BudgetLevel, now, now)
	return err
}

func (s *SQLStore) ListCustomerSegments(ctx context.Context, segmentKey string) ([]CustomerSegment, error) {
	q := `SELECT segment_key, industry, buyer_type, COALESCE(pain_points,''), budget_level, created_at, updated_at
		FROM dw_customer_segments`
	args := []any{}
	if strings.TrimSpace(segmentKey) != "" {
		q += ` WHERE segment_key = ?`
		args = append(args, strings.TrimSpace(segmentKey))
	}
	q += ` ORDER BY segment_key`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomerSegment{}
	for rows.Next() {
		var seg CustomerSegment
		var painPoints string
		if err := rows.Scan(&seg.SegmentKey, &seg.Industry, &seg.BuyerType, &painPoints, &seg.BudgetLevel, &seg.CreatedAt, &seg.UpdatedAt); err != nil {
			return nil, err
		}
		seg.PainPoints = splitCSVField(painPoints)
		out = append(out, seg)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertProductFitScore(ctx context.Context, score ProductFitScore) error {
	score.ProductKey = strings.TrimSpace(score.ProductKey)
	score.CustomerSegment = strings.TrimSpace(score.CustomerSegment)
	if score.ProductKey == "" || score.CustomerSegment == "" {
		return errors.New("product_key and customer_segment are required")
	}
	if score.FitScore < 0 {
		score.FitScore = 0
	}
	if score.FitScore > 100 {
		score.FitScore = 100
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_product_fit_scores
		(product_key, customer_segment, fit_score, reason, evidence_refs, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(product_key, customer_segment) DO UPDATE SET
			fit_score = excluded.fit_score,
			reason = excluded.reason,
			evidence_refs = excluded.evidence_refs,
			updated_at = excluded.updated_at`),
		score.ProductKey, score.CustomerSegment, score.FitScore, score.Reason, strings.Join(score.EvidenceRefs, ","), now)
	return err
}

func (s *SQLStore) ListProductFitScores(ctx context.Context, productKey string, segmentKey string) ([]ProductFitScore, error) {
	q := `SELECT product_key, customer_segment, fit_score, reason, COALESCE(evidence_refs,''), updated_at
		FROM dw_product_fit_scores WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(productKey) != "" {
		q += ` AND product_key = ?`
		args = append(args, strings.TrimSpace(productKey))
	}
	if strings.TrimSpace(segmentKey) != "" {
		q += ` AND customer_segment = ?`
		args = append(args, strings.TrimSpace(segmentKey))
	}
	q += ` ORDER BY fit_score DESC, product_key, customer_segment`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProductFitScore{}
	for rows.Next() {
		var score ProductFitScore
		var refs string
		if err := rows.Scan(&score.ProductKey, &score.CustomerSegment, &score.FitScore, &score.Reason, &refs, &score.UpdatedAt); err != nil {
			return nil, err
		}
		score.EvidenceRefs = splitCSVField(refs)
		out = append(out, score)
	}
	return out, rows.Err()
}

func (s *SQLStore) InsertProductVersion(ctx context.Context, version ProductVersion) (int, error) {
	version.ProductKey = strings.TrimSpace(version.ProductKey)
	if version.ProductKey == "" {
		return 0, errors.New("product_key is required")
	}
	if strings.TrimSpace(version.SnapshotJSON) == "" {
		version.SnapshotJSON = "{}"
	}
	next := version.Version
	if next <= 0 {
		_ = s.db.QueryRowContext(ctx, s.bind(`SELECT COALESCE(MAX(version), 0) + 1 FROM dw_product_versions WHERE product_key = ?`), version.ProductKey).Scan(&next)
	}
	if next <= 0 {
		next = 1
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_product_versions
		(product_key, version, snapshot_json, diff_summary, changed_by, changed_at)
		VALUES (?, ?, ?, ?, ?, ?)`),
		version.ProductKey, next, version.SnapshotJSON, version.DiffSummary, version.ChangedBy, formatTime(time.Now().UTC()))
	return next, err
}

func (s *SQLStore) ListProductVersions(ctx context.Context, productKey string) ([]ProductVersion, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`SELECT product_key, version, snapshot_json, diff_summary, changed_by, changed_at
		FROM dw_product_versions WHERE product_key = ? ORDER BY version DESC`), strings.TrimSpace(productKey))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProductVersion{}
	for rows.Next() {
		var version ProductVersion
		if err := rows.Scan(&version.ProductKey, &version.Version, &version.SnapshotJSON, &version.DiffSummary, &version.ChangedBy, &version.ChangedAt); err != nil {
			return nil, err
		}
		out = append(out, version)
	}
	return out, rows.Err()
}

func (s *SQLStore) GetProductVersion(ctx context.Context, productKey string, versionNo int) (ProductVersion, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT product_key, version, snapshot_json, diff_summary, changed_by, changed_at
		FROM dw_product_versions WHERE product_key = ? AND version = ?`), strings.TrimSpace(productKey), versionNo)
	var version ProductVersion
	err := row.Scan(&version.ProductKey, &version.Version, &version.SnapshotJSON, &version.DiffSummary, &version.ChangedBy, &version.ChangedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductVersion{}, false, nil
	}
	if err != nil {
		return ProductVersion{}, false, err
	}
	return version, true, nil
}

func (s *SQLStore) LatestProductVersion(ctx context.Context, productKey string) (ProductVersion, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT product_key, version, snapshot_json, diff_summary, changed_by, changed_at
		FROM dw_product_versions WHERE product_key = ? ORDER BY version DESC LIMIT 1`), strings.TrimSpace(productKey))
	var version ProductVersion
	err := row.Scan(&version.ProductKey, &version.Version, &version.SnapshotJSON, &version.DiffSummary, &version.ChangedBy, &version.ChangedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductVersion{}, false, nil
	}
	if err != nil {
		return ProductVersion{}, false, err
	}
	return version, true, nil
}

func (s *SQLStore) UpsertContractScope(ctx context.Context, scope ContractScope) error {
	scope.ContractKey = strings.TrimSpace(scope.ContractKey)
	scope.ProductKey = strings.TrimSpace(scope.ProductKey)
	if scope.ContractKey == "" || scope.ProductKey == "" {
		return errors.New("contract_key and product_key are required")
	}
	if strings.TrimSpace(scope.Status) == "" {
		scope.Status = "active"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_contract_scopes
		(contract_key, product_key, customer_key, allowed_fields, rate_limit, valid_from, valid_to, purpose, restrictions, status, created_by, created_at, updated_at, masking_policy)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(contract_key) DO UPDATE SET
			product_key = excluded.product_key,
			customer_key = excluded.customer_key,
			allowed_fields = excluded.allowed_fields,
			rate_limit = excluded.rate_limit,
			valid_from = excluded.valid_from,
			valid_to = excluded.valid_to,
			purpose = excluded.purpose,
			restrictions = excluded.restrictions,
			status = excluded.status,
			created_by = excluded.created_by,
			updated_at = excluded.updated_at,
			masking_policy = excluded.masking_policy`),
		scope.ContractKey, scope.ProductKey, scope.CustomerKey, strings.Join(scope.AllowedFields, ","), scope.RateLimit,
		scope.ValidFrom, scope.ValidTo, scope.Purpose, scope.Restrictions, scope.Status, scope.CreatedBy, now, now, scope.MaskingPolicy)
	return err
}

func (s *SQLStore) ListContractScopes(ctx context.Context, productKey string, customerKey string) ([]ContractScope, error) {
	q := `SELECT contract_key, product_key, customer_key, COALESCE(allowed_fields,''), rate_limit, valid_from, valid_to,
			purpose, restrictions, status, created_by, created_at, updated_at, COALESCE(masking_policy,'')
		FROM dw_contract_scopes WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(productKey) != "" {
		q += ` AND product_key = ?`
		args = append(args, strings.TrimSpace(productKey))
	}
	if strings.TrimSpace(customerKey) != "" {
		q += ` AND customer_key = ?`
		args = append(args, strings.TrimSpace(customerKey))
	}
	q += ` ORDER BY updated_at DESC, contract_key`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContractScope{}
	for rows.Next() {
		scope, err := scanContractScope(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, scope)
	}
	return out, rows.Err()
}

func (s *SQLStore) GetContractScope(ctx context.Context, contractKey string) (ContractScope, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT contract_key, product_key, customer_key, COALESCE(allowed_fields,''), rate_limit, valid_from, valid_to,
			purpose, restrictions, status, created_by, created_at, updated_at, COALESCE(masking_policy,'')
		FROM dw_contract_scopes WHERE contract_key = ?`), strings.TrimSpace(contractKey))
	scope, err := scanContractScope(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ContractScope{}, false, nil
	}
	if err != nil {
		return ContractScope{}, false, err
	}
	return scope, true, nil
}

func scanContractScope(sc interface{ Scan(...any) error }) (ContractScope, error) {
	var scope ContractScope
	var fields string
	if err := sc.Scan(&scope.ContractKey, &scope.ProductKey, &scope.CustomerKey, &fields, &scope.RateLimit,
		&scope.ValidFrom, &scope.ValidTo, &scope.Purpose, &scope.Restrictions, &scope.Status,
		&scope.CreatedBy, &scope.CreatedAt, &scope.UpdatedAt, &scope.MaskingPolicy); err != nil {
		return ContractScope{}, err
	}
	scope.AllowedFields = splitCSVField(fields)
	return scope, nil
}

func (s *SQLStore) UpsertAPIEntitlement(ctx context.Context, ent APIEntitlement) error {
	ent.ID = strings.TrimSpace(ent.ID)
	ent.ProductKey = strings.TrimSpace(ent.ProductKey)
	if ent.ID == "" || ent.ProductKey == "" {
		return errors.New("id and product_key are required")
	}
	if strings.TrimSpace(ent.APIKeyID) == "" && strings.TrimSpace(ent.APIKeyHash) == "" {
		return errors.New("api_key_id or api_key_hash is required")
	}
	if strings.TrimSpace(ent.ContractKey) == "" {
		return errors.New("contract_key is required")
	}
	if strings.TrimSpace(ent.Status) == "" {
		ent.Status = "active"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_api_entitlements
		(id, api_key_id, api_key_hash, customer_key, product_key, contract_key, scope, expires_at, status, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			api_key_id = excluded.api_key_id,
			api_key_hash = excluded.api_key_hash,
			customer_key = excluded.customer_key,
			product_key = excluded.product_key,
			contract_key = excluded.contract_key,
			scope = excluded.scope,
			expires_at = excluded.expires_at,
			status = excluded.status,
			created_by = excluded.created_by,
			updated_at = excluded.updated_at`),
		ent.ID, ent.APIKeyID, ent.APIKeyHash, ent.CustomerKey, ent.ProductKey, ent.ContractKey, ent.Scope,
		ent.ExpiresAt, ent.Status, ent.CreatedBy, now, now)
	return err
}

func (s *SQLStore) ListAPIEntitlements(ctx context.Context, productKey string, apiKeyID string) ([]APIEntitlement, error) {
	q := `SELECT id, api_key_id, api_key_hash, customer_key, product_key, contract_key, scope, expires_at, status, created_by, created_at, updated_at
		FROM dw_api_entitlements WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(productKey) != "" {
		q += ` AND product_key = ?`
		args = append(args, strings.TrimSpace(productKey))
	}
	if strings.TrimSpace(apiKeyID) != "" {
		q += ` AND api_key_id = ?`
		args = append(args, strings.TrimSpace(apiKeyID))
	}
	q += ` ORDER BY updated_at DESC, id`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIEntitlement{}
	for rows.Next() {
		var ent APIEntitlement
		if err := rows.Scan(&ent.ID, &ent.APIKeyID, &ent.APIKeyHash, &ent.CustomerKey, &ent.ProductKey, &ent.ContractKey,
			&ent.Scope, &ent.ExpiresAt, &ent.Status, &ent.CreatedBy, &ent.CreatedAt, &ent.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, ent)
	}
	return out, rows.Err()
}

// ListAPIEntitlementCandidates returns every entitlement row an API key holds for one product,
// ordered the way the runtime access gate should consider them. Entitlements are keyed by id
// alone, so one key can hold several rows for the same product: a renewal issued next to the
// expired trial it replaces, or a revoked row kept for audit. Rows that are active right now
// come first and the newest wins inside each group, so the caller can walk the list until it
// finds a grant whose contract scope is usable too.
func (s *SQLStore) ListAPIEntitlementCandidates(ctx context.Context, productKey string, apiKeyID string, apiKeyHash string) ([]APIEntitlement, error) {
	productKey = strings.TrimSpace(productKey)
	apiKeyID = strings.TrimSpace(apiKeyID)
	apiKeyHash = strings.TrimSpace(apiKeyHash)
	if productKey == "" || (apiKeyID == "" && apiKeyHash == "") {
		return nil, nil
	}
	q := `SELECT id, api_key_id, api_key_hash, customer_key, product_key, contract_key, scope, expires_at, status, created_by, created_at, updated_at
		FROM dw_api_entitlements
		WHERE product_key = ? AND (`
	args := []any{productKey}
	conds := []string{}
	if apiKeyID != "" {
		conds = append(conds, `api_key_id = ?`)
		args = append(args, apiKeyID)
	}
	if apiKeyHash != "" {
		conds = append(conds, `api_key_hash = ?`)
		args = append(args, apiKeyHash)
	}
	// A key holds a handful of rows per product at most; the cap only bounds the scan.
	q += strings.Join(conds, ` OR `) + `) ORDER BY updated_at DESC LIMIT 50`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ents []APIEntitlement
	for rows.Next() {
		var ent APIEntitlement
		if err := rows.Scan(&ent.ID, &ent.APIKeyID, &ent.APIKeyHash, &ent.CustomerKey, &ent.ProductKey, &ent.ContractKey,
			&ent.Scope, &ent.ExpiresAt, &ent.Status, &ent.CreatedBy, &ent.CreatedAt, &ent.UpdatedAt); err != nil {
			return nil, err
		}
		ents = append(ents, ent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	sort.SliceStable(ents, func(i, j int) bool {
		activeI, activeJ := EntitlementActive(ents[i], now), EntitlementActive(ents[j], now)
		if activeI != activeJ {
			return activeI
		}
		return entitlementNewer(ents[i], ents[j])
	})
	return ents, nil
}

// FindAPIEntitlement returns the entitlement the runtime access gate falls back to for an API
// key: the newest active row, or the newest row overall when none is active so the gate reports
// an inactive entitlement rather than a missing one. Callers that can tell whether a grant is
// actually usable should walk ListAPIEntitlementCandidates instead.
func (s *SQLStore) FindAPIEntitlement(ctx context.Context, productKey string, apiKeyID string, apiKeyHash string) (APIEntitlement, bool, error) {
	ents, err := s.ListAPIEntitlementCandidates(ctx, productKey, apiKeyID, apiKeyHash)
	if err != nil || len(ents) == 0 {
		return APIEntitlement{}, false, err
	}
	return ents[0], true, nil
}

// EntitlementActive reports whether an entitlement grants access right now. An expiry the
// gateway cannot parse counts as expired so a broken timestamp never keeps access open.
func EntitlementActive(ent APIEntitlement, now time.Time) bool {
	if !strings.EqualFold(strings.TrimSpace(ent.Status), "active") {
		return false
	}
	if strings.TrimSpace(ent.ExpiresAt) == "" {
		return true
	}
	expiresAt, ok := parseStoredTime(strings.TrimSpace(ent.ExpiresAt))
	return ok && expiresAt.After(now)
}

// entitlementNewer orders two entitlement rows by last write. Rows written in the same instant
// fall back to the id so repeated requests keep resolving to the same grant.
func entitlementNewer(a APIEntitlement, b APIEntitlement) bool {
	at, aok := parseStoredTime(a.UpdatedAt)
	bt, bok := parseStoredTime(b.UpdatedAt)
	if aok && bok && !at.Equal(bt) {
		return at.After(bt)
	}
	if a.UpdatedAt != b.UpdatedAt {
		return a.UpdatedAt > b.UpdatedAt
	}
	return a.ID > b.ID
}

func (s *SQLStore) UpsertProductSLA(ctx context.Context, sla ProductSLA) error {
	sla.ProductKey = strings.TrimSpace(sla.ProductKey)
	if sla.ProductKey == "" {
		return errors.New("product_key is required")
	}
	if sla.AvailabilityTarget < 0 {
		sla.AvailabilityTarget = 0
	}
	if sla.AvailabilityTarget > 1 {
		sla.AvailabilityTarget = 1
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_product_sla
		(product_key, refresh_cycle, latency_target_ms, availability_target, support_level, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(product_key) DO UPDATE SET
			refresh_cycle = excluded.refresh_cycle,
			latency_target_ms = excluded.latency_target_ms,
			availability_target = excluded.availability_target,
			support_level = excluded.support_level,
			updated_by = excluded.updated_by,
			updated_at = excluded.updated_at`),
		sla.ProductKey, sla.RefreshCycle, sla.LatencyTargetMS, sla.AvailabilityTarget, sla.SupportLevel, sla.UpdatedBy, now, now)
	return err
}

func (s *SQLStore) GetProductSLA(ctx context.Context, productKey string) (ProductSLA, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT product_key, refresh_cycle, latency_target_ms, availability_target,
			support_level, updated_by, created_at, updated_at
		FROM dw_product_sla WHERE product_key = ?`), strings.TrimSpace(productKey))
	var sla ProductSLA
	err := row.Scan(&sla.ProductKey, &sla.RefreshCycle, &sla.LatencyTargetMS, &sla.AvailabilityTarget, &sla.SupportLevel, &sla.UpdatedBy, &sla.CreatedAt, &sla.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductSLA{}, false, nil
	}
	if err != nil {
		return ProductSLA{}, false, err
	}
	return sla, true, nil
}

func (s *SQLStore) UpsertDataWatermark(ctx context.Context, wm DataWatermark) error {
	wm.AssetKey = strings.TrimSpace(wm.AssetKey)
	wm.ProductKey = strings.TrimSpace(wm.ProductKey)
	if wm.AssetKey == "" || wm.ProductKey == "" {
		return errors.New("asset_key and product_key are required")
	}
	if strings.TrimSpace(wm.DelayStatus) == "" {
		wm.DelayStatus = "fresh"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_data_watermarks
		(asset_key, product_key, last_loaded_at, data_as_of, delay_status, notes, updated_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(asset_key, product_key) DO UPDATE SET
			last_loaded_at = excluded.last_loaded_at,
			data_as_of = excluded.data_as_of,
			delay_status = excluded.delay_status,
			notes = excluded.notes,
			updated_by = excluded.updated_by,
			updated_at = excluded.updated_at`),
		wm.AssetKey, wm.ProductKey, wm.LastLoadedAt, wm.DataAsOf, wm.DelayStatus, wm.Notes, wm.UpdatedBy, now)
	return err
}

func (s *SQLStore) ListDataWatermarks(ctx context.Context, productKey string, assetKey string) ([]DataWatermark, error) {
	q := `SELECT asset_key, product_key, last_loaded_at, data_as_of, delay_status, notes, updated_by, updated_at
		FROM dw_data_watermarks WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(productKey) != "" {
		q += ` AND product_key = ?`
		args = append(args, strings.TrimSpace(productKey))
	}
	if strings.TrimSpace(assetKey) != "" {
		q += ` AND asset_key = ?`
		args = append(args, strings.TrimSpace(assetKey))
	}
	q += ` ORDER BY updated_at DESC, product_key, asset_key`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWatermark{}
	for rows.Next() {
		var wm DataWatermark
		if err := rows.Scan(&wm.AssetKey, &wm.ProductKey, &wm.LastLoadedAt, &wm.DataAsOf, &wm.DelayStatus, &wm.Notes, &wm.UpdatedBy, &wm.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, wm)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertProductCost(ctx context.Context, cost ProductCost) error {
	cost.ProductKey = strings.TrimSpace(cost.ProductKey)
	if cost.ProductKey == "" {
		return errors.New("product_key is required")
	}
	if strings.TrimSpace(cost.Currency) == "" {
		cost.Currency = "KRW"
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_product_costs
		(product_key, query_cost, llm_cost, ops_cost, data_processing_cost, estimated_margin, currency, updated_by, updated_at, expected_revenue)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(product_key) DO UPDATE SET
			query_cost = excluded.query_cost,
			llm_cost = excluded.llm_cost,
			ops_cost = excluded.ops_cost,
			data_processing_cost = excluded.data_processing_cost,
			estimated_margin = excluded.estimated_margin,
			currency = excluded.currency,
			updated_by = excluded.updated_by,
			updated_at = excluded.updated_at,
			expected_revenue = excluded.expected_revenue`),
		cost.ProductKey, cost.QueryCost, cost.LLMCost, cost.OpsCost, cost.DataProcessingCost,
		cost.EstimatedMargin, cost.Currency, cost.UpdatedBy, now, cost.ExpectedRevenue)
	return err
}

func (s *SQLStore) GetProductCost(ctx context.Context, productKey string) (ProductCost, bool, error) {
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT product_key, query_cost, llm_cost, ops_cost, data_processing_cost,
			estimated_margin, currency, updated_by, updated_at, COALESCE(expected_revenue, 0.0)
		FROM dw_product_costs WHERE product_key = ?`), strings.TrimSpace(productKey))
	var cost ProductCost
	err := row.Scan(&cost.ProductKey, &cost.QueryCost, &cost.LLMCost, &cost.OpsCost, &cost.DataProcessingCost,
		&cost.EstimatedMargin, &cost.Currency, &cost.UpdatedBy, &cost.UpdatedAt, &cost.ExpectedRevenue)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductCost{}, false, nil
	}
	if err != nil {
		return ProductCost{}, false, err
	}
	return cost, true, nil
}

func (s *SQLStore) ListProductCosts(ctx context.Context, productKey string) ([]ProductCost, error) {
	q := `SELECT product_key, query_cost, llm_cost, ops_cost, data_processing_cost, estimated_margin, currency, updated_by, updated_at, COALESCE(expected_revenue, 0.0)
		FROM dw_product_costs`
	args := []any{}
	if strings.TrimSpace(productKey) != "" {
		q += ` WHERE product_key = ?`
		args = append(args, strings.TrimSpace(productKey))
	}
	q += ` ORDER BY estimated_margin ASC, product_key`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProductCost{}
	for rows.Next() {
		var cost ProductCost
		if err := rows.Scan(&cost.ProductKey, &cost.QueryCost, &cost.LLMCost, &cost.OpsCost, &cost.DataProcessingCost,
			&cost.EstimatedMargin, &cost.Currency, &cost.UpdatedBy, &cost.UpdatedAt, &cost.ExpectedRevenue); err != nil {
			return nil, err
		}
		out = append(out, cost)
	}
	return out, rows.Err()
}

func (s *SQLStore) InsertCustomerProposalEvent(ctx context.Context, event CustomerProposalEvent) error {
	event.ID = strings.TrimSpace(event.ID)
	if event.ID == "" || strings.TrimSpace(event.ProductKey) == "" {
		return errors.New("id and product_key are required")
	}
	if strings.TrimSpace(event.EventType) == "" {
		event.EventType = "generated"
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_customer_proposal_events
		(id, customer_key, product_key, proposal_id, variant, event_type, feedback, next_action_at, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		event.ID, event.CustomerKey, event.ProductKey, event.ProposalID, event.Variant, event.EventType,
		event.Feedback, event.NextActionAt, event.CreatedBy, formatTime(time.Now().UTC()))
	return err
}

func (s *SQLStore) ListCustomerProposalEvents(ctx context.Context, productKey string, customerKey string) ([]CustomerProposalEvent, error) {
	q := `SELECT id, customer_key, product_key, proposal_id, variant, event_type, feedback, next_action_at, created_by, created_at
		FROM dw_customer_proposal_events WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(productKey) != "" {
		q += ` AND product_key = ?`
		args = append(args, strings.TrimSpace(productKey))
	}
	if strings.TrimSpace(customerKey) != "" {
		q += ` AND customer_key = ?`
		args = append(args, strings.TrimSpace(customerKey))
	}
	q += ` ORDER BY created_at DESC LIMIT 200`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomerProposalEvent{}
	for rows.Next() {
		var event CustomerProposalEvent
		if err := rows.Scan(&event.ID, &event.CustomerKey, &event.ProductKey, &event.ProposalID, &event.Variant,
			&event.EventType, &event.Feedback, &event.NextActionAt, &event.CreatedBy, &event.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertRetirementCandidate(ctx context.Context, candidate RetirementCandidate) error {
	candidate.ProductKey = strings.TrimSpace(candidate.ProductKey)
	if candidate.ProductKey == "" {
		return errors.New("product_key is required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_retirement_candidates
		(product_key, reason, risk_score, usage_count, last_used_at, recommendation, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(product_key) DO UPDATE SET
			reason = excluded.reason,
			risk_score = excluded.risk_score,
			usage_count = excluded.usage_count,
			last_used_at = excluded.last_used_at,
			recommendation = excluded.recommendation,
			updated_at = excluded.updated_at`),
		candidate.ProductKey, candidate.Reason, candidate.RiskScore, candidate.UsageCount,
		candidate.LastUsedAt, candidate.Recommendation, now, now)
	return err
}

func (s *SQLStore) ListRetirementCandidates(ctx context.Context, productKey string) ([]RetirementCandidate, error) {
	q := `SELECT product_key, reason, risk_score, usage_count, last_used_at, recommendation, created_at, updated_at
		FROM dw_retirement_candidates`
	args := []any{}
	if strings.TrimSpace(productKey) != "" {
		q += ` WHERE product_key = ?`
		args = append(args, strings.TrimSpace(productKey))
	}
	q += ` ORDER BY risk_score DESC, updated_at DESC`
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RetirementCandidate{}
	for rows.Next() {
		var candidate RetirementCandidate
		if err := rows.Scan(&candidate.ProductKey, &candidate.Reason, &candidate.RiskScore, &candidate.UsageCount,
			&candidate.LastUsedAt, &candidate.Recommendation, &candidate.CreatedAt, &candidate.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, candidate)
	}
	return out, rows.Err()
}
