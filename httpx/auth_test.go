package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dxrlabs/chassis/httpx"
)

func TestBearerAuth_static(t *testing.T) {
	t.Parallel()
	h := httpx.BearerAuth(httpx.StaticBearer("secret"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if httpx.BearerClaimsFromContext(r.Context()) == nil {
			t.Fatal("missing claims")
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
