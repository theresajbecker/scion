# Taskless Refactor: Remove Task Requirement from `scion start`

## Overview

The `scion start` command currently treats a task (prompt) as conditionally required — it errors when no task is provided unless the `--attach` flag is set or a `prompt.md` file already has content. This behavior is obsolete. The `--attach` flow already demonstrates the desired behavior: agents should be able to start without an explicit task, relying on their template's built-in prompt, an existing `prompt.md`, or plain interactive use.

This refactor removes the task requirement from the start path and simplifies the conditional logic that currently checks for the attach flag as an exception to that requirement.

## Inventory of Changes

### Layer 1: CLI Commands (`cmd/`)

#### 1.1 `cmd/start.go`
- **Line 23** — Change `Use` from `"start <agent-name> [task...]"` to `"start <agent-name> [task...]"` (no change to syntax — task remains accepted but is fully optional).
- **Lines 26-31** — Update `Long` description to remove language suggesting the agent "looks for a prompt.md file" as a fallback, and instead state that the task is optional: the agent starts interactively or uses its template prompt if none is given.

#### 1.2 `cmd/resume.go`
- **Lines 25-30** — Update `Long` description similarly. Resume already does not require a task, so this is documentation cleanup only.

#### 1.3 `cmd/create.go`
- **Lines 35-40** — Update `Long` description: task is already optional here ("an empty prompt.md is created"). No logic change needed.

#### 1.4 `cmd/common.go` — `RunAgent()` (lines 225-350)
- **Lines 257-271** — The block that checks `if attach` and attaches to an already-running agent can remain as-is. This is useful UX regardless of task presence.
- **Lines 276-280** — The `detached` override when `attach` is set can remain as-is. The `--attach` flag still controls foreground/background behavior independent of task.
- **No changes to task extraction** (line 227) — task is still extracted from args and passed through; it just won't be required.

#### 1.5 `cmd/common.go` — `startAgentViaHub()` (lines 352-603)
- **Lines 409-421** — The workspace bootstrap condition `if hubCtx.GrovePath != "" && task != ""` gates workspace file collection on task presence. This should be changed to `if hubCtx.GrovePath != ""` since workspace files should be bootstrapped regardless of whether a task is provided. A non-git grove starting an agent without a task should still have its workspace uploaded.

### Layer 2: API Types (`pkg/api/`)

#### 2.1 `pkg/api/types.go` — `StartOptions` struct (lines 211-226)
- No structural change. The `Task` field remains as `string` (zero value is empty, which is valid). The `Detached *bool` field remains for controlling foreground/background behavior.
- **Remove the implicit semantic contract** that `Task == ""` combined with `Detached == nil` means "error". After the refactor, `Task == ""` is simply "no task given".

#### 2.2 `pkg/api/harness.go` — `Harness` interface (line 26)
- `GetCommand(task string, resume bool, baseArgs []string) []string` — No signature change. All harness implementations already handle `task == ""` correctly by simply not appending the task argument.

### Layer 3: Agent Lifecycle (`pkg/agent/`)

#### 3.1 `pkg/agent/run.go` — `AgentManager.Start()` (lines 31-324)
- **Lines 85-90** — **CRITICAL CHANGE.** Remove the task-required validation:
  ```go
  // REMOVE this block:
  isAttaching := opts.Detached != nil && !*opts.Detached
  if task == "" && promptFileContent == "" && !isAttaching && !opts.Resume {
      return nil, fmt.Errorf("no task provided: prompt.md is empty at %s and no task was given in options", promptFile)
  }
  ```
  After removal, the `isAttaching` variable is no longer needed.
- **Lines 92-94** — The task conflict check (`if !opts.Resume && task != "" && promptFileContent != "" && task != promptFileContent`) can remain. If both a task arg and a `prompt.md` exist with different content, it is still appropriate to error to avoid ambiguity.
- **Lines 96-100** — The fallback logic (`if task == "" && !opts.Resume { task = promptFileContent }`) can remain. This still makes sense: if `prompt.md` has content and no CLI task was given, use the prompt file.

#### 3.2 `pkg/agent/provision.go` — `Provision()` (lines 80-98)
- No change. Task is already optional here — only written to `prompt.md` if provided.

### Layer 4: Hub Server (`pkg/hub/`)

#### 4.1 `pkg/hub/handlers.go` — `createAgent()` handler

