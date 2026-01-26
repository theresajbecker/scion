# Hub Server Type System Review

## Overview

This document reviews the type system architecture in the new Hub Server implementation, analyzing consistency across layers and identifying opportunities for simplification.

## Current Architecture

The Hub Server currently defines types across three distinct layers:

| Layer | Package | Purpose |
|-------|---------|---------|
| API (shared) | `pkg/api/types.go` | Cross-cutting types used by CLI, runtimes, and harnesses |
| Storage | `pkg/store/models.go` | Persistence models for SQLite/DB |
| REST Handlers | `pkg/hub/handlers.go` | HTTP request/response DTOs |

Additionally, configuration types exist in:
- `pkg/config/hub_config.go` - Server configuration loading
- `pkg/hub/server.go` - `ServerConfig` struct for runtime use

---

## Identified Inconsistencies

### 1. Parallel Agent Type Definitions

**Problem:** `api.AgentInfo` and `store.Agent` both represent agents but with overlapping yet inconsistent field sets.

| Field Purpose | `api.AgentInfo` | `store.Agent` |
|---------------|-----------------|---------------|
| Primary identifier | `ID` (container/runtime ID) | `ID` (UUID PK) |
| URL-safe slug | `AgentID` | `AgentID` |
| Grove reference | `Grove` (string name) | `GroveID` (FK to Grove.ID) |
| Grove path | `GrovePath` (filesystem) | — |
| Hub endpoint | `HubEndpoint` | — |
| Profile | `Profile` | — |
| Runtime Host Type | `RuntimeHostType` | — |
| Connection State | — | `ConnectionState` |

**Impact:** Code converting between these types must handle missing fields, and the semantic meaning of `ID` differs between types.

### 2. Parallel Grove Type Definitions

**Problem:** `api.GroveInfo` and `store.Grove` serve similar purposes with different field emphasis.

| Field Purpose | `api.GroveInfo` | `store.Grove` |
|---------------|-----------------|---------------|
| Filesystem path | `Path` | — |
| Hub endpoint | `HubEndpoint` | — |
| Git remote | — | `GitRemote` (indexed, unique) |
| Active host count | — | `ActiveHostCount` (computed) |

**Impact:** Solo mode (local CLI) uses `api.GroveInfo` while hosted mode uses `store.Grove`. No clear conversion or unification exists.

### 3. REST API Exposes Storage Models Directly

**Problem:** Handler response types embed storage types directly:

```go
// handlers.go
type ListAgentsResponse struct {
    Agents     []store.Agent `json:"agents"`  // Direct storage type exposure
    NextCursor string        `json:"nextCursor,omitempty"`
    TotalCount int           `json:"totalCount"`
}
```

Similar patterns exist for all entities: Grove, RuntimeHost, Template, User.

**Impact:**
- Internal implementation details leak to API consumers
- Storage schema changes force API changes
- No opportunity to filter sensitive fields
- Makes API versioning difficult

### 4. Handler Request Types Reference Storage Types

```go
// handlers.go
type RegisterHostInfo struct {
    Capabilities       *store.HostCapabilities `json:"capabilities,omitempty"`
    Runtimes           []store.HostRuntime     `json:"runtimes,omitempty"`
    SupportedHarnesses []string                `json:"supportedHarnesses,omitempty"`
}
```

**Impact:** REST API is tightly coupled to storage layer. Changes to `store.HostCapabilities` directly affect the public API.

### 5. Duplicate Visibility Constants

```go
// pkg/store/models.go
const (
    VisibilityPrivate = "private"
    VisibilityTeam    = "team"
    VisibilityPublic  = "public"
)

// pkg/api/types.go
const (
    VisibilityPrivate = "private"
    VisibilityTeam    = "team"
    VisibilityPublic  = "public"
)
```

**Impact:** Maintenance burden; potential for drift if one is updated without the other.

### 6. Configuration Type Overlap

Three config-related types have overlapping fields:

| Field | `api.ScionConfig` | `store.TemplateConfig` | `store.AgentAppliedConfig` |
|-------|-------------------|------------------------|----------------------------|
| Harness | ✓ | ✓ | ✓ |
| ConfigDir | ✓ | ✓ | — |
| Env | ✓ | ✓ | ✓ |
| Detached | ✓ (pointer) | ✓ (bool) | — |
| CommandArgs | ✓ | ✓ | — |
| Model | ✓ | ✓ | ✓ |
| Image | ✓ | — | ✓ |
| Volumes | ✓ | — | — |
| Kubernetes | ✓ | — | — |

