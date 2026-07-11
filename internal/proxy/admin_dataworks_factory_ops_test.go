package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"dataworks/internal/store"
)

func TestDataWorksFactoryReplayEvaluationAndPromptRegistry(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "factory-ops.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	source := store.FactoryRun{
		ID: "frun_source", RunType: "products.define", Model: "rules:v1", PromptVersion: "legacy",
		InputHash: "input-abc", OutputRef: "dw_credit_score", PolicyDecision: "approved", Status: "completed", CreatedBy: "tester",
	}
	if err := db.InsertFactoryRun(context.Background(), source); err != nil {
		t.Fatal(err)
	}

	resp := postJSON(t, srv.URL+"/admin/dataworks/prompt-templates", "", map[string]any{
		"template_key": "products.define", "run_type": "products.define", "template_body": "Create {{product}}", "status": "active",
	})
	requireStatus(t, resp, http.StatusCreated)
	var promptBody struct {
		PromptTemplate store.DataWorksPromptTemplate `json:"prompt_template"`
	}
	decodeAndClose(t, resp, &promptBody)
	if promptBody.PromptTemplate.Version != 1 || promptBody.PromptTemplate.Status != "active" {
		t.Fatalf("unexpected prompt template: %+v", promptBody.PromptTemplate)
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/factory/runs/frun_source/replay", "", map[string]any{
		"template_key": "products.define", "model": "rules:v2", "reason": "compare prompt versions", "token_cost": 0.25,
	})
	requireStatus(t, resp, http.StatusCreated)
	var replayBody struct {
		ReplayRun store.FactoryRun `json:"replay_run"`
	}
	decodeAndClose(t, resp, &replayBody)
	if replayBody.ReplayRun.ParentRunID != source.ID || replayBody.ReplayRun.Model != "rules:v2" || replayBody.ReplayRun.PromptVersion != "1" {
		t.Fatalf("unexpected replay lineage: %+v", replayBody.ReplayRun)
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/factory/runs/"+replayBody.ReplayRun.ID+"/evaluate", "", map[string]any{
		"accuracy_score": 90, "usefulness_score": 80, "risk_score": 100, "review_comment": "comparison complete",
	})
	requireStatus(t, resp, http.StatusCreated)
	var evalBody struct {
		Evaluation store.FactoryEvalScore `json:"evaluation"`
		Summary    map[string]any         `json:"evaluation_summary"`
	}
	decodeAndClose(t, resp, &evalBody)
	if evalBody.Evaluation.OutputQualityScore != 90 || evalBody.Evaluation.CreatedAt == "" || evalBody.Summary["count"] != float64(1) {
		t.Fatalf("unexpected evaluation response: %+v", evalBody)
	}

	for _, path := range []string{
		"/admin/dataworks/prompt-templates?status=active",
		"/admin/dataworks/factory/runs?days=30",
		"/admin/dataworks/analytics/funnel?days=30",
	} {
		resp, err = http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		requireStatus(t, resp, http.StatusOK)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if len(body) == 0 {
			t.Fatalf("empty response for %s", path)
		}
	}

	resp, err = http.Get(srv.URL + "/admin/dataworks/factory/runs?days=30")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var runsBody struct {
		Runs                []store.FactoryRun `json:"runs"`
		EvaluationSummaries map[string]any     `json:"evaluation_summaries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&runsBody); err != nil {
		t.Fatal(err)
	}
	if len(runsBody.Runs) != 2 || runsBody.EvaluationSummaries[replayBody.ReplayRun.ID] == nil {
		t.Fatalf("missing replay/evaluation summary: %+v", runsBody)
	}
}