**Existing agent start path (lines 310-364):**
- **Lines 322-337** — **CRITICAL CHANGE.** Remove the task/prompt validation block for existing agents:
  ```go
  // REMOVE this block:
  if req.Task == "" {
      hasPrompt, err := dispatcher.DispatchCheckAgentPrompt(ctx, existingAgent)
      if err != nil {
          writeError(w, http.StatusBadRequest, ...)
          return
      }
      if !hasPrompt && !req.Attach {
          writeError(w, http.StatusBadRequest, ...)
          return
      }
  }
  ```
  Replace with: proceed directly to dispatch. If no task is given, pass empty string — the harness handles it.

**New agent creation path (lines 1676-1733):**
- **Lines 1679-1733** — **CRITICAL CHANGE.** Remove the equivalent validation block for new agent creation:
  ```go
  // REMOVE this block:
  if req.Task == "" {
      slug := api.Slugify(req.Name)
      existingAgent, err := ...
      // ... prompt check ...
      if !hasPrompt && !req.Attach {
          writeError(...)
          return
      }
  }
  ```
  This entire `if req.Task == ""` block (lines 1679-1733) merges the "start existing agent" and "create new" paths. Once the task requirement is removed, this block's logic of looking up an existing agent and checking its prompt is no longer needed as a gate. However, the semantic of "if agent already exists and no task given, just start it" is still valuable. Refactor to: if the agent already exists (regardless of task), start it. Remove the prompt check from this path.

#### 4.2 `pkg/hub/handlers.go` — Dispatch decision (lines 501-532)
- **Line 507** — The condition `if (req.Task != "" || req.Attach) && !req.ProvisionOnly` determines whether to do a full create+start or provision-only. After the refactor, this should become `if !req.ProvisionOnly` — always start the agent unless explicitly provision-only. The `req.Attach` check is no longer needed as a special case to enable taskless start.

#### 4.3 `pkg/hub/handlers.go` — `CreateAgentRequest` struct (lines 148-163)
- **Line 158** — The `Attach bool` field's comment says "If true, dispatch the agent even without a task (for interactive attach mode)." Update comment to reflect that tasks are always optional. The `Attach` field may still be useful for signaling to the broker/harness that the session is interactive, but it no longer serves as a "bypass task validation" flag.

#### 4.4 `pkg/hub/httpdispatcher.go` — `DispatchAgentStart()` (lines 544-562)
- No change needed. Already handles empty task gracefully: falls back to `AppliedConfig.Task`, and if that's also empty, passes empty string to broker.

#### 4.5 `pkg/hub/httpdispatcher.go` — `DispatchCheckAgentPrompt()` (lines 620-632)
- This method will no longer be called from the start path. It may still be useful for UI or status queries. **Keep but mark as unused from the start flow.** If no other callers exist, consider removing it.

### Layer 5: Hub Client (`pkg/hubclient/`)

#### 5.1 `pkg/hubclient/agents.go` — `CreateAgentRequest` struct (lines 98-115)
- **`Task` field** (line 107) — Remains. Still used to pass task if provided.
- **`Attach` field** (line 113) — Remains, but its purpose shifts from "bypass task requirement" to "signal interactive mode to broker." Update comment.

#### 5.2 `pkg/hubclient/types.go` — `AgentConfig` struct (lines 55-62)
- **`Task` field** (line 61) — Remains as-is. Optional field, no semantic change.

### Layer 6: Store Model (`pkg/store/`)

#### 6.1 `pkg/store/models.go` — `AgentAppliedConfig` struct (lines 77-96)
- **`Task` field** (line 83) — Remains. Still stores the initial task if one was given.
- **`Attach` field** (line 84) — Remains, but update comment from "agent should start in attach (interactive) mode" to something that doesn't imply task bypass.

### Layer 7: Runtime Broker (`pkg/runtimebroker/`)

#### 7.1 `pkg/runtimebroker/handlers.go` — `startAgent()` (lines 548-604)
- No change. Already reads task from request body as optional (`if r.Body != nil && r.ContentLength != 0`), passes to `StartOptions`. The agent manager (Layer 3) handles validation.

#### 7.2 `pkg/runtimebroker/handlers.go` — `checkAgentPrompt()` (lines 739-790)
- Keep for now. May be removed later if no callers remain after the hub changes.

#### 7.3 `pkg/runtimebroker/types.go` — `CreateAgentConfig` struct (lines 175-200)
- **`Task` field** (line 192) — Remains as-is. Passthrough to harness.

