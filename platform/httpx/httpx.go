// Package httpx holds shared HTTP helpers: JSON, errors, health and recover.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// JSON writes v as JSON with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error is an error with an HTTP status and a message that is safe to show the client.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func BadRequest(msg string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}

func NotFound(msg string) *Error {
	return &Error{Status: http.StatusNotFound, Code: "not_found", Message: msg}
}

// WriteError writes the error response. Anything that is not an *Error becomes
// a generic 500: the details go to the log, never to the client.
func WriteError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var he *Error
	if errors.As(err, &he) {
		JSON(w, he.Status, map[string]string{"code": he.Code, "message": he.Message})
		return
	}
	logger.ErrorContext(r.Context(), "internal error", "err", err, "path", r.URL.Path)
	JSON(w, http.StatusInternalServerError, map[string]string{"code": "internal", "message": "internal error"})
}

// Healthz always answers ok. It is the liveness probe.
func Healthz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

// Readyz runs check (e.g. a database ping). It is the readiness probe.
func Readyz(check func(context.Context) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := check(ctx); err != nil {
			JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
}

// Recover keeps a panic from taking the process down and logs the stack.
func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				logger.ErrorContext(r.Context(), "panic", "value", v, "stack", string(debug.Stack()))
				JSON(w, http.StatusInternalServerError, map[string]string{"code": "internal", "message": "internal error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
