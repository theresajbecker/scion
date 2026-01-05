# Container Name Collision Strategy

## Problem
Scion supports multiple "groves" (independent projects/contexts). Users often use common agent names like "dev", "app", or "backend" across different groves.
Since container runtimes (like Docker) require unique container names per host, starting an agent named "dev" in Grove A will fail if an agent named "dev" is already running for Grove B.

## Approaches

### 1. Preemptive Prefixing (Namespace everything)
Always prefix container names with the grove name (e.g., `groveA-dev`, `groveB-dev`).

**Pros:**
- **Deterministic:** The container name is always predictable (`{grove}-{agent}`).
- **Simple:** No complex logic to check for collisions or "try-fallback" flows.
- **Clarity:** `docker ps` immediately shows which grove a container belongs to.

**Cons:**
- **Verbosity:** Container names become longer and harder to type for manual `docker` commands.
- **UX Change:** Users accustomed to `scion start dev` -> container `dev` might find the prefix annoying if they only ever use one grove.

### 2. Reactive Prefixing (Prefix on Collision)
Attempt to use the simple agent name (`dev`). If and *only if* a collision occurs with a container belonging to a *different* grove, fall back to a prefixed name (`groveA-dev`).

**Pros:**
- **Clean Default:** For the vast majority of users (single grove or non-overlapping names), container names remain short and simple (`dev`).
- **Minimal Friction:** Preserves the "local feel" of the tool.

**Cons:**
- **Inconsistency:** The container name for a given agent can change depending on what else is running. This creates "state-dependent" behavior.
- **Tooling Complexity:** Scripts or tools expecting container `dev` might break if it silently becomes `groveA-dev`.
- **Confusion:** A user might wonder "Why is my container named `my-project-dev` today when it was just `dev` yesterday?" (Answer: because you left the other project's `dev` container running).

## Detailed Design: Reactive Approach

The user prefers the **Reactive** approach to minimize preemptive clutter.

### Algorithm
When `scion start <agent_name>` is called in Grove `CurrentGrove`:

1. **Check for Existence:** Look for a container named `<agent_name>`.
2. **If NOT Found:**
   - Proceed to start container with name `<agent_name>`.
3. **If Found:**
   - Inspect labels of the existing container.
   - **Case A: Same Grove** (`scion.grove == CurrentGrove`)
     - The container belongs to us.
     - Proceed with standard logic (Resume if running, Recreate if necessary).
   - **Case B: Different Grove** (`scion.grove != CurrentGrove`)
     - **Collision Detected.**
     - Determine new target name: `<CurrentGrove>-<agent_name>`.
     - Repeat existence check for `<CurrentGrove>-<agent_name>`.
       - If found: Check grove ownership (should be ours). Proceed.
       - If not found: Start container with name `<CurrentGrove>-<agent_name>`.
     - Be sure the new container name is formatted as a valid slug regardless of grove name

### UX Considerations
- When a collision causes a rename, the CLI should output a notice:
  `"Notice: Container name 'dev' is in use by another grove. Using 'myproject-dev' instead."`
- The `scion stop`, `scion logs`, and `scion attach` commands must be smart enough to resolve the name.
  - If user runs `scion attach dev`, and only `myproject-dev` exists (for the current grove), it should resolve automatically.


### Resolution Logic (for Stop/Attach/etc)
When performing an operation on agent `dev` in Grove `CurrentGrove`:
1. List containers with label `scion.name=dev` AND `scion.grove=CurrentGrove`.
2. This query returns the specific container, regardless of whether it is named `dev` or `CurrentGrove-dev`.
3. Use that container's ID for the operation.

### Implication
We must ensure all `scion` commands lookup containers by **Labels** (`scion.name` + `scion.grove`), not strictly by container name string.
- `pkg/runtime/docker.go`: `Attach` currently attempts to match by Name/ID. It should be updated to prioritize Label-based lookup restricted to the current grove if possible, or we rely on the Manager to resolve the ID first.
