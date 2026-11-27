// Package telemetry configures OpenTelemetry tracing and metrics providers.
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

const shutdownTimeout = 5 * time.Second

// New installs global TracerProvider and MeterProvider for serviceName.
// When otlpEndpoint is empty, no-op providers are used (local/dev friendly).
// Cleanup flushes pending telemetry with a bounded timeout.
func New(ctx context.Context, serviceName, version, otlpEndpoint string) (func(), error) {
	if otlpEndpoint == "" {
		return func() {}, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: resource: %w", err)
	}

	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(otlpEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(otlpEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		_ = tp.Shutdown(ctx)
		return nil, fmt.Errorf("telemetry: metric exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExp)),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	cleanup := func() {
		cctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = mp.Shutdown(cctx)
		_ = tp.Shutdown(cctx)
	}
	return cleanup, nil
}
