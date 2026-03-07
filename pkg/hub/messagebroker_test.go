// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hub

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/broker"
	"github.com/ptone/scion-agent/pkg/messages"
	"github.com/ptone/scion-agent/pkg/store"
	"github.com/ptone/scion-agent/pkg/store/sqlite"
)

// brokerMockDispatcher records dispatched messages for test assertions.
type brokerMockDispatcher struct {
	mu       sync.Mutex
	messages []brokerDispatchedMsg
}

type brokerDispatchedMsg struct {
	agentSlug  string
	msg        string
	interrupt  bool
	structured *messages.StructuredMessage
}

func (d *brokerMockDispatcher) DispatchAgentCreate(ctx context.Context, agent *store.Agent) error {
	return nil
}
func (d *brokerMockDispatcher) DispatchAgentProvision(ctx context.Context, agent *store.Agent) error {
	return nil
}
func (d *brokerMockDispatcher) DispatchAgentStart(ctx context.Context, agent *store.Agent, task string) error {
	return nil
}
func (d *brokerMockDispatcher) DispatchAgentStop(ctx context.Context, agent *store.Agent) error {
	return nil
}
func (d *brokerMockDispatcher) DispatchAgentRestart(ctx context.Context, agent *store.Agent) error {
	return nil
}
func (d *brokerMockDispatcher) DispatchAgentDelete(ctx context.Context, agent *store.Agent, deleteFiles, removeBranch, softDelete bool, deletedAt time.Time) error {
	return nil
}
func (d *brokerMockDispatcher) DispatchAgentMessage(ctx context.Context, agent *store.Agent, message string, interrupt bool, structuredMsg *messages.StructuredMessage) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.messages = append(d.messages, brokerDispatchedMsg{
		agentSlug:  agent.Slug,
		msg:        message,
		interrupt:  interrupt,
		structured: structuredMsg,
	})
	return nil
}
func (d *brokerMockDispatcher) DispatchCheckAgentPrompt(ctx context.Context, agent *store.Agent) (bool, error) {
	return false, nil
}
func (d *brokerMockDispatcher) DispatchAgentCreateWithGather(ctx context.Context, agent *store.Agent) (*RemoteEnvRequirementsResponse, error) {
	return nil, nil
}
func (d *brokerMockDispatcher) DispatchFinalizeEnv(ctx context.Context, agent *store.Agent, env map[string]string) error {
	return nil
}

func (d *brokerMockDispatcher) getMessages() []brokerDispatchedMsg {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]brokerDispatchedMsg, len(d.messages))
	copy(result, d.messages)
	return result
}

func newBrokerTestStore(t *testing.T) store.Store {
	t.Helper()
	s, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate test store: %v", err)
	}
	return s
}

// setupBrokerTestGrove creates a grove and a runtime broker, returns the grove ID.
func setupBrokerTestGrove(t *testing.T, s store.Store) string {
	t.Helper()
	ctx := context.Background()

	// Create a runtime broker for agent FK constraints
	rb := &store.RuntimeBroker{
		ID:       "broker-1",
		Name:     "test-broker",
		Slug:     "test-broker",
		Endpoint: "http://localhost:9800",
		Status:   store.BrokerStatusOnline,
	}
	if err := s.CreateRuntimeBroker(ctx, rb); err != nil {
		t.Fatalf("failed to create runtime broker: %v", err)
	}

	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "test-grove",
		Slug:       "test-grove",
		Visibility: store.VisibilityPrivate,
	}
	if err := s.CreateGrove(ctx, grove); err != nil {
		t.Fatalf("failed to create grove: %v", err)
	}
	return grove.ID
}

