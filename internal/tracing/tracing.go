package tracing

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

var Tracer trace.Tracer

// InitTracer initializes OpenTelemetry globally and returns provider to be deferred closed.
func InitTracer(ctx context.Context) *sdktrace.TracerProvider {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		// No OTLP Endpoint set; fallback to empty no-op provider
		tp := sdktrace.NewTracerProvider()
		otel.SetTracerProvider(tp)
		Tracer = otel.Tracer("mcpwatch")
		return tp
	}

	slog.Info("OTel trace endpoint found. Initializing pipeline...", "endpoint", endpoint)

	// Configure default HTTP exporter
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(endpoint))
	if err != nil {
		slog.Error("failed to create OTLP trace exporter", "error", err)
		return nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("mcpwatch"),
		),
	)
	if err != nil {
		slog.Error("failed to generate trace resource", "error", err)
		return nil
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	Tracer = otel.Tracer("mcpwatch")

	return tp
}
