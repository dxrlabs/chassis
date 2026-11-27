package telemetry

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// HTTPMiddleware wraps h with OpenTelemetry HTTP server instrumentation.
// Compose in main: handler = telemetry.HTTPMiddleware("myservice", mux)
func HTTPMiddleware(operation string, h http.Handler) http.Handler {
	if operation == "" {
		operation = "http.server"
	}
	return otelhttp.NewHandler(h, operation)
}
