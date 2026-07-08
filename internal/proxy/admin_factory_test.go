package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"dataworks/internal/store"
)

func TestFactoryAPIFlow(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "factory.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/admin/factory/ideas/generate", "", map[string]any{
		"industry": "금융", "customer_type": "은행,보험", "market_need": "소상공인 위험 조기탐지",
		"data_assets": []string{"loan_history", "utility_payment"}, "count": 5,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ideas status = %d", resp.StatusCode)
	}
	var ideaBody struct {
		Ideas []store.ProductIdea `json:"ideas"`
	}
	json.NewDecoder(resp.Body).Decode(&ideaBody)
	resp.Body.Close()
	if len(ideaBody.Ideas) != 5 || ideaBody.Ideas[0].ID == "" {
		t.Fatalf("ideas not generated: %+v", ideaBody)
	}

	resp = postJSON(t, srv.URL+"/admin/factory/products/define", "", map[string]any{"idea_id": ideaBody.Ideas[0].ID})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("define status = %d", resp.StatusCode)
	}
	var defBody struct {
		ProductKey string            `json:"product_key"`
		Product    store.DataProduct `json:"product"`
	}
	json.NewDecoder(resp.Body).Decode(&defBody)
	resp.Body.Close()
	if defBody.ProductKey == "" || defBody.Product.Status != "review" || defBody.Product.APISpec == "" {
		t.Fatalf("definition response mismatch: %+v", defBody)
	}

	resp = postJSON(t, srv.URL+"/admin/factory/risk/check", "", map[string]any{"product_key": defBody.ProductKey})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("risk status = %d", resp.StatusCode)
	}
	resp.Body.Close()
	product, ok, err := db.GetDataProduct(context.Background(), defBody.ProductKey)
	if err != nil || !ok || product.Status != "risk_review" || product.RiskScore <= 0 {
		t.Fatalf("risk did not update product: %+v ok=%v err=%v", product, ok, err)
	}

	resp = postJSON(t, srv.URL+"/admin/factory/poc/plan", "", map[string]any{"product_key": defBody.ProductKey})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("poc status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/factory/scoring/evaluate", "", map[string]any{"product_key": defBody.ProductKey})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("score status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/factory/products/"+defBody.ProductKey+"/approve", "", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve status = %d", resp.StatusCode)
	}
	resp.Body.Close()
	product, _, _ = db.GetDataProduct(context.Background(), defBody.ProductKey)
	if product.Status != "approved" {
		t.Fatalf("approve did not transition product: %+v", product)
	}
}
