# Web Frontend Access Control UX Design

## Status
**Proposed**

## 1. Overview

This document specifies how the Scion web frontend integrates with the Hub permissions system (see `hosted/auth/permissions-design.md`) to render access-aware UI. The goal is to hide, disable, or annotate UI elements based on the current user's effective permissions — rather than relying solely on API-layer enforcement that results in unexpected 403 errors.

### Problem

Without frontend access awareness, every interactive element (buttons, links, forms) is visible to every user. A viewer clicking "Delete Agent" will see a 403 error; a member without grove-level `manage` will see a "New Policy" button that leads nowhere. This is a poor user experience and leaks information about available operations.

### Goals

1. **Consistent pattern** — A single, well-documented mechanism that all components use to gate UI on permissions
2. **Low ceremony** — Adding access checks to a component should require minimal boilerplate
3. **Server-authoritative** — The frontend never makes its own policy decisions; it consumes capability data computed by the Hub
4. **SSR-compatible** — Access-gated rendering works during server-side rendering, not just after hydration
5. **Fail-closed** — If capability data is missing or stale, UI defaults to the most restrictive state (hidden/disabled)
6. **Defense in depth** — Frontend gating is a UX optimization; API-layer enforcement remains the security boundary

### Non-Goals

- Replacing or duplicating the Hub's policy resolution engine on the client
- Fine-grained UI layout changes per role (e.g., entirely different pages for admins vs. viewers)
- Real-time permission change notification (permissions change infrequently; page refresh is acceptable)

---

## 2. Approach Comparison

Three approaches were evaluated. The recommended approach is **A: Server-Computed Capability Maps**.

### Approach A: Server-Computed Capability Maps (Recommended)

The Hub API annotates each resource response with the requesting user's allowed actions for that resource. The frontend reads these annotations and conditionally renders UI elements.

```
Browser                        Hub API
  │                              │
  │  GET /api/v1/groves/abc      │
  │─────────────────────────────►│
  │                              │  1. Fetch grove
  │                              │  2. Resolve user's effective actions
  │                              │  3. Attach _capabilities to response
  │  { id, name, ...,           │
  │    _capabilities: {         │
  │      actions: ["read",      │
  │        "update", "manage"]  │
  │    }                        │
  │  }                          │
  │◄─────────────────────────────│
  │                              │
  │  Component reads             │
  │  _capabilities.actions       │
  │  to show/hide controls       │
```

**How it works:**
- Every resource returned by the Hub API includes a `_capabilities` field
- `_capabilities.actions` lists the actions the requesting user can perform on that resource
- Components check this array to determine what controls to render
- For list responses, each item in the array carries its own `_capabilities`
- For page-level operations (like "create new agent"), a scope-level capability is included in the list response metadata

**Pros:**
- Single source of truth — the Hub's policy engine computes capabilities once
- No policy leakage — the client never sees policy definitions, group memberships, or resolution logic
- Cacheable — capabilities travel with the resource data; no separate cache to manage
- SSR-compatible — server can compute capabilities during page rendering
- Simple component API — check `actions.includes('delete')` instead of evaluating policies

**Cons:**
- Adds latency to API responses (policy resolution per resource)
- Capabilities are a point-in-time snapshot; could become stale if policies change mid-session
- List endpoints with many items require per-item capability resolution

**Mitigation for performance:** The Hub already resolves policies for authorization checks on every request. Capabilities piggyback on this resolution — the cost is computing the full action set instead of checking a single action. For list endpoints, batch resolution can amortize the group-expansion cost across all items (expand groups once, evaluate per item).

### Approach B: Client-Side Policy Evaluation

Ship the user's effective policies and group memberships to the client. A local policy engine mirrors the Hub's resolution logic.

**Pros:**
- No per-resource API overhead
- Instant UI updates when navigating between resources
- Works offline (if policies are cached)

**Cons:**
- **Duplicates the policy engine** — must keep client and server logic in sync
- **Leaks security model** — effective policies and group memberships are exposed to the browser
- **Complex client code** — scope resolution, priority ordering, group expansion all reimplemented in TypeScript
- **SSR mismatch risk** — server and client engines could disagree, causing hydration flicker

**Verdict:** Rejected. The maintenance burden of dual policy engines and the security information leakage outweigh the performance benefits.

