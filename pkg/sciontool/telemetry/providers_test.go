/*
Copyright 2025 The Scion Authors.
*/

package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestNewProviders_NilConfig(t *testing.T) {
	p, err := NewProviders(context.Background(), nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Error("expected nil Providers for nil config")
	}
}

func TestNewProviders_Disabled(t *testing.T) {
	cfg := &Config{
		Enabled:      false,
		CloudEnabled: true,
		Endpoint:     "localhost:4317",
	}
	p, err := NewProviders(context.Background(), cfg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Error("expected nil Providers when disabled")
	}
}

func TestNewProviders_NoEndpoint(t *testing.T) {
	cfg := &Config{
		Enabled:      true,
		CloudEnabled: true,
		Endpoint:     "", // no endpoint
	}
	p, err := NewProviders(context.Background(), cfg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Error("expected nil Providers when endpoint is empty")
	}
}

func TestNewProviders_CloudDisabled(t *testing.T) {
	cfg := &Config{
		Enabled:      true,
		CloudEnabled: false,
		Endpoint:     "localhost:4317",
	}
	p, err := NewProviders(context.Background(), cfg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Error("expected nil Providers when cloud is disabled")
	}
}

func TestProviders_ShutdownNil(t *testing.T) {
	var p *Providers
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown on nil Providers should not error, got: %v", err)
	}
}

func TestNewProviders_SyncMode(t *testing.T) {
	cfg := &Config{
		Enabled:      true,
		CloudEnabled: true,
		Endpoint:     "localhost:4317",
		Insecure:     true,
	}
	p, err := NewProviders(context.Background(), cfg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil Providers")
	}
	if p.TracerProvider == nil {
		t.Error("expected non-nil TracerProvider")
	}
	if p.LoggerProvider == nil {
		t.Error("expected non-nil LoggerProvider")
	}

	// Clean shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown error: %v", err)
	}
}

func TestNewProviders_BatchMode(t *testing.T) {
	cfg := &Config{
		Enabled:      true,
		CloudEnabled: true,
		Endpoint:     "localhost:4317",
		Insecure:     true,
	}
	p, err := NewProviders(context.Background(), cfg, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil Providers")
	}
	if p.TracerProvider == nil {
		t.Error("expected non-nil TracerProvider")
	}
	if p.LoggerProvider == nil {
		t.Error("expected non-nil LoggerProvider")
	}

	// Clean shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown error: %v", err)
	}
}
