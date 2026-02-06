/*
Copyright 2025 The Scion Authors.
*/

package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Providers holds SDK TracerProvider and LoggerProvider for OTel export.
// Both providers share the same OTLP endpoint and resource attributes.
type Providers struct {
	TracerProvider *trace.TracerProvider
	LoggerProvider *log.LoggerProvider
}

// NewProviders creates real SDK TracerProvider and LoggerProvider that export
// to the configured OTLP endpoint. Returns nil if config is nil, disabled,
// or cloud is not configured.
//
// The batch parameter controls processor mode:
//   - batch=false uses synchronous processors (for short-lived hook commands)
//   - batch=true uses batching processors (for long-lived init commands)
func NewProviders(ctx context.Context, config *Config, batch bool) (*Providers, error) {
	if config == nil || !config.Enabled || !config.IsCloudConfigured() {
		return nil, nil
	}

	// Build resource with service name and agent/grove identifiers
	attrs := []resource.Option{
		resource.WithAttributes(semconv.ServiceName("sciontool")),
	}
	if agentID := os.Getenv("SCION_AGENT_ID"); agentID != "" {
		attrs = append(attrs, resource.WithAttributes(semconv.ServiceInstanceID(agentID)))
	}
	if groveID := os.Getenv("SCION_GROVE_ID"); groveID != "" {
		attrs = append(attrs, resource.WithAttributes(
			attribute.String("scion.grove.id", groveID),
		))
	}
	res, err := resource.New(ctx, attrs...)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	// Create trace exporter (gRPC)
	traceOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(config.Endpoint),
	}
	if config.Insecure {
		traceOpts = append(traceOpts, otlptracegrpc.WithInsecure())
	}
	traceExporter, err := otlptracegrpc.New(ctx, traceOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating trace exporter: %w", err)
	}

	// Create log exporter (gRPC)
	logOpts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(config.Endpoint),
	}
	if config.Insecure {
		logOpts = append(logOpts, otlploggrpc.WithInsecure())
	}
	logExporter, err := otlploggrpc.New(ctx, logOpts...)
	if err != nil {
		// Clean up trace exporter on failure
		_ = traceExporter.Shutdown(ctx)
		return nil, fmt.Errorf("creating log exporter: %w", err)
	}

	// Build providers with appropriate processor mode
	var tp *trace.TracerProvider
	var lp *log.LoggerProvider
	if batch {
		tp = trace.NewTracerProvider(
			trace.WithResource(res),
			trace.WithBatcher(traceExporter),
		)
		lp = log.NewLoggerProvider(
			log.WithResource(res),
			log.WithProcessor(log.NewBatchProcessor(logExporter)),
		)
	} else {
		tp = trace.NewTracerProvider(
			trace.WithResource(res),
			trace.WithSyncer(traceExporter),
		)
		lp = log.NewLoggerProvider(
			log.WithResource(res),
			log.WithProcessor(log.NewSimpleProcessor(logExporter)),
		)
	}

	return &Providers{
		TracerProvider: tp,
		LoggerProvider: lp,
	}, nil
}

// Shutdown flushes and shuts down both providers.
func (p *Providers) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}

	var firstErr error
	if p.TracerProvider != nil {
		if err := p.TracerProvider.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if p.LoggerProvider != nil {
		if err := p.LoggerProvider.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