### Approach C: Per-Action Permission Check API

Before rendering each action button, the client calls a dedicated permission-check endpoint.

```
GET /api/v1/permissions/check?resource=agent&id=xyz&action=delete
→ { allowed: true }
```

**Pros:**
- Always accurate — real-time check against current policies
- Minimal data leakage — returns only allow/deny per check

**Cons:**
- **N+1 problem** — a page with 10 agents and 4 actions each makes 40 API calls
- **Latency** — UI controls appear after async permission checks complete, causing layout shift
- **SSR-hostile** — server rendering would need to make internal API calls for each action
- **Batch variant** helps but adds API complexity

**Verdict:** Rejected. The per-action overhead makes this impractical for real-time UI rendering.

---

## 3. Capability Map Design

### 3.1 Response Envelope

Every resource returned by the Hub API includes a `_capabilities` field. The underscore prefix follows the convention for metadata fields that are not part of the resource's domain model.

#### Single Resource Response

```json
{
  "id": "agent-uuid",
  "name": "researcher-1",
  "status": "running",
  "groveId": "grove-uuid",
  "_capabilities": {
    "actions": ["read", "update", "stop", "attach", "delete"]
  }
}
```

#### List Response

Each item carries its own capabilities. The response metadata includes scope-level capabilities for operations like "create new resource."

```json
{
  "items": [
    {
      "id": "agent-1",
      "name": "researcher-1",
      "_capabilities": {
        "actions": ["read", "update", "stop", "attach"]
      }
    },
    {
      "id": "agent-2",
      "name": "builder-1",
      "_capabilities": {
        "actions": ["read"]
      }
    }
  ],
  "_capabilities": {
    "actions": ["create", "list"]
  },
  "cursor": "..."
}
```

In this example, the user can create new agents (scope-level `create` action) and has full control over `agent-1` but read-only access to `agent-2`.

### 3.2 Capability Resolution

The Hub computes capabilities by running the full policy resolution algorithm (see `permissions-design.md` Section 3.3) for every applicable action on the resource type.

```go
// pkg/hub/capabilities.go

// Actions defined per resource type
var resourceActions = map[string][]string{
    "agent":    {"read", "update", "delete", "start", "stop", "message", "attach"},
    "grove":    {"read", "update", "delete", "manage", "register"},
    "template": {"read", "update", "delete"},
    "group":    {"read", "update", "delete", "add_member", "remove_member"},
    "user":     {"read", "update"},
    "policy":   {"read", "update", "delete"},
}

// Scope-level actions (for list responses)
var scopeActions = map[string][]string{
    "agent":    {"create", "list"},
    "grove":    {"create", "list"},
    "template": {"create", "list"},
    "group":    {"create", "list"},
    "policy":   {"create", "list"},
}

type Capabilities struct {
    Actions []string `json:"actions"`
}

// ComputeCapabilities resolves all allowed actions for a user on a resource.
// It reuses the same policy resolution used for authorization checks.
func ComputeCapabilities(
    ctx context.Context,
    authz *AuthzService,
    user *User,
    resource Resource,
) Capabilities {
    allActions := resourceActions[resource.Type]
    allowed := make([]string, 0, len(allActions))

    for _, action := range allActions {
        decision := authz.CheckAccess(ctx, user, resource, Action(action))
        if decision.Allowed {
            allowed = append(allowed, action)
        }
    }

    return Capabilities{Actions: allowed}
}

// ComputeScopeCapabilities resolves scope-level actions (e.g., can user
// create agents in this grove?).
func ComputeScopeCapabilities(
    ctx context.Context,
    authz *AuthzService,
    user *User,
    scopeType string,
    scopeId string,
    resourceType string,
) Capabilities {
    actions := scopeActions[resourceType]
    allowed := make([]string, 0, len(actions))

    resource := Resource{Type: resourceType, ParentType: scopeType, ParentID: scopeId}
    for _, action := range actions {
        decision := authz.CheckAccess(ctx, user, resource, Action(action))
        if decision.Allowed {
            allowed = append(allowed, action)
        }
    }

    return Capabilities{Actions: allowed}
}
```

#### Performance: Batch Group Expansion

For list responses, group expansion (the expensive part of policy resolution) is performed once and reused across all items:

```go
func ComputeCapabilitiesBatch(
    ctx context.Context,
    authz *AuthzService,
    user *User,
    resources []Resource,
) []Capabilities {
    // Expand groups once for this user
    effectiveGroups := authz.ExpandGroups(ctx, user)

    results := make([]Capabilities, len(resources))
    for i, resource := range resources {
        results[i] = authz.CheckAccessWithGroups(ctx, user, effectiveGroups, resource)
    }
    return results
}
```

### 3.3 Admin and Owner Short-Circuits

For admin users, the capability map includes all actions (the admin bypass means every action is allowed). For resource owners, all actions on the owned resource are included. These short-circuits keep the per-resource cost near zero for the common cases.

---

## 4. Frontend Integration

### 4.1 Type Definitions

```typescript
// src/shared/types.ts

/** Capability annotations attached to API responses */
export interface Capabilities {
  actions: string[];
}

/** Helper to check if an action is allowed */
export function can(capabilities: Capabilities | undefined, action: string): boolean {
  if (!capabilities) return false;  // fail-closed
  return capabilities.actions.includes(action);
}

/** Helper to check if any of the given actions are allowed */
export function canAny(capabilities: Capabilities | undefined, ...actions: string[]): boolean {
  if (!capabilities) return false;
  return actions.some(a => capabilities.actions.includes(a));
}

// Extend existing resource types to include capabilities
export interface Agent {
  id: string;
  name: string;
  status: string;
  // ... existing fields ...
  _capabilities?: Capabilities;
}

export interface Grove {
  id: string;
  name: string;
  // ... existing fields ...
  _capabilities?: Capabilities;
}

// List response wrapper
export interface ListResponse<T> {
  items: T[];
  _capabilities?: Capabilities;  // scope-level capabilities
  cursor?: string;
}
```

### 4.2 Component Pattern

Components use the `can()` helper directly in their templates. This is the simplest approach and requires no special framework integration.

#### Example: Agent Card with Access-Aware Controls

```typescript
import { can } from '../../shared/types';

@customElement('scion-agent-card')
export class AgentCard extends LitElement {
  @property({ type: Object }) agent!: Agent;

  render() {
    const { agent } = this;
    const caps = agent._capabilities;

    return html`
      <wa-card>
        <div slot="header" class="header">
          <wa-icon name="cpu"></wa-icon>
          <div>
            <div class="title">${agent.name}</div>
            <div class="template">${agent.template}</div>
          </div>
        </div>

        <div class="status-row">
          <wa-badge variant="${this.getStatusVariant(agent.status)}">
            ${agent.status}
          </wa-badge>
        </div>

        <div slot="footer" class="actions">
          <!-- Terminal button: requires 'attach' action -->
          ${can(caps, 'attach') ? html`
            <wa-button
              variant="primary"
              size="small"
              ?disabled=${agent.status !== 'running'}
              @click=${() => this.handleAction('terminal')}
            >
              <wa-icon slot="prefix" name="terminal"></wa-icon>
              Terminal
            </wa-button>
          ` : nothing}

          <!-- Stop/Start: requires 'stop' or 'start' action -->
          ${agent.status === 'running' && can(caps, 'stop') ? html`
            <wa-button
              variant="danger"
              size="small"
              @click=${() => this.handleAction('stop')}
            >Stop</wa-button>
          ` : nothing}

          ${agent.status !== 'running' && can(caps, 'start') ? html`
            <wa-button
              variant="success"
              size="small"
              @click=${() => this.handleAction('start')}
            >Start</wa-button>
          ` : nothing}

          <!-- Delete: requires 'delete' action -->
          ${can(caps, 'delete') ? html`
            <wa-button
              variant="danger"
              size="small"
              outline
              @click=${() => this.handleAction('delete')}
            >Delete</wa-button>
          ` : nothing}
        </div>
      </wa-card>
    `;
  }
}
```

#### Example: List Page with Scope Capabilities

