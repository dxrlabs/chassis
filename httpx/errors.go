package httpx

import (
	"errors"
	"log/slog"
	"net/http"
)

// Sentinel errors mapped by WriteError to HTTP status codes.
var (
	ErrNotFound     = errors.New("not found")
	ErrBadRequest   = errors.New("bad request")
	ErrUnauthorized = errors.New("unauthorized")
	ErrConflict     = errors.New("conflict")
)

// AppError is an application error with an explicit HTTP status and message.
type AppError struct {
	Status  int
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return http.StatusText(e.Status)
}

func (e *AppError) Unwrap() error { return e.Err }

// ErrorResponse is the JSON body written by WriteError.
type ErrorResponse struct {
	Error string `json:"error"`
}

// WriteError maps err to a JSON error payload and HTTP status.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status, msg := mapError(err)
	if status >= 500 {
		slog.Default().Error("request failed",
			"err", err,
			"path", r.URL.Path,
			"method", r.Method,
			"status", status,
		)
		msg = http.StatusText(status)
	}
	WriteJSON(w, status, ErrorResponse{Error: msg})
}

func mapError(err error) (int, string) {
	if err == nil {
		return http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)
	}

	var app *AppError
	if errors.As(err, &app) {
		msg := app.Message
		if msg == "" {
			msg = app.Error()
		}
		if app.Status > 0 {
			return app.Status, msg
		}
	}

	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized, err.Error()
	case errors.Is(err, ErrConflict):
		return http.StatusConflict, err.Error()
	default:
		return http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)
	}
}
