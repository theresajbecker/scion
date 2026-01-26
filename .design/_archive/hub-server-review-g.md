# Hub Server Implementation Review

## Executive Summary

The current Hub Server implementation establishes a solid foundation for the control plane, with a clear separation between HTTP handlers (`pkg/hub`) and the persistence layer (`pkg/store`). However, there is a significant semantic misalignment between the CLI's internal types (`pkg/api`) and the server's storage models (`pkg/store`), particularly regarding resource identity (`ID` vs `AgentID`).

Furthermore, the API handlers currently leak storage models directly to the wire. While convenient for rapid prototyping, this couples the external API contract to the internal database schema, which will impede future evolution.

## Key Findings

### 1. Identity Ambiguity (`ID` vs `AgentID`)

This is the most critical issue. The semantics of `ID` differ dangerously between the CLI and the Server:

*   **CLI (`pkg/api.AgentInfo`):**
    *   `ID`: Represents the **Container ID** (runtime-assigned, e.g., Docker hash).
    *   `AgentID`: Represents the **Slug** (URL-safe identifier).
*   **Server (`pkg/store.Agent`):**
    *   `ID`: Represents the **Database UUID** (Primary Key).
    *   `AgentID`: Represents the **Slug** (URL-safe identifier).

**Risk:** A client expecting `ID` to be a container hash will break when receiving a UUID from the server, or vice versa. If the CLI sends its `ID` (container hash) in a field where the server expects a UUID, lookups will fail or corrupted data will be persisted.

### 2. Leaking Storage Models

The API handlers in `pkg/hub/handlers.go` return `store` types directly:

```go
type ListAgentsResponse struct {
    Agents []store.Agent `json:"agents"` // Direct dependency on DB model
    // ...
}
```

**Issues:**
*   **Coupling:** A change to the database schema (e.g., renaming a column in `store.Agent`) implicitly changes the public API contract.
*   **Leakage:** Internal DB fields (if added later, like `EncryptedSecrets` or `InternalState`) could be accidentally exposed via JSON serialization unless explicitly ignored.
*   **Validation:** There is no distinct layer to validate "write" operations separate from DB constraints.

### 3. Type Factoring & Location

*   **`pkg/api`**: Currently acts as a "CLI internals" package rather than a shared contract. It contains CLI-specific logic (e.g., `ScionConfig`, `StartOptions`) mixed with domain definitions.
*   **`pkg/store`**: Pure persistence models.
*   **`pkg/hub`**: Defines inline Request/Response structs, but relies on `store` models for nested objects.

## Recommendations

### 1. Unify and Clarify Identity

Adopt a consistent naming convention across the entire system (CLI and Server).

*   **`UUID`**: The system-assigned unique identifier (Database PK).
*   **`Slug`**: The user-friendly URL-safe identifier (formerly `AgentID` in some places).
*   **`ContainerID`**: The runtime-specific container hash.

**Proposed Change:**
Refactor `Agent` types to explicitly name these fields:

```go
// Shared definition (conceptually)
type Agent struct {
    UUID        string // The stable DB ID
    Slug        string // The "name" used in URLs
    ContainerID string // The ephemeral runtime ID (if applicable)
    // ...
}
```

### 2. Introduce Explicit API Resources (DTOs)

Create a dedicated package (e.g., `pkg/hub/api` or `pkg/api/v1`) that defines the **Wire Protocol**.

*   This package should contain the structs used for JSON serialization/deserialization.
*   Handlers should map `store.Agent` -> `api_v1.Agent` before responding.
*   This allows the DB schema to evolve independently of the API contract.

**Example:**

```go
// pkg/api/v1/agent.go
type Agent struct {
    ID        string `json:"id"`        // The UUID
    Name      string `json:"name"`      // Display name
    Slug      string `json:"slug"`      // URL identifier
    Status    string `json:"status"`
    // ... explicit fields only ...
}
```

### 3. Refactor `pkg/api`

The existing `pkg/api` is overloaded. Split it:

*   **`pkg/api/client`** (or keep in `pkg/agent`): CLI-specific configuration and logic (`StartOptions`, `ScionConfig`).
*   **`pkg/api/model`** (or `pkg/domain`): Core domain types shared by CLI and Server (if any logic needs to be shared).
*   **`pkg/api/wire`**: The HTTP JSON contract.

### 4. Alignment Checklist

| Concept | CLI (`api.AgentInfo`) | Store (`store.Agent`) | Recommendation |
| :--- | :--- | :--- | :--- |
| **Primary Key** | N/A (implied `AgentID`) | `ID` (UUID) | Use `UUID` field in API |
| **Human ID** | `AgentID` (Slug) | `AgentID` (Slug) | Rename to `Slug` for clarity |
| **Runtime ID** | `ID` | *Missing* (implied `ContainerStatus`?) | Add `ContainerID` to Store/API |
| **Group Ref** | `Grove` (Name) & `GroveID` (Hosted) | `GroveID` (UUID) | API should expose both `GroveUUID` and `GroveSlug` |

## Conclusion

The Hub Server is well-structured locally but mismatched with the existing CLI's worldview. Prioritize resolving the `ID` vs `AgentID` collision before integrating the CLI with the Hub, as this will cause subtle bugs in resource referencing. Introducing explicit API DTOs now will save significant refactoring effort as the system grows.