```typescript
@customElement('scion-agent-list')
export class AgentList extends LitElement {
  @state() private agents: Agent[] = [];
  @state() private scopeCapabilities?: Capabilities;

  private async loadAgents() {
    const response = await fetch(`/api/groves/${this.groveId}/agents`, {
      credentials: 'include'
    });
    const data: ListResponse<Agent> = await response.json();
    this.agents = data.items;
    this.scopeCapabilities = data._capabilities;
  }

  render() {
    return html`
      <div class="header">
        <h1>Agents</h1>

        <!-- "New Agent" button: requires scope-level 'create' -->
        ${can(this.scopeCapabilities, 'create') ? html`
          <wa-button variant="primary" @click=${this.handleCreate}>
            <wa-icon slot="prefix" name="plus"></wa-icon>
            New Agent
          </wa-button>
        ` : nothing}
      </div>

      <div class="agent-grid">
        ${this.agents.map(agent => html`
          <scion-agent-card .agent=${agent}></scion-agent-card>
        `)}
      </div>
    `;
  }
}
```

### 4.3 Hiding vs. Disabling

Two strategies exist for access-restricted controls. The choice depends on context:

| Strategy | When to Use | Example |
|----------|------------|---------|
| **Hide** (`nothing`) | User has no conceptual need for the action | Viewer doesn't see "Delete" button at all |
| **Disable** (`?disabled`) | User knows the action exists but cannot perform it now | "Terminal" button grayed out because agent isn't running |

**Guidelines:**

1. **Hide** actions the user will never be able to perform (missing permission)
2. **Disable** actions the user could perform but conditions aren't met (agent stopped, field empty, etc.)
3. **Never** hide navigation — if a user can `read` a resource, they can navigate to it
4. **Tooltip on disabled** — when disabling due to permissions (rare), add a tooltip explaining why

```typescript
// Hiding: user lacks the action entirely
${can(caps, 'delete') ? html`
  <wa-button variant="danger" @click=${this.handleDelete}>Delete</wa-button>
` : nothing}

// Disabling: user has the action but conditions aren't met
${can(caps, 'attach') ? html`
  <wa-tooltip content=${agent.status !== 'running' ? 'Agent must be running' : ''}>
    <wa-button
      ?disabled=${agent.status !== 'running'}
      @click=${this.handleTerminal}
    >Terminal</wa-button>
  </wa-tooltip>
` : nothing}
```

### 4.4 Lit Directive (Optional Convenience)

For components with many access-gated elements, a Lit directive can reduce template noise. This is optional — the `can()` helper is sufficient for most cases.

```typescript
// src/client/directives/if-can.ts
import { directive, Directive } from 'lit/directive.js';
import { nothing } from 'lit';
import type { Capabilities } from '../../shared/types';

class IfCanDirective extends Directive {
  render(capabilities: Capabilities | undefined, action: string, content: unknown) {
    if (!capabilities || !capabilities.actions.includes(action)) {
      return nothing;
    }
    return content;
  }
}

export const ifCan = directive(IfCanDirective);

// Usage in a template:
render() {
  return html`
    ${ifCan(this.agent._capabilities, 'delete', html`
      <wa-button variant="danger" @click=${this.handleDelete}>Delete</wa-button>
    `)}
  `;
}
```

This is syntactic sugar and adds no logic beyond what `can()` already provides. Use it when a component has 5+ access-gated elements and the conditional ternaries become hard to read.

---

## 5. SSR Integration

### 5.1 Server-Side Capability Computation

