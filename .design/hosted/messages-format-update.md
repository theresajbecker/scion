# Structured Message Format: Findings and Design

## Status
**Draft** | March 2026

## Summary

This document proposes upgrading Scion's messaging system from plain-text strings to structured JSON messages with well-defined fields, and lays the foundation for extensible notification channels and a future message broker architecture. Structured messages become the default; a `--plain` CLI flag preserves the current raw-text behavior.

---

## Table of Contents

1. [Current State Analysis](#1-current-state-analysis)
2. [Proposed Message Format](#2-proposed-message-format)
3. [CLI Changes](#3-cli-changes)
4. [Message Flow Changes by Sender/Recipient Pair](#4-message-flow-changes-by-senderrecipient-pair)
5. [Harness Delivery Format](#5-harness-delivery-format)
6. [API and Wire Protocol Changes](#6-api-and-wire-protocol-changes)
7. [Storage and Logging](#7-storage-and-logging)
8. [Extensible Notification Channels](#8-extensible-notification-channels)
9. [Message Broker Architecture](#9-message-broker-architecture)
10. [Migration and Backwards Compatibility](#10-migration-and-backwards-compatibility)
11. [Alternative Approaches Considered](#11-alternative-approaches-considered)
12. [Open Questions](#12-open-questions)
13. [Implementation Plan](#13-implementation-plan)
14. [Key Files Affected](#14-key-files-affected)

---

## 1. Current State Analysis

### 1.1 Message Delivery Paths Today

The following sender/recipient combinations are currently supported:

| # | Sender | Recipient | Mechanism | Notes |
|---|--------|-----------|-----------|-------|
| 1 | **User (human)** | **Agent** | CLI `scion message <agent> <msg>` | Via Hub API or local tmux send-keys |
| 2 | **Agent** | **Agent** | CLI `scion message <agent> <msg>` (from inside container) | Agent uses its JWT to message peers |
| 3 | **Agent** | **All Agents (grove)** | CLI `scion message --broadcast <msg>` | Fan-out to all running agents in grove |
| 4 | **Agent** | **All Agents (global)** | CLI `scion message --all <msg>` | Cross-grove broadcast |
| 5 | **System (notification)** | **Agent** | `NotificationDispatcher` dispatches via `DispatchAgentMessage` | Triggered by watched agent state change |
| 6 | **System (notification)** | **User (web)** | SSE `notification.created` event + DB persistence | Real-time via web UI notification tray |
| 7 | **System (notification)** | **User (CLI)** | `scion hub notifications` CLI query | Polling-based retrieval |
| 8 | **System (scheduled)** | **Agent** | Hub scheduler fires `messageEventHandler` at scheduled time | Created via `--in`/`--at` flags |
| 9 | **User (human)** | **Agent (via web)** | Not yet implemented | Web UI message input planned |

### 1.2 Current Message Format

Messages are **plain text strings** at every layer:

- **CLI**: `scion message agent1 "implement the auth module"` - raw string
- **Hub API**: `POST /agents/{id}/message` with `{"message": "string", "interrupt": bool}`
- **Runtime Broker API**: Same `MessageRequest{Message string, Interrupt bool}`
- **Container delivery**: `tmux send-keys -t scion <message> Enter`
- **System notifications**: Plain text with prefix: `"You are being notified by the system because an agent you manage has reached a notable state. {message}"`

There is no structured metadata (timestamp, sender identity, message type, urgency, attachments).

### 1.3 Key Implementation Points

- **CLI command**: `cmd/message.go` - 315 lines, handles local and Hub modes
- **Agent manager**: `pkg/agent/manager.go:110-163` - tmux send-keys injection
- **Hub handler**: `pkg/hub/handlers.go:1472-1515` - `MessageRequest` struct, dispatch to broker
- **Broker handler**: `pkg/runtimebroker/handlers.go:1280-1304` - receives and delivers to container
- **Notification dispatch**: `pkg/hub/notifications.go:200-242` - formats and sends to subscriber agents
- **Hub client**: `pkg/hubclient/agents.go:360-374` - `SendMessage(ctx, agentID, message, interrupt)`
- **Broker client**: `pkg/brokerclient/agents.go:181-192` - mirrors hub client

### 1.4 Notification System Architecture

The existing notification system (`pkg/hub/notifications.go`) implements:
- Event-driven dispatch via `ChannelEventPublisher` (NATS-style subject matching)
- Subscription-based filtering (`notification_subscriptions` table)
- Deduplication (last-status comparison)
- Dual subscriber types: `"agent"` (immediate dispatch) and `"user"` (accumulate + SSE)
- Notification storage for audit and retrieval

This architecture is relevant because system notifications are a sender type that will produce structured messages.

---

## 2. Proposed Message Format

### 2.1 Structured Message Schema

```json
{
  "version": 1,
  "timestamp": "2026-03-07T14:30:00Z",
  "id": "msg_abc123",
  "sender": "user:alice",
  "recipient": "agent:code-reviewer",
  "msg": "Please review the auth module changes",
  "type": "instruction",
  "urgent": false,
  "broadcasted": false,
  "attachments": ["src/auth/handler.go", "src/auth/middleware.go"]
}
```

### 2.2 Field Definitions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `version` | `int` | Yes | Schema version for forward compatibility. Currently `1`. |
| `timestamp` | `string` (ISO 8601) | Yes | UTC time when the message was created. Set by the originating system (CLI, Hub, or notification dispatcher). |
| `id` | `string` | Yes | Unique message identifier. Format: `msg_<uuid>`. Generated at message creation. |
| `sender` | `string` | Yes | Identity of the sender. Format: `<type>:<identifier>` (see Section 2.3). |
| `recipient` | `string` | Yes | Identity of the intended recipient. Same format as sender. **Stripped before delivery to recipient** (see Section 5). |
| `msg` | `string` | Yes | The human-readable message body. |
| `type` | `string` | Yes | Message classification (see Section 2.4). |
| `urgent` | `bool` | No | `true` if sent with `--interrupt` flag. Default `false`. |
| `broadcasted` | `bool` | No | `true` if sent with `--broadcast` or `--all` flag. Default `false`. |
| `attachments` | `[]string` | No | List of relative file paths in the workspace. Omitted if empty. |

### 2.3 Sender/Recipient Identity Format

The identity string uses a `<type>:<identifier>` format:

| Type | Example | Description |
|------|---------|-------------|
| `user:<username>` | `user:alice` | Human user (resolved from Hub identity or local user) |
| `agent:<slug>` | `agent:code-reviewer` | An agent in the system |
| `system:<subsystem>` | `system:notifications` | System-generated messages |
| `system:<agent-slug>` | `system:code-reviewer` | System notification about a specific agent (the subject of the notification is the sender) |
| `grove:<grove-name>` | `grove:my-project` | Used as recipient for grove-wide broadcasts |
| `all` | `all` | Used as recipient for global broadcasts |

For system notifications about agent state changes, the sender is `system:<agent-slug>` where `<agent-slug>` is the agent whose state changed. This follows the user's requirement that the subject of the notification should be the sender.

### 2.4 Message Types

| Type | Description | Typical Sender |
|------|-------------|----------------|
| `instruction` | A task or directive for the recipient to act on | user, agent |
| `input-needed` | Corresponds to `waiting_for_input` state; requests human/agent intervention | system |
| `state-change` | Notification about an agent's lifecycle/activity state change | system |
| `info` | Informational message, no action required | any |
| `response` | Reply to a previous message or query | agent |
| `query` | A question expecting a response | user, agent |

The `type` field is advisory - it helps recipients (especially LLMs) understand intent but does not gate behavior. New types can be added without breaking existing consumers.

### 2.5 Example Messages by Scenario

**User sends instruction to agent:**
```json
{
  "version": 1,
  "timestamp": "2026-03-07T14:30:00Z",
  "id": "msg_a1b2c3",
  "sender": "user:alice",
  "recipient": "agent:backend-dev",
  "msg": "Implement the auth module with JWT support",
  "type": "instruction",
  "urgent": false,
  "broadcasted": false,
  "attachments": ["docs/auth-spec.md"]
}
```

**Agent broadcasts query to grove:**
```json
{
  "version": 1,
  "timestamp": "2026-03-07T14:35:00Z",
  "id": "msg_d4e5f6",
  "sender": "agent:lead-agent",
  "recipient": "grove:my-project",
  "msg": "Has anyone modified the database schema in the last hour?",
  "type": "query",
  "broadcasted": true
}
```

**System notification (agent completed):**
```json
{
  "version": 1,
  "timestamp": "2026-03-07T15:00:00Z",
  "id": "msg_g7h8i9",
  "sender": "system:backend-dev",
  "recipient": "agent:lead-agent",
  "msg": "backend-dev has reached a state of COMPLETED: Auth module implemented with JWT middleware",
  "type": "state-change"
}
```

**System notification (agent needs input):**
```json
{
  "version": 1,
  "timestamp": "2026-03-07T15:10:00Z",
  "id": "msg_j0k1l2",
  "sender": "system:frontend-dev",
  "recipient": "agent:lead-agent",
  "msg": "frontend-dev is WAITING_FOR_INPUT: Should I use the existing API client or create a new one?",
  "type": "input-needed"
}
```

---

## 3. CLI Changes

### 3.1 New and Modified Flags

```
scion message [agent] <message> [flags]

Existing flags (unchanged behavior):
  -i, --interrupt      Interrupt the harness before sending
  -b, --broadcast      Send to all running agents in grove
  -a, --all            Send to all running agents across groves
      --in <duration>  Schedule delivery after duration
      --at <time>      Schedule delivery at absolute time

New flags:
      --plain          Send as plain text (bypass structured format)
      --type <type>    Message type (default: "instruction")
      --attach <path>  Attach file path(s), repeatable
```

### 3.2 Default Behavior Change

**Before**: All messages are plain text.
**After**: All messages are structured JSON by default. `--plain` flag restores plain-text behavior.

When a user runs:
```bash
scion message backend-dev "implement auth"
```

The CLI constructs a `StructuredMessage` with:
- `sender`: resolved from Hub identity (user JWT) or local username
- `recipient`: `agent:backend-dev`
- `type`: `"instruction"` (default)
- `urgent`: from `--interrupt` flag
- `broadcasted`: from `--broadcast`/`--all` flags
- `timestamp`: current UTC time
- `id`: generated UUID

When `--plain` is used:
```bash
scion message --plain backend-dev "just send this raw text"
```
The message is sent as-is (current behavior), with no JSON wrapping.

### 3.3 Sender Resolution

| Context | Sender Value |
|---------|-------------|
| Human user, Hub mode | `user:<username>` from Hub identity/JWT claims |
| Human user, local mode | `user:<os-username>` from `os/user.Current()` |
| Agent, Hub mode | `agent:<agent-slug>` from agent JWT subject |
| Agent, local mode | `agent:<agent-slug>` from `SCION_AGENT_SLUG` env var |
| System notification | `system:<subject-agent-slug>` |

---

## 4. Message Flow Changes by Sender/Recipient Pair

### 4.1 User -> Agent (via CLI)

```
User: scion message backend-dev "implement auth" --attach src/spec.md

CLI (cmd/message.go):
  1. Resolve sender identity
  2. Build StructuredMessage{...}
  3. Serialize to JSON
  4. Send via Hub API or local manager

Hub API (POST /agents/{id}/message):
  Body: { "structured_message": {...}, "interrupt": false }

Hub -> Broker -> Container:
  tmux send-keys delivers formatted text (see Section 5)
```

### 4.2 Agent -> Agent (via CLI inside container)

Same flow as user->agent, but:
- Sender resolved from agent JWT (`agent:<slug>`)
- Agent must have appropriate JWT scope (`grove:agent:message`)

### 4.3 System -> Agent (notification)

```
NotificationDispatcher.storeAndDispatch():
  1. Build StructuredMessage from agent state event
  2. Set sender = "system:<watched-agent-slug>"
  3. Set type = "state-change" or "input-needed"
  4. Dispatch via DispatchAgentMessage with structured payload
```

### 4.4 System -> User (web notification)

```
NotificationDispatcher:
  1. Build StructuredMessage
  2. Store as Notification record (message field = JSON)
  3. Publish via SSE as NotificationCreatedEvent

Web UI:
  - Notification tray receives SSE event
  - Parses structured message for display
  - Browser notification uses msg + type for title/body
```

### 4.5 Broadcast Messages

For broadcasts, each fan-out copy gets:
- `recipient`: set to the specific target agent when delivered
- `broadcasted`: `true`
- The original broadcast recipient (`grove:...` or `all`) is preserved only in logs

---

## 5. Harness Delivery Format

### 5.1 Plain-Text Wrapper for Agents

When a structured message is delivered to an agent via tmux send-keys, it is wrapped in a plain-text introduction to ensure LLM comprehension:

```
You are receiving a message from the orchestration system:

---BEGIN SCION MESSAGE---
{
  "version": 1,
  "timestamp": "2026-03-07T14:30:00Z",
  "id": "msg_a1b2c3",
  "sender": "user:alice",
  "msg": "Implement the auth module with JWT support",
  "type": "instruction",
  "urgent": false,
  "broadcasted": false,
  "attachments": ["docs/auth-spec.md"]
}
---END SCION MESSAGE---
```

Key points:
- The `recipient` field is **stripped** before delivery (the agent doesn't need to know its own identity from the message).
- The delimiters (`---BEGIN/END SCION MESSAGE---`) make it easy for LLMs and harness tooling to parse.
- The plain-text intro ensures the LLM understands this is a system-mediated message, not arbitrary user input.

### 5.2 Plain Mode Delivery

When `--plain` is used, the message is sent as-is (current behavior):
```
tmux send-keys -t scion "just send this raw text" Enter
```

### 5.3 Harness-Specific Considerations

Different harnesses may need different wrapping strategies:

| Harness | Delivery Notes |
|---------|---------------|
| **Claude Code** | Interrupt key: `Escape`. JSON is well-understood by Claude. |
| **Gemini CLI** | Interrupt key: `C-c`. JSON is well-understood by Gemini. |
| **Generic** | Fallback; structured messages still delivered but LLM comprehension not guaranteed. |

Future harnesses could implement a `FormatMessage(StructuredMessage) string` method on the `Harness` interface to customize delivery formatting per-harness.

---

## 6. API and Wire Protocol Changes

### 6.1 Updated MessageRequest

The `MessageRequest` struct (used in both Hub and Runtime Broker) expands to support structured messages while remaining backwards-compatible:

```go
// MessageRequest is the request body for sending a message to an agent.
type MessageRequest struct {
    // Plain text message (existing field, used for --plain mode)
    Message string `json:"message,omitempty"`

    // Structured message (new field, used by default)
    StructuredMessage *StructuredMessage `json:"structured_message,omitempty"`

    // Interrupt the harness before sending
    Interrupt bool `json:"interrupt,omitempty"`
}

// StructuredMessage represents a formatted Scion message.
type StructuredMessage struct {
    Version     int      `json:"version"`
    Timestamp   string   `json:"timestamp"`
    ID          string   `json:"id"`
    Sender      string   `json:"sender"`
    Recipient   string   `json:"recipient"`
    Msg         string   `json:"msg"`
    Type        string   `json:"type"`
    Urgent      bool     `json:"urgent,omitempty"`
    Broadcasted bool     `json:"broadcasted,omitempty"`
    Attachments []string `json:"attachments,omitempty"`
}
```

### 6.2 Backwards Compatibility

The Hub handler (`handleAgentMessage`) checks both fields:
- If `StructuredMessage` is set, use it.
- If only `Message` is set (legacy), treat as plain-text (current behavior).
- If both are set, `StructuredMessage` takes precedence.

This allows old CLI versions to continue working with the Hub.

### 6.3 Hub Client Update

```go
// SendMessage sends a plain text message to an agent (legacy).
func (s *agentService) SendMessage(ctx context.Context, agentID, message string, interrupt bool) error

// SendStructuredMessage sends a structured message to an agent.
func (s *agentService) SendStructuredMessage(ctx context.Context, agentID string, msg *StructuredMessage, interrupt bool) error
```

### 6.4 Scheduled Messages

The `CreateScheduledEventRequest` payload already stores arbitrary JSON. The structured message is stored as the payload and delivered at fire time using the same structured flow.

---

## 7. Storage and Logging

### 7.1 Message Log (New)

For audit, debugging, and future message broker use, messages should be logged to a dedicated store.

**Option A: Hub Database Table (Selected for Phase 1)**

```sql
CREATE TABLE messages (
    id TEXT PRIMARY KEY,
    grove_id TEXT NOT NULL,
    sender TEXT NOT NULL,
    recipient TEXT NOT NULL,
    type TEXT NOT NULL,
    msg TEXT NOT NULL,
    urgent BOOLEAN DEFAULT FALSE,
    broadcasted BOOLEAN DEFAULT FALSE,
    attachments TEXT,          -- JSON array
    created_at TIMESTAMP NOT NULL,
    delivered_at TIMESTAMP,
    delivery_status TEXT DEFAULT 'pending'  -- pending, delivered, failed
);

CREATE INDEX idx_messages_grove ON messages(grove_id);
CREATE INDEX idx_messages_sender ON messages(sender);
CREATE INDEX idx_messages_recipient ON messages(recipient);
CREATE INDEX idx_messages_created ON messages(created_at);
```

Messages are stored at the Hub before dispatch. Delivery status is updated after broker confirms receipt.

**Option B: Append-Only Log File (Deferred)**

For local mode or broker-side logging, messages are appended to a structured log file (JSON lines). This is useful for agent-to-agent message history without a Hub.

### 7.2 Structured Logging Integration

The existing logging subsystem (`hub.messages`, `broker.messages` from `logging-components.md`) already defines structured attributes. The structured message fields map directly:

```go
slog.Info("message dispatched",
    "subsystem", "hub.messages",
    "message_id", msg.ID,
    "sender", msg.Sender,
    "recipient", msg.Recipient,
    "type", msg.Type,
    "urgent", msg.Urgent,
    "broadcasted", msg.Broadcasted,
)
```

---

## 8. Extensible Notification Channels

### 8.1 Motivation

Currently, human user notifications are limited to:
1. Web UI notification tray (SSE + polling)
2. CLI query (`scion hub notifications`)
3. Browser push notifications (via web UI)

Users want notifications in their preferred messaging apps (Slack, Discord, email, SMS, etc.) without modifying core Scion code for each integration.

### 8.2 Notification Channel Interface

```go
// NotificationChannel delivers notifications to external systems.
type NotificationChannel interface {
    // Name returns the channel identifier (e.g., "slack", "email", "webhook").
    Name() string

    // Deliver sends a notification via this channel.
    // The StructuredMessage contains all context needed for formatting.
    Deliver(ctx context.Context, msg *StructuredMessage, config ChannelConfig) error

    // Validate checks that the channel configuration is valid.
    Validate(config ChannelConfig) error
}

// ChannelConfig holds channel-specific configuration.
// Each channel type defines its own config schema within this wrapper.
type ChannelConfig struct {
    Type   string            `json:"type"`   // "slack", "webhook", "email", etc.
    Params map[string]string `json:"params"` // Channel-specific key-value config
}
```

### 8.3 Built-In Channels

**Phase 1: Webhook (generic)**
```json
{
  "type": "webhook",
  "params": {
    "url": "https://hooks.example.com/scion",
    "method": "POST",
    "headers": "Authorization=Bearer xxx",
    "template": "default"
  }
}
```

The webhook channel POSTs the structured message JSON to the configured URL. This enables integration with any system that accepts webhooks (Slack incoming webhooks, Zapier, n8n, custom services).

**Phase 2: Slack (native)**
```json
{
  "type": "slack",
  "params": {
    "webhook_url": "https://hooks.slack.com/services/...",
    "channel": "#scion-notifications",
    "mention_on_urgent": "@here"
  }
}
```

### 8.4 Channel Registration and Configuration

Channels are registered per-user or per-grove in the Hub:

```sql
CREATE TABLE notification_channels (
    id TEXT PRIMARY KEY,
    grove_id TEXT,               -- NULL for user-global channels
    user_id TEXT NOT NULL,
    channel_type TEXT NOT NULL,  -- "webhook", "slack", "email"
    config TEXT NOT NULL,        -- JSON ChannelConfig
    enabled BOOLEAN DEFAULT TRUE,
    filter_types TEXT,           -- JSON array of message types to forward, NULL = all
    filter_urgent_only BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL
);
```

CLI for managing channels:
```bash
scion hub channels add --type webhook --url https://hooks.example.com/scion
scion hub channels add --type slack --webhook-url https://hooks.slack.com/...
scion hub channels list
scion hub channels remove <id>
scion hub channels test <id>   # sends a test notification
```

### 8.5 Dispatch Flow with Channels

```
NotificationDispatcher.storeAndDispatch():
  1. Build StructuredMessage
  2. Store Notification record
  3. Route by subscriber type:
     a. SubscriberTypeAgent -> DispatchAgentMessage (existing)
     b. SubscriberTypeUser  -> PublishNotification (SSE, existing)
                            -> ForEach user notification_channels:
                                 channel.Deliver(msg, config)
```

The channel dispatch is fire-and-forget with logging. Channel delivery failures do not block the notification pipeline.

---

## 9. Message Broker Architecture

### 9.1 Vision

Today, message delivery is point-to-point: CLI -> Hub API -> Broker -> tmux. For broadcast, the Hub fans out by iterating over targets. This works but doesn't scale well and doesn't support features like:

- Message queuing and retry
- Topic-based subscriptions
- Persistent message history
- Cross-grove message routing
- Plugin-based message transformation

A message broker adapter layer provides these capabilities without replacing the existing Hub infrastructure.

### 9.2 Architecture Overview

```
                    ┌──────────────────────┐
                    │   Message Broker     │
                    │   (Adapter Layer)    │
                    │                      │
  Producers:       │  ┌────────────────┐  │    Consumers:
  ─────────────────┤  │   Topic/Queue  │  ├──────────────────
  Agent A (CLI)  ──┤  │   Router       │  ├── Agent B (subscriber)
  User (CLI)     ──┤  │                │  ├── Agent C (subscriber)
  System (notif) ──┤  │   Adapters:    │  ├── Hub (store/forward)
                    │  │   - InProcess  │  │    Notification channels
                    │  │   - NATS       │  │
                    │  │   - Redis      │  │
                    │  └────────────────┘  │
                    └──────────────────────┘
```

### 9.3 Broker Adapter Interface

```go
// MessageBroker abstracts message routing and delivery.
type MessageBroker interface {
    // Publish sends a message to a topic.
    Publish(ctx context.Context, topic string, msg *StructuredMessage) error

    // Subscribe registers a handler for messages on a topic pattern.
    Subscribe(topic string, handler MessageHandler) (Subscription, error)

    // Close shuts down the broker connection.
    Close() error
}

type MessageHandler func(ctx context.Context, msg *StructuredMessage) error

type Subscription interface {
    Unsubscribe() error
}
```

### 9.4 Topic Hierarchy

```
scion.grove.<grove-id>.agent.<agent-slug>.messages    # direct messages to agent
scion.grove.<grove-id>.broadcast                       # grove-wide broadcasts
scion.global.broadcast                                 # global broadcasts
scion.grove.<grove-id>.notifications                   # notification events
scion.grove.<grove-id>.agent.<agent-slug>.status       # status changes
```

### 9.5 Adapter Implementations

**Phase 1: InProcessBroker (Default)**

Uses the existing `ChannelEventPublisher` pattern (Go channels with NATS-style subject matching). No external dependencies. Suitable for single-node deployments.

This is essentially a refactor of the current event system to also handle messages, not just status events. The key addition is that messages published to the broker are also persisted to the message log and forwarded to the Hub dispatcher for agent delivery.

```go
type InProcessBroker struct {
    publisher *ChannelEventPublisher
    store     MessageStore
    dispatcher AgentDispatcher
}
```

**Phase 2: NATS Adapter**

For multi-node deployments, a NATS adapter provides distributed pub/sub:
```go
type NATSBroker struct {
    conn *nats.Conn
    js   nats.JetStreamContext  // for persistence
}
```

**Phase 3: Redis Streams Adapter**

Alternative for deployments already using Redis:
```go
type RedisBroker struct {
    client *redis.Client
}
```

### 9.6 Broadcast Flow with Broker

**Current flow (without broker):**
```
Agent A: scion message --broadcast "whats up?"
  -> CLI iterates over running agents
  -> CLI sends N individual HTTP requests to Hub
  -> Hub dispatches N messages to broker(s)
```

**Proposed flow (with broker):**
```
Agent A: scion message --broadcast "whats up?"
  -> CLI sends 1 publish to Hub
  -> Hub publishes to broker topic: scion.grove.<id>.broadcast
  -> Broker fans out to all subscribed consumers
  -> Hub (subscribed on behalf of all agents in grove) receives
  -> Hub dispatches to each agent's runtime broker
```

The key difference is the CLI makes **one** API call instead of N, and the fan-out logic moves to the broker layer. The Hub subscribes on behalf of agents because agents themselves don't have direct broker connectivity (they're in containers).

### 9.7 Agent-to-Agent Direct Messages

For direct agent-to-agent messages, the broker provides optional logging and routing but is not strictly required:

```
Agent A: scion message agent-b "here are my findings"
  -> CLI sends to Hub API (1 request)
  -> Hub publishes to broker: scion.grove.<id>.agent.agent-b.messages
  -> Hub (subscribed) receives and dispatches to agent-b's broker
  -> Message logged in message store
```

For single-node InProcess mode, this adds minimal overhead (one channel send). For multi-node NATS mode, this enables cross-broker routing transparently.

### 9.8 Hub as Subscription Proxy

Agents run in containers without direct broker connectivity. The Hub acts as a subscription proxy:

1. When an agent starts, the Hub subscribes to `scion.grove.<id>.agent.<slug>.messages` on its behalf.
2. When a message arrives, the Hub dispatches it to the agent via the existing `DispatchAgentMessage` path.
3. When an agent stops, the Hub unsubscribes.

This keeps the agent container interface unchanged (tmux send-keys) while enabling broker-based routing.

---

## 10. Migration and Backwards Compatibility

### 10.1 Phased Rollout

| Phase | Change | Backwards Compatible |
|-------|--------|---------------------|
| 1 | Add `StructuredMessage` to `MessageRequest`, Hub accepts both | Yes - old `Message` field still works |
| 2 | CLI defaults to structured, `--plain` flag added | Yes - `--plain` restores old behavior |
| 3 | Notification dispatcher uses structured messages | Yes - message content is human-readable |
| 4 | Message storage table added | Yes - optional, no breaking changes |
| 5 | Notification channels added | Yes - additive feature |
| 6 | Broker adapter layer introduced | Yes - InProcess adapter preserves current behavior |

### 10.2 Version Negotiation

The `version` field in `StructuredMessage` enables future schema evolution. Consumers that don't understand a version can fall back to reading the `msg` field, which always contains a human-readable string.

### 10.3 CLI Version Mismatch

If an old CLI sends a plain `Message` to a new Hub:
- Hub treats it as a plain-text message (no structured wrapper).
- Delivery works as before.

If a new CLI sends a `StructuredMessage` to an old Hub:
- Old Hub ignores `structured_message` field, reads `message` field.
- New CLI should populate both fields during transition period.

---

## 11. Alternative Approaches Considered

### 11.1 Envelope-Only (No Structured Message Body)

**Idea**: Keep message body as plain text, add metadata only at the transport layer (HTTP headers, wrapper struct).

**Pros**: Simpler, less change to agent-facing delivery.
**Cons**: Metadata lost at delivery boundary (tmux send-keys). Agents can't distinguish message types. No attachment support. Notification channels can't format messages intelligently.

**Verdict**: Rejected. The value of structured messages comes from agents being able to parse and act on metadata (urgency, type, sender identity).

### 11.2 Protobuf/Binary Format

**Idea**: Use Protocol Buffers or similar binary format instead of JSON.

**Pros**: Smaller wire size, strong typing, code generation.
**Cons**: Not human-readable (problematic for tmux send-keys delivery to LLMs). Requires build tooling. Overkill for current message sizes.

**Verdict**: Rejected. Messages are ultimately consumed by LLMs via text injection. JSON is the natural format.

### 11.3 Full External Message Broker from Day 1

**Idea**: Skip the InProcess adapter, require NATS/Redis/RabbitMQ from the start.

**Pros**: Production-ready pub/sub immediately.
**Cons**: Adds external dependency for all users (including local-only mode). Operational complexity. Unnecessary for single-node deployments.

**Verdict**: Rejected in favor of phased approach. InProcess first, external adapters when needed.

### 11.4 Agent-Direct Broker Connectivity

**Idea**: Give agents direct access to the message broker (e.g., NATS client in container).

**Pros**: Lower latency, true pub/sub semantics.
**Cons**: Requires additional network configuration per container. Security implications (agents can subscribe to arbitrary topics). Breaks the current model where the Hub mediates all agent communication. Container images would need broker client libraries.

**Verdict**: Deferred. The Hub-as-proxy model is simpler and more secure. Direct connectivity could be an opt-in capability for advanced deployments.

### 11.5 GraphQL Subscriptions Instead of SSE

**Idea**: Use GraphQL subscriptions for web real-time notifications.

**Pros**: Typed queries, selective field retrieval.
**Cons**: The web frontend already uses SSE successfully. Adding GraphQL is a large architectural change for marginal benefit. SSE is simpler and well-suited to the current event model.

**Verdict**: Rejected. SSE continues to serve web notification delivery well.

---

## 12. Open Questions

### High Priority

1. **Message size limits**: Should we enforce a max message size? Current tmux send-keys has practical limits. Proposed: 64KB for `msg` field, 10 items for `attachments`.

2. **Attachment delivery**: The `attachments` field lists relative file paths. Should the system verify these files exist before sending? Should it include file content inline for cross-broker scenarios where the recipient is on a different machine? Or is path-reference sufficient since agents share a git repo?

3. **Message persistence scope**: Should messages be stored permanently or with TTL? Local mode messages (no Hub) - should they be logged at all, or only in Hub mode?

4. **Recipient stripping granularity**: The spec says "recipient field is stripped before delivery." Should we also strip `id` (to save tokens) or keep it for agent-side deduplication?

5. **Notification channel secrets**: Webhook URLs and Slack tokens are secrets. Should they be stored encrypted in the Hub DB, or referenced from an external secret manager?

### Medium Priority

6. **Message acknowledgment**: Should agents be able to acknowledge receipt of messages? This would enable delivery confirmation and retry logic. Current system is fire-and-forget.

7. **Reply threading**: Should messages support a `reply_to` field referencing a previous `id`? This would enable conversation threading between agents.

8. **Rate limiting**: Should there be rate limits on broadcasts? An agent could flood the system with `--broadcast` in a loop.

9. **Broker adapter selection**: Should the broker adapter be configurable per-grove or globally? A grove might want NATS while the system default is InProcess.

10. **Message type extensibility**: Should message types be a closed enum (validated at send time) or open (any string accepted)?

### Low Priority

11. **Message encryption**: Should messages support end-to-end encryption for sensitive content? The Hub currently sees all message content in plaintext.

12. **Message priority queue**: Beyond `urgent: bool`, should there be a priority level for message ordering?

13. **Cross-grove messaging**: Should agents in different groves be able to message each other directly, or only via global broadcast?

---

## 13. Implementation Plan

### Phase 1: Core Structured Message (Foundation)
- Define `StructuredMessage` Go struct in a shared package (e.g., `pkg/messages/`)
- Update `MessageRequest` in Hub and Broker to include `StructuredMessage` field
- Update Hub handler to accept and forward structured messages
- Preserve backwards compatibility with plain `Message` field
- Add `--plain` flag to CLI
- Update CLI to construct `StructuredMessage` by default
- Update sender resolution logic in CLI

### Phase 2: Delivery and Harness Updates
- Update `AgentManager.Message()` to format structured messages for tmux delivery
- Add `FormatMessage` method to `Harness` interface (optional per-harness customization)
- Strip `recipient` field before delivery
- Update notification dispatcher to produce `StructuredMessage` instead of plain text
- Update system notification prefix to use structured format

### Phase 3: Storage and Logging
- Add `messages` table to Hub store
- Store messages at Hub before dispatch
- Update delivery status on confirmation
- Add structured logging attributes for message fields
- Add CLI commands for message history (`scion hub messages list`)

### Phase 4: Notification Channels
- Define `NotificationChannel` interface
- Implement webhook channel
- Add `notification_channels` table
- Wire channel dispatch into `NotificationDispatcher`
- Add CLI commands for channel management
- Implement Slack channel adapter

### Phase 5: Message Broker Adapter Layer
- Define `MessageBroker` interface
- Implement `InProcessBroker` using existing `ChannelEventPublisher` patterns
- Refactor broadcast to use broker publish instead of CLI-side fan-out
- Add Hub subscription proxy for agent message topics
- Wire broker into Hub server initialization
- Add configuration for broker adapter selection

### Phase 6: External Broker Adapters
- Implement NATS adapter
- Implement Redis Streams adapter
- Add broker health check and reconnection logic
- Performance testing and tuning

---

## 14. Key Files Affected

### CLI Layer
- `cmd/message.go` - Add `--plain`, `--type`, `--attach` flags; default to structured messages
- `cmd/message_test.go` - Update tests for structured message construction

### Shared Types
- `pkg/messages/types.go` (new) - `StructuredMessage` struct, message type constants
- `pkg/messages/format.go` (new) - Formatting utilities for harness delivery

### Hub
- `pkg/hub/handlers.go` - Update `MessageRequest`, `handleAgentMessage`
- `pkg/hub/notifications.go` - Update `formatNotificationMessage` to produce `StructuredMessage`
- `pkg/hub/events.go` - Add message-related event types

### Runtime Broker
- `pkg/runtimebroker/types.go` - Update `MessageRequest` to include structured message
- `pkg/runtimebroker/handlers.go` - Update `sendMessage` to handle structured messages

### Agent Manager
- `pkg/agent/manager.go` - Update `Message()` to format structured messages for tmux delivery

### Harness
- `pkg/harness/harness.go` - Add optional `FormatMessage` to interface
- `pkg/harness/claude_code.go` - Claude-specific formatting (if needed)
- `pkg/harness/gemini_cli.go` - Gemini-specific formatting (if needed)

### Hub Client
- `pkg/hubclient/agents.go` - Add `SendStructuredMessage` method

### Broker Client
- `pkg/brokerclient/agents.go` - Update `SendMessage` to support structured format

### Store
- `pkg/store/store.go` - Add `MessageStore` interface
- `pkg/store/models.go` - Add `Message` model
- `pkg/store/sqlite/messages.go` (new) - SQLite implementation

### Web Frontend
- `web/src/components/shared/notification-tray.ts` - Parse structured message format
- `web/src/client/api.ts` - Update notification types

### Notification Channels (New)
- `pkg/hub/channels.go` (new) - Channel interface, registry, dispatch
- `pkg/hub/channels_webhook.go` (new) - Webhook channel implementation
- `pkg/hub/handlers_channels.go` (new) - Channel management API handlers
- `pkg/hubclient/channels.go` (new) - Channel management client
- `cmd/hub_channels.go` (new) - CLI commands for channel management

### Message Broker (New)
- `pkg/broker/broker.go` (new) - Broker interface
- `pkg/broker/inprocess.go` (new) - InProcess adapter
- `pkg/broker/nats.go` (new) - NATS adapter (Phase 6)