### Layer 8: Runtime (`pkg/runtime/`)

#### 8.1 `pkg/runtime/interface.go` — `RunConfig` struct (line 39)
- **`Task` field** — Remains. Passed to harness `GetCommand()`. Already optional.

#### 8.2 `pkg/runtime/common.go` (line 263) and `pkg/runtime/k8s_runtime.go` (line 206)
- No change. Both call `config.Harness.GetCommand(config.Task, ...)` which handles empty task.

### Layer 9: Harness Implementations (`pkg/harness/`)

All harness implementations already handle `task == ""` correctly:
- **Claude** (`claude_code.go:51-61`): Only appends task if non-empty.
- **Gemini** (`gemini_cli.go:148-158`): Only adds `--prompt-interactive` if task non-empty.
- **Generic** (`generic.go:106-112`): Only appends if non-empty.
- **Codex** (`codex.go:79-91`): Only appends if non-empty.
- **OpenCode** (`opencode.go:77-90`): Only appends task arg if non-empty.

**No changes needed in harness layer.**

### Layer 10: Tests

#### 10.1 `pkg/hub/bootstrap_test.go`
- **`TestCreateThenStartWithoutTask`** (around line 630) — Currently tests that `Attach: true` bypasses prompt check. Update to verify that starting without a task succeeds *without* needing the `Attach` flag.

#### 10.2 `pkg/hub/handlers_test.go`
- **`TestAgentCreate_AttachNoTask`** (lines 421-490) — Rename/repurpose. The test should verify that agents can be created and started without a task, period. The attach flag is no longer the thing being tested here.
- **`TestAgentCreate_NoTask`** (lines 283-353) — Verify this test reflects the new behavior (agent creation without a task succeeds and dispatches).

#### 10.3 `pkg/agent/run_test.go` or `pkg/agent/provision_test.go`
- If any test asserts the "no task provided" error, update or remove it.

### Layer 11: Web Frontend (`web/`)

#### 11.1 `web/src/shared/types.ts`
- **`taskSummary`** field (line 92) — Remains. Purely display metadata, not related to requirement logic.

#### 11.2 `web/src/components/pages/agents.ts`
- **Lines 405** — Conditional display of `taskSummary`. No change needed; it already handles the absent case.

### Layer 12: Broker Client (`pkg/hub/brokerclient.go`)

#### 12.1 `StartAgent()` (lines 150-174)
- No change. Already sends task in body only if non-empty, handles empty task gracefully.

## Summary of Critical Changes

| # | File | Change |
|---|------|--------|
| 1 | `cmd/start.go` | Update command description |
| 2 | `cmd/common.go:409` | Remove `task != ""` gate on workspace bootstrap |
| 3 | `pkg/agent/run.go:85-90` | **Remove task-required validation block** |
| 4 | `pkg/hub/handlers.go:322-337` | **Remove prompt check for existing agent start** |
| 5 | `pkg/hub/handlers.go:1679-1733` | **Remove prompt check for new agent start** |
| 6 | `pkg/hub/handlers.go:507` | Change dispatch condition from `(task \|\| attach) && !provisionOnly` to `!provisionOnly` |
| 7 | Tests | Update tests that assert task-required errors or attach-bypass behavior |

## Non-Changes (Explicitly Kept)

- **Task field in all structs**: Remains as an optional string throughout the stack. Providing a task still works exactly as before.
- **`--attach` flag**: Remains useful for its primary purpose — controlling foreground/background behavior and attaching to running agents. It just no longer serves double-duty as a task-requirement bypass.
- **`prompt.md` system**: Remains. Templates can still provide a prompt, and users can still write to `prompt.md` before starting. The fallback from `prompt.md` content is preserved.
- **Task conflict error**: If both `prompt.md` and a CLI task are provided with different content, the error is preserved to prevent ambiguity.
- **`TaskSummary` display field**: Purely cosmetic, unrelated to this refactor.

## Execution Order

1. Update `pkg/agent/run.go` — remove validation (core local-mode change)
2. Update `pkg/hub/handlers.go` — remove prompt checks and simplify dispatch condition (core hub-mode change)
3. Update `cmd/common.go` — remove workspace bootstrap task gate
4. Update `cmd/start.go` and `cmd/resume.go` — description text
5. Update `pkg/hub/handlers.go` and `pkg/store/models.go` — comments
6. Update tests to match new behavior
7. Run full test suite