During SSR, the Go server fetches resources from the Hub API on behalf of the authenticated user. The Hub API responses already include `_capabilities` (since the server passes the user's session/token). The SSR renderer passes this data through to the component properties, and the same `can()` checks execute during server rendering.

```
Browser Request                Go Server                      Hub API
     │                            │                              │
     │  GET /groves/abc/agents    │                              │
     │───────────────────────────►│                              │
     │                            │  GET /api/v1/groves/abc/     │
     │                            │    agents                    │
     │                            │  (with user's session token) │
     │                            │─────────────────────────────►│
     │                            │                              │
     │                            │  Response includes           │
     │                            │  _capabilities per agent     │
     │                            │◄─────────────────────────────│
     │                            │                              │
     │  Server renders HTML       │                              │
     │  with can() checks         │                              │
     │  already evaluated         │                              │
     │◄───────────────────────────│                              │
```

Because the Go server is the BFF (backend-for-frontend), it fetches data with the user's credentials. The Hub API sees the requesting user and computes capabilities accordingly. The SSR output already reflects the correct access state — no hydration mismatch.

### 5.2 Initial State Hydration

The server includes the full resource data (with capabilities) in the `__SCION_DATA__` script tag. The client hydrates from this data, preserving the same capability information.

```html
<script id="__SCION_DATA__" type="application/json">
  {
    "agents": [
      {
        "id": "agent-1",
        "name": "researcher-1",
        "_capabilities": { "actions": ["read", "update", "stop"] }
      }
    ],
    "_capabilities": { "actions": ["create", "list"] }
  }
</script>
```

---

## 6. Staleness and Freshness

### 6.1 When Capabilities Become Stale

Capabilities are computed at fetch time. They can become stale when:

1. A policy is added/removed/modified
2. The user is added to or removed from a group
3. Resource ownership changes

These events are infrequent (minutes to days between changes, not seconds).

### 6.2 Staleness Strategy

**Acceptable staleness window:** Capabilities are valid for the duration of a page view. When the user navigates to a new page or refreshes, fresh capabilities are fetched with the new data.

**No proactive invalidation.** Policy changes don't push capability updates to connected clients. Rationale:

- Policy changes are rare and usually affect administrative users who understand the system
- The API layer still enforces authorization — stale capabilities can only cause a user to see a button they can't use (resulting in a 403), not bypass security
- Proactive invalidation would require tracking which capabilities are affected by a policy change, which is equivalent to re-running capability resolution for all connected users

**Explicit refresh:** Components can re-fetch data (and capabilities) via a manual refresh action or when an API call returns 403:

```typescript
private async handleAction(action: string) {
  const response = await fetch(`/api/agents/${this.agent.id}/${action}`, {
    method: 'POST',
    credentials: 'include'
  });

  if (response.status === 403) {
    // Capabilities were stale — re-fetch resource to get updated capabilities
    await this.refreshAgent();
    // Optionally show a toast: "Your permissions have changed"
  }
}
```

### 6.3 Future: SSE-Based Capability Refresh

If proactive capability freshness becomes important, the SSE event stream could include a lightweight `capabilities.changed` event when policies affecting the user change. The client would then re-fetch the current page's data. This is documented as a future optimization, not part of the initial design.

---

## 7. Current User Context

### 7.1 User Role Checks

Some UI decisions depend on the user's system role (`admin`, `member`, `viewer`) rather than per-resource capabilities. For example, admin-only navigation items like "Hub Settings" or "All Users."

The current user's role is available in the session and passed through the app shell:

```typescript
// In app-shell or any component with user context
render() {
  return html`
    <aside class="sidebar">
      <scion-nav .user=${this.user}></scion-nav>
    </aside>
    <!-- ... -->
  `;
}

// In navigation component
render() {
  return html`
    <nav>
      <a href="/groves">Groves</a>
      <a href="/agents">Agents</a>
      <a href="/templates">Templates</a>

      ${this.user?.role === 'admin' ? html`
        <div class="admin-section">
          <span class="section-label">Administration</span>
          <a href="/users">Users</a>
          <a href="/groups">Groups</a>
          <a href="/policies">Policies</a>
          <a href="/settings">Hub Settings</a>
        </div>
      ` : nothing}
    </nav>
  `;
}
```

### 7.2 Role vs. Capabilities

| Check Type | Use For | Mechanism |
|------------|---------|-----------|
| `user.role === 'admin'` | Admin-only navigation, admin-only pages, Hub-level settings | User session property |
| `can(resource._capabilities, action)` | Per-resource action buttons, forms, controls | Capability map from API |
| `can(listResponse._capabilities, 'create')` | "Create new" buttons on list pages | Scope-level capability from list API |

**Do not mix these.** Never check `user.role` to decide whether to show a "Delete Agent" button — use the resource's capabilities. The role check is for navigation-level concerns only.

---

## 8. Action-to-UI Mapping

This table documents which UI elements each action controls, to ensure consistency across components.

### Agent Actions

| Action | UI Element | Behavior When Absent |
|--------|-----------|---------------------|
| `read` | Agent visible in lists, navigable | Agent not shown (filtered by API) |
| `update` | Edit agent name/config, reassign template | Edit controls hidden |
| `delete` | Delete button | Delete button hidden |
| `start` | Start button (when stopped) | Start button hidden |
| `stop` | Stop button (when running) | Stop button hidden |
| `message` | Message input in agent detail | Message input hidden |
| `attach` | Terminal button | Terminal button hidden |
| `create` (scope) | "New Agent" button on list page | Button hidden |

### Grove Actions

| Action | UI Element | Behavior When Absent |
|--------|-----------|---------------------|
| `read` | Grove visible in lists, navigable | Grove not shown |
| `update` | Edit grove name/description/settings | Edit controls hidden |
| `delete` | Delete grove button | Delete button hidden |
| `manage` | Policy management tab, contributor management | Tabs hidden |
| `create` (scope) | "New Grove" button on dashboard | Button hidden |

### Template Actions

| Action | UI Element | Behavior When Absent |
|--------|-----------|---------------------|
| `read` | Template visible in list, viewable | Template not shown |
| `update` | Edit template metadata | Edit controls hidden |
| `delete` | Delete button | Delete button hidden (also hidden for global templates) |
| `create` (scope) | "New Template" button | Button hidden |

### Group Actions

| Action | UI Element | Behavior When Absent |
|--------|-----------|---------------------|
| `read` | Group visible in list, viewable | Group not shown |
| `update` | Edit group name/description | Edit controls hidden |
| `delete` | Delete group button | Delete button hidden |
| `add_member` | "Add Member" button | Button hidden |
| `remove_member` | Remove member button per row | Remove buttons hidden |

### Policy Actions

| Action | UI Element | Behavior When Absent |
|--------|-----------|---------------------|
| `read` | Policy visible in list, viewable | Policy not shown |
| `update` | Edit policy button | Edit button hidden |
| `delete` | Delete policy button | Delete button hidden |
| `create` (scope) | "New Policy" button | Button hidden |

---

## 9. Empty States and Read-Only Views

### 9.1 Empty State When No Create Permission

When a user can list resources but cannot create them, the empty state should not show a "Create" CTA:

```typescript
renderEmptyState() {
  if (can(this.scopeCapabilities, 'create')) {
    return html`
      <div class="empty-state">
        <wa-icon name="inbox"></wa-icon>
        <h3>No agents yet</h3>
        <p>Create your first agent to get started.</p>
        <wa-button variant="primary" @click=${this.handleCreate}>
          Create Agent
        </wa-button>
      </div>
    `;
  }

  return html`
    <div class="empty-state">
      <wa-icon name="inbox"></wa-icon>
      <h3>No agents</h3>
      <p>There are no agents in this grove yet.</p>
    </div>
  `;
}
```

### 9.2 Read-Only Detail Views

Detail pages should render as read-only when the user lacks `update` permission. Forms become display-only; edit buttons are hidden.

```typescript
// Pattern: form fields become display text when read-only
renderNameField() {
  if (can(this.grove?._capabilities, 'update')) {
    return html`
      <wa-input
        label="Grove Name"
        .value=${this.grove.name}
        @wa-input=${this.handleNameChange}
      ></wa-input>
    `;
  }

  return html`
    <div class="field">
      <label>Grove Name</label>
      <span>${this.grove.name}</span>
    </div>
  `;
}
```

---

## 10. Error Handling: 403 Recovery

Even with capability maps, 403 errors can occur (stale capabilities, race conditions, or bugs). The frontend should handle these gracefully.

### 10.1 Global 403 Handler

```typescript
// src/client/api.ts

export async function apiFetch(path: string, options?: RequestInit): Promise<Response> {
  const response = await fetch(path, {
    ...options,
    credentials: 'include',
  });

  if (response.status === 403) {
    const error = await response.json();

    // Dispatch a global event that components can listen for
    window.dispatchEvent(new CustomEvent('scion:access-denied', {
      detail: {
        resource: error.error?.details?.resource,
        action: error.error?.details?.action,
        reason: error.error?.details?.reason,
      }
    }));
  }

  return response;
}
```

### 10.2 Toast Notification

The app shell listens for access-denied events and shows a toast:

```typescript
// In app-shell.ts
connectedCallback() {
  super.connectedCallback();
  window.addEventListener('scion:access-denied', this.handleAccessDenied);
}

private handleAccessDenied = (e: CustomEvent) => {
  const { action, resource } = e.detail;
  // Show toast
  const alert = Object.assign(document.createElement('wa-alert'), {
    variant: 'warning',
    closable: true,
    duration: 5000,
  });
  alert.textContent = `You don't have permission to ${action} this resource.`;
  document.body.append(alert);
  alert.toast();
};
```

---

## 11. Implementation Checklist

### Phase 1: Hub API — Capability Map Support
- [ ] Define `Capabilities` type in Hub API models
- [ ] Implement `ComputeCapabilities` and `ComputeScopeCapabilities`
- [ ] Implement batch group expansion for list responses
- [ ] Add `_capabilities` to all resource GET endpoints
- [ ] Add `_capabilities` to list response metadata
- [ ] Add admin and owner short-circuit paths

### Phase 2: Frontend Types and Helpers
- [ ] Add `Capabilities` interface and `can()` / `canAny()` helpers to `shared/types.ts`
- [ ] Update `Agent`, `Grove`, `Template`, `Group`, `Policy` types with optional `_capabilities`
- [ ] Add `ListResponse<T>` type with scope capabilities
- [ ] Add `apiFetch` wrapper with 403 handling
- [ ] Add global access-denied toast handler in app shell

### Phase 3: Component Integration
- [ ] Update `scion-agent-card` with capability-gated actions
- [ ] Update `scion-agent-list` with scope capability for "New Agent"
- [ ] Update `scion-grove-detail` with capability-gated edit/manage controls
- [ ] Update `scion-template-card` with capability-gated actions
- [ ] Update `scion-nav` with role-based admin section
- [ ] Update empty states to be capability-aware
- [ ] Add read-only mode for detail pages without `update` capability

### Phase 4: SSR Integration
- [ ] Ensure Go SSR renderer passes `_capabilities` through to component properties
- [ ] Verify hydration preserves capability data from `__SCION_DATA__`
- [ ] Test SSR output matches hydrated output for access-gated elements

---

## 12. Open Questions

### 12.1 Granularity of Scope-Level Capabilities

**Question:** For list endpoints, should `_capabilities` include only the actions for the immediate scope, or also inherited capabilities from parent scopes?

**Example:** On the agent list page within a grove, should `_capabilities.actions` include `manage` (a grove-level action) alongside `create` and `list` (agent-level actions)?

**Recommendation:** Scope-level capabilities should only include actions relevant to the resource type being listed. Grove-level capabilities belong on the grove resource itself. Components that need both should fetch both.

### 12.2 Capability Map Size on Large Lists

**Question:** For a list of 500 agents, including `_capabilities` on each adds ~50 bytes per item (25KB total). Is this acceptable?

**Context:** The alternative is omitting per-item capabilities from list responses and only computing them on detail views. This would mean action buttons on list-item cards are either always shown (with 403 risk) or always hidden (requiring navigation to detail for actions).

**Recommendation:** Include per-item capabilities. 25KB is negligible relative to the rest of the response payload, and the UX cost of hiding all action buttons on list items is high.

### 12.3 Policy Management Page Self-Referentiality

**Question:** The policy management UI itself requires `manage` permission. Should the policy editor show its own `_capabilities` for the policy being edited? Can a user with `update` on a policy add actions they themselves don't have?

**Recommendation:** Defer to the Hub API for validation. The frontend shows the policy editor UI if the user has `update` capability on the policy. The Hub validates that the policy being created/modified doesn't grant actions beyond what the creating user can delegate. This is a Hub-side authorization concern, not a frontend concern.

### 12.4 State Manager Integration

**Question:** Should the StateManager (which manages real-time SSE updates) also track capabilities? When a delta update arrives via SSE, it currently merges resource fields — should it preserve or update `_capabilities`?

**Recommendation:** SSE delta updates should **not** include `_capabilities`. Capabilities are only computed on explicit data fetches (page load, refresh, navigation). The StateManager should preserve existing `_capabilities` when merging deltas. If a resource is deleted and recreated via SSE events, it won't have capabilities until the next fetch — this is acceptable since the user would need to interact with the resource (triggering a fetch) before needing capability data.

---

## 13. References

- **Permissions Design:** `hosted/auth/permissions-design.md`
- **Web Frontend Design:** `hosted/web-frontend-design.md`
- **Hub API:** `hosted/hub-api.md`
- **Web Auth Design:** `hosted/auth/web-auth.md`
