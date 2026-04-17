package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/chifamba/vashandi/vashandi/backend/shared/telemetry"
)

// Body capture limits for request logging and error context.
const (
	// MaxBodyCaptureSize is the maximum request body size to capture (1MB).
	MaxBodyCaptureSize = 1024 * 1024
	// MaxBodyLogSize is the maximum body size to include in log output (10KB).
	MaxBodyLogSize = 10000
)

// Telemetry event constants.
const (
	TelemetryEventErrorHandlerCrash = "error.handler_crash"
	TelemetryErrorCodePanic         = "panic"
)

// APIError is the standard error response format.
type APIError struct {
	Error   string      `json:"error"`
	Code    string      `json:"code,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

// HttpError represents an HTTP error with status code and optional details.
type HttpError struct {
	Status  int
	Message string
	Details interface{}
	Err     error
}

func (e *HttpError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *HttpError) Unwrap() error {
	return e.Err
}

// NewHttpError creates a new HttpError.
func NewHttpError(status int, message string, details ...interface{}) *HttpError {
	err := &HttpError{
		Status:  status,
		Message: message,
	}
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}

// BadRequest creates a 400 Bad Request error.
func BadRequest(message string, details ...interface{}) *HttpError {
	return NewHttpError(http.StatusBadRequest, message, details...)
}

// Unauthorized creates a 401 Unauthorized error.
func Unauthorized(message ...string) *HttpError {
	msg := "Unauthorized"
	if len(message) > 0 {
		msg = message[0]
	}
	return NewHttpError(http.StatusUnauthorized, msg)
}

// Forbidden creates a 403 Forbidden error.
func Forbidden(message ...string) *HttpError {
	msg := "Forbidden"
	if len(message) > 0 {
		msg = message[0]
	}
	return NewHttpError(http.StatusForbidden, msg)
}

// NotFound creates a 404 Not Found error.
func NotFound(message ...string) *HttpError {
	msg := "Not found"
	if len(message) > 0 {
		msg = message[0]
	}
	return NewHttpError(http.StatusNotFound, msg)
}

// Conflict creates a 409 Conflict error.
func Conflict(message string, details ...interface{}) *HttpError {
	return NewHttpError(http.StatusConflict, message, details...)
}

// Unprocessable creates a 422 Unprocessable Entity error.
func Unprocessable(message string, details ...interface{}) *HttpError {
	return NewHttpError(http.StatusUnprocessableEntity, message, details...)
}

// ValidationError represents a validation error with field details.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "validation error"
	}
	return fmt.Sprintf("validation error: %s: %s", e[0].Field, e[0].Message)
}

// ErrorContext captures context about an error for logging and telemetry.
type ErrorContext struct {
	Error     ErrorInfo              `json:"error"`
	Method    string                 `json:"method"`
	URL       string                 `json:"url"`
	ReqBody   interface{}            `json:"reqBody,omitempty"`
	ReqParams map[string]string      `json:"reqParams,omitempty"`
	ReqQuery  map[string][]string    `json:"reqQuery,omitempty"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
}

