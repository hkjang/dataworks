package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"dataworks/internal/store"
)

func TestDataWorksProductRetiredKeyReturnsConflict(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "retired-product.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	productKey := "retired_product_key"
	resp := postJSON(t, srv.URL+"/admin/dataworks/products", "", map[string]any{
		"id": "dprod_retired_old", "product_key": productKey, "name_ko": "삭제 전 상품", "status": "draft",
	})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	deleteReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/admin/dataworks/products?product_key="+productKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/products", "", map[string]any{
		"id": "dprod_retired_new", "product_key": productKey, "name_ko": "동일 키 신규 상품", "status": "draft",
	})
	requireStatus(t, resp, http.StatusConflict)
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeAndClose(t, resp, &body)
	if body.Error.Code != "product_key_retired" {
		t.Fatalf("error code = %q, want product_key_retired", body.Error.Code)
	}
	if _, ok, err := db.GetDataProduct(context.Background(), productKey); err != nil || ok {
		t.Fatalf("retired product was recreated: ok=%v err=%v", ok, err)
	}
}
