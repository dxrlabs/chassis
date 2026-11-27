// Package httpx provides stateless HTTP helpers for JSON APIs.
package httpx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// DecodeJSON decodes the request body into T with unknown fields rejected
// and a 1 MiB body size limit.
func DecodeJSON[T any](r *http.Request) (T, error) {
	var zero T
	if r.Body == nil {
		return zero, fmt.Errorf("%w: empty body", ErrBadRequest)
	}
	defer r.Body.Close()

	limited := io.LimitReader(r.Body, maxBodyBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return zero, fmt.Errorf("%w: read body: %v", ErrBadRequest, err)
	}
	if len(raw) > maxBodyBytes {
		return zero, fmt.Errorf("%w: body too large", ErrBadRequest)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var v T
	if err := dec.Decode(&v); err != nil {
		return zero, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return zero, fmt.Errorf("%w: trailing data", ErrBadRequest)
	}
	return v, nil
}

// WriteJSON writes data as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if data == nil {
		return
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(data)
}

// RequireJSON rejects non-JSON Content-Type on methods with bodies.
func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			ct := r.Header.Get("Content-Type")
			if ct == "" || !strings.HasPrefix(strings.ToLower(ct), "application/json") {
				WriteError(w, r, &AppError{Status: http.StatusUnsupportedMediaType, Message: "Content-Type must be application/json"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
