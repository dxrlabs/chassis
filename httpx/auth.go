package httpx

import (
	"context"
	"net/http"
	"strings"
)

type claimsKey int

const bearerClaimsKey claimsKey = 1

// TokenValidator validates a bearer token and returns claims stored in context.
type TokenValidator func(ctx context.Context, token string) (claims any, err error)

// BearerClaimsFromContext returns claims previously set by BearerAuth.
func BearerClaimsFromContext(ctx context.Context) any {
	return ctx.Value(bearerClaimsKey)
}

// BearerAuth requires Authorization: Bearer <token> and runs validate.
// On failure it writes a JSON 401 via WriteError.
func BearerAuth(validate TokenValidator) func(http.Handler) http.Handler {
	if validate == nil {
		panic("httpx: BearerAuth requires a TokenValidator")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			if raw == "" || !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
				WriteError(w, r, ErrUnauthorized)
				return
			}
			token := strings.TrimSpace(raw[len("Bearer "):])
			if token == "" {
				WriteError(w, r, ErrUnauthorized)
				return
			}
			claims, err := validate(r.Context(), token)
			if err != nil {
				WriteError(w, r, &AppError{Status: http.StatusUnauthorized, Message: "unauthorized", Err: err})
				return
			}
			ctx := context.WithValue(r.Context(), bearerClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// StaticBearer accepts a single shared secret token (dev / service-to-service).
func StaticBearer(secret string) TokenValidator {
	return func(_ context.Context, token string) (any, error) {
		if secret == "" || token != secret {
			return nil, ErrUnauthorized
		}
		return map[string]string{"sub": "static"}, nil
	}
}