// ErrorInfo contains error details for logging.
type ErrorInfo struct {
	Message string      `json:"message"`
	Stack   string      `json:"stack,omitempty"`
	Name    string      `json:"name,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

// errorContextKey is the context key for storing error context.
type errorContextKey struct{}

// SetErrorContext stores error context in the response for the logger middleware.
func SetErrorContext(w http.ResponseWriter, ctx ErrorContext) {
	if rw, ok := w.(*errorResponseWriter); ok {
		rw.errorContext = &ctx
	}
}

// errorResponseWriter wraps ResponseWriter to capture error context.
type errorResponseWriter struct {
	http.ResponseWriter
	errorContext *ErrorContext
	status       int
	written      bool
}

func (w *errorResponseWriter) WriteHeader(code int) {
	if !w.written {
		w.status = code
		w.written = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *errorResponseWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.status = http.StatusOK
		w.written = true
	}
	return w.ResponseWriter.Write(b)
}

// ErrorHandlerConfig configures the error handler middleware.
type ErrorHandlerConfig struct {
	// Telemetry is the telemetry client for tracking errors.
	Telemetry *telemetry.Client

	// IncludeStack includes stack traces in 5xx error logs. Defaults to true.
	IncludeStack *bool
}

// ErrorHandlerMiddleware creates a middleware that catches panics and handles errors.
// It provides:
// - Panic recovery with stack trace logging
// - Validation error handling (equivalent to ZodError)
// - Error context capture (request body, params, query, stack)
// - Telemetry integration
// - Consistent JSON error responses
func ErrorHandlerMiddleware(config ErrorHandlerConfig) func(http.Handler) http.Handler {
	includeStack := true
	if config.IncludeStack != nil {
		includeStack = *config.IncludeStack
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap response writer to capture error context
			erw := &errorResponseWriter{ResponseWriter: w, status: http.StatusOK}

			// Capture request body for error logging
			var bodyBytes []byte
			if r.Body != nil && r.ContentLength > 0 && r.ContentLength <= MaxBodyCaptureSize {
				bodyBytes, _ = io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}

			// Store body in context for error handlers
			ctx := context.WithValue(r.Context(), requestBodyKey{}, bodyBytes)

			// Recover from panics
			defer func() {
				if rec := recover(); rec != nil {
					stack := ""
					if includeStack {
						stack = string(debug.Stack())
					}

					errMsg := fmt.Sprintf("%v", rec)
					errCtx := buildErrorContext(r, errMsg, stack, bodyBytes)

					// Log the error
					slog.Error("panic recovered",
						"error", errMsg,
						"method", r.Method,
						"url", r.URL.String(),
						"stack", stack,
					)

					// Track in telemetry
					if config.Telemetry != nil {
						config.Telemetry.Track(TelemetryEventErrorHandlerCrash, map[string]interface{}{
							"error_code": TelemetryErrorCodePanic,
						})
					}

					// Store error context for logger middleware
					erw.errorContext = &errCtx

					// Return JSON error response
					WriteError(erw, http.StatusInternalServerError, "Internal server error")
				}
			}()

			next.ServeHTTP(erw, r.WithContext(ctx))
		})
	}
}

// requestBodyKey is the context key for the request body.
type requestBodyKey struct{}

// GetRequestBody retrieves the captured request body from the context.
func GetRequestBody(ctx context.Context) []byte {
	if body, ok := ctx.Value(requestBodyKey{}).([]byte); ok {
		return body
	}
	return nil
}

// buildErrorContext creates an ErrorContext from the request.
func buildErrorContext(r *http.Request, errMsg, stack string, bodyBytes []byte) ErrorContext {
	ctx := ErrorContext{
		Error: ErrorInfo{
			Message: errMsg,
			Stack:   stack,
			Name:    "Error",
		},
		Method:    r.Method,
		URL:       r.URL.String(),
		ReqQuery:  r.URL.Query(),
		ReqParams: extractRouteParams(r),
	}

	// Parse request body
	if len(bodyBytes) > 0 && len(bodyBytes) <= MaxBodyLogSize {
		var bodyJSON interface{}
		if err := json.Unmarshal(bodyBytes, &bodyJSON); err == nil {
			ctx.ReqBody = bodyJSON
		} else {
			ctx.ReqBody = string(bodyBytes)
		}
	}

	return ctx
}

// extractRouteParams extracts chi route parameters from the request.
func extractRouteParams(r *http.Request) map[string]string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return nil
	}

	params := make(map[string]string)
	for i, key := range rctx.URLParams.Keys {
		if i < len(rctx.URLParams.Values) {
			params[key] = rctx.URLParams.Values[i]
		}
	}

	if len(params) == 0 {
		return nil
	}
	return params
}

// HandleError is a helper function to handle errors in route handlers.
// It writes the appropriate JSON error response and logs the error.
func HandleError(w http.ResponseWriter, r *http.Request, err error, tc *telemetry.Client) {
	var httpErr *HttpError
	var validationErrs ValidationErrors

	switch e := err.(type) {
	case *HttpError:
		httpErr = e
	case ValidationErrors:
		validationErrs = e
		WriteErrorWithDetails(w, http.StatusBadRequest, "Validation error", validationErrs)
		return
	default:
		httpErr = &HttpError{
			Status:  http.StatusInternalServerError,
			Message: "Internal server error",
			Err:     err,
		}
	}

	// Log 5xx errors and track in telemetry
	if httpErr.Status >= 500 {
		stack := string(debug.Stack())
		bodyBytes := GetRequestBody(r.Context())
		errCtx := buildErrorContext(r, httpErr.Message, stack, bodyBytes)

		slog.Error("request error",
			"error", httpErr.Error(),
			"status", httpErr.Status,
			"method", r.Method,
			"url", r.URL.String(),
		)

		// Store error context for logger middleware
		SetErrorContext(w, errCtx)

		// Track in telemetry
		if tc != nil {
			tc.Track(TelemetryEventErrorHandlerCrash, map[string]interface{}{
				"error_code": fmt.Sprintf("http_%d", httpErr.Status),
			})
		}
	}

	if httpErr.Details != nil {
		WriteErrorWithDetails(w, httpErr.Status, httpErr.Message, httpErr.Details)
	} else {
		WriteError(w, httpErr.Status, httpErr.Message)
	}
}

// WriteError writes a JSON error response.
func WriteError(w http.ResponseWriter, status int, message string, code ...string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	errResp := APIError{Error: message}
	if len(code) > 0 {
		errResp.Code = code[0]
	}
	json.NewEncoder(w).Encode(errResp) //nolint:errcheck
}

// WriteErrorWithDetails writes a JSON error response with details.
func WriteErrorWithDetails(w http.ResponseWriter, status int, message string, details interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	errResp := APIError{
		Error:   message,
		Details: details,
	}
	json.NewEncoder(w).Encode(errResp) //nolint:errcheck
}

// WriteJSON writes a JSON response.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data) //nolint:errcheck
}

// RecovererWithTelemetry is a chi recoverer middleware that integrates with telemetry.
func RecovererWithTelemetry(tc *telemetry.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					reqID := middleware.GetReqID(r.Context())
					stack := string(debug.Stack())

					slog.Error("panic recovered",
						"panic", rec,
						"request_id", reqID,
						"method", r.Method,
						"url", r.URL.String(),
						"stack", stack,
					)

					if tc != nil {
						tc.Track(TelemetryEventErrorHandlerCrash, map[string]interface{}{
							"error_code": TelemetryErrorCodePanic,
						})
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(APIError{Error: "Internal server error"}) //nolint:errcheck
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
