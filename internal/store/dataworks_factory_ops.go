package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type DataWorksPromptTemplate struct {
	TemplateKey  string `json:"template_key"`
	RunType      string `json:"run_type"`
	Version      int    `json:"version"`
	TemplateBody string `json:"template_body"`
	Status       string `json:"status"`
	CreatedBy    string `json:"created_by"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type FactoryEvalScore struct {
	ID                 string `json:"id"`
	RunID              string `json:"run_id"`
	AccuracyScore      int    `json:"accuracy_score"`
	UsefulnessScore    int    `json:"usefulness_score"`
	RiskScore          int    `json:"risk_score"`
	OutputQualityScore int    `json:"output_quality_score"`
	ReviewComment      string `json:"review_comment"`
	Reviewer           string `json:"reviewer"`
	CreatedAt          string `json:"created_at"`
}

type ProductFunnelDaily struct {
	Date        string `json:"date"`
	Ideas       int64  `json:"ideas"`
	Definitions int64  `json:"definitions"`
	Reviews     int64  `json:"reviews"`
	Proposals   int64  `json:"proposals"`
	POCs        int64  `json:"pocs"`
	Published   int64  `json:"published"`
	UpdatedAt   string `json:"updated_at"`
}

type ProductRelationship struct {
	FromType     string  `json:"from_type"`
	FromKey      string  `json:"from_key"`
	ToType       string  `json:"to_type"`
	ToKey        string  `json:"to_key"`
	RelationType string  `json:"relation_type"`
	Weight       float64 `json:"weight"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

func (s *SQLStore) InsertDataWorksPromptTemplate(ctx context.Context, template DataWorksPromptTemplate) (DataWorksPromptTemplate, error) {
	template.TemplateKey = strings.TrimSpace(template.TemplateKey)
	template.RunType = strings.TrimSpace(template.RunType)
	template.TemplateBody = strings.TrimSpace(template.TemplateBody)
	template.Status = strings.ToLower(strings.TrimSpace(template.Status))
	if template.TemplateKey == "" || template.RunType == "" || template.TemplateBody == "" {
		return DataWorksPromptTemplate{}, errors.New("template_key, run_type, and template_body are required")
	}
	if template.Status == "" {
		template.Status = "draft"
	}
	if template.Status != "draft" && template.Status != "active" && template.Status != "retired" {
		return DataWorksPromptTemplate{}, errors.New("status must be draft, active, or retired")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DataWorksPromptTemplate{}, err
	}
	defer tx.Rollback()
	var existingRunType string
	if err := tx.QueryRowContext(ctx, s.bind(`SELECT COALESCE(MAX(version), 0) + 1, COALESCE(MIN(run_type), '')
		FROM dw_prompt_templates WHERE template_key = ?`), template.TemplateKey).Scan(&template.Version, &existingRunType); err != nil {
		return DataWorksPromptTemplate{}, err
	}
	if existingRunType != "" && existingRunType != template.RunType {
		return DataWorksPromptTemplate{}, errors.New("run_type cannot change across versions of a template_key")
	}
	now := formatTime(time.Now().UTC())
	if template.Status == "active" {
		if _, err := tx.ExecContext(ctx, s.bind(`UPDATE dw_prompt_templates SET status = 'retired', updated_at = ?
			WHERE template_key = ? AND status = 'active'`), now, template.TemplateKey); err != nil {
			return DataWorksPromptTemplate{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, s.bind(`INSERT INTO dw_prompt_templates
		(template_key, run_type, version, template_body, status, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`), template.TemplateKey, template.RunType, template.Version,
		template.TemplateBody, template.Status, template.CreatedBy, now, now); err != nil {
		return DataWorksPromptTemplate{}, err
	}
	if err := tx.Commit(); err != nil {
		return DataWorksPromptTemplate{}, err
	}
	template.CreatedAt = now
	template.UpdatedAt = now
	return template, nil
}

func (s *SQLStore) ListDataWorksPromptTemplates(ctx context.Context, templateKey, runType, status string, limit int) ([]DataWorksPromptTemplate, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT template_key, run_type, version, template_body, status, created_by, created_at, updated_at
		FROM dw_prompt_templates WHERE 1=1`
	args := []any{}
	if templateKey = strings.TrimSpace(templateKey); templateKey != "" {
		q += ` AND template_key = ?`
		args = append(args, templateKey)
	}
	if runType = strings.TrimSpace(runType); runType != "" {
		q += ` AND run_type = ?`
		args = append(args, runType)
	}
	if status = strings.ToLower(strings.TrimSpace(status)); status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY template_key, version DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DataWorksPromptTemplate{}
	for rows.Next() {
		var template DataWorksPromptTemplate
		if err := rows.Scan(&template.TemplateKey, &template.RunType, &template.Version, &template.TemplateBody,
			&template.Status, &template.CreatedBy, &template.CreatedAt, &template.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, template)
	}
	return out, rows.Err()
}

func (s *SQLStore) GetDataWorksPromptTemplate(ctx context.Context, templateKey string, version int) (DataWorksPromptTemplate, bool, error) {
	q := `SELECT template_key, run_type, version, template_body, status, created_by, created_at, updated_at
		FROM dw_prompt_templates WHERE template_key = ?`
	args := []any{strings.TrimSpace(templateKey)}
	if version > 0 {
		q += ` AND version = ?`
		args = append(args, version)
	} else {
		q += ` AND status = 'active' ORDER BY version DESC LIMIT 1`
	}
	row := s.db.QueryRowContext(ctx, s.bind(q), args...)
	var template DataWorksPromptTemplate
	err := row.Scan(&template.TemplateKey, &template.RunType, &template.Version, &template.TemplateBody,
		&template.Status, &template.CreatedBy, &template.CreatedAt, &template.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DataWorksPromptTemplate{}, false, nil
	}
	if err != nil {
		return DataWorksPromptTemplate{}, false, err
	}
	return template, true, nil
}

func (s *SQLStore) InsertFactoryEvalScore(ctx context.Context, score FactoryEvalScore) error {
	score.ID = strings.TrimSpace(score.ID)
	score.RunID = strings.TrimSpace(score.RunID)
	if score.ID == "" || score.RunID == "" {
		return errors.New("id and run_id are required")
	}
	for name, value := range map[string]int{
		"accuracy_score": score.AccuracyScore, "usefulness_score": score.UsefulnessScore,
		"risk_score": score.RiskScore, "output_quality_score": score.OutputQualityScore,
	} {
		if value < 0 || value > 100 {
			return fmt.Errorf("%s must be between 0 and 100", name)
		}
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_factory_eval_scores
		(id, run_id, accuracy_score, usefulness_score, risk_score, output_quality_score, review_comment, reviewer, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), score.ID, score.RunID, score.AccuracyScore, score.UsefulnessScore,
		score.RiskScore, score.OutputQualityScore, score.ReviewComment, score.Reviewer, formatTime(time.Now().UTC()))
	return err
}

func (s *SQLStore) ListFactoryEvalScores(ctx context.Context, runID string, limit int) ([]FactoryEvalScore, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, run_id, accuracy_score, usefulness_score, risk_score, output_quality_score,
		review_comment, reviewer, created_at FROM dw_factory_eval_scores`
	args := []any{}
	if runID = strings.TrimSpace(runID); runID != "" {
		q += ` WHERE run_id = ?`
		args = append(args, runID)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FactoryEvalScore{}
	for rows.Next() {
		var score FactoryEvalScore
		if err := rows.Scan(&score.ID, &score.RunID, &score.AccuracyScore, &score.UsefulnessScore, &score.RiskScore,
			&score.OutputQualityScore, &score.ReviewComment, &score.Reviewer, &score.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, score)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertProductFunnelDaily(ctx context.Context, snapshot ProductFunnelDaily) error {
	snapshot.Date = strings.TrimSpace(snapshot.Date)
	if snapshot.Date == "" {
		snapshot.Date = time.Now().UTC().Format("2006-01-02")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_product_funnel_daily
		(date, ideas, definitions, reviews, proposals, pocs, published, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET ideas = excluded.ideas, definitions = excluded.definitions,
			reviews = excluded.reviews, proposals = excluded.proposals, pocs = excluded.pocs,
			published = excluded.published, updated_at = excluded.updated_at`), snapshot.Date, snapshot.Ideas,
		snapshot.Definitions, snapshot.Reviews, snapshot.Proposals, snapshot.POCs, snapshot.Published, now)
	return err
}

func (s *SQLStore) ListProductFunnelDaily(ctx context.Context, since string, limit int) ([]ProductFunnelDaily, error) {
	if limit <= 0 || limit > 366 {
		limit = 30
	}
	q := `SELECT date, ideas, definitions, reviews, proposals, pocs, published, updated_at
		FROM dw_product_funnel_daily`
	args := []any{}
	if since = strings.TrimSpace(since); since != "" {
		q += ` WHERE date >= ?`
		args = append(args, since)
	}
	q += ` ORDER BY date DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProductFunnelDaily{}
	for rows.Next() {
		var snapshot ProductFunnelDaily
		if err := rows.Scan(&snapshot.Date, &snapshot.Ideas, &snapshot.Definitions, &snapshot.Reviews,
			&snapshot.Proposals, &snapshot.POCs, &snapshot.Published, &snapshot.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, snapshot)
	}
	return out, rows.Err()
}

func (s *SQLStore) UpsertProductRelationship(ctx context.Context, rel ProductRelationship) error {
	rel.FromType = strings.TrimSpace(rel.FromType)
	rel.FromKey = strings.TrimSpace(rel.FromKey)
	rel.ToType = strings.TrimSpace(rel.ToType)
	rel.ToKey = strings.TrimSpace(rel.ToKey)
	rel.RelationType = strings.TrimSpace(rel.RelationType)
	if rel.FromType == "" || rel.FromKey == "" || rel.ToType == "" || rel.ToKey == "" || rel.RelationType == "" {
		return errors.New("from_type, from_key, to_type, to_key, and relation_type are required")
	}
	if rel.Weight <= 0 {
		rel.Weight = 1
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO dw_product_relationships
		(from_type, from_key, to_type, to_key, relation_type, weight, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(from_type, from_key, to_type, to_key, relation_type) DO UPDATE SET
			weight = excluded.weight, updated_at = excluded.updated_at`), rel.FromType, rel.FromKey, rel.ToType,
		rel.ToKey, rel.RelationType, rel.Weight, now, now)
	return err
}

func (s *SQLStore) ListProductRelationships(ctx context.Context, nodeType, nodeKey string, limit int) ([]ProductRelationship, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	q := `SELECT from_type, from_key, to_type, to_key, relation_type, weight, created_at, updated_at
		FROM dw_product_relationships WHERE 1=1`
	args := []any{}
	if nodeType = strings.TrimSpace(nodeType); nodeType != "" {
		q += ` AND (from_type = ? OR to_type = ?)`
		args = append(args, nodeType, nodeType)
	}
	if nodeKey = strings.TrimSpace(nodeKey); nodeKey != "" {
		q += ` AND (from_key = ? OR to_key = ?)`
		args = append(args, nodeKey, nodeKey)
	}
	q += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.bind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProductRelationship{}
	for rows.Next() {
		var rel ProductRelationship
		if err := rows.Scan(&rel.FromType, &rel.FromKey, &rel.ToType, &rel.ToKey, &rel.RelationType,
			&rel.Weight, &rel.CreatedAt, &rel.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}