**Impact:** Unclear which type to use where; field updates may need replication across types.

### 7. JSON Tag Inconsistency

The `api` package uses snake_case in some places while `store` uses camelCase consistently:

```go
// api/types.go - mixed case
type ScionConfig struct {
    ConfigDir   string `json:"config_dir,omitempty" yaml:"config_dir,omitempty"`  // snake_case
    CommandArgs []string `json:"command_args,omitempty"`                           // snake_case
}

// store/models.go - consistent camelCase
type Agent struct {
    AgentID  string `json:"agentId"`   // camelCase
    GroveID  string `json:"groveId"`   // camelCase
}
```

**Impact:** API consumers face inconsistent naming conventions.

### 8. ServerConfig Duplication

```go
// pkg/hub/server.go
type ServerConfig struct { ... }

// pkg/config/hub_config.go
type HubServerConfig struct { ... }  // Very similar fields
type ServerConfig struct { Hub HubServerConfig; Database DatabaseConfig; ... }
```

**Impact:** Two `ServerConfig` types exist with slightly different purposes, causing confusion.

---

## Recommendations for Simplification

### Option A: API Layer as Canonical DTOs (Recommended)

Establish `pkg/api` as the single source of truth for data transfer objects:

1. **Rename and consolidate:**
   - `api.AgentInfo` → `api.Agent` (the canonical type)
   - Keep `store.Agent` as internal persistence model
   - Add explicit conversion functions: `store.Agent.ToAPI() *api.Agent`

2. **Handler responses use API types:**
   ```go
   type ListAgentsResponse struct {
       Agents     []api.Agent `json:"agents"`
       NextCursor string      `json:"nextCursor,omitempty"`
       TotalCount int         `json:"totalCount"`
   }
   ```

3. **Move shared constants to api package:**
   ```go
   // pkg/api/constants.go
   const (
       VisibilityPrivate = "private"
       VisibilityTeam    = "team"
       VisibilityPublic  = "public"
   )
   ```
   Store package references these constants.

### Option B: Dedicated DTO Package

Create `pkg/hub/dto` for REST-specific types:

```
pkg/
├── hub/
│   ├── dto/
│   │   ├── agent.go      // REST DTOs for agents
│   │   ├── grove.go      // REST DTOs for groves
│   │   └── convert.go    // Conversion functions
│   ├── handlers.go       // Uses dto types
│   └── server.go
```

Benefits:
- Clear separation of concerns
- Store types can evolve independently
- Explicit conversion catches field mismatches at compile time

### Option C: Unified Core Types

Define a single set of core domain types used everywhere:

```
pkg/
├── domain/
│   ├── agent.go          // Core Agent type
│   ├── grove.go          // Core Grove type
│   └── ...
├── store/
│   └── sqlite/           // Implements persistence using domain types
├── hub/
│   └── handlers.go       // Uses domain types directly
└── api/                  // Deprecated, merged into domain
```

This is more invasive but eliminates all duplication.

---

## Specific Action Items

### High Priority

1. **Consolidate Visibility constants** in one location (suggest `pkg/api/constants.go`)
2. **Create explicit conversion functions** between store and API types
3. **Standardize JSON tag casing** - recommend camelCase throughout for REST APIs
4. **Rename `config.ServerConfig`** to `config.GlobalConfig` to avoid confusion with `hub.ServerConfig`

### Medium Priority

5. **Define REST DTOs separately from storage models** to decouple API evolution
6. **Unify config types** - consolidate `ScionConfig`, `TemplateConfig`, and `AgentAppliedConfig` with clear hierarchy
7. **Resolve ID semantics** - clarify that `ID` is always UUID, `AgentID`/`Slug` is the user-facing identifier

### Low Priority (Future Work)

8. **Consider API versioning strategy** - with dedicated DTOs, v2 API becomes feasible
9. **Add OpenAPI spec generation** from DTO types for documentation

---

## Summary

The current implementation has grown organically with types defined per-layer rather than per-domain. While functional, this creates:

- **Duplication:** Same concepts defined in multiple places
- **Coupling:** REST API tied to storage implementation
- **Inconsistency:** Naming and field presence varies between parallel types

The recommended path is **Option A** (API Layer as Canonical DTOs) as it:
- Requires minimal restructuring
- Establishes clear ownership
- Enables future API versioning
- Maintains backward compatibility with existing CLI code using `api.AgentInfo`

Estimated effort: ~1-2 days for high-priority items, ~1 week for full consolidation.
