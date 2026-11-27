package httpx_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dxrlabs/chassis/httpx"
)

func TestDecodeJSON_ok(t *testing.T) {
	t.Parallel()
	type body struct {
		Name string `json:"name"`
	}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ada"}`))
	got, err := httpx.DecodeJSON[body](r)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if got.Name != "ada" {
		t.Fatalf("Name = %q, want ada", got.Name)
	}
}

func TestDecodeJSON_unknownField(t *testing.T) {
	t.Parallel()
	type body struct {
		Name string `json:"name"`
	}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ada","extra":1}`))
	_, err := httpx.DecodeJSON[body](r)
	if !errors.Is(err, httpx.ErrBadRequest) {
		t.Fatalf("err = %v, want ErrBadRequest", err)
	}
}

func TestDecodeJSON_tooLarge(t *testing.T) {
	t.Parallel()
	type body struct {
		Data string `json:"data"`
	}
	payload := `{"data":"` + strings.Repeat("x", 1<<20) + `"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	_, err := httpx.DecodeJSON[body](r)
	if !errors.Is(err, httpx.ErrBadRequest) {
		t.Fatalf("err = %v, want ErrBadRequest", err)
	}
}

func TestWriteJSON_and_WriteError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	httpx.WriteJSON(rec, http.StatusCreated, map[string]string{"ok": "yes"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"ok"`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}

	cases := []struct {
		err    error
		status int
	}{
		{httpx.ErrNotFound, http.StatusNotFound},
		{httpx.ErrBadRequest, http.StatusBadRequest},
		{httpx.ErrUnauthorized, http.StatusUnauthorized},
		{httpx.ErrConflict, http.StatusConflict},
		{&httpx.AppError{Status: http.StatusTeapot, Message: "short"}, http.StatusTeapot},
		{errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		httpx.WriteError(rec, req, tc.err)
		if rec.Code != tc.status {
			t.Fatalf("%v: status = %d, want %d", tc.err, rec.Code, tc.status)
		}
	}
}

func TestRequestID_middleware(t *testing.T) {
	t.Parallel()
	var seen string
	h := httpx.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	h.ServeHTTP(rec, req)
	if seen != "abc-123" {
		t.Fatalf("context id = %q", seen)
	}
	if rec.Header().Get("X-Request-ID") != "abc-123" {
		t.Fatalf("response header = %q", rec.Header().Get("X-Request-ID"))
	}
}