// setupBrokerTestAgent creates a running agent and returns it.
func setupBrokerTestAgent(t *testing.T, s store.Store, groveID, slug, phase string) *store.Agent {
	t.Helper()
	agent := &store.Agent{
		ID:              api.NewUUID(),
		Name:            slug,
		Slug:            slug,
		GroveID:         groveID,
		Phase:           phase,
		RuntimeBrokerID: "broker-1",
		Visibility:      store.VisibilityPrivate,
	}
	if err := s.CreateAgent(context.Background(), agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	return agent
}

func TestMessageBrokerProxy_DirectMessage(t *testing.T) {
	s := newBrokerTestStore(t)
	groveID := setupBrokerTestGrove(t, s)
	setupBrokerTestAgent(t, s, groveID, "test-agent", "running")

	events := NewChannelEventPublisher()
	defer events.Close()

	b := broker.NewInProcessBroker(slog.Default())
	defer b.Close()

	dispatcher := &brokerMockDispatcher{}

	proxy := NewMessageBrokerProxy(b, s, events, func() AgentDispatcher { return dispatcher }, slog.Default())
	proxy.Start()
	defer proxy.Stop()

	proxy.subscribeAgent(groveID, "test-agent")

	msg := messages.NewInstruction("user:alice", "agent:test-agent", "hello agent")
	if err := proxy.PublishMessage(context.Background(), groveID, msg); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	dispatched := dispatcher.getMessages()
	if len(dispatched) != 1 {
		t.Fatalf("expected 1 dispatched message, got %d", len(dispatched))
	}
	if dispatched[0].agentSlug != "test-agent" {
		t.Errorf("expected agent slug 'test-agent', got %q", dispatched[0].agentSlug)
	}
	if dispatched[0].msg != "hello agent" {
		t.Errorf("expected message 'hello agent', got %q", dispatched[0].msg)
	}
}

func TestMessageBrokerProxy_GroveBroadcast(t *testing.T) {
	s := newBrokerTestStore(t)
	groveID := setupBrokerTestGrove(t, s)
	setupBrokerTestAgent(t, s, groveID, "agent-a", "running")
	setupBrokerTestAgent(t, s, groveID, "agent-b", "running")

	events := NewChannelEventPublisher()
	defer events.Close()

	b := broker.NewInProcessBroker(slog.Default())
	defer b.Close()

	dispatcher := &brokerMockDispatcher{}

	proxy := NewMessageBrokerProxy(b, s, events, func() AgentDispatcher { return dispatcher }, slog.Default())
	proxy.Start()
	defer proxy.Stop()

	proxy.subscribeGroveBroadcast(groveID)

	msg := messages.NewInstruction("user:alice", "grove:test-grove", "hello everyone")
	msg.Broadcasted = true
	if err := proxy.PublishBroadcast(context.Background(), groveID, msg); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	dispatched := dispatcher.getMessages()
	if len(dispatched) != 2 {
		t.Fatalf("expected 2 dispatched messages (fan-out), got %d", len(dispatched))
	}

	slugs := map[string]bool{}
	for _, d := range dispatched {
		slugs[d.agentSlug] = true
	}
	if !slugs["agent-a"] || !slugs["agent-b"] {
		t.Errorf("expected both agent-a and agent-b to receive broadcast, got %v", slugs)
	}
}

func TestMessageBrokerProxy_BroadcastSkipsSender(t *testing.T) {
	s := newBrokerTestStore(t)
	groveID := setupBrokerTestGrove(t, s)
	setupBrokerTestAgent(t, s, groveID, "sender-agent", "running")
	setupBrokerTestAgent(t, s, groveID, "other-agent", "running")

	events := NewChannelEventPublisher()
	defer events.Close()

	b := broker.NewInProcessBroker(slog.Default())
	defer b.Close()

	dispatcher := &brokerMockDispatcher{}

	proxy := NewMessageBrokerProxy(b, s, events, func() AgentDispatcher { return dispatcher }, slog.Default())
	proxy.Start()
	defer proxy.Stop()

	proxy.subscribeGroveBroadcast(groveID)

	msg := messages.NewInstruction("agent:sender-agent", "grove:test-grove", "any updates?")
	msg.Broadcasted = true
	proxy.PublishBroadcast(context.Background(), groveID, msg)

	time.Sleep(100 * time.Millisecond)

	dispatched := dispatcher.getMessages()
	if len(dispatched) != 1 {
		t.Fatalf("expected 1 message (sender excluded), got %d", len(dispatched))
	}
	if dispatched[0].agentSlug != "other-agent" {
		t.Errorf("expected message delivered to 'other-agent', got %q", dispatched[0].agentSlug)
	}
}

func TestMessageBrokerProxy_EnsureGroveSubscriptions(t *testing.T) {
	s := newBrokerTestStore(t)
	groveID := setupBrokerTestGrove(t, s)
	setupBrokerTestAgent(t, s, groveID, "running-agent", "running")
	setupBrokerTestAgent(t, s, groveID, "stopped-agent", "stopped")

	events := NewChannelEventPublisher()
	defer events.Close()

	b := broker.NewInProcessBroker(slog.Default())
	defer b.Close()

	dispatcher := &brokerMockDispatcher{}

	proxy := NewMessageBrokerProxy(b, s, events, func() AgentDispatcher { return dispatcher }, slog.Default())
	proxy.Start()
	defer proxy.Stop()

	if err := proxy.EnsureGroveSubscriptions(context.Background(), groveID); err != nil {
		t.Fatal(err)
	}

	msg := messages.NewInstruction("user:alice", "agent:running-agent", "hello")
	proxy.PublishMessage(context.Background(), groveID, msg)

	time.Sleep(100 * time.Millisecond)

	dispatched := dispatcher.getMessages()
	if len(dispatched) != 1 {
		t.Fatalf("expected 1 dispatched message, got %d", len(dispatched))
	}
}

func TestRecipientSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"agent:code-reviewer", "code-reviewer"},
		{"user:alice", "alice"},
		{"no-prefix", "no-prefix"},
	}
	for _, tt := range tests {
		got := recipientSlug(tt.input)
		if got != tt.expected {
			t.Errorf("recipientSlug(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestContainsSuffix(t *testing.T) {
	tests := []struct {
		subject string
		suffix  string
		match   bool
	}{
		{"grove.g1.agent.created", ".agent.created", true},
		{"grove.g1.agent.status", ".agent.status", true},
		{"grove.g1.agent.deleted", ".agent.deleted", true},
		{"grove.g1.agent.status", ".agent.created", false},
		{"short", ".agent.created", false},
	}
	for _, tt := range tests {
		got := containsSuffix(tt.subject, tt.suffix)
		if got != tt.match {
			t.Errorf("containsSuffix(%q, %q) = %v, want %v", tt.subject, tt.suffix, got, tt.match)
		}
	}
}
